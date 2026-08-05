// Package render creates harness-specific skill folders from portable bundles.
package render

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/danieljustus/symaira-skills/internal/fsutil"
	"github.com/danieljustus/symaira-skills/internal/skill"
	"gopkg.in/yaml.v3"
)

type Target string

const (
	TargetOpenCode    Target = "opencode"
	TargetClaude      Target = "claude"
	TargetCodex       Target = "codex"
	TargetHermes      Target = "hermes"
	TargetAntigravity Target = "antigravity"
	TargetOpenClaw    Target = "openclaw"
)

// DefaultTargets returns the list of all registered target names.
func DefaultTargets() []Target {
	targets := make([]Target, len(Targets))
	for i, spec := range Targets {
		targets[i] = spec.Name
	}
	return targets
}

// Scope represents the installation scope for a target harness.
type Scope string

const (
	ScopeUser    Scope = "user"
	ScopeProject Scope = "project"
)

// TargetSpec holds metadata and path functions for a single harness target.
// Targets is the single registry; any code that needs per-target information
// (paths, display names, quirks) should read from here.
type TargetSpec struct {
	Name        Target
	DisplayName string
	BinaryName  string
	ConfigDir   func(home, project string, scope Scope) string
	SkillRoot   func(home, project string, scope Scope) string
	Quirks      string
}

// Targets is the single registry of all supported harness targets.
var Targets = []TargetSpec{
	{
		Name:        TargetOpenCode,
		DisplayName: "OpenCode",
		BinaryName:  "opencode",
		ConfigDir: func(home, project string, scope Scope) string {
			if scope == ScopeProject && project != "" {
				return filepath.Join(project, ".opencode")
			}
			return filepath.Join(home, ".config", "opencode")
		},
		SkillRoot: func(home, project string, scope Scope) string {
			if scope == ScopeProject && project != "" {
				return filepath.Join(project, ".opencode", "skills")
			}
			return filepath.Join(home, ".config", "opencode", "skills")
		},
	},
	{
		Name:        TargetClaude,
		DisplayName: "Claude Code",
		BinaryName:  "claude",
		ConfigDir: func(home, project string, scope Scope) string {
			if scope == ScopeProject && project != "" {
				return filepath.Join(project, ".claude")
			}
			return filepath.Join(home, ".claude")
		},
		SkillRoot: func(home, project string, scope Scope) string {
			if scope == ScopeProject && project != "" {
				return filepath.Join(project, ".claude", "skills")
			}
			return filepath.Join(home, ".claude", "skills")
		},
	},
	{
		Name:        TargetCodex,
		DisplayName: "Codex",
		BinaryName:  "codex",
		ConfigDir: func(home, project string, scope Scope) string {
			if scope == ScopeProject && project != "" {
				return filepath.Join(project, ".agents")
			}
			return filepath.Join(home, ".agents")
		},
		SkillRoot: func(home, project string, scope Scope) string {
			if scope == ScopeProject && project != "" {
				return filepath.Join(project, ".agents", "skills")
			}
			return filepath.Join(home, ".agents", "skills")
		},
		Quirks: "Writes agents/openai.yaml metadata on render",
	},
	{
		Name:        TargetHermes,
		DisplayName: "Hermes",
		BinaryName:  "hermes",
		ConfigDir: func(home, project string, scope Scope) string {
			if scope == ScopeProject && project != "" {
				return filepath.Join(project, ".hermes")
			}
			return filepath.Join(home, ".hermes")
		},
		SkillRoot: func(home, project string, scope Scope) string {
			if scope == ScopeProject && project != "" {
				return filepath.Join(project, ".hermes", "skills")
			}
			return filepath.Join(home, ".hermes", "skills", "symaira")
		},
	},
	{
		Name:        TargetAntigravity,
		DisplayName: "Antigravity",
		BinaryName:  "agy",
		ConfigDir: func(home, project string, scope Scope) string {
			if scope == ScopeProject && project != "" {
				return filepath.Join(project, ".agents")
			}
			return filepath.Join(home, ".gemini", "antigravity-cli")
		},
		SkillRoot: func(home, project string, scope Scope) string {
			if scope == ScopeProject && project != "" {
				return filepath.Join(project, ".agents", "skills")
			}
			return filepath.Join(home, ".gemini", "antigravity-cli", "skills")
		},
		Quirks: "Global skills live in ~/.gemini/antigravity-cli/skills (docs: antigravity.google/docs/skills); workspace skills share <project>/.agents/skills with Codex/OpenClaw",
	},
	{
		Name:        TargetOpenClaw,
		DisplayName: "OpenClaw",
		BinaryName:  "openclaw",
		ConfigDir: func(home, project string, scope Scope) string {
			if scope == ScopeProject && project != "" {
				return filepath.Join(project, ".agents")
			}
			return filepath.Join(home, ".openclaw")
		},
		SkillRoot: func(home, project string, scope Scope) string {
			if scope == ScopeProject && project != "" {
				return filepath.Join(project, ".agents", "skills")
			}
			return filepath.Join(home, ".openclaw", "skills")
		},
		Quirks: "Managed skills load from ~/.openclaw/skills (default state dir, docs: docs.openclaw.ai/tools/skills); also reads ~/.agents/skills and <workspace>/skills",
	},
}

