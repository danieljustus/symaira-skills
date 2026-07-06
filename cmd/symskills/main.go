// Command symskills manages portable Agent Skill bundles and renders them for
// local AI harnesses.
package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/danieljustus/symaira-corekit/exitcodes"
	"github.com/danieljustus/symaira-corekit/logkit"
	"github.com/danieljustus/symaira-corekit/versionkit"
	"github.com/spf13/cobra"

	"github.com/danieljustus/symaira-skills/internal/config"
	"github.com/danieljustus/symaira-skills/internal/install"
	"github.com/danieljustus/symaira-skills/internal/mcptools"
	"github.com/danieljustus/symaira-skills/internal/render"
	"github.com/danieljustus/symaira-skills/internal/skill"
)

var version = "0.1.0"

func main() {
	slog.SetDefault(logkit.NewFromEnv("symskills"))
	if err := newRootCmd(version).Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "symskills:", exitcodes.FormatCLIError(err))
		os.Exit(int(exitcodes.ExitCodeFromError(err)))
	}
}

func newRootCmd(version string) *cobra.Command {
	root := &cobra.Command{
		Use:           "symskills",
		Short:         "Manage portable Agent Skills across local AI harnesses",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(
		newInitCmd(),
		newImportCmd(),
		newListCmd(),
		newInspectCmd(),
		newValidateCmd(),
		newRenderCmd(),
		newDiffCmd(),
		newInstallCmd(),
		newUninstallCmd(),
		newDoctorCmd(),
		newServeCmd(version),
		newVersionCmd(version),
	)
	return root
}

func newInitCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create the symskills config and local directories",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg := config.Defaults()
			if err := config.EnsureDirs(cfg); err != nil {
				return exitcodes.Wrap(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "create symskills directories")
			}
			path := config.ConfigPath()
			if _, err := os.Stat(path); err == nil && !force {
				fmt.Fprintf(cmd.OutOrStdout(), "Config already exists at %s\n", path)
				return nil
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
			if err != nil {
				return err
			}
			if err := toml.NewEncoder(f).Encode(cfg); err != nil {
				_ = f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Created %s\n", path)
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing config")
	return cmd
}

func newImportCmd() *cobra.Command {
	var library string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "import <skill-dir>",
		Short: "Import an existing skill directory into the symskills library",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return exitcodes.Wrap(err, exitcodes.ExitConfig, exitcodes.KindConfig, "load config")
			}
			if library == "" {
				library = cfg.LibraryDir
			}
			result, err := skill.ImportSkill(args[0], library)
			if err != nil {
				return exitcodes.Wrap(err, exitcodes.ExitConflict, exitcodes.KindConflict, "import skill")
			}
			if jsonOut {
				return printJSON(cmd, result)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Imported %s to %s\n", result.Name, result.Path)
			return nil
		},
	}
	cmd.Flags().StringVar(&library, "library", "", "Library directory")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON")
	return cmd
}

func newListCmd() *cobra.Command {
	var library string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List skills in the symskills library",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if library == "" {
				library = cfg.LibraryDir
			}
			bundles, issues := skill.ListLibrary(library)
			type item struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				Path        string `json:"path"`
			}
			items := make([]item, 0, len(bundles))
			for _, b := range bundles {
				items = append(items, item{Name: b.Frontmatter.Name, Description: b.Frontmatter.Description, Path: b.Root})
			}
			if jsonOut {
				return printJSON(cmd, map[string]any{"skills": items, "issues": issues})
			}
			for _, item := range items {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", item.Name, item.Description, item.Path)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&library, "library", "", "Library directory")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON")
	return cmd
}

func newInspectCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "inspect <skill-dir>",
		Short: "Inspect a skill directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			bundle, err := skill.LoadBundle(args[0])
			if err != nil {
				return exitcodes.Wrap(err, exitcodes.ExitData, exitcodes.KindValidation, "inspect skill")
			}
			if jsonOut {
				return printJSON(cmd, bundle)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s\n%s\n", bundle.Frontmatter.Name, bundle.Frontmatter.Description)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON")
	return cmd
}

func newValidateCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "validate <skill-dir>",
		Short: "Validate a skill directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			bundle, err := skill.LoadBundle(args[0])
			if err != nil {
				return exitcodes.Wrap(err, exitcodes.ExitData, exitcodes.KindValidation, "load skill")
			}
			issues := skill.Validate(bundle)
			result := map[string]any{"valid": len(issues) == 0, "issues": issues}
			if jsonOut {
				return printJSON(cmd, result)
			}
			if len(issues) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "valid")
				return nil
			}
			for _, issue := range issues {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", issue.Severity, issue.Code, issue.Message)
			}
			return exitcodes.Wrap(fmt.Errorf("validation failed"), exitcodes.ExitData, exitcodes.KindValidation, "validate skill")
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON")
	return cmd
}

func newRenderCmd() *cobra.Command {
	var targetName, output string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "render <skill-dir>",
		Short: "Render a skill for supported harness targets",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if output == "" {
				output = cfg.RenderDir
			}
			targets, err := targetsFromFlag(targetName)
			if err != nil {
				return exitcodes.Wrap(err, exitcodes.ExitConfig, exitcodes.KindValidation, "parse target")
			}
			bundle, err := skill.LoadBundle(args[0])
			if err != nil {
				return exitcodes.Wrap(err, exitcodes.ExitData, exitcodes.KindValidation, "load skill")
			}
			results, err := render.RenderAll(bundle, output, targets)
			if err != nil {
				return exitcodes.Wrap(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "render skill")
			}
			if jsonOut {
				return printJSON(cmd, results)
			}
			for _, result := range results {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", result.Target, result.Path)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&targetName, "target", "all", "Target harness: all, opencode, claude, codex, hermes")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Render output directory")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON")
	return cmd
}

