// Package render creates harness-specific skill folders from portable bundles.
package render

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
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

type Rendered struct {
	Target      Target            `json:"target"`
	Name        string            `json:"name"`
	Path        string            `json:"path,omitempty"`
	Frontmatter skill.Frontmatter `json:"frontmatter"`
	SkillMD     string            `json:"skill_md,omitempty"`
}

// RenderTarget returns a target-specific SKILL.md without writing files.
func RenderTarget(bundle *skill.Bundle, target Target) (Rendered, error) {
	if bundle == nil {
		return Rendered{}, fmt.Errorf("bundle is nil")
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
	if cfg.Alias != "" {
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
	return Rendered{Target: target, Name: fm.Name, Frontmatter: fm, SkillMD: skillMD}, nil
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
		return readOptional(filepath.Join(root, configured))
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

// RenderAll writes target-specific skill folders under outDir.
func RenderAll(bundle *skill.Bundle, outDir string, targets []Target) ([]Rendered, error) {
	if len(targets) == 0 {
		targets = DefaultTargets
	}
	var rendered []Rendered
	for _, target := range targets {
		item, err := RenderTarget(bundle, target)
		if err != nil {
			continue
		}
		dst := filepath.Join(outDir, string(target), item.Name)
		if err := writeRendered(bundle.Root, dst, item, target); err != nil {
			return nil, err
		}
		item.Path = dst
		rendered = append(rendered, item)
	}
	return rendered, nil
}

func writeRendered(root, dst string, item Rendered, target Target) error {
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
	return nil
}

func copySupportFiles(src, dst string) error {
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
		if d.IsDir() && (d.Name() == ".git" || d.Name() == "overlays") {
			return filepath.SkipDir
		}
		if rel == "SKILL.md" || rel == "symskills.toml" {
			return nil
		}
		targetPath := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(targetPath, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return copyFile(path, targetPath, info.Mode().Perm())
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
