package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/danieljustus/symaira-skills/internal/events"
	"github.com/danieljustus/symaira-skills/internal/render"
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
	home := t.TempDir()
	t.Setenv("HOME", home)
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	code := runMain(newRootCmd("test-version"), []string{"version"})
	_ = w.Close()
	os.Stderr = oldStderr
	_, _ = io.ReadAll(r)
	if code != 0 {
		t.Fatalf("expected exit code 0 for version command, got %d", code)
	}
}

func TestRunMainErrorPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	code := runMain(newRootCmd("test-version"), []string{"nonexistent-command"})
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stderr = oldStderr
	stderr, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}

	if code == 0 {
		t.Fatal("expected nonzero exit code for unknown command")
	}
	if !strings.Contains(string(stderr), "symskills:") {
		t.Errorf("expected 'symskills:' error prefix on stderr, got %q", string(stderr))
	}
}

func TestRegisterCustomTargetsFromConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfgDir := filepath.Join(home, ".config", "symskills")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	config := "[[targets]]\nname = \"configured-agent\"\nskill_root_user = \"/tmp/configured-agent/skills\"\n"
	if err := os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	before := len(render.Targets)
	defer func() { render.Targets = render.Targets[:before] }()
	if err := registerCustomTargets(); err != nil {
		t.Fatalf("registerCustomTargets: %v", err)
	}
	spec, ok := render.LookupSpec("configured-agent")
	if !ok {
		t.Fatal("configured custom target was not registered")
	}
	if got := spec.SkillRoot(home, "/project", render.ScopeUser); got != "/tmp/configured-agent/skills" {
		t.Fatalf("configured target skill root = %q", got)
	}
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
	// The bundle directory must match the frontmatter name (agentskills
	// spec requirement enforced as name_dir_mismatch).
	skillDir := filepath.Join(t.TempDir(), "validate-test")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
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
	invalidDir := filepath.Join(t.TempDir(), "bad-skill")
	if err := os.MkdirAll(invalidDir, 0o755); err != nil {
		t.Fatal(err)
	}
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

	skillDir := filepath.Join(t.TempDir(), "cwd-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
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

func eventsLogPath(home string) string {
	return filepath.Join(home, ".local", "share", "symskills", "events.jsonl")
}

func readEvents(t *testing.T, home string) []events.Event {
	t.Helper()
	data, err := os.ReadFile(eventsLogPath(home))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatalf("read events log: %v", err)
	}
	var out []events.Event
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var ev events.Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("malformed event line %q: %v", line, err)
		}
		out = append(out, ev)
	}
	return out
}

func TestEventLogImportAppendsExactlyOneRecord(t *testing.T) {
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")

	skillDir := t.TempDir()
	writeTestSkill(t, skillDir, "log-import", "For testing the event log")

	if _, _, err := runCmd(t, home, "import", skillDir); err != nil {
		t.Fatalf("import failed: %v", err)
	}

	records := readEvents(t, home)
	if len(records) != 1 {
		t.Fatalf("expected exactly one event record after import, got %d: %+v", len(records), records)
	}
	ev := records[0]
	if ev.Event != "import" || ev.Skill != "log-import" || ev.Outcome != "ok" || ev.Actor != "cli" {
		t.Errorf("unexpected event record: %+v", ev)
	}
	if _, err := time.Parse(time.RFC3339, ev.TS); err != nil {
		t.Errorf("ts is not RFC3339: %q (%v)", ev.TS, err)
	}
}

func TestEventLogInstallUninstallAppendRecords(t *testing.T) {
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")

	skillDir := t.TempDir()
	writeTestSkill(t, skillDir, "log-install", "For testing the event log")

	if _, _, err := runCmd(t, home, "import", skillDir); err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if _, _, err := runCmd(t, home, "install", "--mode", "copy", skillDir); err != nil {
		t.Fatalf("install failed: %v", err)
	}
	if _, _, err := runCmd(t, home, "uninstall", "log-install"); err != nil {
		t.Fatalf("uninstall failed: %v", err)
	}

	records := readEvents(t, home)
	if len(records) != 3 {
		t.Fatalf("expected 3 event records (import, install, uninstall), got %d: %+v", len(records), records)
	}
	byEvent := map[string]events.Event{}
	for _, ev := range records {
		byEvent[ev.Event] = ev
	}
	if ev := byEvent["import"]; ev.Skill != "log-install" || ev.Outcome != "ok" {
		t.Errorf("unexpected import record: %+v", ev)
	}
	if ev := byEvent["install"]; ev.Skill != "log-install" || ev.Mode != "copy" || ev.Outcome != "ok" {
		t.Errorf("unexpected install record: %+v", ev)
	}
	if ev := byEvent["uninstall"]; ev.Skill != "log-install" || ev.Outcome != "ok" {
		t.Errorf("unexpected uninstall record: %+v", ev)
	}
}

