// Package harness provides read-only inventory and readiness status for AI-agent harnesses.
package harness

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/danieljustus/symaira-skills/internal/install"
	"github.com/danieljustus/symaira-skills/internal/render"
)

type Status struct {
	Target               render.Target `json:"target"`
	DisplayName          string        `json:"display_name"`
	Installed            bool          `json:"installed"`
	Evidence             string        `json:"evidence"`
	EffectiveSkillRoot   string        `json:"effective_skill_root"`
	SkillRootExists      bool          `json:"skill_root_exists"`
	SkillRootReadable    bool          `json:"skill_root_readable"`
	ManagedSkillsCount   int           `json:"managed_skills_count"`
	UnmanagedSkillsCount int           `json:"unmanaged_skills_count"`
	InstallState         string        `json:"install_state"`
	Capabilities         []string      `json:"capabilities"`
	SetupHint            string        `json:"setup_hint"`
	VerificationStatus   string        `json:"verification_status"`
}

type Options struct {
	HomeDir    string        `json:"home_dir,omitempty"`
	ProjectDir string        `json:"project_dir,omitempty"`
	Scope      install.Scope `json:"scope,omitempty"`
}

type Descriptor struct {
	Target      render.Target
	DisplayName string
	BinaryName  string
	ConfigDir   func(home, project string, scope install.Scope) string
	SkillRoot   func(home, project string, scope install.Scope) string
}

var Descriptors = []Descriptor{
	{
		Target:      render.TargetOpenCode,
		DisplayName: "OpenCode",
		BinaryName:  "opencode",
		ConfigDir: func(home, project string, scope install.Scope) string {
			if scope == install.ScopeProject && project != "" {
				return filepath.Join(project, ".opencode")
			}
			return filepath.Join(home, ".config", "opencode")
		},
		SkillRoot: func(home, project string, scope install.Scope) string {
			if scope == install.ScopeProject && project != "" {
				return filepath.Join(project, ".opencode", "skills")
			}
			return filepath.Join(home, ".config", "opencode", "skills")
		},
	},
	{
		Target:      render.TargetClaude,
		DisplayName: "Claude Code",
		BinaryName:  "claude",
		ConfigDir: func(home, project string, scope install.Scope) string {
			if scope == install.ScopeProject && project != "" {
				return filepath.Join(project, ".claude")
			}
			return filepath.Join(home, ".claude")
		},
		SkillRoot: func(home, project string, scope install.Scope) string {
			if scope == install.ScopeProject && project != "" {
				return filepath.Join(project, ".claude", "skills")
			}
			return filepath.Join(home, ".claude", "skills")
		},
	},
	{
		Target:      render.TargetCodex,
		DisplayName: "Codex",
		BinaryName:  "codex",
		ConfigDir: func(home, project string, scope install.Scope) string {
			if scope == install.ScopeProject && project != "" {
				return filepath.Join(project, ".agents")
			}
			return filepath.Join(home, ".agents")
		},
		SkillRoot: func(home, project string, scope install.Scope) string {
			if scope == install.ScopeProject && project != "" {
				return filepath.Join(project, ".agents", "skills")
			}
			return filepath.Join(home, ".agents", "skills")
		},
	},
	{
		Target:      render.TargetHermes,
		DisplayName: "Hermes",
		BinaryName:  "hermes",
		ConfigDir: func(home, project string, scope install.Scope) string {
			if scope == install.ScopeProject && project != "" {
				return filepath.Join(project, ".hermes")
			}
			return filepath.Join(home, ".hermes")
		},
		SkillRoot: func(home, project string, scope install.Scope) string {
			if scope == install.ScopeProject && project != "" {
				return filepath.Join(project, ".hermes", "skills")
			}
			return filepath.Join(home, ".hermes", "skills", "symaira")
		},
	},
}

// ListStatus returns the harness inventory and readiness status for all supported targets.
func ListStatus(opts Options) []Status {
	if opts.HomeDir == "" {
		if h, err := os.UserHomeDir(); err == nil {
			opts.HomeDir = h
		}
	}
	if opts.Scope == "" {
		opts.Scope = install.ScopeUser
	}

	results := make([]Status, 0, len(Descriptors))
	for _, desc := range Descriptors {
		results = append(results, inspectTarget(desc, opts))
	}
	return results
}

