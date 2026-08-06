package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// writeTestSkillBody writes a SKILL.md with the given name and a body
// marker (distinct from the shared writeTestSkill helper so v1/v2 states
// can be told apart on disk).
func writeTestSkillBody(t *testing.T, dir, name, body string) {
	t.Helper()
	data := "---\nname: " + name + "\ndescription: " + name + "\n---\n\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

// gitHeadCLI returns the full HEAD hash of the repo at dir.
func gitHeadCLI(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// gitCommitCLI makes a hand-made commit in the repo at dir.
func gitCommitCLI(t *testing.T, dir, subject string) {
	t.Helper()
	cmd := exec.Command("git", "-c", "user.name=symskills", "-c", "user.email=symskills@localhost", "-c", "commit.gpgsign=false", "commit", "-am", subject)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit %q: %v: %s", subject, err, out)
	}
}

// TestHistoryCommandListsCommits locks the #119 acceptance criterion:
// `symskills history` lists the commits produced by the import/update
// flow with their operation labels.
func TestHistoryCommandListsCommits(t *testing.T) {
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")

	skillDir := t.TempDir()
	writeTestSkillBody(t, skillDir, "hist-e2e", "v1")
	if _, stderr, err := runCmd(t, home, "import", skillDir); err != nil {
		t.Fatalf("import failed: %v, stderr: %s", err, stderr)
	}
	writeTestSkillBody(t, skillDir, "hist-e2e", "v2")
	if _, stderr, err := runCmd(t, home, "import", "--update", skillDir); err != nil {
		t.Fatalf("update failed: %v, stderr: %s", err, stderr)
	}

	stdout, _, err := runCmd(t, home, "history", "hist-e2e")
	if err != nil {
		t.Fatalf("history failed: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 history rows, got %d: %q", len(lines), stdout)
	}
	fields := strings.Split(lines[0], "\t")
	if len(fields) != 4 || fields[2] != "update" {
		t.Errorf("expected newest row with operation update, got %q", lines[0])
	}
	fields = strings.Split(lines[1], "\t")
	if fields[2] != "import" {
		t.Errorf("expected oldest row with operation import, got %q", lines[1])
	}
	if len(fields[0]) != 40 {
		t.Errorf("expected a full revision hash, got %q", fields[0])
	}

	// JSON shape.
	stdout, _, err = runCmd(t, home, "history", "--json", "hist-e2e")
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Name    string `json:"name"`
		History []struct {
			Revision  string   `json:"revision"`
			Timestamp string   `json:"timestamp"`
			Operation string   `json:"operation"`
			Files     []string `json:"files"`
		} `json:"history"`
	}
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		t.Fatalf("parse history JSON: %v\nraw: %s", err, stdout)
	}
	if parsed.Name != "hist-e2e" || len(parsed.History) != 2 {
		t.Fatalf("unexpected JSON payload: %s", stdout)
	}
	if parsed.History[0].Operation != "update" || parsed.History[1].Operation != "import" {
		t.Errorf("unexpected operations in JSON: %s", stdout)
	}
	if len(parsed.History[1].Files) != 1 || parsed.History[1].Files[0] != "SKILL.md" {
		t.Errorf("expected the import commit to touch SKILL.md, got %#v", parsed.History[1].Files)
	}
}

func TestHistoryCommandLimitsAndRejects(t *testing.T) {
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")
	skillDir := t.TempDir()
	writeTestSkillBody(t, skillDir, "hist-limit", "v1")
	if _, _, err := runCmd(t, home, "import", skillDir); err != nil {
		t.Fatal(err)
	}
	stdout, _, err := runCmd(t, home, "history", "--limit", "1", "hist-limit")
	if err != nil {
		t.Fatal(err)
	}
	if n := len(strings.Split(strings.TrimSpace(stdout), "\n")); n != 1 {
		t.Fatalf("expected 1 row with --limit 1, got %d", n)
	}
	if _, _, err := runCmd(t, home, "history", "--limit", "0", "hist-limit"); err == nil {
		t.Fatal("expected --limit 0 to be rejected")
	}
	if _, _, err := runCmd(t, home, "history", "missing-skill"); err == nil {
		t.Fatal("expected an error for an unknown skill")
	}
}

