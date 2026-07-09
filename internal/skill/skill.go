// Package skill loads, validates, and imports portable Agent Skill bundles.
package skill

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/danieljustus/symaira-skills/internal/fsutil"
	"gopkg.in/yaml.v3"
)

var skillNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// Frontmatter is the portable SKILL.md metadata symskills understands.
type Frontmatter struct {
	Name                         string         `yaml:"name" json:"name"`
	Description                  string         `yaml:"description" json:"description"`
	Version                      string         `yaml:"version,omitempty" json:"version,omitempty"`
	Author                       string         `yaml:"author,omitempty" json:"author,omitempty"`
	License                      string         `yaml:"license,omitempty" json:"license,omitempty"`
	Compatibility                string         `yaml:"compatibility,omitempty" json:"compatibility,omitempty"`
	Platforms                    []string       `yaml:"platforms,omitempty" json:"platforms,omitempty"`
	RequiredEnvironmentVariables []string       `yaml:"required_environment_variables,omitempty" json:"required_environment_variables,omitempty"`
	Metadata                     map[string]any `yaml:"metadata,omitempty" json:"metadata,omitempty"`
}

// Manifest describes symskills-specific SSOT settings.
type Manifest struct {
	Skill   ManifestSkill           `toml:"skill" json:"skill"`
	Targets map[string]TargetConfig `toml:"targets" json:"targets"`
}

// ManifestSkill contains portable source metadata.
type ManifestSkill struct {
	Name    string `toml:"name" json:"name"`
	Version string `toml:"version" json:"version"`
	Source  string `toml:"source" json:"source"`
}

// TargetConfig controls rendering and installation for one harness target.
type TargetConfig struct {
	Enabled     bool              `toml:"enabled" json:"enabled"`
	Alias       string            `toml:"alias" json:"alias"`
	Description string            `toml:"description" json:"description"`
	Scope       string            `toml:"scope" json:"scope"`
	Category    string            `toml:"category" json:"category"`
	Prepend     string            `toml:"prepend" json:"prepend"`
	Append      string            `toml:"append" json:"append"`
	Metadata    map[string]string `toml:"metadata" json:"metadata"`
}

// Bundle is a loaded skill directory.
type Bundle struct {
	Root        string      `json:"root"`
	Frontmatter Frontmatter `json:"frontmatter"`
	Manifest    Manifest    `json:"manifest"`
	Body        string      `json:"body"`
}

// Issue is one validation finding.
type Issue struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Path     string `json:"path,omitempty"`
}

// ImportResult describes an imported bundle.
type ImportResult struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// LoadBundle reads SKILL.md and optional symskills.toml from a skill root.
func LoadBundle(root string) (*Bundle, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(filepath.Join(abs, "SKILL.md"))
	if err != nil {
		return nil, fmt.Errorf("read SKILL.md: %w", err)
	}
	fm, body, err := parseSkillMD(raw)
	if err != nil {
		return nil, err
	}

	manifest := Manifest{Targets: map[string]TargetConfig{}}
	manifestPath := filepath.Join(abs, "symskills.toml")
	if _, err := os.Stat(manifestPath); err == nil {
		if _, err := toml.DecodeFile(manifestPath, &manifest); err != nil {
			return nil, fmt.Errorf("parse symskills.toml: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if manifest.Targets == nil {
		manifest.Targets = map[string]TargetConfig{}
	}
	if manifest.Skill.Name == "" {
		manifest.Skill.Name = fm.Name
	}
	if manifest.Skill.Version == "" {
		manifest.Skill.Version = fm.Version
	}

	return &Bundle{Root: abs, Frontmatter: fm, Manifest: manifest, Body: body}, nil
}

func parseSkillMD(raw []byte) (Frontmatter, string, error) {
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return Frontmatter{}, "", fmt.Errorf("SKILL.md must start with YAML frontmatter")
	}
	rest := strings.TrimPrefix(text, "---\n")
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return Frontmatter{}, "", fmt.Errorf("SKILL.md frontmatter is not closed")
	}
	fmText := rest[:end]
	body := rest[end+len("\n---"):]
	body = strings.TrimLeft(body, "\n")

	var fm Frontmatter
	if err := yaml.Unmarshal([]byte(fmText), &fm); err != nil {
		return Frontmatter{}, "", fmt.Errorf("parse SKILL.md frontmatter: %w", err)
	}
	if fm.Metadata == nil {
		fm.Metadata = map[string]any{}
	}
	return fm, body, nil
}