func inspectTarget(desc Descriptor, opts Options) Status {
	capabilities := []string{"render", "install", "symlink", "copy"}
	skillRoot := desc.SkillRoot(opts.HomeDir, opts.ProjectDir, opts.Scope)
	configDir := desc.ConfigDir(opts.HomeDir, opts.ProjectDir, opts.Scope)

	st := Status{
		Target:             desc.Target,
		DisplayName:        desc.DisplayName,
		EffectiveSkillRoot: skillRoot,
		Capabilities:       capabilities,
		VerificationStatus: "unknown",
		Evidence:           "none",
	}

	// 1. Evidence / Installed check
	var evidenceParts []string
	if binPath, err := exec.LookPath(desc.BinaryName); err == nil && binPath != "" {
		evidenceParts = append(evidenceParts, fmt.Sprintf("binary:%s", binPath))
		st.Installed = true
		st.VerificationStatus = "verified"
	}
	if info, err := os.Stat(configDir); err == nil && info.IsDir() {
		evidenceParts = append(evidenceParts, fmt.Sprintf("config_dir:%s", configDir))
		st.Installed = true
		if st.VerificationStatus == "unknown" {
			st.VerificationStatus = "verified"
		}
	}
	if len(evidenceParts) > 0 {
		st.Evidence = evidenceParts[0]
		if len(evidenceParts) > 1 {
			st.Evidence = fmt.Sprintf("%s,%s", evidenceParts[0], evidenceParts[1])
		}
	} else {
		st.VerificationStatus = "not_verified"
	}

	// 2. Skill root check
	_, err := os.Lstat(skillRoot)
	if err == nil {
		st.SkillRootExists = true
		// Verify directory readable by reading entries
		entries, readErr := os.ReadDir(skillRoot)
		if readErr == nil {
			st.SkillRootReadable = true
			for _, entry := range entries {
				itemPath := filepath.Join(skillRoot, entry.Name())
				if isManagedSkill(itemPath) {
					st.ManagedSkillsCount++
				} else if entry.IsDir() || (entry.Type()&os.ModeSymlink != 0) {
					st.UnmanagedSkillsCount++
				}
			}
		} else {
			st.SkillRootReadable = false
		}
	} else if os.IsNotExist(err) {
		st.SkillRootExists = false
		st.SkillRootReadable = false
	}

	// 3. Install State & Setup Hint derivation
	if !st.SkillRootExists {
		st.InstallState = "missing"
		st.SetupHint = fmt.Sprintf("Create skill directory %s or run 'symskills install --target %s <skill>'", skillRoot, desc.Target)
	} else if !st.SkillRootReadable {
		st.InstallState = "unreadable"
		st.SetupHint = fmt.Sprintf("Check permissions for skill root %s", skillRoot)
	} else if st.ManagedSkillsCount > 0 && st.UnmanagedSkillsCount == 0 {
		st.InstallState = "managed"
		st.SetupHint = fmt.Sprintf("Harness is active with %d managed skill(s)", st.ManagedSkillsCount)
	} else if st.ManagedSkillsCount > 0 && st.UnmanagedSkillsCount > 0 {
		st.InstallState = "mixed"
		st.SetupHint = fmt.Sprintf("Harness contains %d managed and %d unmanaged skill(s)", st.ManagedSkillsCount, st.UnmanagedSkillsCount)
	} else if st.UnmanagedSkillsCount > 0 {
		st.InstallState = "unmanaged"
		st.SetupHint = fmt.Sprintf("Harness contains %d unmanaged skill(s); consider importing into library", st.UnmanagedSkillsCount)
	} else {
		st.InstallState = "empty"
		st.SetupHint = fmt.Sprintf("Harness skill directory is ready; install skills with 'symskills install --target %s <skill>'", desc.Target)
	}

	return st
}

func isManagedSkill(path string) bool {
	// A skill install directory is managed if it contains .symskills.json or if symlink target contains .symskills.json
	markerPath := filepath.Join(path, ".symskills.json")
	if _, err := os.Stat(markerPath); err == nil {
		return true
	}
	// Check if symlink
	if st, err := os.Lstat(path); err == nil && (st.Mode()&os.ModeSymlink != 0) {
		if target, err := os.Readlink(path); err == nil {
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(path), target)
			}
			if _, err := os.Stat(filepath.Join(target, ".symskills.json")); err == nil {
				return true
			}
		}
	}
	return false
}

func FormatJSON(statuses []Status) (string, error) {
	data, err := json.MarshalIndent(statuses, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