// TestShowCommandPrintsStateAtRevision locks the #119 acceptance
// criterion for `symskills show`: the skill's state at the revision.
func TestShowCommandPrintsStateAtRevision(t *testing.T) {
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")
	skillDir := t.TempDir()
	writeTestSkillBody(t, skillDir, "show-e2e", "v1 body")
	if _, _, err := runCmd(t, home, "import", skillDir); err != nil {
		t.Fatal(err)
	}
	writeTestSkillBody(t, skillDir, "show-e2e", "v2 body")
	if _, _, err := runCmd(t, home, "import", "--update", skillDir); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := runCmd(t, home, "show", "show-e2e", "--rev", "HEAD~1")
	if err != nil {
		t.Fatalf("show failed: %v", err)
	}
	if !strings.Contains(stdout, "v1 body") {
		t.Errorf("expected v1 body at HEAD~1, got %q", stdout)
	}
	if strings.Contains(stdout, "v2 body") {
		t.Errorf("unexpected v2 content at HEAD~1: %q", stdout)
	}

	// --diff against the current state shows the v1 -> v2 change.
	stdout, _, err = runCmd(t, home, "show", "show-e2e", "--rev", "HEAD~1", "--diff")
	if err != nil {
		t.Fatalf("show --diff failed: %v", err)
	}
	if !strings.Contains(stdout, "Diff against current state") || !strings.Contains(stdout, "-v1 body") || !strings.Contains(stdout, "+v2 body") {
		t.Errorf("expected diff section with v1->v2 change, got %q", stdout)
	}

	// JSON shape carries the same state.
	stdout, _, err = runCmd(t, home, "show", "--json", "show-e2e", "--rev", "HEAD~1")
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Name    string   `json:"name"`
		Rev     string   `json:"rev"`
		SkillMD string   `json:"skill_md"`
		Files   []string `json:"files"`
	}
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		t.Fatalf("parse show JSON: %v", err)
	}
	if parsed.Name != "show-e2e" || !strings.Contains(parsed.SkillMD, "v1 body") {
		t.Errorf("unexpected show JSON: %s", stdout)
	}
}

// TestRestoreCommandRollsBackFiles locks the #119 acceptance criterion:
// `symskills restore --to <rev>` returns the skill's files to that state
// and leaves the intermediate history intact (verified by git log).
func TestRestoreCommandRollsBackFiles(t *testing.T) {
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")
	skillDir := t.TempDir()
	writeTestSkillBody(t, skillDir, "restore-e2e", "v1 body")
	if _, _, err := runCmd(t, home, "import", skillDir); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(libraryDirFor(home), "restore-e2e")
	first := gitHeadCLI(t, repo)
	writeTestSkillBody(t, skillDir, "restore-e2e", "v2 body")
	if _, _, err := runCmd(t, home, "import", "--update", skillDir); err != nil {
		t.Fatal(err)
	}
	writeTestSkillBody(t, skillDir, "restore-e2e", "v3 body")
	if _, _, err := runCmd(t, home, "import", "--update", skillDir); err != nil {
		t.Fatal(err)
	}
	if n := gitLogCountCLI(t, repo); n != 3 {
		t.Fatalf("expected 3 commits before restore, got %d", n)
	}

	stdout, _, err := runCmd(t, home, "restore", "restore-e2e", "--to", first)
	if err != nil {
		t.Fatalf("restore failed: %v", err)
	}
	if !strings.Contains(stdout, "restored restore-e2e to "+first) {
		t.Errorf("expected restored line, got %q", stdout)
	}
	data, err := os.ReadFile(filepath.Join(repo, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "v1 body") {
		t.Errorf("expected v1 content after restore, got %q", data)
	}
	if _, err := os.Stat(filepath.Join(repo, ".git")); err != nil {
		t.Errorf("restore must preserve .git: %v", err)
	}
	// Intermediate history intact: 4 commits, the v2 commit still present.
	if n := gitLogCountCLI(t, repo); n != 4 {
		t.Fatalf("expected 4 commits after restore (import, update, update, restore), got %d", n)
	}
	cmd := exec.Command("git", "log", "--format=%s")
	cmd.Dir = repo
	subjects, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(subjects), "restore: skill restore-e2e to "+first) {
		t.Errorf("expected a forward restore commit, got %q", subjects)
	}
}

