// Package vcs implements per-skill git versioning of the skill library
// (#118). Every skill in the managed library owns an independent git
// repository; symskills initializes it on import and commits automatically
// after every library write it performs.
//
// All repository access goes through the git binary — no git library is
// vendored and go.mod is never touched for this feature. When the git
// binary is missing, every operation degrades to ErrUnavailable and the
// caller is expected to continue without versioning (the operation itself
// still succeeds).
package vcs

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrUnavailable reports that git cannot be run because the binary is
// missing from PATH. Callers must treat this as a non-fatal degradation.
var ErrUnavailable = errors.New("git binary not available")

// commitName and commitEmail are the identity used for symskills-made
// commits. They are applied per-invocation with -c flags, so the user's
// own git configuration is never modified and user commits keep their
// identity.
const (
	commitName  = "symskills"
	commitEmail = "symskills@localhost"
)

// Available reports whether the git binary can be found on PATH.
func Available() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

// runGit runs git in dir with args and returns its combined output. It
// returns ErrUnavailable when git is missing; any non-zero exit becomes
// an error that carries the command output for diagnostics.
func runGit(dir string, args ...string) (string, error) {
	path, err := exec.LookPath("git")
	if err != nil {
		return "", ErrUnavailable
	}
	cmd := exec.Command(path, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return out.String(), fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(out.String()))
	}
	return out.String(), nil
}

// commitFlags returns the -c overrides applied to every symskills commit.
// Signing is force-disabled so a user's commit.gpgsign setting cannot make
// automatic commits fail when no key is available.
func commitFlags() []string {
	return []string{
		"-c", "user.name=" + commitName,
		"-c", "user.email=" + commitEmail,
		"-c", "commit.gpgsign=false",
	}
}

// IsRepo reports whether dir contains a working git repository.
func IsRepo(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		return false
	}
	if _, err := runGit(dir, "rev-parse", "--git-dir"); err != nil {
		return false
	}
	return true
}

// Init creates a git repository at dir and records an initial commit of
// the current tree. It reports created=false when dir is already a
// repository, so calling it unconditionally after a write is safe. A tree
// with no files produces a repository with no commits yet (the initial
// commit is made lazily by Commit once content exists).
func Init(dir string) (bool, error) {
	if IsRepo(dir) {
		return false, nil
	}
	if _, err := runGit(dir, "init"); err != nil {
		return false, err
	}
	if _, err := runGit(dir, "add", "-A"); err != nil {
		return false, err
	}
	if _, err := runGit(dir, append(commitFlags(), "commit", "-m", "import: initial library import")...); err != nil {
		// An empty tree cannot be committed; the repo is still valid and
		// Commit() will create the first commit once content exists.
		if !strings.Contains(err.Error(), "nothing to commit") {
			return false, err
		}
	}
	return true, nil
}

// Commit stages every change in the working tree and creates one commit
// with the given message ("<operation>: <summary>"). A clean tree is a
// no-op that returns ""; otherwise the full hash of the new commit is
// returned. History is never rewritten: this only ever adds a commit on
// top of HEAD, so user commits are preserved.
func Commit(dir, message string) (string, error) {
	dirty, err := Dirty(dir)
	if err != nil {
		return "", err
	}
	if !dirty {
		return "", nil
	}
	if _, err := runGit(dir, "add", "-A"); err != nil {
		return "", err
	}
	if _, err := runGit(dir, append(commitFlags(), "commit", "-m", message)...); err != nil {
		return "", err
	}
	return Head(dir)
}

// Head returns the full hash of the current HEAD.
func Head(dir string) (string, error) {
	out, err := runGit(dir, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// Dirty reports whether the working tree has staged, unstaged or
// untracked changes.
func Dirty(dir string) (bool, error) {
	out, err := runGit(dir, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}