func TestLogCommandFiltersAndJSON(t *testing.T) {
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")

	for _, name := range []string{"log-alpha", "log-beta"} {
		skillDir := t.TempDir()
		writeTestSkill(t, skillDir, name, "For testing the log command")
		if _, _, err := runCmd(t, home, "import", skillDir); err != nil {
			t.Fatalf("import %s failed: %v", name, err)
		}
	}
	// A failed duplicate import also logs, with outcome error.
	dupDir := t.TempDir()
	writeTestSkill(t, dupDir, "log-alpha", "Duplicate")
	if _, _, err := runCmd(t, home, "import", dupDir); err == nil {
		t.Fatal("expected duplicate import to fail")
	}

	// Filter by skill: only log-alpha records, chronological, both outcomes.
	stdout, _, err := runCmd(t, home, "log", "--skill", "log-alpha")
	if err != nil {
		t.Fatalf("log --skill failed: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 log-alpha records, got %d: %q", len(lines), stdout)
	}
	if !strings.Contains(lines[0], "import") || !strings.Contains(lines[0], "log-alpha") {
		t.Errorf("unexpected log line: %q", lines[0])
	}
	if !strings.Contains(lines[1], "error") {
		t.Errorf("expected the duplicate-import failure line to carry outcome error: %q", lines[1])
	}

	// --json emits raw records.
	stdout, _, err = runCmd(t, home, "log", "--json")
	if err != nil {
		t.Fatalf("log --json failed: %v", err)
	}
	var records []events.Event
	if err := json.Unmarshal([]byte(stdout), &records); err != nil {
		t.Fatalf("parse log --json: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("expected 3 records in --json output, got %d", len(records))
	}
	if records[0].TS > records[1].TS || records[1].TS > records[2].TS {
		t.Errorf("records are not in chronological order: %+v", records)
	}
}

func TestInstallSurvivesUnwritableLog(t *testing.T) {
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")

	// Block the log location: a directory at events.jsonl makes the append
	// fail with EISDIR. The operation must still succeed (best-effort log).
	if err := os.MkdirAll(eventsLogPath(home), 0o755); err != nil {
		t.Fatal(err)
	}

	skillDir := t.TempDir()
	writeTestSkill(t, skillDir, "log-blocked", "Install must survive a blocked log")
	if _, _, err := runCmd(t, home, "import", skillDir); err != nil {
		t.Fatalf("import failed although the log is blocked: %v", err)
	}
	if _, _, err := runCmd(t, home, "install", "--mode", "copy", skillDir); err != nil {
		t.Fatalf("install failed although the log is blocked: %v", err)
	}
}

func TestDoctorPrintsLogPath(t *testing.T) {
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")

	stdout, _, err := runCmd(t, home, "doctor")
	if err != nil {
		t.Fatalf("doctor failed: %v", err)
	}
	if !strings.Contains(stdout, "log:") || !strings.Contains(stdout, "events.jsonl") {
		t.Errorf("expected doctor to print the log path, got: %q", stdout)
	}

	stdout, _, err = runCmd(t, home, "doctor", "--json")
	if err != nil {
		t.Fatalf("doctor --json failed: %v", err)
	}
	var resp struct {
		LogPath string `json:"log_path"`
	}
	if err := json.Unmarshal([]byte(stdout), &resp); err != nil {
		t.Fatalf("parse doctor --json: %v", err)
	}
	if !strings.HasSuffix(resp.LogPath, "events.jsonl") {
		t.Errorf("expected log_path to end in events.jsonl, got %q", resp.LogPath)
	}
}

func TestServeCommand(t *testing.T) {
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")

	// stdio is the only transport: `serve` must work without the flag, and
	// `serve --stdio` must remain accepted for backward compatibility with
	// existing MCP client configs. stdout carries only JSON-RPC frames.
	for _, args := range [][]string{{"serve"}, {"serve", "--stdio"}} {
		req := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}` + "\n"

		stdinR, stdinW, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		stdoutR, stdoutW, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		oldStdin, oldStdout := os.Stdin, os.Stdout
		os.Stdin, os.Stdout = stdinR, stdoutW

		var cmdErr error
		done := make(chan struct{})
		go func() {
			defer close(done)
			cmd := newRootCmd("test-version")
			cmd.SetArgs(args)
			cmdErr = cmd.Execute()
		}()

		if _, err := stdinW.WriteString(req); err != nil {
			t.Fatal(err)
		}
		if err := stdinW.Close(); err != nil {
			t.Fatal(err)
		}
		<-done

		if err := stdoutW.Close(); err != nil {
			t.Fatal(err)
		}
		stdout, err := io.ReadAll(stdoutR)
		os.Stdin, os.Stdout = oldStdin, oldStdout
		if err != nil {
			t.Fatal(err)
		}

		if cmdErr != nil {
			t.Fatalf("serve %v failed: %v", args, cmdErr)
		}
		var resp struct {
			JSONRPC string `json:"jsonrpc"`
			ID      int    `json:"id"`
			Result  struct {
				ServerInfo struct {
					Name string `json:"name"`
				} `json:"serverInfo"`
			} `json:"result"`
		}
		// The whole stdout stream must parse as a single JSON-RPC frame:
		// any diagnostic line on stdout would break this assertion.
		if err := json.Unmarshal(stdout, &resp); err != nil {
			t.Fatalf("serve %v: stdout is not a clean JSON-RPC frame: %v; got %q", args, err, stdout)
		}
		if resp.JSONRPC != "2.0" || resp.ID != 1 {
			t.Fatalf("serve %v: unexpected JSON-RPC frame: %q", args, stdout)
		}
		if resp.Result.ServerInfo.Name != "symskills" {
			t.Fatalf("serve %v: unexpected server name in %q", args, stdout)
		}
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
	skillDir := filepath.Join(t.TempDir(), "valid-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
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

func TestDiffSymlinkReportsHarnessEdits(t *testing.T) {
	// #123 regression: in the default symlink mode the installed path and
	// the rendered path resolve to the same directory, so the legacy
	// two-way comparison could never report drift. Anchored on the base
	// snapshot, diff must report harness-side edits.
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")

	skillDir := t.TempDir()
	writeTestSkill(t, skillDir, "diff-symlink-edit", "For testing symlink diff drift")
	if err := os.WriteFile(filepath.Join(skillDir, "notes.txt"), []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Default install mode is symlink.
	if _, stderr, err := runCmd(t, home, "install", "--target", "opencode", skillDir); err != nil {
		t.Fatalf("install failed: %v, stderr: %s", err, stderr)
	}

	installPath := filepath.Join(home, ".config", "opencode", "skills", "diff-symlink-edit")
	fi, err := os.Lstat(installPath)
	if err != nil {
		t.Fatalf("lstat install path: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("install path is not a symlink")
	}

	// Pristine diff: nothing changed.
	stdout, _, err := runCmd(t, home, "diff", "--target", "opencode", skillDir)
	if err != nil {
		t.Fatalf("diff failed: %v", err)
	}
	if !strings.Contains(stdout, "No changes detected.") {
		t.Fatalf("expected no changes on pristine install, got: %q", stdout)
	}

	// Harness-side edit: modify the installed SKILL.md through the symlink.
	installedSkill := filepath.Join(installPath, "SKILL.md")
	if err := os.WriteFile(installedSkill, []byte("---\nname: diff-symlink-edit\ndescription: edited by hand\n---\n\n# Changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, _, err = runCmd(t, home, "diff", "--target", "opencode", skillDir)
	if err != nil {
		t.Fatalf("diff failed: %v", err)
	}
	if !strings.Contains(stdout, "modified") || !strings.Contains(stdout, "SKILL.md") {
		t.Fatalf("expected modified SKILL.md after harness edit, got: %q", stdout)
	}

	// Harness-side addition and deletion of support files.
	if err := os.WriteFile(filepath.Join(installPath, "user-added.txt"), []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(installPath, "notes.txt")); err != nil {
		t.Fatal(err)
	}
	stdout, _, err = runCmd(t, home, "diff", "--target", "opencode", "--json", skillDir)
	if err != nil {
		t.Fatalf("diff --json failed: %v", err)
	}
	var resp []struct {
		Path   string `json:"path"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(stdout), &resp); err != nil {
		t.Fatalf("parse JSON %q: %v", stdout, err)
	}
	byPath := map[string]string{}
	for _, c := range resp {
		byPath[c.Path] = c.Status
	}
	if byPath["user-added.txt"] != "added" {
		t.Errorf("expected user-added.txt added, got %q (all: %+v)", byPath["user-added.txt"], resp)
	}
	if byPath["notes.txt"] != "removed" {
		t.Errorf("expected notes.txt removed, got %q (all: %+v)", byPath["notes.txt"], resp)
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
	if len(resp.Targets) != 6 {
		t.Fatalf("expected 6 targets, got %d", len(resp.Targets))
	}
}

func TestTargetsCommandHumanReadable(t *testing.T) {
	home := t.TempDir()

	// Default user scope: human-readable lines, exit code 0.
	stdout, stderr, err := runCmd(t, home, "targets")
	if err != nil {
		t.Fatalf("targets failed: %v, stderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "Target:") {
		t.Errorf("expected 'Target:' line, got: %q", stdout)
	}
	if !strings.Contains(stdout, "Status:") || !strings.Contains(stdout, "Setup Hint:") {
		t.Errorf("expected status/setup-hint lines, got: %q", stdout)
	}

	// Project scope branch.
	stdout, stderr, err = runCmd(t, home, "targets", "--scope=project")
	if err != nil {
		t.Fatalf("targets --scope=project failed: %v, stderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "Target:") {
		t.Errorf("expected project-scope output, got: %q", stdout)
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
		SchemaVersion int `json:"schema_version"`
		Candidates    []struct {
			DisplayName string `json:"display_name"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal([]byte(stdout), &resp); err != nil {
		t.Fatalf("unmarshal json: %v, stdout: %s", err, stdout)
	}
	if resp.SchemaVersion != 1 {
		t.Fatalf("expected schema_version 1, got %d", resp.SchemaVersion)
	}
	if len(resp.Candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(resp.Candidates))
	}
}

func TestDiscoverCommandHumanReadable(t *testing.T) {
	home := t.TempDir()
	skillDir := filepath.Join(home, "my-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	skillMD := "---\nname: my-skill\ndescription: Test skill\n---\n\nBody"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	// Candidate branch: human-readable lines, exit code 0.
	stdout, stderr, err := runCmd(t, home, "discover", skillDir)
	if err != nil {
		t.Fatalf("discover failed: %v, stderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "(unmanaged") || !strings.Contains(stdout, "my-skill") {
		t.Errorf("expected unmanaged my-skill line, got: %q", stdout)
	}
	if !strings.Contains(stdout, "Location:") {
		t.Errorf("expected 'Location:' line, got: %q", stdout)
	}

	// Empty branch: no harness roots in a fresh HOME, no explicit paths.
	stdout, stderr, err = runCmd(t, home, "discover")
	if err != nil {
		t.Fatalf("discover (empty) failed: %v, stderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "No skill candidates discovered.") {
		t.Errorf("expected empty-discovery message, got: %q", stdout)
	}
}

func TestListJSONIncludesMetadata(t *testing.T) {
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")

	skillDir := filepath.Join(t.TempDir(), "meta-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestSkill(t, skillDir, "meta-skill", "For testing metadata")

	lib := filepath.Join(home, ".local", "share", "symskills", "library")
	if err := os.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(skillDir, filepath.Join(lib, "meta-skill")); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := runCmd(t, home, "list", "--json")
	if err != nil {
		t.Fatalf("list --json failed: %v", err)
	}
	var resp struct {
		Skills []struct {
			Name           string  `json:"name"`
			CreatedAt      string  `json:"created_at"`
			ModifiedAt     string  `json:"modified_at"`
			LastRenderedAt string  `json:"last_rendered_at"`
			Installs       []any   `json:"installs"`
			LastUsed       *string `json:"last_used"`
			LastUsedSource string  `json:"last_used_source"`
		} `json:"skills"`
	}
	if err := json.Unmarshal([]byte(stdout), &resp); err != nil {
		t.Fatalf("parse JSON %q: %v", stdout, err)
	}
	if len(resp.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(resp.Skills))
	}
	skill := resp.Skills[0]
	if skill.Name != "meta-skill" {
		t.Errorf("expected meta-skill, got %q", skill.Name)
	}
	if skill.CreatedAt == "" {
		t.Error("expected created_at from the filesystem")
	}
	if skill.ModifiedAt == "" {
		t.Error("expected modified_at from the filesystem")
	}
	if skill.LastRenderedAt != "" {
		t.Errorf("expected empty last_rendered_at without evidence, got %q", skill.LastRenderedAt)
	}
	if skill.Installs == nil {
		t.Error("expected installs to be [] instead of null")
	}
	if len(skill.Installs) != 0 {
		t.Errorf("expected no installs, got %v", skill.Installs)
	}
	if skill.LastUsed != nil {
		t.Errorf("expected null last_used, got %q", *skill.LastUsed)
	}
	if skill.LastUsedSource != "" {
		t.Errorf("expected empty last_used_source, got %q", skill.LastUsedSource)
	}
}

func TestListSortByChanged(t *testing.T) {
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")
	lib := filepath.Join(home, ".local", "share", "symskills", "library")
	if err := os.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}

	old := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for name, ts := range map[string]time.Time{"old-skill": old, "new-skill": newer} {
		dir := filepath.Join(lib, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		writeTestSkill(t, dir, name, "For testing sort")
		if err := os.Chtimes(filepath.Join(dir, "SKILL.md"), ts, ts); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(dir, ts, ts); err != nil {
			t.Fatal(err)
		}
	}

	stdout, _, err := runCmd(t, home, "list", "--json", "--sort", "changed")
	if err != nil {
		t.Fatalf("list --sort changed failed: %v", err)
	}
	var resp struct {
		Skills []struct {
			Name string `json:"name"`
		} `json:"skills"`
	}
	if err := json.Unmarshal([]byte(stdout), &resp); err != nil {
		t.Fatalf("parse JSON %q: %v", stdout, err)
	}
	if len(resp.Skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(resp.Skills))
	}
	if resp.Skills[0].Name != "new-skill" || resp.Skills[1].Name != "old-skill" {
		t.Errorf("expected most-recently-changed first, got %q then %q", resp.Skills[0].Name, resp.Skills[1].Name)
	}

	// Default sort is by name.
	stdout, _, err = runCmd(t, home, "list", "--json")
	if err != nil {
		t.Fatalf("list --json failed: %v", err)
	}
	if err := json.Unmarshal([]byte(stdout), &resp); err != nil {
		t.Fatalf("parse JSON %q: %v", stdout, err)
	}
	if resp.Skills[0].Name != "new-skill" || resp.Skills[1].Name != "old-skill" {
		t.Errorf("expected name order, got %q then %q", resp.Skills[0].Name, resp.Skills[1].Name)
	}
}

func TestListSortInvalid(t *testing.T) {
	home := t.TempDir()
	_, _, err := runCmd(t, home, "list", "--sort", "bogus")
	if err == nil {
		t.Fatal("expected error for invalid --sort value")
	}
	if !strings.Contains(err.Error(), "bogus") {
		t.Errorf("expected sort value in error, got: %v", err)
	}
}

func TestListTableIncludesChangedAndInstalledColumns(t *testing.T) {
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")
	lib := filepath.Join(home, ".local", "share", "symskills", "library")
	if err := os.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(lib, "table-skill")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestSkill(t, dir, "table-skill", "For testing table")

	stdout, _, err := runCmd(t, home, "list")
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	fields := strings.Split(strings.TrimSpace(stdout), "\t")
	if len(fields) != 5 {
		t.Fatalf("expected 5 tab-separated columns, got %d: %q", len(fields), stdout)
	}
	if fields[2] == "" || fields[3] != "never" {
		t.Errorf("expected changed date and never-installed column, got %q", stdout)
	}
}

func TestInspectJSONIncludesMetadata(t *testing.T) {
	home := t.TempDir()
	skillDir := filepath.Join(t.TempDir(), "inspect-meta")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestSkill(t, skillDir, "inspect-meta", "For testing inspect metadata")

	stdout, _, err := runCmd(t, home, "inspect", "--json", skillDir)
	if err != nil {
		t.Fatalf("inspect --json failed: %v", err)
	}
	var resp struct {
		Frontmatter struct {
			Name string `json:"name"`
		} `json:"frontmatter"`
		CreatedAt      string  `json:"created_at"`
		ModifiedAt     string  `json:"modified_at"`
		LastRenderedAt string  `json:"last_rendered_at"`
		Installs       []any   `json:"installs"`
		LastUsed       *string `json:"last_used"`
		LastUsedSource string  `json:"last_used_source"`
	}
	if err := json.Unmarshal([]byte(stdout), &resp); err != nil {
		t.Fatalf("parse JSON %q: %v", stdout, err)
	}
	if resp.Frontmatter.Name != "inspect-meta" {
		t.Errorf("expected inspect-meta, got %q", resp.Frontmatter.Name)
	}
	if resp.CreatedAt == "" || resp.ModifiedAt == "" {
		t.Errorf("expected created_at and modified_at, got %q / %q", resp.CreatedAt, resp.ModifiedAt)
	}
	if resp.LastUsed != nil {
		t.Errorf("expected null last_used, got %q", *resp.LastUsed)
	}
	if resp.LastUsedSource != "" {
		t.Errorf("expected empty last_used_source, got %q", resp.LastUsedSource)
	}
}

func TestInspectTextShowsMetadata(t *testing.T) {
	home := t.TempDir()
	skillDir := t.TempDir()
	writeTestSkill(t, skillDir, "inspect-text", "For testing inspect text")

	stdout, _, err := runCmd(t, home, "inspect", skillDir)
	if err != nil {
		t.Fatalf("inspect failed: %v", err)
	}
	for _, want := range []string{"Created:", "Modified:", "Last rendered:", "Installs: none", "Last used: unknown"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("expected %q in inspect output, got: %q", want, stdout)
		}
	}
}

// --- Tests for #115: status + sync ---

type statusJSON struct {
	Installs []struct {
		Target string `json:"target"`
		Name   string `json:"name"`
		Path   string `json:"path"`
		Status string `json:"status"`
		Mode   string `json:"mode"`
	} `json:"installs"`
	Summary struct {
		InSync    int `json:"in_sync"`
		Stale     int `json:"stale"`
		Orphaned  int `json:"orphaned"`
		Unmanaged int `json:"unmanaged"`
	} `json:"summary"`
}

func parseStatusJSON(t *testing.T, stdout string) statusJSON {
	t.Helper()
	var resp statusJSON
	if err := json.Unmarshal([]byte(stdout), &resp); err != nil {
		t.Fatalf("parse status JSON %q: %v", stdout, err)
	}
	return resp
}

func assertStatusKind(t *testing.T, resp statusJSON, target, name, want string) {
	t.Helper()
	for _, st := range resp.Installs {
		if st.Target == target && st.Name == name {
			if st.Status != want {
				t.Fatalf("status %s/%s = %q, want %q (all: %+v)", target, name, st.Status, want, resp.Installs)
			}
			return
		}
	}
	t.Fatalf("no install %s/%s in %+v", target, name, resp.Installs)
}

// TestStatusSyncCommand is the #115 acceptance loop: editing a library
// skill installed into two targets makes status report both stale, sync
// returns both to in-sync, a second sync reports no work, and
// sync --dry-run writes nothing.
func TestStatusSyncCommand(t *testing.T) {
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")

	lib := filepath.Join(home, ".local", "share", "symskills", "library", "fleet-skill")
	if err := os.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestSkill(t, lib, "fleet-skill", "Fleet status test")
	if err := os.WriteFile(filepath.Join(lib, "notes.txt"), []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Install into two harness targets.
	if _, stderr, err := runCmd(t, home, "install", "--target", "opencode", lib); err != nil {
		t.Fatalf("install opencode: %v, stderr: %s", err, stderr)
	}
	if _, stderr, err := runCmd(t, home, "install", "--target", "claude", lib); err != nil {
		t.Fatalf("install claude: %v, stderr: %s", err, stderr)
	}
	opencodePath := filepath.Join(home, ".config", "opencode", "skills", "fleet-skill")
	claudePath := filepath.Join(home, ".claude", "skills", "fleet-skill")
	renderRoot := filepath.Join(home, ".local", "share", "symskills", "rendered")

	// Fresh installs report in-sync.
	stdout, stderr, err := runCmd(t, home, "status", "--json")
	if err != nil {
		t.Fatalf("status: %v, stderr: %s", err, stderr)
	}
	resp := parseStatusJSON(t, stdout)
	assertStatusKind(t, resp, "opencode", "fleet-skill", "in-sync")
	assertStatusKind(t, resp, "claude", "fleet-skill", "in-sync")
	if resp.Summary.Stale != 0 || resp.Summary.InSync != 2 {
		t.Fatalf("unexpected summary after install: %+v", resp.Summary)
	}

	// Editing the library source makes both targets stale, naming target,
	// skill and install path.
	data, err := os.ReadFile(filepath.Join(lib, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lib, "SKILL.md"), append(data, []byte("\n# v2\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, _, err = runCmd(t, home, "status", "--json")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	resp = parseStatusJSON(t, stdout)
	assertStatusKind(t, resp, "opencode", "fleet-skill", "stale")
	assertStatusKind(t, resp, "claude", "fleet-skill", "stale")
	if resp.Summary.Stale != 2 {
		t.Fatalf("expected 2 stale after edit, got %+v", resp.Summary)
	}
	for _, st := range resp.Installs {
		if st.Status != "stale" {
			continue
		}
		want := opencodePath
		if st.Target == "claude" {
			want = claudePath
		}
		if st.Path != want {
			t.Errorf("stale install path = %q, want %q", st.Path, want)
		}
	}

	// status --strict gates on drift.
	if _, _, err := runCmd(t, home, "status", "--strict"); err == nil {
		t.Fatal("expected status --strict to fail on drift")
	}

	// sync --dry-run prints the plan and writes nothing.
	before := listDirFiles(t, renderRoot)
	stdout, _, err = runCmd(t, home, "sync", "--dry-run", "--json")
	if err != nil {
		t.Fatalf("sync --dry-run: %v", err)
	}
	var plan struct {
		Results []struct {
			Action string `json:"action"`
			Name   string `json:"name"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(stdout), &plan); err != nil {
		t.Fatalf("parse sync plan %q: %v", stdout, err)
	}
	if len(plan.Results) != 2 {
		t.Fatalf("expected 2 planned actions, got %+v", plan.Results)
	}
	for _, p := range plan.Results {
		if p.Action != "planned" {
			t.Errorf("expected planned action, got %+v", p)
		}
	}
	after := listDirFiles(t, renderRoot)
	if len(before) != len(after) {
		t.Fatalf("sync --dry-run changed the render cache: %d -> %d files", len(before), len(after))
	}
	for path, beforeHash := range before {
		if afterHash, ok := after[path]; !ok || afterHash != beforeHash {
			t.Fatalf("sync --dry-run modified %q in the render cache", path)
		}
	}

	// Real sync repairs both installs.
	stdout, stderr, err = runCmd(t, home, "sync")
	if err != nil {
		t.Fatalf("sync: %v, stderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "installed") {
		t.Errorf("expected installed actions, got: %q", stdout)
	}

	// Both installs are in-sync again and a second sync reports no work.
	stdout, _, err = runCmd(t, home, "status", "--json")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	resp = parseStatusJSON(t, stdout)
	assertStatusKind(t, resp, "opencode", "fleet-skill", "in-sync")
	assertStatusKind(t, resp, "claude", "fleet-skill", "in-sync")
	if resp.Summary.Stale != 0 {
		t.Fatalf("expected no stale after sync, got %+v", resp.Summary)
	}
	stdout, _, err = runCmd(t, home, "sync")
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if !strings.Contains(stdout, "No stale installs.") {
		t.Errorf("expected no work on second sync, got: %q", stdout)
	}

	// The install mode from the original install is preserved (symlink).
	fi, err := os.Lstat(claudePath)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Error("claude install is no longer a symlink after sync")
	}
}

// TestStatusSyncOrphanedUntouched pins that an install whose library source
// was deleted is reported orphaned and left untouched by sync.
func TestStatusSyncOrphanedUntouched(t *testing.T) {
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")
	lib := filepath.Join(home, ".local", "share", "symskills", "library", "ghost-skill")
	if err := os.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestSkill(t, lib, "ghost-skill", "Will be deleted from the library")
	if _, stderr, err := runCmd(t, home, "install", "--target", "opencode", lib); err != nil {
		t.Fatalf("install: %v, stderr: %s", err, stderr)
	}
	// The library source is deleted.
	if err := os.RemoveAll(filepath.Join(home, ".local", "share", "symskills", "library", "ghost-skill")); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := runCmd(t, home, "status", "--json")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	resp := parseStatusJSON(t, stdout)
	assertStatusKind(t, resp, "opencode", "ghost-skill", "orphaned")

	// status --strict treats orphaned as drift.
	if _, _, err := runCmd(t, home, "status", "--strict"); err == nil {
		t.Fatal("expected status --strict to fail on orphaned install")
	}

	// sync must not remove or rewrite the orphaned install.
	stdout, _, err = runCmd(t, home, "sync")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if !strings.Contains(stdout, "No stale installs.") {
		t.Errorf("expected orphaned install to be left untouched, got: %q", stdout)
	}
	installPath := filepath.Join(home, ".config", "opencode", "skills", "ghost-skill")
	if _, err := os.Stat(filepath.Join(installPath, ".symskills.json")); err != nil {
		t.Fatalf("orphaned install lost its marker: %v", err)
	}
	if _, err := os.Stat(filepath.Join(installPath, "SKILL.md")); err != nil {
		t.Fatalf("orphaned install lost its content: %v", err)
	}
}

// TestStatusSyncUnmanagedUntouched pins that hand-installed skills (no
// marker) are reported unmanaged and never written to.
func TestStatusSyncUnmanagedUntouched(t *testing.T) {
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")
	handmade := filepath.Join(home, ".config", "opencode", "skills", "handmade")
	if err := os.MkdirAll(handmade, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestSkill(t, handmade, "handmade", "Hand-written, never installed by symskills")

	stdout, _, err := runCmd(t, home, "status", "--json")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	resp := parseStatusJSON(t, stdout)
	assertStatusKind(t, resp, "opencode", "handmade", "unmanaged")

	// sync must not write into the unmanaged directory.
	if _, _, err := runCmd(t, home, "sync"); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if _, err := os.Stat(filepath.Join(handmade, ".symskills.json")); !os.IsNotExist(err) {
		t.Fatal("sync wrote a marker into an unmanaged skill")
	}
	data, err := os.ReadFile(filepath.Join(handmade, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Hand-written") {
		t.Fatalf("unmanaged skill content was modified: %q", string(data))
	}
}

// TestStatusSyncProfileFilter pins the --profile filter on sync.
func TestStatusSyncProfileFilter(t *testing.T) {
	home, _, _ := setupProfileTest(t, "my-profile",
		`name = "my-profile"
description = "A test profile"

[links]
test-skill = { skill = "test-skill" }
`)
	lib := filepath.Join(home, ".local", "share", "symskills", "library", "test-skill")
	if _, stderr, err := runCmd(t, home, "install", "--target", "opencode", lib); err != nil {
		t.Fatalf("install: %v, stderr: %s", err, stderr)
	}
	data, err := os.ReadFile(filepath.Join(lib, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lib, "SKILL.md"), append(data, []byte("\n# v2\n")...), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := runCmd(t, home, "sync", "--profile", "my-profile", "--json")
	if err != nil {
		t.Fatalf("sync --profile: %v, stderr: %s", err, stderr)
	}
	var results struct {
		Results []struct {
			Name   string `json:"name"`
			Action string `json:"action"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(stdout), &results); err != nil {
		t.Fatalf("parse sync results %q: %v", stdout, err)
	}
	if len(results.Results) != 1 || results.Results[0].Name != "test-skill" || results.Results[0].Action != "installed" {
		t.Fatalf("expected one installed test-skill, got %+v", results.Results)
	}

	stdout, _, err = runCmd(t, home, "status", "--json", "--skill", "test-skill")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	resp := parseStatusJSON(t, stdout)
	assertStatusKind(t, resp, "opencode", "test-skill", "in-sync")
}
