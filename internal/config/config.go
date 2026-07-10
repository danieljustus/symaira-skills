// Package config provides symskills configuration defaults.
package config

import (
	"os"
	"path/filepath"

	"github.com/danieljustus/symaira-corekit/configkit"
)

type Config struct {
	LibraryDir  string `json:"library_dir" toml:"library_dir"`
	RenderDir   string `json:"render_dir" toml:"render_dir"`
	CacheDir    string `json:"cache_dir" toml:"cache_dir"`
	ProfilesDir string `json:"profiles_dir" toml:"profiles_dir"`
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
	}
}

func ConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".config", "symskills", "config.toml")
	}
	return filepath.Join(home, ".config", "symskills", "config.toml")
}

func EnsureDirs(cfg *Config) error {
	for _, dir := range []string{filepath.Dir(ConfigPath()), cfg.LibraryDir, cfg.RenderDir, cfg.CacheDir, cfg.ProfilesDir} {
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
