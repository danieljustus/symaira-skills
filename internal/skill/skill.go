// Package skill loads, validates, and imports portable Agent Skill bundles.
package skill

import (
	"bufio"
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

// skillNamePattern follows the agentskills specification: lowercase
// alphanumeric segments joined by single hyphens — no consecutive, leading,
// or trailing hyphens (e.g. "pdf--processing" and "pdf-" are invalid).
var skillNamePattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// Size limits for portable skill bundles, fixed as a contract following the
// agentskills specification rather than tuned per install.
const (
	// MaxNameLength caps the skill name in characters.
	MaxNameLength = 64
	// MaxDescriptionLength caps the frontmatter description in characters.
	MaxDescriptionLength = 1024
	// MaxBodyLength caps the SKILL.md markdown body in characters.
	MaxBodyLength = 50000
)

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

// BatchImportStatus is the outcome of one batch-imported skill.
type BatchImportStatus string

const (
	BatchImported BatchImportStatus = "imported"
	BatchSkipped  BatchImportStatus = "skipped"
	BatchFailed   BatchImportStatus = "failed"
)

// BatchImportResult describes one item in a batch import.
type BatchImportResult struct {
	Name   string            `json:"name"`
	Path   string            `json:"path,omitempty"`
	Status BatchImportStatus `json:"status"`
	Error  string            `json:"error,omitempty"`
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

// UnmarshalYAML accepts scalar values and YAML string lists for fields that
// are represented as strings in the public Frontmatter type.
func (fm *Frontmatter) UnmarshalYAML(value *yaml.Node) error {
	var fields map[string]yaml.Node
	if err := value.Decode(&fields); err != nil {
		return err
	}

	normalized := make(map[string]any, len(fields))
	for name, node := range fields {
		if isStringFrontmatterField(name) {
			text, err := decodeStringFrontmatterField(name, node)
			if err != nil {
				return err
			}
			normalized[name] = text
			continue
		}
		var field any
		if err := node.Decode(&field); err != nil {
			return fmt.Errorf("frontmatter field %q: %w", name, err)
		}
		normalized[name] = field
	}

	data, err := yaml.Marshal(normalized)
	if err != nil {
		return fmt.Errorf("normalize frontmatter: %w", err)
	}
	type frontmatter Frontmatter
	var decoded frontmatter
	if err := yaml.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*fm = Frontmatter(decoded)
	return nil
}

func isStringFrontmatterField(name string) bool {
	switch name {
	case "name", "description", "version", "author", "license", "compatibility":
		return true
	default:
		return false
	}
}

func decodeStringFrontmatterField(name string, node yaml.Node) (string, error) {
	if node.Kind == 0 || node.Tag == "!!null" {
		return "", nil
	}
	if node.Kind == yaml.ScalarNode {
		var text string
		if err := node.Decode(&text); err != nil {
			return "", fmt.Errorf("frontmatter field %q must be a string or a list of strings: %w", name, err)
		}
		return text, nil
	}
	if node.Kind == yaml.SequenceNode {
		var values []string
		if err := node.Decode(&values); err != nil {
			return "", fmt.Errorf("frontmatter field %q must be a string or a list of strings: %w", name, err)
		}
		return strings.Join(values, ", "), nil
	}
	return "", fmt.Errorf("frontmatter field %q must be a string or a list of strings", name)
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

// loadFrontmatterOnly reads SKILL.md's YAML frontmatter without allocating
// the (often much larger) Markdown body, for callers that only need the
// header metadata (e.g. ListLibrary).
func loadFrontmatterOnly(root string) (Frontmatter, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return Frontmatter{}, err
	}
	f, err := os.Open(filepath.Join(abs, "SKILL.md"))
	if err != nil {
		return Frontmatter{}, fmt.Errorf("read SKILL.md: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() || scanner.Text() != "---" {
		return Frontmatter{}, fmt.Errorf("SKILL.md must start with YAML frontmatter")
	}

	var fmText strings.Builder
	closed := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "---") {
			closed = true
			break
		}
		fmText.WriteString(line)
		fmText.WriteByte('\n')
	}
	if err := scanner.Err(); err != nil {
		return Frontmatter{}, fmt.Errorf("read SKILL.md: %w", err)
	}
	if !closed {
		return Frontmatter{}, fmt.Errorf("SKILL.md frontmatter is not closed")
	}

	var fm Frontmatter
	if err := yaml.Unmarshal([]byte(fmText.String()), &fm); err != nil {
		return Frontmatter{}, fmt.Errorf("parse SKILL.md frontmatter: %w", err)
	}
	if fm.Metadata == nil {
		fm.Metadata = map[string]any{}
	}
	return fm, nil
}

func ValidateSkillName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("skill name is required")
	}
	if len(name) > MaxNameLength {
		return fmt.Errorf("skill name %q exceeds maximum length of %d characters", name, MaxNameLength)
	}
	if !skillNamePattern.MatchString(name) {
		return fmt.Errorf("skill name %q must be a single lowercase alphanumeric-dash segment without consecutive, leading, or trailing hyphens", name)
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
	} else {
		if len(name) > MaxNameLength {
			issues = append(issues, Issue{Code: "name_too_long", Severity: "error", Message: fmt.Sprintf("name exceeds maximum length of %d characters (actual: %d)", MaxNameLength, len(name)), Path: "SKILL.md"})
		}
		if !skillNamePattern.MatchString(name) {
			issues = append(issues, Issue{Code: "name_format", Severity: "error", Message: "name must use lowercase letters, numbers, and single dashes (no consecutive, leading, or trailing dashes)", Path: "SKILL.md"})
		}
		// The agentskills specification requires the skill name to match the
		// parent directory name, which is how the library layout stores
		// bundles (library/<name>).
		if filepath.Base(bundle.Root) != name {
			issues = append(issues, Issue{Code: "name_dir_mismatch", Severity: "error", Message: fmt.Sprintf("frontmatter name %q must match the parent directory name %q", name, filepath.Base(bundle.Root)), Path: "SKILL.md"})
		}
	}
	if strings.TrimSpace(bundle.Frontmatter.Description) == "" {
		issues = append(issues, Issue{Code: "description_required", Severity: "error", Message: "frontmatter description is required", Path: "SKILL.md"})
	} else if len(bundle.Frontmatter.Description) > MaxDescriptionLength {
		issues = append(issues, Issue{Code: "description_too_long", Severity: "error", Message: fmt.Sprintf("frontmatter description exceeds maximum length of %d characters (actual: %d)", MaxDescriptionLength, len(bundle.Frontmatter.Description)), Path: "SKILL.md"})
	}
	if strings.TrimSpace(bundle.Body) == "" {
		issues = append(issues, Issue{Code: "body_required", Severity: "error", Message: "SKILL.md body is empty", Path: "SKILL.md"})
	} else if len(bundle.Body) > MaxBodyLength {
		issues = append(issues, Issue{Code: "body_too_long", Severity: "warning", Message: fmt.Sprintf("SKILL.md body exceeds maximum length of %d characters (actual: %d)", MaxBodyLength, len(bundle.Body)), Path: "SKILL.md"})
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

// isSkillDir reports whether path contains a SKILL.md file.
func isSkillDir(path string) bool {
	fi, err := os.Stat(filepath.Join(path, "SKILL.md"))
	return err == nil && fi.Mode().IsRegular()
}

// ImportSkills discovers and imports skill directories from a parent directory.
// If the given directory itself contains SKILL.md it falls back to a single-skill import.
// Otherwise it scans immediate subdirectories for SKILL.md and imports each one.
// It never fails entirely on a single bad subdirectory — per-item results are returned.
func ImportSkills(parentDir, libraryDir string) []BatchImportResult {
	// If the directory itself is a skill, fall back to single import.
	if isSkillDir(parentDir) {
		res, err := ImportSkill(parentDir, libraryDir)
		if err != nil {
			return []BatchImportResult{{
				Name:   filepath.Base(parentDir),
				Status: BatchFailed,
				Error:  err.Error(),
			}}
		}
		return []BatchImportResult{{
			Name:   res.Name,
			Path:   res.Path,
			Status: BatchImported,
		}}
	}

	entries, err := os.ReadDir(parentDir)
	if err != nil {
		return []BatchImportResult{{
			Name:   filepath.Base(parentDir),
			Status: BatchFailed,
			Error:  err.Error(),
		}}
	}

	var results []BatchImportResult
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		subdir := filepath.Join(parentDir, entry.Name())
		if !isSkillDir(subdir) {
			continue
		}
		res, err := ImportSkill(subdir, libraryDir)
		if err != nil {
			results = append(results, BatchImportResult{
				Name:   entry.Name(),
				Status: BatchFailed,
				Error:  err.Error(),
			})
			continue
		}
		if res.Name != "" && res.Path != "" {
			results = append(results, BatchImportResult{
				Name:   res.Name,
				Path:   res.Path,
				Status: BatchImported,
			})
		}
	}
	return results
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
		root := filepath.Join(libraryDir, entry.Name())
		fm, err := loadFrontmatterOnly(root)
		if err != nil {
			issues = append(issues, Issue{Code: "skill_load", Severity: "error", Message: err.Error(), Path: entry.Name()})
			continue
		}
		abs, err := filepath.Abs(root)
		if err != nil {
			issues = append(issues, Issue{Code: "skill_load", Severity: "error", Message: err.Error(), Path: entry.Name()})
			continue
		}
		bundles = append(bundles, &Bundle{Root: abs, Frontmatter: fm, Manifest: Manifest{Targets: map[string]TargetConfig{}}})
	}
	return bundles, issues
}
