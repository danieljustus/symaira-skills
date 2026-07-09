package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

func TestListCommandPrintsLoadIssuesToStderr(t *testing.T) {
	root := t.TempDir()
	library := filepath.Join(root, "library")
	healthy := filepath.Join(library, "healthy-skill")
	broken := filepath.Join(library, "broken-skill")
	if err := os.MkdirAll(healthy, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestSkill(t, healthy, "healthy-skill", "Healthy fixture.")
	if err := os.MkdirAll(broken, 0o755); err != nil {
		t.Fatal(err)
	}

	var out, stderr bytes.Buffer
	cmd := newRootCmd("test")
	cmd.SetOut(&out)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"list", "--library", library})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("list: %v\nstdout: %s\nstderr: %s", err, out.String(), stderr.String())
	}

	stdout := out.String()
	if !strings.Contains(stdout, "healthy-skill") {
		t.Errorf("expected healthy skill in stdout, got: %q", stdout)
	}

	stderrStr := stderr.String()
	if !strings.Contains(stderrStr, "warning:") {
		t.Errorf("expected warning in stderr, got: %q", stderrStr)
	}
	if !strings.Contains(stderrStr, "broken-skill") {
		t.Errorf("expected broken skill path in stderr, got: %q", stderrStr)
	}
}

func TestListCommandStrictExitsNonZero(t *testing.T) {
	root := t.TempDir()
	library := filepath.Join(root, "library")
	broken := filepath.Join(library, "broken-skill")
	if err := os.MkdirAll(broken, 0o755); err != nil {
		t.Fatal(err)
	}

	var out, stderr bytes.Buffer
	cmd := newRootCmd("test")
	cmd.SetOut(&out)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"list", "--library", library, "--strict"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected non-zero exit in strict mode")
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

func TestVersionCommand(t *testing.T) {
	var out bytes.Buffer
	cmd := newRootCmd("1.2.3")
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"version"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("version execute error: %v", err)
	}
	if !strings.Contains(out.String(), "symskills 1.2.3") {
		t.Errorf("expected version output, got: %q", out.String())
	}

	out.Reset()
	cmdJSON := newRootCmd("1.2.3")
	cmdJSON.SetOut(&out)
	cmdJSON.SetErr(&out)
	cmdJSON.SetArgs([]string{"version", "--json"})
	if err := cmdJSON.Execute(); err != nil {
		t.Fatalf("version json execute error: %v", err)
	}
	var resp struct {
		Tool          string `json:"tool"`
		Version       string `json:"version"`
		SchemaVersion int    `json:"schema_version"`
	}
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("parse version JSON: %v", err)
	}
	if resp.Tool != "symskills" || resp.Version != "1.2.3" || resp.SchemaVersion != 1 {
		t.Errorf("unexpected version response payload: %+v", resp)
	}
}

func writeTestSkill(t *testing.T, dir, name, description string) {
	t.Helper()
	data := "---\nname: " + name + "\ndescription: " + description + "\n---\n\n# Body\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}
