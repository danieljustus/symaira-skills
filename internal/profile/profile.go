// Package profile loads and resolves context-profile files that select skills
// from the managed library with global, parent, and project precedence.
package profile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/danieljustus/symaira-skills/internal/skill"
)

// Profile is a named collection of skill links with optional inheritance.
type Profile struct {
	Name        string          `toml:"name" json:"name"`
	Description string          `toml:"description" json:"description,omitempty"`
	Inherits    []string        `toml:"inherits" json:"inherits,omitempty"`
	Links       map[string]Link `toml:"links" json:"links"`
}

// Link references a skill in the managed library by stable name.
type Link struct {
	Skill string `toml:"skill" json:"skill"`
	Alias string `toml:"alias,omitempty" json:"alias,omitempty"`
}

// Ref describes a discovered profile file.
type Ref struct {
	Name        string `json:"name"`
	Source      string `json:"source"`
	Path        string `json:"path"`
	Description string `json:"description,omitempty"`
}

// Load reads a profile TOML file from the given path.
func Load(path string) (*Profile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read profile %s: %w", filepath.Base(path), err)
	}
	var p Profile
	if _, err := toml.Decode(string(raw), &p); err != nil {
		return nil, fmt.Errorf("parse profile %s: %w", filepath.Base(path), err)
	}
	if p.Links == nil {
		p.Links = map[string]Link{}
	}
	return &p, nil
}

// Validate returns structural issues for a profile.
func Validate(p *Profile) []skill.Issue {
	if p == nil {
		return []skill.Issue{{Code: "profile_required", Severity: "error", Message: "profile is nil"}}
	}
	var issues []skill.Issue
	if strings.TrimSpace(p.Name) == "" {
		issues = append(issues, skill.Issue{Code: "profile_name_required", Severity: "error", Message: "profile name is required"})
	} else if err := skill.ValidateSkillName(p.Name); err != nil {
		issues = append(issues, skill.Issue{Code: "profile_name_format", Severity: "error", Message: err.Error()})
	}
	for linkName, link := range p.Links {
		if strings.TrimSpace(linkName) == "" {
			issues = append(issues, skill.Issue{Code: "profile_link_name_required", Severity: "error", Message: "link name is required"})
			continue
		}
		if err := skill.ValidateSkillName(linkName); err != nil {
			issues = append(issues, skill.Issue{Code: "profile_link_name_format", Severity: "error", Message: err.Error(), Path: linkName})
		}
		if strings.TrimSpace(link.Skill) == "" {
			issues = append(issues, skill.Issue{Code: "profile_link_skill_required", Severity: "error", Message: "link skill is required", Path: linkName})
		} else if err := skill.ValidateSkillName(link.Skill); err != nil {
			issues = append(issues, skill.Issue{Code: "profile_link_skill_format", Severity: "error", Message: err.Error(), Path: linkName})
		}
	}
	return issues
}

// List discovers profile files across the configured contexts. The most
// specific context wins when the same profile name exists in multiple places.
func List(globalProfilesDir, projectDir string) ([]Ref, error) {
	ctxs := buildContexts(globalProfilesDir, projectDir)
	seen := map[string]bool{}
	var refs []Ref
	for _, ctx := range ctxs {
		entries, err := os.ReadDir(ctx.dir)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("list profiles %s: %w", ctx.dir, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".toml" {
				continue
			}
			name := strings.TrimSuffix(entry.Name(), ".toml")
			if seen[name] {
				continue
			}
			seen[name] = true
			path := filepath.Join(ctx.dir, entry.Name())
			p, err := Load(path)
			if err != nil {
				return nil, fmt.Errorf("load profile %s: %w", path, err)
			}
			refs = append(refs, Ref{
				Name:        name,
				Source:      ctx.name,
				Path:        path,
				Description: p.Description,
			})
		}
	}
	return refs, nil
}
