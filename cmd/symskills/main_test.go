package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateCommandJSON(t *testing.T) {
	root := t.TempDir()
	writeTestSkill(t, root, "cli-skill", "CLI validation fixture.")

	var out bytes.Buffer
	cmd := newRootCmd("test")
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"validate", "--json", root})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("validate: %v\n%s", err, out.String())
	}

	var resp struct {
		Valid bool `json:"valid"`
	}
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("parse JSON %q: %v", out.String(), err)
	}
	if !resp.Valid {
		t.Fatalf("expected valid response, got %s", out.String())
	}
}

func TestRenderCommandWritesCodexMetadata(t *testing.T) {
	root := t.TempDir()
	writeTestSkill(t, root, "cli-render", "CLI render fixture.")
	outDir := filepath.Join(t.TempDir(), "rendered")

	var out bytes.Buffer
	cmd := newRootCmd("test")
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"render", "--target", "codex", "--output", outDir, root})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("render: %v\n%s", err, out.String())
	}

	if _, err := os.Stat(filepath.Join(outDir, "codex", "cli-render", "agents", "openai.yaml")); err != nil {
		t.Fatalf("codex metadata missing: %v", err)
	}
}

func writeTestSkill(t *testing.T, dir, name, description string) {
	t.Helper()
	data := "---\nname: " + name + "\ndescription: " + description + "\n---\n\n# Body\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}
