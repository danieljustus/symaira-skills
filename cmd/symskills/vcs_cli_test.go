package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitLogCountCLI returns the number of commits in the repo at dir.
func gitLogCountCLI(t *testing.T, dir string) int {
	t.Helper()
	cmd := exec.Command("git", "log", "--format=%s")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git log in %s: %v", dir, err)
	}
	if strings.TrimSpace(string(out)) == "" {
		return 0
	}
	return len(strings.Split(strings.TrimSpace(string(out)), "\n"))
}

func libraryDirFor(home string) string {
	return filepath.Join(home, ".local", "share", "symskills", "library")
}

// TestImportCommandInitializesGitRepo locks the #118 acceptance criterion:
// `symskills import` leaves a git repo with exactly one commit containing
// the full skill.
func TestImportCommandInitializesGitRepo(t *testing.T) {
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")

	skillDir := t.TempDir()
	writeTestSkill(t, skillDir, "vcs-e2e", "For versioning e2e")

	if _, stderr, err := runCmd(t, home, "import", skillDir); err != nil {
		t.Fatalf("import failed: %v, stderr: %s", err, stderr)
	}
	repo := filepath.Join(libraryDirFor(home), "vcs-e2e")
	if _, err := os.Stat(filepath.Join(repo, ".git")); err != nil {
		t.Fatalf("expected .git in imported skill: %v", err)
	}
	if n := gitLogCountCLI(t, repo); n != 1 {
		t.Fatalf("expected exactly one commit, got %d", n)
	}
}

// TestReimportWithUpdateCreatesSecondCommit locks the #118 acceptance
// criterion: re-importing over an existing skill produces a second commit
// and git log shows the previous state intact.
func TestReimportWithUpdateCreatesSecondCommit(t *testing.T) {
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")

	skillDir := t.TempDir()
	writeTestSkill(t, skillDir, "vcs-update-e2e", "v1")
	if _, _, err := runCmd(t, home, "import", skillDir); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(libraryDirFor(home), "vcs-update-e2e")
	first := gitLogCountCLI(t, repo)

	writeTestSkill(t, skillDir, "vcs-update-e2e", "v2")
	if _, stderr, err := runCmd(t, home, "import", "--update", skillDir); err != nil {
		t.Fatalf("re-import --update failed: %v, stderr: %s", err, stderr)
	}
	if n := gitLogCountCLI(t, repo); n != first+1 {
		t.Fatalf("expected %d commits after update, got %d", first+1, n)
	}
	data, err := os.ReadFile(filepath.Join(repo, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "v2") {
		t.Fatalf("expected updated content on disk, got %q", data)
	}
	// Previous state intact: HEAD~1 still carries v1.
	cmd := exec.Command("git", "show", "HEAD~1:SKILL.md")
	cmd.Dir = repo
	prev, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(prev), "v1") {
		t.Fatalf("previous state lost: HEAD~1 holds %q", prev)
	}
}

// TestImportGitMissingDegradationReportedOnce locks the #118 acceptance
// criterion: with git unavailable every command still succeeds and the
// degradation is reported once, not per file.
func TestImportGitMissingDegradationReportedOnce(t *testing.T) {
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")
	t.Setenv("PATH", t.TempDir()) // hide git from the process

	parent := t.TempDir()
	for _, name := range []string{"deg-one", "deg-two"} {
		dir := filepath.Join(parent, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		writeTestSkill(t, dir, name, "degraded")
	}
	stdout, stderr, err := runCmd(t, home, "import", "--batch", parent)
	if err != nil {
		t.Fatalf("batch import must succeed without git: %v", err)
	}
	if !strings.Contains(stdout, "Summary: 2 imported") {
		t.Fatalf("expected both imports to succeed, got: %q", stdout)
	}
	if got := strings.Count(stderr, "git not found on PATH"); got != 1 {
		t.Fatalf("expected exactly one degradation report, got %d in %q", got, stderr)
	}
}

// TestVCSDisabledConfigWritesNoRepos locks the #118 acceptance criterion:
// vcs.enabled = false disables all repository writes.
func TestVCSDisabledConfigWritesNoRepos(t *testing.T) {
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")
	cfgDir := filepath.Join(home, ".config", "symskills")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte("[vcs]\nenabled = false\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	skillDir := t.TempDir()
	writeTestSkill(t, skillDir, "vcs-off-e2e", "unversioned")
	if _, stderr, err := runCmd(t, home, "import", skillDir); err != nil {
		t.Fatalf("import failed: %v, stderr: %s", err, stderr)
	}
	if _, err := os.Stat(filepath.Join(libraryDirFor(home), "vcs-off-e2e", ".git")); err == nil {
		t.Fatal("expected no repository with vcs.enabled = false")
	}
}

// TestDoctorReportsVersioningStatus locks the #118 requirement that
// `symskills doctor` reports the versioning status per skill.
func TestDoctorReportsVersioningStatus(t *testing.T) {
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")

	skillDir := t.TempDir()
	writeTestSkill(t, skillDir, "doc-versioned", "versioned")
	if _, _, err := runCmd(t, home, "import", skillDir); err != nil {
		t.Fatal(err)
	}
	// An unversioned skill: written by hand, never imported.
	plain := filepath.Join(libraryDirFor(home), "doc-plain")
	if err := os.MkdirAll(plain, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestSkill(t, plain, "doc-plain", "plain")

	stdout, _, err := runCmd(t, home, "doctor", "--json")
	if err != nil {
		t.Fatalf("doctor --json failed: %v", err)
	}
	var resp struct {
		VCS struct {
			Enabled      bool `json:"enabled"`
			GitAvailable bool `json:"git_available"`
			Skills       []struct {
				Name   string `json:"name"`
				Status string `json:"status"`
			} `json:"skills"`
		} `json:"vcs"`
	}
	if err := json.Unmarshal([]byte(stdout), &resp); err != nil {
		t.Fatalf("parse doctor JSON: %v\nraw: %s", err, stdout)
	}
	if !resp.VCS.Enabled {
		t.Error("expected vcs.enabled=true in doctor output")
	}
	if !resp.VCS.GitAvailable {
		t.Error("expected git_available=true in doctor output")
	}
	byName := map[string]string{}
	for _, s := range resp.VCS.Skills {
		byName[s.Name] = s.Status
	}
	if byName["doc-versioned"] != "versioned" {
		t.Errorf("expected doc-versioned to be versioned, got %#v", byName)
	}
	if byName["doc-plain"] != "unversioned" {
		t.Errorf("expected doc-plain to be unversioned, got %#v", byName)
	}

	// Human-readable output carries the versioning line too.
	stdout, _, err = runCmd(t, home, "doctor")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "versioning: enabled") {
		t.Errorf("expected versioning line in doctor text output, got %q", stdout)
	}
}

// TestInstalledBundleContainsNoGitDirectory locks the #118 acceptance
// criterion: rendered and installed bundles contain no .git directory.
func TestInstalledBundleContainsNoGitDirectory(t *testing.T) {
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")

	skillDir := t.TempDir()
	writeTestSkill(t, skillDir, "no-git-install", "no git in bundles")
	if _, _, err := runCmd(t, home, "import", skillDir); err != nil {
		t.Fatal(err)
	}
	stdout, _, err := runCmd(t, home, "install", "--json", "--mode", "copy", filepath.Join(libraryDirFor(home), "no-git-install"))
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}
	var result struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("parse install JSON: %v", err)
	}
	if result.Path == "" {
		t.Fatal("install produced no path")
	}
	err = filepath.WalkDir(result.Path, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			t.Errorf("installed bundle contains .git at %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
