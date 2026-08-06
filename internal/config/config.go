// Package config provides symskills configuration defaults.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/danieljustus/symaira-corekit/configkit"
)

type Config struct {
	LibraryDir  string `json:"library_dir" toml:"library_dir"`
	RenderDir   string `json:"render_dir" toml:"render_dir"`
	CacheDir    string `json:"cache_dir" toml:"cache_dir"`
	ProfilesDir string `json:"profiles_dir" toml:"profiles_dir"`
	// BaseDir holds the frozen per-skill base snapshots written on install
	// (base/<target>/<name>/ plus manifest.json). It is the stable third
	// reference that makes three-way comparisons possible (#124).
	BaseDir string `json:"base_dir" toml:"base_dir"`
	// Targets holds user-defined harness targets. It intentionally has no
	// json tag: configkit's generic loader walks json-tagged fields and
	// cannot decode slices of structs, so targets are loaded separately by
	// LoadTargets with a dedicated TOML decode.
	Targets []CustomTarget `toml:"targets"`
	// VCS configures per-skill git versioning of the library (#118).
	// Versioning is on by default; set vcs.enabled = false to opt out.
	VCS VCSConfig `json:"vcs" toml:"vcs"`
}

// VCSConfig controls per-skill git versioning. Enabled is a pointer so
// configkit's non-zero merge distinguishes "unset" (default on) from an
// explicit false, which is the zero value for bool.
type VCSConfig struct {
	Enabled *bool `json:"enabled" toml:"enabled"`
}

// VCSEnabled reports whether per-skill git versioning is on. The default
// is true; only an explicit vcs.enabled = false turns it off.
func (c *Config) VCSEnabled() bool {
	return c.VCS.Enabled == nil || *c.VCS.Enabled
}

// targetsFile is the minimal TOML shape used to decode the optional
// `[[targets]]` table from the global and project config files.
type targetsFile struct {
	Targets []CustomTarget `toml:"targets"`
}

// LoadTargets decodes the `[[targets]]` tables from the global config
// (~/.config/symskills/config.toml) and the project config (./.symskills.toml,
// which overrides the global file), mirroring configkit's precedence. Missing
// files are silently skipped, matching config.Load behavior.
func LoadTargets() ([]CustomTarget, error) {
	var merged targetsFile

	globalPath := filepath.Join(os.Getenv("HOME"), ".config", "symskills", "config.toml")
	if home, err := os.UserHomeDir(); err == nil && os.Getenv("HOME") == "" {
		globalPath = filepath.Join(home, ".config", "symskills", "config.toml")
	}
	paths := []string{globalPath}
	if cwd, err := os.Getwd(); err == nil {
		paths = append(paths, filepath.Join(cwd, ".symskills.toml"))
	}
	for _, path := range paths {
		if err := mergeTargetsFile(&merged, path); err != nil {
			return nil, err
		}
	}
	return merged.Targets, nil
}

func mergeTargetsFile(dst *targetsFile, path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	var file targetsFile
	if _, err := toml.DecodeFile(path, &file); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	if len(file.Targets) > 0 {
		dst.Targets = append(dst.Targets[:0], file.Targets...)
	}
	return nil
}

// CustomTarget declares an additional harness target that render, install,
// uninstall, targets and discover treat exactly like a built-in target.
// SkillRootUser is required; SkillRootProject is optional (defaults to the
// user root when unset). MetadataFile names a relative output path inside the
// rendered skill (e.g. "agents/openai.yaml") whose content is taken verbatim
// from MetadataTemplate. OverlayDir overrides the overlay directory name
// (defaults to the target name).
type CustomTarget struct {
	Name             string `json:"name" toml:"name"`
	DisplayName      string `json:"display_name,omitempty" toml:"display_name,omitempty"`
	BinaryName       string `json:"binary_name,omitempty" toml:"binary_name,omitempty"`
	SkillRootUser    string `json:"skill_root_user" toml:"skill_root_user"`
	SkillRootProject string `json:"skill_root_project,omitempty" toml:"skill_root_project,omitempty"`
	MetadataFile     string `json:"metadata_file,omitempty" toml:"metadata_file,omitempty"`
	MetadataTemplate string `json:"metadata_template,omitempty" toml:"metadata_template,omitempty"`
	OverlayDir       string `json:"overlay_dir,omitempty" toml:"overlay_dir,omitempty"`
}

func Defaults() *Config {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return &Config{
		LibraryDir:  filepath.Join(home, ".local", "share", "symskills", "library"),
		RenderDir:   filepath.Join(home, ".local", "share", "symskills", "rendered"),
		CacheDir:    filepath.Join(home, ".cache", "symskills"),
		ProfilesDir: filepath.Join(home, ".config", "symskills", "profiles"),
		BaseDir:     filepath.Join(home, ".local", "share", "symskills", "base"),
		VCS:         VCSConfig{Enabled: boolPtr(true)},
	}
}

func boolPtr(v bool) *bool { return &v }

func ConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".config", "symskills", "config.toml")
	}
	return filepath.Join(home, ".config", "symskills", "config.toml")
}

func EnsureDirs(cfg *Config) error {
	for _, dir := range []string{filepath.Dir(ConfigPath()), cfg.LibraryDir, cfg.RenderDir, cfg.CacheDir, cfg.ProfilesDir, cfg.BaseDir} {
		if dir == "" {
			continue
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func Load() (*Config, error) {
	loader := configkit.NewLoader[Config](configkit.Options{AppName: "symskills", ConfigName: "symskills"}, Defaults)
	return loader.Reload()
}
