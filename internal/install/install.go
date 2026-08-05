// Package install installs rendered skill folders into supported harness paths.
package install

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/danieljustus/symaira-skills/internal/fsutil"
	"github.com/danieljustus/symaira-skills/internal/render"
	"github.com/danieljustus/symaira-skills/internal/skill"
)

const markerFile = ".symskills.json"

type Mode string

const (
	ModeSymlink Mode = "symlink"
	ModeCopy    Mode = "copy"
)

type RenderedSkill struct {
	Target render.Target `json:"target"`
	Name   string        `json:"name"`
	Path   string        `json:"path"`
}

type Options struct {
	HomeDir    string       `json:"home_dir"`
	ProjectDir string       `json:"project_dir"`
	Scope      render.Scope `json:"scope"`
	Mode       Mode         `json:"mode"`
	DryRun     bool         `json:"dry_run"`
	// Force adopts a destination that was not installed by symskills. The
	// existing directory is moved to a backup location instead of being
	// deleted, so a hand-written skill is never lost silently.
	Force bool `json:"force"`
}

type Result struct {
	Action string        `json:"action"`
	Target render.Target `json:"target"`
	Name   string        `json:"name"`
	Path   string        `json:"path"`
	Mode   Mode          `json:"mode"`
	// BackupPath is set when --force adopted an unmanaged destination and
	// moved it aside.
	BackupPath string `json:"backup_path,omitempty"`
}

type Marker struct {
	ManagedBy  string        `json:"managed_by"`
	Target     render.Target `json:"target"`
	Name       string        `json:"name"`
	RenderedAt string        `json:"rendered_at"`
	Mode       Mode          `json:"mode"`
	Installed  string        `json:"installed"`
	SourceHash string        `json:"source_hash,omitempty"`
}

type Change struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

func Install(item RenderedSkill, opts Options) (Result, error) {
	if opts.Scope == "" {
		opts.Scope = render.ScopeUser
	}
	if opts.Mode == "" {
		opts.Mode = ModeSymlink
	}
	dest, err := InstallPath(item.Target, item.Name, opts)
	if err != nil {
		return Result{}, err
	}
	result := Result{Action: "installed", Target: item.Target, Name: item.Name, Path: dest, Mode: opts.Mode}
	// When the harness skills directory is itself a symlink into the render
	// cache, dest and the rendered source are the same location. Removing dest
	// would delete the very content we are about to link to, so treat this as
	// already installed and only refresh the marker.
	same, err := sameLocation(dest, item.Path)
	if err != nil {
		return Result{}, err
	}
	if same {
		result.Action = "current"
		if opts.DryRun {
			result.Action = "planned"
			return result, nil
		}
		if err := os.WriteFile(filepath.Join(item.Path, markerFile), markerBytes(item, opts.Mode), 0o644); err != nil {
			return Result{}, err
		}
		return result, nil
	}
	if opts.DryRun {
		result.Action = "planned"
		return result, nil
	}
	backup, err := prepareDest(dest, opts)
	if err != nil {
		return Result{}, err
	}
	result.BackupPath = backup
	if err := os.WriteFile(filepath.Join(item.Path, markerFile), markerBytes(item, opts.Mode), 0o644); err != nil {
		return Result{}, err
	}
	if err := os.RemoveAll(dest); err != nil {
		return Result{}, err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return Result{}, err
	}
	if opts.Mode == ModeSymlink {
		if err := os.Symlink(item.Path, dest); err == nil {
			return result, nil
		}
		opts.Mode = ModeCopy
		result.Mode = ModeCopy
	}
	if err := copyDir(item.Path, dest); err != nil {
		return Result{}, err
	}
	return result, nil
}

// sameLocation reports whether dest and src denote the same directory once
// symlinked path components are resolved. dest itself is deliberately not
// followed: a dest symlink pointing at src is a normal, already-done install,
// whereas a dest whose *parent* resolves into src's directory is the dangerous
// case this guards.
func sameLocation(dest, src string) (bool, error) {
	srcReal, err := filepath.EvalSymlinks(src)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	destParent, err := filepath.EvalSymlinks(filepath.Dir(dest))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return filepath.Join(destParent, filepath.Base(dest)) == srcReal, nil
}

// prepareDest clears the way for an install. Without opts.Force it only
// verifies that dest is absent or symskills-managed. With opts.Force an
// unmanaged dest is moved to a backup directory and its path returned.
func prepareDest(dest string, opts Options) (string, error) {
	err := ensureManagedOrAbsent(dest)
	if err == nil || !opts.Force {
		return "", err
	}
	st, lerr := os.Lstat(dest)
	if lerr != nil {
		return "", lerr
	}
	// A symlink holds no content of its own; drop it and leave its target alone.
	if st.Mode()&os.ModeSymlink != 0 {
		return "", os.Remove(dest)
	}
	backup, berr := backupPath(dest, opts)
	if berr != nil {
		return "", berr
	}
	if err := os.MkdirAll(filepath.Dir(backup), 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(dest, backup); err != nil {
		return "", fmt.Errorf("backing up unmanaged skill at %s: %w", dest, err)
	}
	return backup, nil
}

// backupPath returns a collision-free location outside every harness directory,
// so the moved-aside skill is not picked up as a skill by any agent.
func backupPath(dest string, opts Options) (string, error) {
	home := opts.HomeDir
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return "", err
		}
	}
	base := filepath.Join(home, ".local", "share", "symskills", "backups",
		fmt.Sprintf("%s-%s", filepath.Base(dest), time.Now().UTC().Format("20060102T150405Z")))
	path := base
	for i := 1; ; i++ {
		if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
			return path, nil
		} else if err != nil {
			return "", err
		}
		path = fmt.Sprintf("%s-%d", base, i)
	}
}