// LookupSpec returns the TargetSpec for the given target and a boolean
// indicating whether it was found.
func LookupSpec(t Target) (TargetSpec, bool) {
	for _, spec := range Targets {
		if spec.Name == t {
			return spec, true
		}
	}
	return TargetSpec{}, false
}

// MustLookupSpec returns the TargetSpec for the given target, panicking
// if the target is unknown.
func MustLookupSpec(t Target) TargetSpec {
	spec, ok := LookupSpec(t)
	if !ok {
		panic(fmt.Sprintf("render: unknown target %q", t))
	}
	return spec
}

// RenderMeta carries optional provenance metadata for profile-aware rendering.
type RenderMeta struct {
	Source  string
	Profile string
	Alias   string // profile alias overrides TargetConfig.Alias
}

type Rendered struct {
	Target      Target            `json:"target"`
	Name        string            `json:"name"`
	Path        string            `json:"path,omitempty"`
	Frontmatter skill.Frontmatter `json:"frontmatter"`
	SkillMD     string            `json:"skill_md,omitempty"`
	Source      string            `json:"source,omitempty"`
	Profile     string            `json:"profile,omitempty"`
}

// RenderTarget returns a target-specific SKILL.md without writing files.
func RenderTarget(bundle *skill.Bundle, target Target, meta ...RenderMeta) (Rendered, error) {
	if bundle == nil {
		return Rendered{}, fmt.Errorf("bundle is nil")
	}

	// Reject bundles that have error-severity overlay reference issues.
	// This catches path traversal in prepend/append before any file is
	// read or written (#80).  Other validation problems (missing
	// description, empty body, etc.) are reported by `skills_validate`
	// but do not block rendering here.
	for _, issue := range skill.Validate(bundle) {
		if issue.Severity == "error" && issue.Code == "overlay_reference_missing" {
			return Rendered{}, fmt.Errorf("validation error: %s", issue.Message)
		}
	}

	cfg, hasCfg := bundle.Manifest.Targets[string(target)]
	if hasCfg && !cfg.Enabled {
		return Rendered{}, fmt.Errorf("target %s is disabled", target)
	}

	fm := bundle.Frontmatter
	if fm.Metadata == nil {
		fm.Metadata = map[string]any{}
	}
	metadata := map[string]any{}
	for k, v := range fm.Metadata {
		metadata[k] = v
	}
	for k, v := range cfg.Metadata {
		metadata[k] = v
	}
	fm.Metadata = metadata
	fm.Compatibility = string(target)
	// Alias precedence: profile alias (RenderMeta) > target config alias > manifest name.
	var profileAlias string
	if len(meta) > 0 {
		profileAlias = meta[0].Alias
	}
	if profileAlias != "" {
		fm.Name = profileAlias
	} else if cfg.Alias != "" {
		fm.Name = cfg.Alias
	} else if bundle.Manifest.Skill.Name != "" {
		fm.Name = bundle.Manifest.Skill.Name
	}
	if cfg.Description != "" {
		fm.Description = cfg.Description
	}

	if err := applyFrontmatterOverlay(bundle.Root, target, &fm); err != nil {
		return Rendered{}, err
	}
	if err := skill.ValidateSkillName(fm.Name); err != nil {
		return Rendered{}, fmt.Errorf("invalid resolved name for target %s: %w", target, err)
	}
	body, err := renderBody(bundle, target, cfg)
	if err != nil {
		return Rendered{}, err
	}
	skillMD, err := encodeSkillMD(fm, body)
	if err != nil {
		return Rendered{}, err
	}
	item := Rendered{Target: target, Name: fm.Name, Frontmatter: fm, SkillMD: skillMD}
	if len(meta) > 0 {
		item.Source = meta[0].Source
		item.Profile = meta[0].Profile
	}
	return item, nil
}

