package main

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/danieljustus/symaira-skills/internal/config"
	"github.com/danieljustus/symaira-skills/internal/events"
	"github.com/danieljustus/symaira-skills/internal/install"
	"github.com/danieljustus/symaira-skills/internal/render"
	"github.com/danieljustus/symaira-skills/internal/skill"
	"github.com/spf13/cobra"
)

// resyncTestSetup builds an isolated cfg, logger and bundle for the resync
// error-path tests. HOME is pointed at a temp dir because resyncOneStaleTarget
// resolves the pull-lock home through userHomeDir().
func resyncTestSetup(t *testing.T, manifest string) (*config.Config, *events.Logger, *skill.Bundle) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	lib := t.TempDir()
	bundleRoot := filepath.Join(lib, "resync-skill")
	if err := os.MkdirAll(bundleRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundleRoot, "SKILL.md"), []byte("---\nname: resync-skill\ndescription: test\n---\n\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if manifest != "" {
		if err := os.WriteFile(filepath.Join(bundleRoot, "symskills.toml"), []byte(manifest), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	bundle, err := skill.LoadBundle(bundleRoot)
	if err != nil {
		t.Fatal(err)
	}
	renderDir := t.TempDir()
	cfg := &config.Config{LibraryDir: lib, RenderDir: renderDir, BaseDir: filepath.Join(home, ".base")}
	logger := events.New(filepath.Join(home, "events.jsonl"), "test")
	return cfg, logger, bundle
}

func staleStatus(bundle *skill.Bundle) install.InstallStatus {
	return install.InstallStatus{Target: render.TargetOpenCode, Name: bundle.Manifest.Skill.Name, Path: "", Mode: "copy"}
}

func TestResyncOneStaleTargetBundleMissing(t *testing.T) {
	cfg, logger, _ := resyncTestSetup(t, "")
	var mu sync.Mutex
	res := resyncOneStaleTarget(&cobra.Command{}, cfg, install.InstallStatus{Target: render.TargetOpenCode, Name: "missing", Mode: "copy"}, render.ScopeUser, logger, &mu)
	if res.Action != "failed" || res.Error == "" {
		t.Fatalf("expected failed row, got %+v", res)
	}
}

func TestResyncOneStaleTargetNoRenderOutput(t *testing.T) {
	cfg, logger, bundle := resyncTestSetup(t, `
[skill]
name = "resync-skill"

[targets.opencode]
enabled = false
`)
	var mu sync.Mutex
	res := resyncOneStaleTarget(&cobra.Command{}, cfg, staleStatus(bundle), render.ScopeUser, logger, &mu)
	if res.Action != "failed" || !strings.Contains(res.Error, "disabled") {
		t.Fatalf("expected disabled-target failure, got %+v", res)
	}
	evs, err := logger.Read(events.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 || evs[0].Outcome != events.OutcomeError || evs[0].Skill != "resync-skill" {
		t.Fatalf("expected one error event, got %+v", evs)
	}
}

func TestResyncOneStaleTargetLockHeld(t *testing.T) {
	cfg, logger, bundle := resyncTestSetup(t, "")
	// Pre-create the pull lock for the rendered name so AcquirePullLock fails.
	home, _ := os.UserHomeDir()
	lockPath := filepath.Join(home, ".local", "share", "symskills", "pending", ".locks", "opencode", "resync-skill.lock")
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	res := resyncOneStaleTarget(&cobra.Command{}, cfg, staleStatus(bundle), render.ScopeUser, logger, &mu)
	if res.Action != "skipped" || !strings.Contains(res.Error, "pull lock held") {
		t.Fatalf("expected lock skip, got %+v", res)
	}
}

func TestResyncOneStaleTargetInstallFailure(t *testing.T) {
	cfg, logger, bundle := resyncTestSetup(t, "")
	rendered, errs := render.RenderAll(bundle, cfg.RenderDir, []render.Target{render.TargetOpenCode})
	if len(rendered) == 0 {
		t.Fatalf("render failed: %v", errs)
	}
	// Sabotage the destination parent: with HOME pointing at the test dir,
	// Install resolves the opencode skill root to <home>/.config/opencode;
	// making that a plain file forces prepareDest's MkdirAll to fail after
	// the pull lock succeeded.
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(home, ".config", "opencode")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	res := resyncOneStaleTarget(&cobra.Command{}, cfg, staleStatus(bundle), render.ScopeUser, logger, &mu)
	if res.Action != "failed" || res.Error == "" {
		t.Fatalf("expected install failure row, got %+v", res)
	}
	evs, err := logger.Read(events.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 || evs[0].Outcome != events.OutcomeError {
		t.Fatalf("expected one error event, got %+v", evs)
	}
}
