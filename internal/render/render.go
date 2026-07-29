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
	TargetOpenCode Target = "opencode"
	TargetClaude   Target = "claude"
	TargetCodex    Target = "codex"
	TargetHermes   Target = "hermes"
)

var DefaultTargets = []Target{TargetOpenCode, TargetClaude, TargetCodex, TargetHermes}

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
		targets = DefaultTargets
	}
	var rendered []Rendered
	var errs []error
	for _, target := range targets {
		item, err := RenderTarget(bundle, target, meta...)
		if err != nil {
			errs = append(errs, fmt.Errorf("target %s: %w", target, err))
			continue
		}
		dst := filepath.Join(outDir, string(target), item.Name)
		if err := writeRendered(bundle.Root, dst, item, target); err != nil {
			errs = append(errs, fmt.Errorf("target %s: %w", target, err))
			continue
		}
		item.Path = dst
		rendered = append(rendered, item)
	}
	return rendered, errs
}

// sourceHash computes a content hash of the source tree plus rendered output
// so re-renders with unchanged input can be skipped.
func sourceHash(bundleRoot, renderedSkillMD string, target Target) string {
	h := sha256.New()
	// Walk all files in the source tree, hashing support files (excluding
	// SKILL.md and symskills.toml which are part of the rendered body).
	_ = filepath.WalkDir(bundleRoot, func(path string, d os.DirEntry, err error) error {
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
	// Include the rendered SKILL.md content and the target in the hash.
	h.Write([]byte(renderedSkillMD))
	h.Write([]byte{0})
	h.Write([]byte(target))
	return hex.EncodeToString(h.Sum(nil))
}

func writeRendered(root, dst string, item Rendered, target Target) error {
	sh := sourceHash(root, item.SkillMD, target)

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
	switch Target(s) {
	case TargetOpenCode, TargetClaude, TargetCodex, TargetHermes:
		return Target(s), nil
	default:
		return "", fmt.Errorf("unknown target %q", s)
	}
}
