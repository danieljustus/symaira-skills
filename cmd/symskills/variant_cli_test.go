package main

import (
	"encoding/json"
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

func TestInspectReportsPerTargetDivergence(t *testing.T) {
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")

	skillDir := filepath.Join(t.TempDir(), "variant-skill")
	writeVariantSkill(t, skillDir)

	stdout, _, err := runCmd(t, home, "inspect", skillDir)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	for _, want := range []string{"Variants:", "claude", "% replaced", "blocks: worker", "term report_dir:"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("inspect output missing %q:\n%s", want, stdout)
		}
	}

	stdout, _, err = runCmd(t, home, "inspect", "--json", skillDir)
	if err != nil {
		t.Fatalf("inspect --json: %v", err)
	}
	var payload struct {
		Variants []struct {
			Target     string   `json:"target"`
			Status     string   `json:"status"`
			Blocks     []string `json:"blocks"`
			Divergence float64  `json:"divergence"`
		} `json:"variants"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("parse JSON: %v", err)
	}
	for _, entry := range payload.Variants {
		if entry.Target != "claude" {
			continue
		}
		if entry.Status != "rendered" || entry.Divergence <= 0 || len(entry.Blocks) != 1 {
			t.Errorf("claude entry: %+v", entry)
		}
		return
	}
	t.Fatalf("claude missing from inspect --json variants:\n%s", stdout)
}

// TestInspectOmitsVariantsForPlainSkill: an ordinary skill must not grow a
// wall of zeroes in its inspect output.
func TestInspectOmitsVariantsForPlainSkill(t *testing.T) {
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")

	skillDir := t.TempDir()
	writeTestSkill(t, skillDir, "plain-skill", "A skill with no harness variants")

	stdout, _, err := runCmd(t, home, "inspect", skillDir)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if strings.Contains(stdout, "Variants:") {
		t.Errorf("expected no variants section:\n%s", stdout)
	}
}

func TestListVariantsReportsDivergence(t *testing.T) {
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")

	library := filepath.Join(home, ".local", "share", "symskills", "library")
	writeVariantSkill(t, filepath.Join(library, "variant-skill"))

	stdout, _, err := runCmd(t, home, "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if strings.Contains(stdout, "%") {
		t.Errorf("the default listing must stay on the fast path, without divergence:\n%s", stdout)
	}

	stdout, _, err = runCmd(t, home, "list", "--variants")
	if err != nil {
		t.Fatalf("list --variants: %v", err)
	}
	if !strings.Contains(stdout, "%") {
		t.Errorf("expected a divergence column:\n%s", stdout)
	}

	stdout, _, err = runCmd(t, home, "list", "--variants", "--json")
	if err != nil {
		t.Fatalf("list --variants --json: %v", err)
	}
	var payload struct {
		Skills []struct {
			Name          string   `json:"name"`
			MaxDivergence *float64 `json:"max_divergence"`
			Variants      []struct {
				Target string `json:"target"`
			} `json:"variants"`
		} `json:"skills"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("parse JSON: %v", err)
	}
	if len(payload.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(payload.Skills))
	}
	if payload.Skills[0].MaxDivergence == nil || *payload.Skills[0].MaxDivergence <= 0 {
		t.Errorf("max_divergence: got %v", payload.Skills[0].MaxDivergence)
	}
	if len(payload.Skills[0].Variants) == 0 {
		t.Error("expected per-target variant summaries")
	}
}
