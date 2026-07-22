package profile

import (
	"path/filepath"
	"testing"

	"github.com/danieljustus/symaira-skills/internal/install"
	"github.com/danieljustus/symaira-skills/internal/render"
)

func TestRenderProfile(t *testing.T) {
	lib := t.TempDir()
	profiles := t.TempDir()
	project := t.TempDir()
	output := t.TempDir()
	makeSkill(t, lib, "debug")

	writeFile(t, filepath.Join(profiles, "default.toml"), `name = "default"

[links.debug]
skill = "debug"
`)

	rendered, issues, err := RenderProfile(lib, profiles, project, output, []render.Target{render.TargetOpenCode}, "default")
	if err != nil {
		t.Fatalf("RenderProfile: %v", err)
	}
	if len(issues) != 0 {
		t.Fatalf("unexpected issues: %v", issues)
	}
	if len(rendered) != 1 {
		t.Fatalf("want 1 rendered skill, got %d", len(rendered))
	}
	if rendered[0].Name != "debug" {
		t.Fatalf("rendered name: want debug, got %q", rendered[0].Name)
	}
}

func TestRenderProfileResolveIssues(t *testing.T) {
	lib := t.TempDir()
	profiles := t.TempDir()
	project := t.TempDir()
	output := t.TempDir()

	writeFile(t, filepath.Join(profiles, "default.toml"), `name = "default"

[links.missing]
skill = "missing"
`)

	rendered, issues, err := RenderProfile(lib, profiles, project, output, []render.Target{render.TargetOpenCode}, "default")
	if err != nil {
		t.Fatalf("RenderProfile: %v", err)
	}
	if len(issues) == 0 {
		t.Fatal("expected issues for missing linked skill, got none")
	}
	if rendered != nil {
		t.Fatalf("expected no rendered output when issues present, got %v", rendered)
	}
}

func TestRenderProfileResolveError(t *testing.T) {
	lib := t.TempDir()
	profiles := t.TempDir()
	project := t.TempDir()
	output := t.TempDir()

	_, _, err := RenderProfile(lib, profiles, project, output, []render.Target{render.TargetOpenCode}, "Invalid Name!")
	if err == nil {
		t.Fatal("expected error for invalid profile name")
	}
}

func TestInstallProfile(t *testing.T) {
	lib := t.TempDir()
	profiles := t.TempDir()
	project := t.TempDir()
	output := t.TempDir()
	home := t.TempDir()
	makeSkill(t, lib, "debug")

	writeFile(t, filepath.Join(profiles, "default.toml"), `name = "default"

[links.debug]
skill = "debug"
`)

	results, issues, err := InstallProfile(lib, profiles, project, output, render.TargetOpenCode, "default", install.Options{
		HomeDir: home,
		Scope:   install.ScopeUser,
		Mode:    install.ModeCopy,
	})
	if err != nil {
		t.Fatalf("InstallProfile: %v", err)
	}
	if len(issues) != 0 {
		t.Fatalf("unexpected issues: %v", issues)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 install result, got %d", len(results))
	}
	if results[0].Action != "installed" {
		t.Fatalf("action: want installed, got %q", results[0].Action)
	}
}

func TestInstallProfileResolveIssues(t *testing.T) {
	lib := t.TempDir()
	profiles := t.TempDir()
	project := t.TempDir()
	output := t.TempDir()
	home := t.TempDir()

	writeFile(t, filepath.Join(profiles, "default.toml"), `name = "default"

[links.missing]
skill = "missing"
`)

	results, issues, err := InstallProfile(lib, profiles, project, output, render.TargetOpenCode, "default", install.Options{
		HomeDir: home,
		Scope:   install.ScopeUser,
		Mode:    install.ModeCopy,
	})
	if err != nil {
		t.Fatalf("InstallProfile: %v", err)
	}
	if len(issues) == 0 {
		t.Fatal("expected issues for missing linked skill, got none")
	}
	if results != nil {
		t.Fatalf("expected no install results when issues present, got %v", results)
	}
}

func TestInstallProfileInstallFailure(t *testing.T) {
	lib := t.TempDir()
	profiles := t.TempDir()
	project := t.TempDir()
	output := t.TempDir()
	home := t.TempDir()
	makeSkill(t, lib, "debug")

	writeFile(t, filepath.Join(profiles, "default.toml"), `name = "default"

[links.debug]
skill = "debug"
`)

	// A home directory that is a regular file (not a directory) makes any
	// install path underneath it fail, exercising the InstallProfile error path.
	badHome := filepath.Join(home, "not-a-dir")
	writeFile(t, badHome, "blocker")

	_, issues, err := InstallProfile(lib, profiles, project, output, render.TargetOpenCode, "default", install.Options{
		HomeDir: badHome,
		Scope:   install.ScopeUser,
		Mode:    install.ModeCopy,
	})
	if len(issues) != 0 {
		t.Fatalf("unexpected issues: %v", issues)
	}
	if err == nil {
		t.Fatal("expected install error when home dir is not a directory")
	}
}
