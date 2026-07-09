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

	"github.com/danieljustus/symaira-skills/internal/render"
	"github.com/danieljustus/symaira-skills/internal/skill"
)

const markerFile = ".symskills.json"

type Scope string

const (
	ScopeUser    Scope = "user"
	ScopeProject Scope = "project"
)

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
	HomeDir    string `json:"home_dir"`
	ProjectDir string `json:"project_dir"`
	Scope      Scope  `json:"scope"`
	Mode       Mode   `json:"mode"`
	DryRun     bool   `json:"dry_run"`
}

type Result struct {
	Action string        `json:"action"`
	Target render.Target `json:"target"`
	Name   string        `json:"name"`
	Path   string        `json:"path"`
	Mode   Mode          `json:"mode"`
}

type Marker struct {
	ManagedBy  string        `json:"managed_by"`
	Target     render.Target `json:"target"`
	Name       string        `json:"name"`
	RenderedAt string        `json:"rendered_at"`
	Mode       Mode          `json:"mode"`
	Installed  string        `json:"installed"`
}

type Change struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

func Install(item RenderedSkill, opts Options) (Result, error) {
	if opts.Scope == "" {
		opts.Scope = ScopeUser
	}
	if opts.Mode == "" {
		opts.Mode = ModeSymlink
	}
	dest, err := InstallPath(item.Target, item.Name, opts)
	if err != nil {
		return Result{}, err
	}
	result := Result{Action: "installed", Target: item.Target, Name: item.Name, Path: dest, Mode: opts.Mode}
	if opts.DryRun {
		result.Action = "planned"
		return result, nil
	}
	if err := ensureManagedOrAbsent(dest); err != nil {
		return Result{}, err
	}
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

func ensureManagedOrAbsent(path string) error {
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(path, markerFile)); err != nil {
		return fmt.Errorf("refusing to overwrite unmanaged skill at %s", path)
	}
	return nil
}

func markerBytes(item RenderedSkill, mode Mode) []byte {
	data, _ := json.MarshalIndent(Marker{
		ManagedBy:  "symskills",
		Target:     item.Target,
		Name:       item.Name,
		RenderedAt: item.Path,
		Mode:       mode,
		Installed:  time.Now().UTC().Format(time.RFC3339),
	}, "", "  ")
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
	if opts.Scope == ScopeProject {
		switch target {
		case render.TargetOpenCode:
			return filepath.Join(project, ".opencode", "skills", name), nil
		case render.TargetClaude:
			return filepath.Join(project, ".claude", "skills", name), nil
		case render.TargetCodex:
			return filepath.Join(project, ".agents", "skills", name), nil
		case render.TargetHermes:
			return filepath.Join(project, ".hermes", "skills", name), nil
		default:
			return "", fmt.Errorf("unknown target %s", target)
		}
	}
	switch target {
	case render.TargetOpenCode:
		return filepath.Join(home, ".config", "opencode", "skills", name), nil
	case render.TargetClaude:
		return filepath.Join(home, ".claude", "skills", name), nil
	case render.TargetCodex:
		return filepath.Join(home, ".agents", "skills", name), nil
	case render.TargetHermes:
		return filepath.Join(home, ".hermes", "skills", "symaira", name), nil
	default:
		return "", fmt.Errorf("unknown target %s", target)
	}
}

func Uninstall(target render.Target, name string, opts Options) error {
	dest, err := InstallPath(target, name, opts)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(dest); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(dest, markerFile)); err != nil {
		return fmt.Errorf("refusing to remove unmanaged skill at %s", dest)
	}
	return os.RemoveAll(dest)
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
	var changes []Change
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
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		out[rel] = hex.EncodeToString(sum[:])
		return nil
	})
	return out, err
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o755)
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return copyFile(path, target, info.Mode().Perm())
	})
}

func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