func renderBody(bundle *skill.Bundle, target Target, cfg skill.TargetConfig) (string, error) {
	prepend, err := overlayText(bundle.Root, target, "prepend.md", cfg.Prepend)
	if err != nil {
		return "", err
	}
	appendText, err := overlayText(bundle.Root, target, "append.md", cfg.Append)
	if err != nil {
		return "", err
	}
	var parts []string
	if strings.TrimSpace(prepend) != "" {
		parts = append(parts, strings.TrimRight(prepend, "\n"))
	}
	parts = append(parts, strings.TrimRight(bundle.Body, "\n"))
	if strings.TrimSpace(appendText) != "" {
		parts = append(parts, strings.TrimRight(appendText, "\n"))
	}
	return strings.Join(parts, "\n\n") + "\n", nil
}

func overlayText(root string, target Target, defaultName, configured string) (string, error) {
	if configured != "" {
		if filepath.IsAbs(configured) {
			return "", fmt.Errorf("overlay reference %q must be relative", configured)
		}
		clean := filepath.Clean(configured)
		if strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
			return "", fmt.Errorf("overlay reference %q escapes skill root", configured)
		}
		return readOptional(filepath.Join(root, clean))
	}
	return readOptional(filepath.Join(root, "overlays", string(target), defaultName))
}

func readOptional(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	return string(raw), nil
}

func applyFrontmatterOverlay(root string, target Target, fm *skill.Frontmatter) error {
	path := filepath.Join(root, "overlays", string(target), "frontmatter.toml")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	var raw map[string]any
	if _, err := toml.DecodeFile(path, &raw); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	if v, ok := raw["name"].(string); ok && v != "" {
		fm.Name = v
	}
	if v, ok := raw["description"].(string); ok && v != "" {
		fm.Description = v
	}
	if v, ok := raw["compatibility"].(string); ok && v != "" {
		fm.Compatibility = v
	}
	if meta, ok := raw["metadata"].(map[string]any); ok {
		if fm.Metadata == nil {
			fm.Metadata = map[string]any{}
		}
		for k, v := range meta {
			fm.Metadata[k] = v
		}
	}
	return nil
}

func encodeSkillMD(fm skill.Frontmatter, body string) (string, error) {
	data, err := yaml.Marshal(fm)
	if err != nil {
		return "", err
	}
	return "---\n" + string(data) + "---\n\n" + body, nil
}

// RenderAll writes target-specific skill folders under outDir and returns the
// successfully rendered items along with any per-target errors.
func RenderAll(bundle *skill.Bundle, outDir string, targets []Target, meta ...RenderMeta) ([]Rendered, []error) {
	if len(targets) == 0 {
		targets = DefaultTargets()
	}
	// The source tree hash is a per-bundle property; compute it once and
	// reuse it for every target instead of walking the tree per target.
	treeHash := sourceTreeHash(bundle.Root)
	var rendered []Rendered
	var errs []error
	for _, target := range targets {
		item, err := RenderTarget(bundle, target, meta...)
		if err != nil {
			errs = append(errs, fmt.Errorf("target %s: %w", target, err))
			continue
		}
		dst := filepath.Join(outDir, string(target), item.Name)
		if err := writeRendered(bundle.Root, dst, item, target, treeHash); err != nil {
			errs = append(errs, fmt.Errorf("target %s: %w", target, err))
			continue
		}
		item.Path = dst
		rendered = append(rendered, item)
	}
	return rendered, errs
}