func newDiffCmd() *cobra.Command {
	var targetName, output string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "diff <skill-dir>",
		Short: "Compare rendered skill output with the installed target path",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := render.ParseTarget(targetName)
			if err != nil {
				return err
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if output == "" {
				output = cfg.RenderDir
			}
			bundle, err := skill.LoadBundle(args[0])
			if err != nil {
				return err
			}
			rendered, err := render.RenderAll(bundle, output, []render.Target{target})
			if err != nil {
				return err
			}
			installedPath, err := install.InstallPath(target, rendered[0].Name, install.Options{Scope: install.ScopeUser})
			if err != nil {
				return err
			}
			changes, err := install.Diff(rendered[0].Path, installedPath)
			if err != nil {
				return err
			}
			if jsonOut {
				return printJSON(cmd, changes)
			}
			for _, change := range changes {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", change.Status, change.Path)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&targetName, "target", string(render.TargetOpenCode), "Target harness")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Render output directory")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON")
	return cmd
}

func newInstallCmd() *cobra.Command {
	var targetName, output, scopeName, modeName string
	var jsonOut, dryRun bool
	cmd := &cobra.Command{
		Use:   "install <skill-dir>",
		Short: "Render and install a skill into a supported harness",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := render.ParseTarget(targetName)
			if err != nil {
				return err
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if output == "" {
				output = cfg.RenderDir
			}
			bundle, err := skill.LoadBundle(args[0])
			if err != nil {
				return err
			}
			rendered, err := render.RenderAll(bundle, output, []render.Target{target})
			if err != nil {
				return err
			}
			opts := install.Options{Scope: install.Scope(scopeName), Mode: install.Mode(modeName), DryRun: dryRun}
			result, err := install.Install(install.RenderedSkill{Target: target, Name: rendered[0].Name, Path: rendered[0].Path}, opts)
			if err != nil {
				return exitcodes.Wrap(err, exitcodes.ExitConflict, exitcodes.KindConflict, "install skill")
			}
			if jsonOut {
				return printJSON(cmd, result)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s at %s\n", result.Action, result.Name, result.Path)
			return nil
		},
	}
	cmd.Flags().StringVar(&targetName, "target", string(render.TargetOpenCode), "Target harness")
	cmd.Flags().StringVar(&scopeName, "scope", string(install.ScopeUser), "Install scope: user or project")
	cmd.Flags().StringVar(&modeName, "mode", string(install.ModeSymlink), "Install mode: symlink or copy")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Render output directory")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Plan install without writing")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON")
	return cmd
}

func newUninstallCmd() *cobra.Command {
	var targetName, scopeName string
	cmd := &cobra.Command{
		Use:   "uninstall <name>",
		Short: "Remove a managed installed skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := render.ParseTarget(targetName)
			if err != nil {
				return err
			}
			if err := install.Uninstall(target, args[0], install.Options{Scope: install.Scope(scopeName)}); err != nil {
				return exitcodes.Wrap(err, exitcodes.ExitConflict, exitcodes.KindConflict, "uninstall skill")
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Uninstalled %s from %s\n", args[0], target)
			return nil
		},
	}
	cmd.Flags().StringVar(&targetName, "target", string(render.TargetOpenCode), "Target harness")
	cmd.Flags().StringVar(&scopeName, "scope", string(install.ScopeUser), "Install scope: user or project")
	return cmd
}

func newDoctorCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Report symskills paths and target install locations",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			type targetPath struct {
				Target render.Target `json:"target"`
				User   string        `json:"user"`
			}
			paths := []targetPath{}
			for _, target := range render.DefaultTargets {
				path, _ := install.InstallPath(target, "<name>", install.Options{Scope: install.ScopeUser})
				paths = append(paths, targetPath{Target: target, User: path})
			}
			result := map[string]any{"config_path": config.ConfigPath(), "config": cfg, "targets": paths}
			if jsonOut {
				return printJSON(cmd, result)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "config: %s\nlibrary: %s\nrendered: %s\n", config.ConfigPath(), cfg.LibraryDir, cfg.RenderDir)
			for _, p := range paths {
				fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", p.Target, p.User)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON")
	return cmd
}

func newServeCmd(version string) *cobra.Command {
	var stdio bool
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve symskills MCP tools over stdio",
		RunE: func(_ *cobra.Command, _ []string) error {
			if !stdio {
				return exitcodes.Wrap(fmt.Errorf("--stdio is required"), exitcodes.ExitConfig, exitcodes.KindValidation, "serve")
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			return mcptools.Serve(version, mcptools.Options{LibraryDir: cfg.LibraryDir, RenderDir: cfg.RenderDir})
		},
	}
	cmd.Flags().BoolVar(&stdio, "stdio", false, "Serve over stdio")
	return cmd
}

func targetsFromFlag(name string) ([]render.Target, error) {
	if name == "" || name == "all" {
		return render.DefaultTargets, nil
	}
	names := strings.Split(name, ",")
	targets := make([]render.Target, 0, len(names))
	for _, n := range names {
		target, err := render.ParseTarget(strings.TrimSpace(n))
		if err != nil {
			return nil, err
		}
		targets = append(targets, target)
	}
	return targets, nil
}

func newVersionCmd(version string) *cobra.Command {
	var flagJSON bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version",
		RunE: func(cmd *cobra.Command, _ []string) error {
			info := versionkit.New("symskills", version, 1)
			if flagJSON {
				return info.Write(cmd.OutOrStdout())
			}
			fmt.Fprintln(cmd.OutOrStdout(), info.String())
			return nil
		},
	}
	cmd.Flags().BoolVar(&flagJSON, "json", false, "Emit version as machine-readable JSON")
	return cmd
}

func printJSON(cmd *cobra.Command, v any) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}