func ensureManagedOrAbsent(path string) error {
	st, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if st.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		if err != nil {
			return err
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(path), target)
		}
		if _, err := os.Stat(target); errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if _, err := os.Stat(filepath.Join(target, markerFile)); err != nil {
			return fmt.Errorf("refusing to overwrite unmanaged skill at %s", path)
		}
		return nil
	}
	if _, err := os.Stat(filepath.Join(path, markerFile)); err != nil {
		return fmt.Errorf("refusing to overwrite unmanaged skill at %s", path)
	}
	return nil
}

func markerBytes(item RenderedSkill, mode Mode) []byte {
	// Preserve source_hash from any existing marker so the render-cache
	// freshness check survives install (#87).
	var srcHash string
	if data, err := os.ReadFile(filepath.Join(item.Path, markerFile)); err == nil {
		var existing struct {
			SourceHash string `json:"source_hash,omitempty"`
		}
		if json.Unmarshal(data, &existing) == nil {
			srcHash = existing.SourceHash
		}
	}
	m := Marker{
		ManagedBy:  "symskills",
		Target:     item.Target,
		Name:       item.Name,
		RenderedAt: item.Path,
		Mode:       mode,
		Installed:  time.Now().UTC().Format(time.RFC3339),
		SourceHash: srcHash,
	}
	data, _ := json.MarshalIndent(m, "", "  ")
	return append(data, '\n')
}

func InstallPath(target render.Target, name string, opts Options) (string, error) {
	if err := skill.ValidateSkillName(name); err != nil {
		return "", fmt.Errorf("invalid install name for target %s: %w", target, err)
	}
	home := opts.HomeDir
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return "", err
		}
	}
	project := opts.ProjectDir
	if project == "" {
		cwd, err := os.Getwd()
		if err == nil {
			project = cwd
		}
	}
	if opts.Scope == "" {
		opts.Scope = render.ScopeUser
	}
	spec, ok := render.LookupSpec(target)
	if !ok {
		return "", fmt.Errorf("unknown target %s", target)
	}
	root := spec.SkillRoot(home, project, opts.Scope)
	return filepath.Join(root, name), nil
}

// TargetDir returns the base installation directory for a target without requiring a skill name.
func TargetDir(target render.Target, opts Options) (string, error) {
	path, err := InstallPath(target, "placeholder", opts)
	if err != nil {
		return "", err
	}
	return filepath.Dir(path), nil
}

// Uninstall removes a managed installed skill. It reports whether an
// installation was actually removed (removed == false means nothing was
// installed at the resolved path).
func Uninstall(target render.Target, name string, opts Options) (removed bool, err error) {
	dest, err := InstallPath(target, name, opts)
	if err != nil {
		return false, err
	}
	st, err := os.Lstat(dest)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	if st.Mode()&os.ModeSymlink != 0 {
		linkTarget, err := os.Readlink(dest)
		if err != nil {
			return false, err
		}
		if !filepath.IsAbs(linkTarget) {
			linkTarget = filepath.Join(filepath.Dir(dest), linkTarget)
		}
		if _, err := os.Stat(linkTarget); !errors.Is(err, os.ErrNotExist) {
			if _, err := os.Stat(filepath.Join(linkTarget, markerFile)); err != nil {
				return false, fmt.Errorf("refusing to remove unmanaged skill at %s", dest)
			}
		}
		if err := os.RemoveAll(dest); err != nil {
			return false, err
		}
		return true, nil
	}
	if _, err := os.Stat(filepath.Join(dest, markerFile)); err != nil {
		return false, fmt.Errorf("refusing to remove unmanaged skill at %s", dest)
	}
	if err := os.RemoveAll(dest); err != nil {
		return false, err
	}
	return true, nil
}

func Diff(renderedPath, installedPath string) ([]Change, error) {
	left, err := fileHashes(renderedPath)
	if err != nil {
		return nil, err
	}
	right, err := fileHashes(installedPath)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	changes := []Change{}
	for path, lhash := range left {
		seen[path] = true
		if rhash, ok := right[path]; !ok {
			changes = append(changes, Change{Path: path, Status: "added"})
		} else if rhash != lhash {
			changes = append(changes, Change{Path: path, Status: "modified"})
		}
	}
	for path := range right {
		if !seen[path] {
			changes = append(changes, Change{Path: path, Status: "removed"})
		}
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	return changes, nil
}

func fileHashes(root string) (map[string]string, error) {
	out := map[string]string{}
	if _, err := os.Stat(root); errors.Is(err, os.ErrNotExist) {
		return out, nil
	}
	// Resolve any symlink in the root path before walking, so WalkDir
	// sees the actual directory and Lstat-based DirEntry.IsDir() works
	// for symlink-mode installs where root is a symlink to a directory.
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == markerFile {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		hasher := sha256.New()
		if _, err := io.Copy(hasher, f); err != nil {
			_ = f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
		out[rel] = hex.EncodeToString(hasher.Sum(nil))
		return nil
	})
	return out, err
}

func copyDir(src, dst string) error {
	return fsutil.CopyTree(src, dst, func(rel string, d os.DirEntry) bool { return false })
}
