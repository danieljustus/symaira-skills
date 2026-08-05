package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runCmd(t *testing.T, homeDir string, args ...string) (string, string, error) {
	t.Helper()
	var out, errOut bytes.Buffer
	cmd := newRootCmd("test-version")
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	t.Setenv("HOME", homeDir)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

func TestMainCmd(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"symskills", "version"}
	main()
}

func TestInitCommand(t *testing.T) {
	home := t.TempDir()

	// First init
	stdout, stderr, err := runCmd(t, home, "init")
	if err != nil {
		t.Fatalf("init failed: %v, stderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "Created") {
		t.Errorf("expected Created in output, got: %q", stdout)
	}

	// Second init without force
	stdout, _, err = runCmd(t, home, "init")
	if err != nil {
		t.Fatalf("init failed: %v", err)
	}
	if !strings.Contains(stdout, "already exists") {
		t.Errorf("expected already exists in output, got: %q", stdout)
	}

	// Third init with force
	stdout, _, err = runCmd(t, home, "init", "--force")
	if err != nil {
		t.Fatalf("init failed: %v", err)
	}
	if !strings.Contains(stdout, "Created") {
		t.Errorf("expected Created in output, got: %q", stdout)
	}
}

func TestImportCommand(t *testing.T) {
	home := t.TempDir()
	// Initialize config
	_, _, _ = runCmd(t, home, "init")

	skillDir := t.TempDir()
	writeTestSkill(t, skillDir, "import-test", "For testing import")

	// Standard import
	stdout, stderr, err := runCmd(t, home, "import", skillDir)
	if err != nil {
		t.Fatalf("import failed: %v, stderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "Imported import-test") {
		t.Errorf("unexpected output: %s", stdout)
	}

	// Duplicate import (should fail)
	_, _, err = runCmd(t, home, "import", skillDir)
	if err == nil {
		t.Fatal("expected import duplicate to fail")
	}

	// Import with --json
	skillDir2 := t.TempDir()
	writeTestSkill(t, skillDir2, "import-json", "For testing JSON import")
	stdout, _, err = runCmd(t, home, "import", "--json", skillDir2)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	var resp struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(stdout), &resp); err != nil {
		t.Fatalf("parse JSON output: %v, raw: %q", err, stdout)
	}
	if resp.Name != "import-json" {
		t.Errorf("expected import-json, got: %s", resp.Name)
	}
}

func TestBatchImportCommand(t *testing.T) {
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")

	// Create a parent directory with two skill subdirectories
	parent := t.TempDir()
	// Create the skill subdirectories first (writeTestSkill writes SKILL.md but doesn't create the dir)
	mkdir := func(name string) string {
		dir := filepath.Join(parent, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	writeTestSkill(t, mkdir("batch-one"), "batch-one", "First batch skill")
	writeTestSkill(t, mkdir("batch-two"), "batch-two", "Second batch skill")

	// Create a non-skill directory (should be skipped silently)
	if err := os.MkdirAll(filepath.Join(parent, "not-skill"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Batch import
	stdout, stderr, err := runCmd(t, home, "import", "--batch", parent)
	if err != nil {
		t.Fatalf("batch import failed: %v, stderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "Imported batch-one") {
		t.Errorf("expected batch-one imported, got: %q", stdout)
	}
	if !strings.Contains(stdout, "Imported batch-two") {
		t.Errorf("expected batch-two imported, got: %q", stdout)
	}
	if !strings.Contains(stdout, "Summary: 2 imported, 0 skipped, 0 failed") {
		t.Errorf("expected summary in output, got: %q", stdout)
	}

	// Re-import (skills already exist) — should report failed
	stdout, stderr, err = runCmd(t, home, "import", "--batch", parent)
	if err != nil {
		t.Fatalf("batch re-import failed: %v, stderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "Summary: 0 imported, 0 skipped, 2 failed") {
		t.Errorf("expected 2 failed on re-import, got: %q", stdout)
	}

	// Batch import from a fresh parent with --json
	parent2 := t.TempDir()
	dir1 := filepath.Join(parent2, "json-one")
	dir2 := filepath.Join(parent2, "json-two")
	if err := os.MkdirAll(dir1, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir2, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestSkill(t, dir1, "json-one", "JSON batch skill")
	writeTestSkill(t, dir2, "json-two", "Second JSON skill")

	stdout, _, err = runCmd(t, home, "import", "--batch", "--json", parent2)
	if err != nil {
		t.Fatalf("batch import --json failed: %v", err)
	}
	var results []struct {
		Name   string `json:"name"`
		Status string `json:"status"`
		Error  string `json:"error,omitempty"`
	}
	if err := json.Unmarshal([]byte(stdout), &results); err != nil {
		t.Fatalf("parse batch JSON output: %v, raw: %q", err, stdout)
	}
	if len(results) < 2 {
		t.Fatalf("expected at least 2 results, got %d: %s", len(results), stdout)
	}
	imported := 0
	for _, r := range results {
		t.Logf("result: name=%s status=%s error=%q", r.Name, r.Status, r.Error)
		if r.Status == "imported" {
			imported++
		}
	}
	if imported < 2 {
		t.Errorf("expected 2 imported, got %d: %+v", imported, results)
	}

	// Batch import from a single-skill directory (fallback)
	single := t.TempDir()
	writeTestSkill(t, single, "single-skill", "A single skill dir")
	stdout, _, err = runCmd(t, home, "import", "--batch", single)
	if err != nil {
		t.Fatalf("batch import on single skill failed: %v", err)
	}
	if !strings.Contains(stdout, "Imported single-skill") {
		t.Errorf("expected single-skill imported in fallback, got: %q", stdout)
	}
}

func TestInspectCommand(t *testing.T) {
	home := t.TempDir()
	skillDir := t.TempDir()
	writeTestSkill(t, skillDir, "inspect-test", "For testing inspect")

	// Inspect standard
	stdout, _, err := runCmd(t, home, "inspect", skillDir)
	if err != nil {
		t.Fatalf("inspect failed: %v", err)
	}
	if !strings.Contains(stdout, "inspect-test") || !strings.Contains(stdout, "For testing inspect") {
		t.Errorf("unexpected output: %s", stdout)
	}

	// Inspect JSON
	stdout, _, err = runCmd(t, home, "inspect", "--json", skillDir)
	if err != nil {
		t.Fatalf("inspect failed: %v", err)
	}
	var resp struct {
		Frontmatter struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"frontmatter"`
	}
	if err := json.Unmarshal([]byte(stdout), &resp); err != nil {
		t.Fatalf("parse json: %v", err)
	}
	if resp.Frontmatter.Name != "inspect-test" {
		t.Errorf("expected inspect-test, got: %s", resp.Frontmatter.Name)
	}
}

func TestValidateCommand(t *testing.T) {
	home := t.TempDir()
	skillDir := t.TempDir()
	writeTestSkill(t, skillDir, "validate-test", "For testing validate")

	// Validate standard
	stdout, _, err := runCmd(t, home, "validate", skillDir)
	if err != nil {
		t.Fatalf("validate failed: %v", err)
	}
	if !strings.Contains(stdout, "valid") {
		t.Errorf("unexpected output: %s", stdout)
	}

	// Validate JSON
	stdout, _, err = runCmd(t, home, "validate", "--json", skillDir)
	if err != nil {
		t.Fatalf("validate failed: %v", err)
	}
	var respJSON struct {
		Valid bool `json:"valid"`
	}
	if err := json.Unmarshal([]byte(stdout), &respJSON); err != nil {
		t.Fatalf("parse JSON %q: %v", stdout, err)
	}
	if !respJSON.Valid {
		t.Fatalf("expected valid response, got %s", stdout)
	}

	// Validate invalid skill
	invalidDir := t.TempDir()
	// Write invalid SKILL.md (missing description)
	err = os.WriteFile(filepath.Join(invalidDir, "SKILL.md"), []byte("---\nname: bad-skill\ndescription: \"\"\n---\nbody\n"), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	stdout, _, err = runCmd(t, home, "validate", invalidDir)
	if err == nil {
		t.Fatal("expected validate to fail on invalid skill")
	}
	if !strings.Contains(stdout, "description_required") {
		t.Errorf("expected error details in stdout, got: %s", stdout)
	}

	// Validate invalid path
	_, _, err = runCmd(t, home, "validate", "/nonexistent/path")
	if err == nil {
		t.Fatal("expected validate to fail on nonexistent path")
	}
}

func TestRenderCommand(t *testing.T) {
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")

	skillDir := t.TempDir()
	writeTestSkill(t, skillDir, "render-test", "For testing render")

	// Render standard
	stdout, _, err := runCmd(t, home, "render", skillDir)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if !strings.Contains(stdout, "opencode") {
		t.Errorf("unexpected output: %s", stdout)
	}

	// Render JSON
	stdout, _, err = runCmd(t, home, "render", "--json", skillDir)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	var resp []struct {
		Target string `json:"target"`
		Path   string `json:"path"`
	}
	if err := json.Unmarshal([]byte(stdout), &resp); err != nil {
		t.Fatalf("parse JSON: %v", err)
	}
	if len(resp) == 0 {
		t.Fatal("expected render results")
	}

	// Render with target
	stdout, _, err = runCmd(t, home, "render", "--target", "opencode,claude", skillDir)
	if err != nil {
		t.Fatalf("render target failed: %v", err)
	}
	if !strings.Contains(stdout, "claude") {
		t.Errorf("unexpected output: %s", stdout)
	}

	// Render invalid target
	_, _, err = runCmd(t, home, "render", "--target", "invalid", skillDir)
	if err == nil {
		t.Fatal("expected render to fail on invalid target")
	}

	// Render nonexistent path
	_, _, err = runCmd(t, home, "render", "/nonexistent")
	if err == nil {
		t.Fatal("expected render to fail on nonexistent path")
	}
}

func TestDiffCommand(t *testing.T) {
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")

	skillDir := t.TempDir()
	writeTestSkill(t, skillDir, "diff-test", "For testing diff")

	// Install it first so we can diff
	_, _, err := runCmd(t, home, "install", "--mode", "copy", skillDir)
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}

	// Diff standard (should show no changes message)
	stdout, _, err := runCmd(t, home, "diff", skillDir)
	if err != nil {
		t.Fatalf("diff failed: %v", err)
	}
	if !strings.Contains(stdout, "No changes detected.") {
		t.Errorf("expected 'No changes detected.' for no changes, got: %q", stdout)
	}

	// Diff JSON (should show [])
	stdout, _, err = runCmd(t, home, "diff", "--json", skillDir)
	if err != nil {
		t.Fatalf("diff json failed: %v", err)
	}
	if strings.TrimSpace(stdout) != "[]" {
		t.Errorf("expected '[]' for no changes in json, got: %q", stdout)
	}

	// Modify skill and diff
	writeTestSkill(t, skillDir, "diff-test", "Modified description")
	stdout, _, err = runCmd(t, home, "diff", skillDir)
	if err != nil {
		t.Fatalf("diff failed: %v", err)
	}
	if !strings.Contains(stdout, "modified") {
		t.Errorf("expected modified in output, got: %s", stdout)
	}

	// Diff JSON
	stdout, _, err = runCmd(t, home, "diff", "--json", skillDir)
	if err != nil {
		t.Fatalf("diff json failed: %v", err)
	}
	var resp []struct {
		Path   string `json:"path"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(stdout), &resp); err != nil {
		t.Fatalf("parse JSON: %v", err)
	}
	if len(resp) == 0 || resp[0].Status != "modified" {
		t.Errorf("unexpected JSON resp: %+v", resp)
	}

	// Diff with invalid target
	_, _, err = runCmd(t, home, "diff", "--target", "invalid", skillDir)
	if err == nil {
		t.Fatal("expected diff to fail on invalid target")
	}
}

func TestOmittedSkillDirDefaultsToCwd(t *testing.T) {
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")

	skillDir := t.TempDir()
	writeTestSkill(t, skillDir, "cwd-skill", "Testing cwd default")

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	if err := os.Chdir(skillDir); err != nil {
		t.Fatal(err)
	}

	// inspect without arg inside skill dir
	stdout, _, err := runCmd(t, home, "inspect")
	if err != nil {
		t.Fatalf("inspect without arg failed: %v", err)
	}
	if !strings.Contains(stdout, "cwd-skill") {
		t.Errorf("expected cwd-skill in inspect output, got: %q", stdout)
	}

	// validate without arg inside skill dir
	stdout, _, err = runCmd(t, home, "validate")
	if err != nil {
		t.Fatalf("validate without arg failed: %v", err)
	}
	if !strings.Contains(stdout, "valid") {
		t.Errorf("expected valid in validate output, got: %q", stdout)
	}

	// render without arg inside skill dir
	stdout, _, err = runCmd(t, home, "render")
	if err != nil {
		t.Fatalf("render without arg failed: %v", err)
	}
	if !strings.Contains(stdout, "cwd-skill") {
		t.Errorf("expected cwd-skill in render output, got: %q", stdout)
	}

	// install without arg inside skill dir
	stdout, _, err = runCmd(t, home, "install", "--mode", "copy")
	if err != nil {
		t.Fatalf("install without arg failed: %v", err)
	}
	if !strings.Contains(stdout, "installed") {
		t.Errorf("expected installed in install output, got: %q", stdout)
	}

	// diff without arg inside skill dir (now installed, should report No changes detected.)
	stdout, _, err = runCmd(t, home, "diff")
	if err != nil {
		t.Fatalf("diff without arg failed: %v", err)
	}
	if !strings.Contains(stdout, "No changes detected.") {
		t.Errorf("expected 'No changes detected.' in diff output, got: %q", stdout)
	}

	// Switch to an empty dir (not a skill dir)
	emptyDir := t.TempDir()
	if err := os.Chdir(emptyDir); err != nil {
		t.Fatal(err)
	}

	// inspect without arg outside skill dir should fail
	_, _, err = runCmd(t, home, "inspect")
	if err == nil {
		t.Fatal("expected inspect without arg outside skill dir to fail")
	}

	// validate without arg outside skill dir should fail
	_, _, err = runCmd(t, home, "validate")
	if err == nil {
		t.Fatal("expected validate without arg outside skill dir to fail")
	}

	// render without arg outside skill dir should fail
	_, _, err = runCmd(t, home, "render")
	if err == nil {
		t.Fatal("expected render without arg outside skill dir to fail")
	}

	// install without arg outside skill dir should fail
	_, _, err = runCmd(t, home, "install")
	if err == nil {
		t.Fatal("expected install without arg outside skill dir to fail")
	}

	// diff without arg outside skill dir should fail
	_, _, err = runCmd(t, home, "diff")
	if err == nil {
		t.Fatal("expected diff without arg outside skill dir to fail")
	}
}

func TestInstallUninstallCommand(t *testing.T) {
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")

	skillDir := t.TempDir()
	writeTestSkill(t, skillDir, "install-test", "For testing install")

	// Dry run install
	stdout, _, err := runCmd(t, home, "install", "--dry-run", skillDir)
	if err != nil {
		t.Fatalf("dry run install failed: %v", err)
	}
	if !strings.Contains(stdout, "planned") {
		t.Errorf("expected planned in stdout, got: %s", stdout)
	}

	// Install JSON
	stdout, _, err = runCmd(t, home, "install", "--json", "--mode", "copy", skillDir)
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}
	var result struct {
		Action string `json:"action"`
		Name   string `json:"name"`
		Path   string `json:"path"`
	}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("parse JSON: %v", err)
	}
	if result.Action != "installed" || result.Name != "install-test" {
		t.Errorf("unexpected install result: %+v", result)
	}

	// Uninstall standard
	stdout, _, err = runCmd(t, home, "uninstall", "install-test")
	if err != nil {
		t.Fatalf("uninstall failed: %v", err)
	}
	if !strings.Contains(stdout, "Uninstalled install-test") {
		t.Errorf("unexpected output: %s", stdout)
	}

	// Uninstall invalid target
	_, _, err = runCmd(t, home, "uninstall", "--target", "invalid", "install-test")
	if err == nil {
		t.Fatal("expected uninstall to fail on invalid target")
	}
}

func TestDoctorCommand(t *testing.T) {
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")

	// Doctor standard
	stdout, _, err := runCmd(t, home, "doctor")
	if err != nil {
		t.Fatalf("doctor failed: %v", err)
	}
	if !strings.Contains(stdout, "config:") || !strings.Contains(stdout, "library:") {
		t.Errorf("unexpected output: %s", stdout)
	}

	// Doctor JSON
	stdout, _, err = runCmd(t, home, "doctor", "--json")
	if err != nil {
		t.Fatalf("doctor failed: %v", err)
	}
	type targetEntry struct {
		Target string `json:"target"`
		User   string `json:"user"`
	}
	var resp struct {
		ConfigPath string        `json:"config_path"`
		Config     any           `json:"config"`
		Targets    []targetEntry `json:"targets"`
	}
	if err := json.Unmarshal([]byte(stdout), &resp); err != nil {
		t.Fatalf("parse JSON: %v", err)
	}
	if resp.ConfigPath == "" {
		t.Error("expected config_path to be populated")
	}
	if len(resp.Targets) == 0 {
		t.Error("expected targets array to be populated")
	}
	for _, entry := range resp.Targets {
		if entry.User == "" {
			t.Errorf("expected target %q user path to be non-empty", entry.Target)
		}
	}
}

func TestServeCommand(t *testing.T) {
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")

	// Serve without stdio (should fail)
	_, _, err := runCmd(t, home, "serve")
	if err == nil {
		t.Fatal("expected serve without --stdio to fail")
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

// setupProfileTest creates a temp HOME with config, library containing a test
// skill, and a global profiles dir with the given profile TOML content.
// Returns (home, profilesDir, libraryDir).
func setupProfileTest(t *testing.T, profileName, profileTOML string) (home, profilesDir, libraryDir string) {
	t.Helper()
	home = t.TempDir()
	_, _, _ = runCmd(t, home, "init")

	profilesDir = filepath.Join(home, ".config", "symskills", "profiles")
	libraryDir = filepath.Join(home, ".local", "share", "symskills", "library")

	skillDir := filepath.Join(libraryDir, "test-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestSkill(t, skillDir, "test-skill", "A test skill for profile tests")

	if err := os.MkdirAll(profilesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profilesDir, profileName+".toml"), []byte(profileTOML), 0o644); err != nil {
		t.Fatal(err)
	}
	return
}

func TestProfileListCommand(t *testing.T) {
	home, _, _ := setupProfileTest(t, "my-profile",
		`name = "my-profile"
description = "A test profile"

[links]
test-skill = { skill = "test-skill" }
`)

	stdout, stderr, err := runCmd(t, home, "profile", "list")
	if err != nil {
		t.Fatalf("profile list failed: %v, stderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "my-profile") {
		t.Errorf("expected my-profile in output, got: %q", stdout)
	}

	stdout, stderr, err = runCmd(t, home, "profile", "list", "--json")
	if err != nil {
		t.Fatalf("profile list --json failed: %v, stderr: %s", err, stderr)
	}
	var refs []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(stdout), &refs); err != nil {
		t.Fatalf("parse JSON: %v, raw: %q", err, stdout)
	}
	if len(refs) == 0 || refs[0].Name != "my-profile" {
		t.Errorf("expected my-profile in JSON, got: %+v", refs)
	}
}

func TestProfileResolveCommand(t *testing.T) {
	home, _, _ := setupProfileTest(t, "my-profile",
		`name = "my-profile"
description = "A test profile"

[links]
test-skill = { skill = "test-skill" }
`)

	stdout, stderr, err := runCmd(t, home, "profile", "resolve", "my-profile")
	if err != nil {
		t.Fatalf("profile resolve failed: %v, stderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "test-skill") {
		t.Errorf("expected test-skill in output, got: %q", stdout)
	}

	stdout, stderr, err = runCmd(t, home, "profile", "resolve", "my-profile", "--json")
	if err != nil {
		t.Fatalf("profile resolve --json failed: %v, stderr: %s", err, stderr)
	}
	var result struct {
		Skills []struct {
			Name string `json:"name"`
		} `json:"skills"`
	}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("parse JSON: %v, raw: %q", err, stdout)
	}
	if len(result.Skills) == 0 || result.Skills[0].Name != "test-skill" {
		t.Errorf("expected test-skill in resolved skills, got: %+v", result)
	}
}

func TestProfileValidateCommand(t *testing.T) {
	home, _, _ := setupProfileTest(t, "my-profile",
		`name = "my-profile"
description = "A test profile"

[links]
test-skill = { skill = "test-skill" }
`)

	stdout, stderr, err := runCmd(t, home, "profile", "validate", "my-profile")
	if err != nil {
		t.Fatalf("profile validate failed: %v, stderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "valid") {
		t.Errorf("expected 'valid' in output, got: %q", stdout)
	}

	stdout, stderr, err = runCmd(t, home, "profile", "validate", "my-profile", "--json")
	if err != nil {
		t.Fatalf("profile validate --json failed: %v, stderr: %s", err, stderr)
	}
	var resp struct {
		Valid  bool  `json:"valid"`
		Issues []any `json:"issues"`
	}
	if err := json.Unmarshal([]byte(stdout), &resp); err != nil {
		t.Fatalf("parse JSON: %v, raw: %q", err, stdout)
	}
	if !resp.Valid {
		t.Errorf("expected valid=true, got: %+v", resp)
	}
}

func TestRenderProfileCommand(t *testing.T) {
	home, _, _ := setupProfileTest(t, "my-profile",
		`name = "my-profile"
description = "A test profile"

[links]
test-skill = { skill = "test-skill" }
`)

	stdout, stderr, err := runCmd(t, home, "render", "--profile", "my-profile")
	if err != nil {
		t.Fatalf("render --profile failed: %v, stderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "test-skill") {
		t.Errorf("expected test-skill in render output, got: %q", stdout)
	}

	stdout, stderr, err = runCmd(t, home, "render", "--profile", "my-profile", "--json")
	if err != nil {
		t.Fatalf("render --profile --json failed: %v, stderr: %s", err, stderr)
	}
	var results []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(stdout), &results); err != nil {
		t.Fatalf("parse JSON: %v, raw: %q", err, stdout)
	}
	if len(results) == 0 || results[0].Name != "test-skill" {
		t.Errorf("expected test-skill in render results, got: %+v", results)
	}

	_, _, err = runCmd(t, home, "render", "--profile", "my-profile", "/some/dir")
	if err == nil {
		t.Fatal("expected render --profile with positional arg to fail")
	}
}

func TestInstallProfileCommand(t *testing.T) {
	home, _, _ := setupProfileTest(t, "my-profile",
		`name = "my-profile"
description = "A test profile"

[links]
test-skill = { skill = "test-skill" }
`)

	stdout, stderr, err := runCmd(t, home, "install", "--profile", "my-profile", "--dry-run")
	if err != nil {
		t.Fatalf("install --profile --dry-run failed: %v, stderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "planned") {
		t.Errorf("expected 'planned' in dry-run output, got: %q", stdout)
	}

	stdout, stderr, err = runCmd(t, home, "install", "--profile", "my-profile", "--dry-run", "--json")
	if err != nil {
		t.Fatalf("install --profile --dry-run --json failed: %v, stderr: %s", err, stderr)
	}
	var results []struct {
		Action string `json:"action"`
		Name   string `json:"name"`
	}
	if err := json.Unmarshal([]byte(stdout), &results); err != nil {
		t.Fatalf("parse JSON: %v, raw: %q", err, stdout)
	}
	if len(results) == 0 || results[0].Name != "test-skill" {
		t.Errorf("expected test-skill in install results, got: %+v", results)
	}

	_, _, err = runCmd(t, home, "install", "--profile", "my-profile", "/some/dir")
	if err == nil {
		t.Fatal("expected install --profile with positional arg to fail")
	}
}

func TestRenderProfileMissingProfile(t *testing.T) {
	home, _, _ := setupProfileTest(t, "existing",
		`name = "existing"
description = "Exists"
[links]
test-skill = { skill = "test-skill" }
`)

	stdout, _, err := runCmd(t, home, "render", "--profile", "nonexistent")
	if err != nil {
		t.Fatalf("render --profile nonexistent should succeed (empty profile), got: %v", err)
	}
	if !strings.Contains(stdout, "No skills in profile") {
		t.Errorf("expected 'No skills in profile', got: %q", stdout)
	}
}

func TestRenderProfileInvalidName(t *testing.T) {
	home, _, _ := setupProfileTest(t, "existing",
		`name = "existing"
description = "Exists"
[links]
test-skill = { skill = "test-skill" }
`)

	_, stderr, err := runCmd(t, home, "render", "--profile", "INVALID NAME")
	if err == nil {
		t.Fatal("expected render --profile with invalid name to fail")
	}
	if !strings.Contains(stderr, "invalid profile name") && !strings.Contains(err.Error(), "invalid profile name") {
		t.Errorf("expected 'invalid profile name' in error, got stderr: %q err: %v", stderr, err)
	}
}

func TestRenderProfileMissingSkill(t *testing.T) {
	home, _, _ := setupProfileTest(t, "broken",
		`name = "broken"
description = "Profile with missing skill"
[links]
nonexistent = { skill = "nonexistent-skill" }
`)

	_, stderr, err := runCmd(t, home, "render", "--profile", "broken")
	if err == nil {
		t.Fatal("expected render --profile with missing skill to fail")
	}
	if !strings.Contains(stderr, "nonexistent-skill") && !strings.Contains(err.Error(), "nonexistent-skill") {
		t.Errorf("expected nonexistent-skill in error, got stderr: %q err: %v", stderr, err)
	}
}

func TestInstallProfileEmptyResolved(t *testing.T) {
	home, _, _ := setupProfileTest(t, "existing",
		`name = "existing"
description = "Exists"
[links]
test-skill = { skill = "test-skill" }
`)

	stdout, _, err := runCmd(t, home, "install", "--profile", "nonexistent", "--dry-run")
	if err != nil {
		t.Fatalf("install --profile nonexistent should succeed (empty profile), got: %v", err)
	}
	if !strings.Contains(stdout, "No skills in profile") {
		t.Errorf("expected 'No skills in profile', got: %q", stdout)
	}
}

func TestProfileValidateInvalidProfile(t *testing.T) {
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")

	profilesDir := filepath.Join(home, ".config", "symskills", "profiles")
	if err := os.MkdirAll(profilesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profilesDir, "broken.toml"), []byte(`name = "broken"
description = "Missing skill"
[links]
nonexistent = { skill = "nonexistent-skill" }
`), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := runCmd(t, home, "profile", "validate", "broken")
	if err == nil {
		t.Fatal("expected profile validate to fail on profile with missing skill")
	}
	if !strings.Contains(stdout, "nonexistent-skill") && !strings.Contains(stderr, "nonexistent-skill") {
		t.Errorf("expected nonexistent-skill in output, got stdout: %q stderr: %q", stdout, stderr)
	}
}

func writeTestSkill(t *testing.T, dir, name, description string) {
	t.Helper()
	data := "---\nname: " + name + "\ndescription: " + description + "\n---\n\n# Body\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestListJSONEmitsEmptyIssuesArray(t *testing.T) {
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")

	stdout, _, err := runCmd(t, home, "list", "--json")
	if err != nil {
		t.Fatalf("list --json failed: %v", err)
	}
	var resp map[string]any
	if err := json.Unmarshal([]byte(stdout), &resp); err != nil {
		t.Fatalf("parse JSON %q: %v", stdout, err)
	}
	issues, ok := resp["issues"]
	if !ok {
		t.Fatalf("expected issues field in JSON output: %s", stdout)
	}
	if issues == nil {
		t.Fatalf("expected issues to be [] instead of null: %s", stdout)
	}
	arr, ok := issues.([]any)
	if !ok {
		t.Fatalf("expected issues to be a JSON array, got: %s", stdout)
	}
	if len(arr) != 0 {
		t.Fatalf("expected empty issues array, got: %s", stdout)
	}
}

func TestValidateJSONEmitsEmptyIssuesArray(t *testing.T) {
	home := t.TempDir()
	skillDir := t.TempDir()
	writeTestSkill(t, skillDir, "valid-skill", "For testing validate JSON issues array")

	stdout, _, err := runCmd(t, home, "validate", "--json", skillDir)
	if err != nil {
		t.Fatalf("validate --json failed: %v", err)
	}
	var resp map[string]any
	if err := json.Unmarshal([]byte(stdout), &resp); err != nil {
		t.Fatalf("parse JSON %q: %v", stdout, err)
	}
	issues, ok := resp["issues"]
	if !ok {
		t.Fatalf("expected issues field in JSON output: %s", stdout)
	}
	if issues == nil {
		t.Fatalf("expected issues to be [] instead of null: %s", stdout)
	}
	arr, ok := issues.([]any)
	if !ok {
		t.Fatalf("expected issues to be a JSON array, got: %s", stdout)
	}
	if len(arr) != 0 {
		t.Fatalf("expected empty issues array, got: %s", stdout)
	}
}

func TestUninstallNotInstalledReportsNothingToDo(t *testing.T) {
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")

	// Text output for a skill that was never installed
	stdout, _, err := runCmd(t, home, "uninstall", "ghost-skill")
	if err != nil {
		t.Fatalf("uninstall of non-installed skill failed: %v", err)
	}
	if !strings.Contains(stdout, "ghost-skill was not installed for opencode") {
		t.Errorf("expected 'not installed' message, got: %q", stdout)
	}
	if strings.Contains(stdout, "Uninstalled") {
		t.Errorf("must not claim success when nothing was installed, got: %q", stdout)
	}

	// JSON output exposes removed:false
	stdout, _, err = runCmd(t, home, "uninstall", "--json", "ghost-skill")
	if err != nil {
		t.Fatalf("uninstall --json failed: %v", err)
	}
	var resp struct {
		Name    string `json:"name"`
		Target  string `json:"target"`
		Removed bool   `json:"removed"`
	}
	if err := json.Unmarshal([]byte(stdout), &resp); err != nil {
		t.Fatalf("parse JSON %q: %v", stdout, err)
	}
	if resp.Removed {
		t.Errorf("expected removed:false for no-op uninstall, got: %s", stdout)
	}
	if resp.Name != "ghost-skill" || resp.Target != "opencode" {
		t.Errorf("unexpected uninstall JSON: %+v", resp)
	}

	// JSON output exposes removed:true after a real uninstall
	skillDir := t.TempDir()
	writeTestSkill(t, skillDir, "real-skill", "For testing uninstall JSON removed true")
	if _, _, err := runCmd(t, home, "install", "--mode", "copy", skillDir); err != nil {
		t.Fatalf("install failed: %v", err)
	}
	stdout, _, err = runCmd(t, home, "uninstall", "--json", "real-skill")
	if err != nil {
		t.Fatalf("uninstall --json failed: %v", err)
	}
	if err := json.Unmarshal([]byte(stdout), &resp); err != nil {
		t.Fatalf("parse JSON %q: %v", stdout, err)
	}
	if !resp.Removed {
		t.Errorf("expected removed:true after real uninstall, got: %s", stdout)
	}
}

// --- Tests for #86: diff must not destructively re-render ---

func TestDiffLeavesRenderDirIntact(t *testing.T) {
	// Running diff must not modify the render directory, even when a
	// symlink install points at it (#86).
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")

	skillDir := t.TempDir()
	writeTestSkill(t, skillDir, "diff-safe", "For testing diff safety")

	// Do a render first so the render cache is populated.
	_, stderr, err := runCmd(t, home, "render", "--target", "opencode", skillDir)
	if err != nil {
		t.Fatalf("render failed: %v, stderr: %s", err, stderr)
	}

	// Find the rendered output path.
	renderDir := filepath.Join(home, ".local", "share", "symskills", "rendered", "opencode", "diff-safe")

	// Verify render output exists.
	if _, err := os.Stat(filepath.Join(renderDir, "SKILL.md")); os.IsNotExist(err) {
		t.Fatalf("render output not found at %s", renderDir)
	}

	// Snapshot the render directory before diff.
	beforeFiles := listDirFiles(t, renderDir)

	// Run diff — this used to call RenderAll on the render dir directly,
	// which would do RemoveAll + re-copy. Now it uses a temp dir.
	_, stderr, err = runCmd(t, home, "diff", "--target", "opencode", skillDir)
	if err != nil {
		t.Fatalf("diff failed: %v, stderr: %s", err, stderr)
	}

	// Verify the render directory is byte-identical after diff.
	afterFiles := listDirFiles(t, renderDir)
	if len(beforeFiles) != len(afterFiles) {
		t.Fatalf("render dir file count changed: %d -> %d", len(beforeFiles), len(afterFiles))
	}
	for path, beforeHash := range beforeFiles {
		afterHash, ok := afterFiles[path]
		if !ok {
			t.Fatalf("file %q disappeared from render dir after diff", path)
		}
		if beforeHash != afterHash {
			t.Fatalf("file %q was modified by diff: hash changed", path)
		}
	}
}

func TestDiffWithSymlinkInstallDoesNotDeleteTarget(t *testing.T) {
	// Install via symlink, then run diff — the symlink target (render dir)
	// must not be deleted (#86).
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")

	skillDir := t.TempDir()
	writeTestSkill(t, skillDir, "diff-symlink", "Symlink diff safety test")

	// Install with symlink mode (default).
	_, stderr, err := runCmd(t, home, "install", "--target", "opencode", "--mode", "symlink", skillDir)
	if err != nil {
		t.Fatalf("install failed: %v, stderr: %s", err, stderr)
	}

	// The installed path should be a symlink.
	installPath := filepath.Join(home, ".config", "opencode", "skills", "diff-symlink")
	fi, err := os.Lstat(installPath)
	if err != nil {
		t.Fatalf("lstat install path: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("install path is not a symlink")
	}

	// Resolve the symlink to find the render dir.
	renderDir, err := os.Readlink(installPath)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if !filepath.IsAbs(renderDir) {
		renderDir = filepath.Join(filepath.Dir(installPath), renderDir)
	}

	// Snapshot the render dir.
	beforeFiles := listDirFiles(t, renderDir)

	// Run diff.
	_, stderr, err = runCmd(t, home, "diff", "--target", "opencode", skillDir)
	if err != nil {
		t.Fatalf("diff failed: %v, stderr: %s", err, stderr)
	}

	// Symlink must still exist and point to an existing directory.
	if _, err := os.Stat(installPath); err != nil {
		t.Fatalf("install symlink broken after diff: %v", err)
	}
	if _, err := os.Stat(renderDir); err != nil {
		t.Fatalf("render dir (symlink target) missing after diff: %v", err)
	}

	// Render dir must be intact.
	afterFiles := listDirFiles(t, renderDir)
	for path, beforeHash := range beforeFiles {
		afterHash, ok := afterFiles[path]
		if !ok {
			t.Fatalf("file %q disappeared from render dir after diff", path)
		}
		if beforeHash != afterHash {
			t.Fatalf("file %q was modified by diff: hash changed", path)
		}
	}
}

func listDirFiles(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[rel] = string(data)
		return nil
	})
	if err != nil {
		t.Fatalf("walkDir %s: %v", dir, err)
	}
	return out
}

func TestTargetsCommand(t *testing.T) {
	home := t.TempDir()
	stdout, stderr, err := runCmd(t, home, "targets", "--json")
	if err != nil {
		t.Fatalf("targets failed: %v, stderr: %s", err, stderr)
	}
	var resp struct {
		Targets []struct {
			Target string `json:"target"`
		} `json:"targets"`
	}
	if err := json.Unmarshal([]byte(stdout), &resp); err != nil {
		t.Fatalf("unmarshal json: %v, stdout: %s", err, stdout)
	}
	if len(resp.Targets) != 4 {
		t.Fatalf("expected 4 targets, got %d", len(resp.Targets))
	}
}

func TestDiscoverCommand(t *testing.T) {
	home := t.TempDir()
	skillDir := filepath.Join(home, "my-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	skillMD := "---\nname: my-skill\ndescription: Test skill\n---\n\nBody"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	stdout, stderr, err := runCmd(t, home, "discover", "--json", skillDir)
	if err != nil {
		t.Fatalf("discover failed: %v, stderr: %s", err, stderr)
	}
	var resp struct {
		Candidates []struct {
			DisplayName string `json:"display_name"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal([]byte(stdout), &resp); err != nil {
		t.Fatalf("unmarshal json: %v, stdout: %s", err, stdout)
	}
	if len(resp.Candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(resp.Candidates))
	}
}
