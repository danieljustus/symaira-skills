package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeVariantSkill creates a skill whose worker contract is harness-bound in
// the canonical body and overridden for Claude.
func writeVariantSkill(t *testing.T, root string) {
	t.Helper()
	write := func(rel, content string) {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("SKILL.md", `---
name: variant-skill
description: A skill whose worker contract differs per harness.
---

# Variant Skill

<!-- symskills:block worker -->
Hermes only. Dispatch with delegate_task.
<!-- /symskills:block -->

Reports land in {{term:report_dir}}.
`)
	write("symskills.toml", `[skill]
name = "variant-skill"
version = "1.0.0"

[terms.report_dir]
default = "~/.local/state/symskills/reports"
hermes = "~/.hermes/reports"
`)
	write("overlays/claude/blocks/worker.md", "Dispatch each worker with the Agent tool.\n")
}

func TestRenderExplainReportsPerTargetVariants(t *testing.T) {
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")

	skillDir := filepath.Join(t.TempDir(), "variant-skill")
	writeVariantSkill(t, skillDir)

	stdout, _, err := runCmd(t, home, "render", "--target", "claude", "--explain", skillDir)
	if err != nil {
		t.Fatalf("render --explain: %v", err)
	}
	for _, want := range []string{"blocks:", "worker", "term report_dir:", "~/.local/state/symskills/reports", "divergence:"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("explain output missing %q:\n%s", want, stdout)
		}
	}

	stdout, _, err = runCmd(t, home, "render", "--target", "hermes", "--explain", skillDir)
	if err != nil {
		t.Fatalf("render --explain (hermes): %v", err)
	}
	if !strings.Contains(stdout, "~/.hermes/reports") {
		t.Errorf("hermes explain output missing its term override:\n%s", stdout)
	}
	if strings.Contains(stdout, "blocks:") {
		t.Errorf("hermes overrides no block, so none may be reported:\n%s", stdout)
	}
}

func TestRenderExplainOnPlainSkillSaysSo(t *testing.T) {
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")

	skillDir := t.TempDir()
	writeTestSkill(t, skillDir, "plain-skill", "A skill with no harness variants")

	stdout, _, err := runCmd(t, home, "render", "--target", "claude", "--explain", skillDir)
	if err != nil {
		t.Fatalf("render --explain: %v", err)
	}
	if !strings.Contains(stdout, "no harness-specific variants") {
		t.Errorf("expected the no-variant note:\n%s", stdout)
	}
}

func TestValidateReportsVariantErrors(t *testing.T) {
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")

	skillDir := filepath.Join(t.TempDir(), "broken-skill")
	writeVariantSkill(t, skillDir)
	if err := os.WriteFile(filepath.Join(skillDir, "overlays", "claude", "blocks", "invented.md"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, _ := runCmd(t, home, "validate", skillDir)
	combined := stdout + stderr
	if !strings.Contains(combined, "block_override_unknown") {
		t.Errorf("expected block_override_unknown in validate output:\n%s", combined)
	}
}