// TestRestoreDryRunWritesNothing locks the #119 acceptance criterion:
// --dry-run writes nothing.
func TestRestoreDryRunWritesNothing(t *testing.T) {
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")
	skillDir := t.TempDir()
	writeTestSkillBody(t, skillDir, "dryrun-e2e", "v1")
	if _, _, err := runCmd(t, home, "import", skillDir); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(libraryDirFor(home), "dryrun-e2e")
	first := gitHeadCLI(t, repo)
	writeTestSkillBody(t, skillDir, "dryrun-e2e", "v2")
	if _, _, err := runCmd(t, home, "import", "--update", skillDir); err != nil {
		t.Fatal(err)
	}
	before := gitLogCountCLI(t, repo)

	stdout, _, err := runCmd(t, home, "restore", "dryrun-e2e", "--to", first, "--dry-run")
	if err != nil {
		t.Fatalf("restore --dry-run failed: %v", err)
	}
	if !strings.Contains(stdout, "would restore dryrun-e2e to "+first) {
		t.Errorf("expected plan line, got %q", stdout)
	}
	if !strings.Contains(stdout, "would change SKILL.md") {
		t.Errorf("expected changed-files plan, got %q", stdout)
	}
	if after := gitLogCountCLI(t, repo); after != before {
		t.Fatalf("dry-run wrote a commit: %d -> %d", before, after)
	}
	data, err := os.ReadFile(filepath.Join(repo, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "v2") {
		t.Fatalf("dry-run modified the skill files: %q", data)
	}
}

// TestRestoreRefusesInvalidRevisionState locks the #119 acceptance
// criterion: restoring to a revision that fails validation is refused
// with the validation error.
func TestRestoreRefusesInvalidRevisionState(t *testing.T) {
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")
	skillDir := t.TempDir()
	writeTestSkillBody(t, skillDir, "bad-restore", "v1")
	if _, _, err := runCmd(t, home, "import", skillDir); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(libraryDirFor(home), "bad-restore")
	// Hand-made commit with a broken SKILL.md.
	if err := os.WriteFile(filepath.Join(repo, "SKILL.md"), []byte("not a skill at all\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCommitCLI(t, repo, "user: broken skill")

	_, _, err := runCmd(t, home, "restore", "bad-restore", "--to", "HEAD")
	if err == nil {
		t.Fatal("expected restore to refuse an invalid revision state")
	}
	if !strings.Contains(err.Error(), "refusing restore") || !strings.Contains(err.Error(), "frontmatter") {
		t.Errorf("expected refusal naming the validation error, got %v", err)
	}
	// Nothing was written.
	if n := gitLogCountCLI(t, repo); n != 2 {
		t.Fatalf("expected no new commit after refused restore, got %d", n)
	}
}

// TestRestoreRefusesDirtyWithoutFlag locks the #119 acceptance criterion:
// uncommitted changes are never lost without an explicit flag.
func TestRestoreRefusesDirtyWithoutFlag(t *testing.T) {
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")
	skillDir := t.TempDir()
	writeTestSkillBody(t, skillDir, "dirty-restore", "v1")
	if _, _, err := runCmd(t, home, "import", skillDir); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(libraryDirFor(home), "dirty-restore")
	if err := os.WriteFile(filepath.Join(repo, "SKILL.md"), []byte("---\nname: dirty-restore\ndescription: dirty-restore\n---\n\nuncommitted edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, err := runCmd(t, home, "restore", "dirty-restore", "--to", "HEAD")
	if err == nil {
		t.Fatal("expected restore to refuse a dirty tree")
	}
	if !strings.Contains(err.Error(), "uncommitted changes") || !strings.Contains(err.Error(), "--allow-dirty") {
		t.Errorf("expected refusal naming uncommitted changes and --allow-dirty, got %v", err)
	}
	// The dirty edit is untouched.
	data, err := os.ReadFile(filepath.Join(repo, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "uncommitted edit") {
		t.Fatalf("refused restore must not touch the working tree: %q", data)
	}
}

// TestRestoreAllowDirtySnapshots locks the #119 acceptance criterion:
// with the explicit flag the uncommitted changes are committed, never
// discarded.
func TestRestoreAllowDirtySnapshots(t *testing.T) {
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")
	skillDir := t.TempDir()
	writeTestSkillBody(t, skillDir, "dirty-ok", "v1")
	if _, _, err := runCmd(t, home, "import", skillDir); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(libraryDirFor(home), "dirty-ok")
	first := gitHeadCLI(t, repo)
	writeTestSkillBody(t, skillDir, "dirty-ok", "v2")
	if _, _, err := runCmd(t, home, "import", "--update", skillDir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "SKILL.md"), []byte("---\nname: dirty-ok\ndescription: dirty-ok\n---\n\nuncommitted edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := gitLogCountCLI(t, repo)

	stdout, _, err := runCmd(t, home, "restore", "dirty-ok", "--to", first, "--allow-dirty")
	if err != nil {
		t.Fatalf("restore --allow-dirty failed: %v", err)
	}
	if !strings.Contains(stdout, "restored") {
		t.Errorf("expected restored line, got %q", stdout)
	}
	if n := gitLogCountCLI(t, repo); n != before+2 {
		t.Fatalf("expected snapshot + restore commits, got %d -> %d", before, n)
	}
	// The snapshot commit holds the local edit.
	cmd := exec.Command("git", "show", "HEAD~1:SKILL.md")
	cmd.Dir = repo
	snapshot, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(snapshot), "uncommitted edit") {
		t.Errorf("uncommitted changes were lost: snapshot holds %q", snapshot)
	}
}

func TestRestoreNotVersionedAndBadRevision(t *testing.T) {
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")
	// Hand-written library skill, never imported: no repository.
	plain := filepath.Join(libraryDirFor(home), "plain-skill")
	if err := os.MkdirAll(plain, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestSkillBody(t, plain, "plain-skill", "unversioned")
	_, _, err := runCmd(t, home, "restore", "plain-skill", "--to", "HEAD")
	if err == nil || !strings.Contains(err.Error(), "not versioned") {
		t.Fatalf("expected not-versioned refusal, got %v", err)
	}

	skillDir := t.TempDir()
	writeTestSkillBody(t, skillDir, "rev-check", "v1")
	if _, _, err := runCmd(t, home, "import", skillDir); err != nil {
		t.Fatal(err)
	}
	_, _, err = runCmd(t, home, "restore", "rev-check", "--to", "no-such-rev")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected unknown-revision error, got %v", err)
	}
	if _, _, err := runCmd(t, home, "restore", "rev-check"); err == nil {
		t.Fatal("expected --to to be required")
	}
	if _, _, err := runCmd(t, home, "history", "plain-skill"); err == nil {
		t.Fatal("expected history to fail for an unversioned skill")
	}
}

// TestRestoreReportsStaleTargetsAndSync locks the #119 acceptance
// criterion: after a restore the stale installs are reported with the
// #115 resync path, and --sync performs it.
func TestRestoreReportsStaleTargetsAndSync(t *testing.T) {
	home := t.TempDir()
	_, _, _ = runCmd(t, home, "init")
	skillDir := t.TempDir()
	writeTestSkillBody(t, skillDir, "stale-e2e", "v1 body")
	if _, _, err := runCmd(t, home, "import", skillDir); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(libraryDirFor(home), "stale-e2e")
	first := gitHeadCLI(t, repo)
	writeTestSkillBody(t, skillDir, "stale-e2e", "v2 body")
	if _, _, err := runCmd(t, home, "import", "--update", skillDir); err != nil {
		t.Fatal(err)
	}
	// Install the v2 state.
	if _, stderr, err := runCmd(t, home, "install", "--json", "--mode", "copy", repo); err != nil {
		t.Fatalf("install failed: %v, stderr: %s", err, stderr)
	}

	// Restore to v1: the installed copy (v2) is now stale.
	stdout, _, err := runCmd(t, home, "restore", "stale-e2e", "--to", first)
	if err != nil {
		t.Fatalf("restore failed: %v", err)
	}
	if !strings.Contains(stdout, "stale: opencode") {
		t.Errorf("expected the stale opencode target to be reported, got %q", stdout)
	}
	if !strings.Contains(stdout, "sync --skill stale-e2e") {
		t.Errorf("expected the #115 resync path to be named, got %q", stdout)
	}

	// Restore again with --sync: no state change, but the stale install is
	// resynced from the restored library state.
	stdout, _, err = runCmd(t, home, "restore", "stale-e2e", "--to", first, "--sync")
	if err != nil {
		t.Fatalf("restore --sync failed: %v", err)
	}
	if !strings.Contains(stdout, "already at") {
		t.Errorf("expected already-at-rev line, got %q", stdout)
	}
	if !strings.Contains(stdout, "synced opencode") {
		t.Errorf("expected the stale target to be resynced, got %q", stdout)
	}
	// The install now matches the library again.
	stdout, _, err = runCmd(t, home, "status", "--json", "--target", "opencode", "--skill", "stale-e2e")
	if err != nil {
		t.Fatal(err)
	}
	var status struct {
		Installs []struct {
			Status string `json:"status"`
		} `json:"installs"`
	}
	if err := json.Unmarshal([]byte(stdout), &status); err != nil {
		t.Fatalf("parse status JSON: %v", err)
	}
	if len(status.Installs) != 1 || status.Installs[0].Status != "in-sync" {
		t.Errorf("expected the install to be in-sync after --sync, got %s", stdout)
	}
}
