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
	stdout, stderr, err = runCmd(t, home, "init")
	if err != nil {
		t.Fatalf("init failed: %v", err)
	}
	if !strings.Contains(stdout, "already exists") {
		t.Errorf("expected already exists in output, got: %q", stdout)
	}

	// Third init with force
	stdout, stderr, err = runCmd(t, home, "init", "--force")
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

	// Diff standard (should show no changes)
	stdout, _, err := runCmd(t, home, "diff", skillDir)
	if err != nil {
		t.Fatalf("diff failed: %v", err)
	}
	if stdout != "" {
		t.Errorf("expected empty stdout for no changes, got: %q", stdout)
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
	var resp struct {
		ConfigPath string `json:"config_path"`
		Config     any    `json:"config"`
		Targets    []any  `json:"targets"`
	}
	if err := json.Unmarshal([]byte(stdout), &resp); err != nil {
		t.Fatalf("parse JSON: %v", err)
	}
	if resp.ConfigPath == "" {
		t.Error("expected config_path to be populated")
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
		Valid  bool `json:"valid"`
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