func ValidateSkillName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("skill name is required")
	}
	if !skillNamePattern.MatchString(name) {
		return fmt.Errorf("skill name %q must be a single lowercase alphanumeric-dash segment", name)
	}
	return nil
}

// Validate returns non-fatal validation issues for a loaded bundle.
func Validate(bundle *Bundle) []Issue {
	if bundle == nil {
		return []Issue{{Code: "bundle_required", Severity: "error", Message: "bundle is nil"}}
	}
	var issues []Issue
	name := bundle.Frontmatter.Name
	if strings.TrimSpace(name) == "" {
		issues = append(issues, Issue{Code: "name_required", Severity: "error", Message: "frontmatter name is required", Path: "SKILL.md"})
	} else if !skillNamePattern.MatchString(name) {
		issues = append(issues, Issue{Code: "name_format", Severity: "error", Message: "name must use lowercase letters, numbers, and dashes", Path: "SKILL.md"})
	}
	if strings.TrimSpace(bundle.Frontmatter.Description) == "" {
		issues = append(issues, Issue{Code: "description_required", Severity: "error", Message: "frontmatter description is required", Path: "SKILL.md"})
	}
	if strings.TrimSpace(bundle.Body) == "" {
		issues = append(issues, Issue{Code: "body_required", Severity: "error", Message: "SKILL.md body is empty", Path: "SKILL.md"})
	}
	for target, cfg := range bundle.Manifest.Targets {
		if !cfg.Enabled {
			continue
		}
		for _, rel := range []string{cfg.Prepend, cfg.Append} {
			if rel == "" {
				continue
			}
			if err := safeRelativeFile(bundle.Root, rel); err != nil {
				issues = append(issues, Issue{Code: "overlay_reference_missing", Severity: "error", Message: err.Error(), Path: "symskills.toml:" + target})
			}
		}
	}
	return issues
}

func safeRelativeFile(root, rel string) error {
	if filepath.IsAbs(rel) {
		return fmt.Errorf("overlay reference %q must be relative", rel)
	}
	clean := filepath.Clean(rel)
	if strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return fmt.Errorf("overlay reference %q escapes skill root", rel)
	}
	if _, err := os.Stat(filepath.Join(root, clean)); err != nil {
		return fmt.Errorf("overlay reference %q: %w", rel, err)
	}
	return nil
}

// HasIssue reports whether issues contains code. It is public for tests and CLI checks.
func HasIssue(issues []Issue, code string) bool {
	return slices.ContainsFunc(issues, func(issue Issue) bool {
		return issue.Code == code
	})
}

// ImportSkill copies an existing skill directory into a managed library.
func ImportSkill(srcRoot, libraryDir string) (ImportResult, error) {
	bundle, err := LoadBundle(srcRoot)
	if err != nil {
		return ImportResult{}, err
	}
	name := bundle.Frontmatter.Name
	if name == "" {
		return ImportResult{}, fmt.Errorf("cannot import skill without name")
	}
	dst := filepath.Join(libraryDir, name)
	if _, err := os.Stat(dst); err == nil {
		return ImportResult{}, fmt.Errorf("skill %q already exists in library", name)
	} else if !errors.Is(err, os.ErrNotExist) {
		return ImportResult{}, err
	}
	if err := fsutil.CopyTree(srcRoot, dst, func(rel string, d os.DirEntry) bool {
		return d.Name() == ".git" && d.IsDir()
	}); err != nil {
		return ImportResult{}, err
	}
	return ImportResult{Name: name, Path: dst}, nil
}

// ListLibrary returns loaded bundles under a library directory.
func ListLibrary(libraryDir string) ([]*Bundle, []Issue) {
	entries, err := os.ReadDir(libraryDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, []Issue{{Code: "library_read", Severity: "error", Message: err.Error(), Path: libraryDir}}
	}
	var bundles []*Bundle
	var issues []Issue
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		bundle, err := LoadBundle(filepath.Join(libraryDir, entry.Name()))
		if err != nil {
			issues = append(issues, Issue{Code: "skill_load", Severity: "error", Message: err.Error(), Path: entry.Name()})
			continue
		}
		bundles = append(bundles, bundle)
	}
	return bundles, issues
}