// walkBundleDir is the tree walker used by sourceTreeHash. It is a variable
// so tests can count invocations and prove the tree is walked exactly once
// per bundle.
var walkBundleDir = filepath.WalkDir

// sourceTreeHash computes a content hash of the source tree (support files
// only; SKILL.md and symskills.toml are part of the rendered body). It is
// expensive — it reads every support file — so callers must compute it once
// per bundle and reuse the result across targets.
func sourceTreeHash(bundleRoot string) string {
	h := sha256.New()
	_ = walkBundleDir(bundleRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "overlays" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(bundleRoot, path)
		if err != nil {
			return err
		}
		if rel == "SKILL.md" || rel == "symskills.toml" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		h.Write([]byte(rel))
		h.Write([]byte{0})
		h.Write(data)
		h.Write([]byte{0})
		return nil
	})
	return hex.EncodeToString(h.Sum(nil))
}

// sourceHash combines the once-per-bundle source tree hash with the
// per-target rendered SKILL.md content and target name so re-renders with
// unchanged input can be skipped.
func sourceHash(treeHash, renderedSkillMD string, target Target) string {
	h := sha256.New()
	h.Write([]byte(treeHash))
	h.Write([]byte{0})
	h.Write([]byte(renderedSkillMD))
	h.Write([]byte{0})
	h.Write([]byte(target))
	return hex.EncodeToString(h.Sum(nil))
}

func writeRendered(root, dst string, item Rendered, target Target, treeHash string) error {
	sh := sourceHash(treeHash, item.SkillMD, target)

	// Read any existing marker so we can preserve install-time fields and
	// check whether the output is already current.
	markerPath := filepath.Join(dst, ".symskills.json")
	var existingMarker map[string]interface{}
	if data, err := os.ReadFile(markerPath); err == nil {
		_ = json.Unmarshal(data, &existingMarker)
	}

	// Skip the rewrite when the source hasn't changed.
	if existingMarker != nil {
		if storedHash, ok := existingMarker["source_hash"].(string); ok && storedHash == sh {
			return nil
		}
	}

	// Build the updated marker, preserving any pre-existing fields.
	if existingMarker == nil {
		existingMarker = make(map[string]interface{})
	}
	existingMarker["source_hash"] = sh
	markerBytes, err := json.MarshalIndent(existingMarker, "", "  ")
	if err != nil {
		return err
	}
	markerBytes = append(markerBytes, '\n')

	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	if err := copySupportFiles(root, dst); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dst, "SKILL.md"), []byte(item.SkillMD), 0o644); err != nil {
		return err
	}
	if target == TargetCodex {
		if err := writeCodexMetadata(dst, item); err != nil {
			return err
		}
	}
	if err := os.WriteFile(markerPath, markerBytes, 0o644); err != nil {
		return err
	}
	return nil
}

func copySupportFiles(src, dst string) error {
	return fsutil.CopyTree(src, dst, func(rel string, d os.DirEntry) bool {
		if d.IsDir() && (d.Name() == ".git" || d.Name() == "overlays") {
			return true
		}
		return rel == "SKILL.md" || rel == "symskills.toml"
	})
}

func writeCodexMetadata(dst string, item Rendered) error {
	content := fmt.Sprintf(`interface:
  display_name: %q
  short_description: %q
policy:
  allow_implicit_invocation: true
`, item.Name, item.Frontmatter.Description)
	path := filepath.Join(dst, "agents", "openai.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// ParseTarget converts a user-facing target string.
func ParseTarget(s string) (Target, error) {
	for _, spec := range Targets {
		if string(spec.Name) == s {
			return Target(s), nil
		}
	}
	return "", fmt.Errorf("unknown target %q", s)
}
