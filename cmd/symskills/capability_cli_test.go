package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danieljustus/symaira-skills/internal/config"
	"github.com/danieljustus/symaira-skills/internal/render"
)

// writeRequiringSkill creates a skill that cannot work without harness-managed
// child agents.
func writeRequiringSkill(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `---
name: needs-subagents
description: A skill that fans work out across harness-managed child agents.
---

# Needs Subagents

Fan the work out across child agents.
`
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := "[skill]\nname = \"needs-subagents\"\nrequires = [\"subagents\"]\n"
	if err := os.WriteFile(filepath.Join(root, "symskills.toml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
}

// declareCapabilities writes a capabilities table into the global config and
// registers it, mirroring what runMain does before the root command runs. The
// target registry is global, so it is restored afterwards.
func declareCapabilities(t *testing.T, home, content string) error {
	t.Helper()
	cfgDir := filepath.Join(home, ".config", "symskills")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	saved := make([]render.TargetSpec, len(render.Targets))
	copy(saved, render.Targets)
	for i := range saved {
		if saved[i].Capabilities != nil {
			clone := make(map[string]bool, len(saved[i].Capabilities))
			for k, v := range saved[i].Capabilities {
				clone[k] = v
			}
			saved[i].Capabilities = clone
		}
	}
	t.Cleanup(func() { render.Targets = saved })

	declarations, err := config.LoadCapabilities()
	if err != nil {
		return err
	}
	return render.DeclareCapabilities(declarations)
}

func TestTargetsReportsRuntimeCapabilities(t *testing.T) {
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")

	stdout, _, err := runCmd(t, home, "targets")
	if err != nil {
		t.Fatalf("targets: %v", err)
	}
	if !strings.Contains(stdout, "Runtime:") {
		t.Errorf("targets output missing the runtime capability line:\n%s", stdout)
	}
	if !strings.Contains(stdout, "?hooks") {
		t.Errorf("undeclared capabilities must be visible as unknown:\n%s", stdout)
	}

	stdout, _, err = runCmd(t, home, "targets", "--json")
	if err != nil {
		t.Fatalf("targets --json: %v", err)
	}
	var payload struct {
		Targets []struct {
			Target              string            `json:"target"`
			RuntimeCapabilities map[string]string `json:"runtime_capabilities"`
		} `json:"targets"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("parse JSON: %v", err)
	}
	for _, target := range payload.Targets {
		if target.Target != "claude" {
			continue
		}
		if target.RuntimeCapabilities["subagents"] != "supported" {
			t.Errorf("claude subagents: got %q", target.RuntimeCapabilities["subagents"])
		}
		if target.RuntimeCapabilities["hooks"] != "unknown" {
			t.Errorf("claude hooks: got %q", target.RuntimeCapabilities["hooks"])
		}
		return
	}
	t.Fatal("claude missing from targets --json")
}

func TestRenderRefusesTargetLackingRequiredCapability(t *testing.T) {
	home := t.TempDir()
	if err := declareCapabilities(t, home, "[capabilities.codex]\nsubagents = false\n"); err != nil {
		t.Fatalf("declare capabilities: %v", err)
	}
	_, _, _ = runCmd(t, home, "init")

	skillDir := filepath.Join(t.TempDir(), "needs-subagents")
	writeRequiringSkill(t, skillDir)

	_, _, err := runCmd(t, home, "render", "--target", "codex", skillDir)
	if err == nil {
		t.Fatal("expected render to refuse a target that declares it lacks the capability")
	}
	if !strings.Contains(err.Error(), "subagents") {
		t.Errorf("error must name the capability: %v", err)
	}

	stdout, stderr, err := runCmd(t, home, "render", "--target", "codex", "--ignore-capabilities", skillDir)
	if err != nil {
		t.Fatalf("forced render: %v", err)
	}
	if !strings.Contains(stderr, "warning") {
		t.Errorf("a forced render must warn on stderr:\n%s", stderr)
	}
	if !strings.Contains(stdout, "codex") {
		t.Errorf("forced render produced no output:\n%s", stdout)
	}

	rendered, err := os.ReadFile(filepath.Join(home, ".local", "share", "symskills", "rendered", "codex", "needs-subagents", "SKILL.md"))
	if err != nil {
		t.Fatalf("read forced render: %v", err)
	}
	if strings.Contains(string(rendered), "compatibility:") {
		t.Errorf("a forced render must not claim compatibility:\n%s", rendered)
	}
}

func TestRenderWarnsWhenCapabilityUndeclared(t *testing.T) {
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")

	skillDir := filepath.Join(t.TempDir(), "needs-subagents")
	writeRequiringSkill(t, skillDir)

	_, stderr, err := runCmd(t, home, "render", "--target", "opencode", skillDir)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(stderr, "capabilities.opencode") {
		t.Errorf("warning must point at the config fix:\n%s", stderr)
	}
}

func TestRenderIgnoreCapabilitiesRejectedWithProfile(t *testing.T) {
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")

	_, _, err := runCmd(t, home, "render", "--profile", "anything", "--ignore-capabilities")
	if err == nil || !strings.Contains(err.Error(), "--ignore-capabilities") {
		t.Fatalf("expected a refusal naming the flag, got %v", err)
	}
}

func TestUnknownCapabilityInConfigIsRejected(t *testing.T) {
	home := t.TempDir()
	err := declareCapabilities(t, home, "[capabilities.claude]\nsubagant = true\n")
	if err == nil || !strings.Contains(err.Error(), "subagant") {
		t.Fatalf("expected a typo in the capabilities table to be rejected, got %v", err)
	}
}

// TestValidateSurfacesWarningsOnValidSkill: a skill can be valid and still be
// harness-coupled or ship dead overrides. Warnings must not be swallowed.
func TestValidateSurfacesWarningsOnValidSkill(t *testing.T) {
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")

	skillDir := filepath.Join(t.TempDir(), "coupled")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `---
name: coupled
description: A valid skill whose body is nevertheless bound to one harness.
category: developer-tools
---

Persist the report under ~/.hermes/reports/coupled/.
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := runCmd(t, home, "validate", skillDir)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if strings.TrimSpace(stdout) != "valid" {
		t.Errorf("stdout must stay exactly \"valid\" for callers that test it, got %q", stdout)
	}
	if !strings.Contains(stderr, "harness_coupling") {
		t.Errorf("warnings must reach stderr:\n%s", stderr)
	}
}
