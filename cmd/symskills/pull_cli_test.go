package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPullCLIStagesAndApplies(t *testing.T) {
	home := t.TempDir()
	if _, _, err := runCmd(t, home, "init"); err != nil {
		t.Fatal(err)
	}
	skillDir := t.TempDir()
	writeTestSkillBody(t, skillDir, "pull-cli", "v1 body")
	if _, _, err := runCmd(t, home, "import", skillDir); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(libraryDirFor(home), "pull-cli")
	if _, stderr, err := runCmd(t, home, "install", "--json", "--mode", "copy", repo); err != nil {
		t.Fatalf("install failed: %v, stderr: %s", err, stderr)
	}
	installed := filepath.Join(home, ".config", "opencode", "skills", "pull-cli", "SKILL.md")
	editInstalled := func(old, new string) {
		t.Helper()
		data, err := os.ReadFile(installed)
		if err != nil {
			t.Fatal(err)
		}
		updated := strings.Replace(string(data), old, new, 1)
		if updated == string(data) {
			t.Fatalf("edit target %q not found in installed SKILL.md", old)
		}
		if err := os.WriteFile(installed, []byte(updated), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Plain-text path: stage a harness edit and check the human output lines.
	editInstalled("v1 body", "v2 harness edit")
	stdout, _, err := runCmd(t, home, "pull", "pull-cli")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "staged pull-cli") || !strings.Contains(stdout, "modified SKILL.md") || !strings.Contains(stdout, "pending:") {
		t.Fatalf("expected human pull output, got %q", stdout)
	}
	// The library must not change before apply.
	if data, _ := os.ReadFile(filepath.Join(repo, "SKILL.md")); strings.Contains(string(data), "v2 harness edit") {
		t.Fatal("library changed before apply")
	}

	// JSON path: same staged pull as JSON, then apply it into the library.
	stdout, _, err = runCmd(t, home, "pull", "--json", "pull-cli")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "staged") {
		t.Fatalf("expected staged result, got %q", stdout)
	}
	stdout, _, err = runCmd(t, home, "pull", "--json", "--apply", "pull-cli")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "applied") {
		t.Fatalf("expected applied result, got %q", stdout)
	}
	if data, _ := os.ReadFile(filepath.Join(repo, "SKILL.md")); !strings.Contains(string(data), "v2 harness edit") {
		t.Fatal("library not updated by apply")
	}

	// After the apply both sides carry v2; a further harness edit while the
	// library diverged from the base snapshot is a two-sided conflict and
	// must be refused with the drift protection visible on stderr.
	editInstalled("v2 harness edit", "v3 harness edit")
	_, stderr, err := runCmd(t, home, "pull", "pull-cli")
	if err == nil {
		t.Fatal("expected drift-conflict refusal after apply + second edit")
	}
	if !strings.Contains(stderr, "refused:") || !strings.Contains(stderr, "conflict in SKILL.md") {
		t.Fatalf("expected conflict refusal on stderr, got %q", stderr)
	}
}

func TestPullCLIRefusesOverlayEditAndPrintsRefusal(t *testing.T) {
	home := t.TempDir()
	if _, _, err := runCmd(t, home, "init"); err != nil {
		t.Fatal(err)
	}
	skillDir := t.TempDir()
	writeTestSkillBody(t, skillDir, "pull-refuse", "v1 body")
	// Add a prepend overlay so the installed skill contains a protected region.
	if err := os.MkdirAll(filepath.Join(skillDir, "overlays", "opencode"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "overlays", "opencode", "prepend.md"), []byte("overlay header\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runCmd(t, home, "import", skillDir); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(libraryDirFor(home), "pull-refuse")
	if _, stderr, err := runCmd(t, home, "install", "--json", "--mode", "copy", repo); err != nil {
		t.Fatalf("install failed: %v, stderr: %s", err, stderr)
	}
	installed := filepath.Join(home, ".config", "opencode", "skills", "pull-refuse", "SKILL.md")
	data, err := os.ReadFile(installed)
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(data), "overlay header", "changed header", 1)
	if updated == string(data) {
		t.Fatal("overlay header not found in installed SKILL.md")
	}
	if err := os.WriteFile(installed, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}
	_, stderr, err := runCmd(t, home, "pull", "pull-refuse")
	if err == nil {
		t.Fatal("expected refusal error")
	}
	if !strings.Contains(stderr, "refused:") || !strings.Contains(stderr, "prepend") {
		t.Fatalf("expected refusal on stderr, got %q", stderr)
	}
}
