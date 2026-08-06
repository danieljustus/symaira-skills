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
	"time"

	"github.com/BurntSushi/toml"
	"github.com/danieljustus/symaira-corekit/exitcodes"
	"github.com/danieljustus/symaira-corekit/logkit"
	"github.com/danieljustus/symaira-corekit/versionkit"
	"github.com/spf13/cobra"

	"github.com/danieljustus/symaira-skills/internal/config"
	"github.com/danieljustus/symaira-skills/internal/discover"
	"github.com/danieljustus/symaira-skills/internal/events"
	"github.com/danieljustus/symaira-skills/internal/harness"
	"github.com/danieljustus/symaira-skills/internal/install"
	"github.com/danieljustus/symaira-skills/internal/mcptools"
	"github.com/danieljustus/symaira-skills/internal/profile"
	"github.com/danieljustus/symaira-skills/internal/render"
	"github.com/danieljustus/symaira-skills/internal/skill"
)

var version = "0.1.9"

func main() {
	os.Exit(runMain(newRootCmd(version), os.Args[1:]))
}

// runMain executes the root command with the given arguments and returns the
// process exit code. It is separated from main so the error path can be
// tested without terminating the test process via os.Exit.
func runMain(root *cobra.Command, args []string) int {
	slog.SetDefault(logkit.NewFromEnv("symskills"))
	if err := registerCustomTargets(); err != nil {
		fmt.Fprintln(os.Stderr, "symskills:", exitcodes.FormatCLIError(err))
		return int(exitcodes.ExitCodeFromError(err))
	}
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "symskills:", exitcodes.FormatCLIError(err))
		return int(exitcodes.ExitCodeFromError(err))
	}
	return 0
}

// registerCustomTargets loads the user config and registers any declared
// custom harness targets into the render registry. A missing config file is
// not an error (defaults apply); a malformed or colliding custom target is.
func registerCustomTargets() error {
	targets, err := config.LoadTargets()
	if err != nil {
		return exitcodes.Wrap(err, exitcodes.ExitConfig, exitcodes.KindConfig, "load config")
	}
	specs := make([]render.CustomTargetSpec, 0, len(targets))
	for _, t := range targets {
		specs = append(specs, render.CustomTargetSpec{
			Name:             t.Name,
			DisplayName:      t.DisplayName,
			BinaryName:       t.BinaryName,
			SkillRootUser:    t.SkillRootUser,
			SkillRootProject: t.SkillRootProject,
			MetadataFile:     t.MetadataFile,
			MetadataTemplate: t.MetadataTemplate,
			OverlayDir:       t.OverlayDir,
		})
	}
	if err := render.RegisterCustomTargets(specs); err != nil {
		return exitcodes.Wrap(err, exitcodes.ExitConfig, exitcodes.KindConfig, "register custom targets")
	}
	return nil
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
		newProfileCmd(),
		newTargetsCmd(),
		newDiscoverCmd(),
		newLogCmd(),
		newDoctorCmd(),
		newServeCmd(version),
		newVersionCmd(version),
	)
	return root
}

// newEventLogger returns the operation logger for the current invocation.
// The log is a file under the current HOME; it is never written to stdout
// (stdout is reserved for JSON-RPC frames while serving MCP).
func newEventLogger() *events.Logger {
	return events.New(events.DefaultPath(), version)
}

// skillVersion reads the version from a skill directory's frontmatter,
// returning "" when the directory is not a loadable skill.
func skillVersion(dir string) string {
	bundle, err := skill.LoadBundle(dir)
	if err != nil {
		return ""
	}
	return bundle.Frontmatter.Version
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
	var batch bool
	cmd := &cobra.Command{
		Use:   "import <skill-dir>",
		Short: "Import an existing skill directory into the symskills library",
		Long: `Import a skill directory or batch-import skills from a parent directory.

Without --batch: imports a single skill directory containing SKILL.md.
With --batch: scans the given directory for immediate subdirectories
containing SKILL.md and imports each one. If the directory itself is a
skill, it falls back to single-skill import.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return exitcodes.Wrap(err, exitcodes.ExitConfig, exitcodes.KindConfig, "load config")
			}
			lib := library
			if lib == "" {
				lib = cfg.LibraryDir
			}

			if batch {
				results := skill.ImportSkills(args[0], lib)
				logger := newEventLogger()
				for _, r := range results {
					switch r.Status {
					case skill.BatchImported:
						logger.Record(events.Event{Event: events.EventImport, Skill: r.Name, Path: r.Path, Outcome: events.OutcomeOK, Actor: events.ActorCLI})
					case skill.BatchFailed:
						logger.Record(events.Event{Event: events.EventImport, Skill: r.Name, Outcome: events.OutcomeError, Error: r.Error, Actor: events.ActorCLI})
					}
				}
				if jsonOut {
					return printJSON(cmd, results)
				}
				imported, skipped, failed := 0, 0, 0
				for _, r := range results {
					switch r.Status {
					case skill.BatchImported:
						fmt.Fprintf(cmd.OutOrStdout(), "Imported %s to %s\n", r.Name, r.Path)
						imported++
					case skill.BatchSkipped:
						fmt.Fprintf(cmd.OutOrStdout(), "Skipped %s: %s\n", r.Name, r.Error)
						skipped++
					case skill.BatchFailed:
						fmt.Fprintf(cmd.OutOrStdout(), "Failed %s: %s\n", r.Name, r.Error)
						failed++
					}
				}
				fmt.Fprintf(cmd.OutOrStdout(), "\nSummary: %d imported, %d skipped, %d failed\n", imported, skipped, failed)
				return nil
			}

			result, err := skill.ImportSkill(args[0], lib)
			logger := newEventLogger()
			if err != nil {
				name := filepath.Base(args[0])
				if b, lerr := skill.LoadBundle(args[0]); lerr == nil && b.Frontmatter.Name != "" {
					name = b.Frontmatter.Name
				}
				logger.Record(events.Event{Event: events.EventImport, Skill: name, Outcome: events.OutcomeError, Error: err.Error(), Actor: events.ActorCLI})
				return exitcodes.Wrap(err, exitcodes.ExitConflict, exitcodes.KindConflict, "import skill")
			}
			logger.Record(events.Event{Event: events.EventImport, Skill: result.Name, SkillVersion: skillVersion(args[0]), Path: result.Path, Outcome: events.OutcomeOK, Actor: events.ActorCLI})
			if jsonOut {
				return printJSON(cmd, result)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Imported %s to %s\n", result.Name, result.Path)
			return nil
		},
	}
	cmd.Flags().StringVar(&library, "library", "", "Library directory")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON")
	cmd.Flags().BoolVar(&batch, "batch", false, "Batch-import all skills from subdirectories")
	return cmd
}

func newListCmd() *cobra.Command {
	var library string
	var jsonOut bool
	var strict bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List skills in the symskills library",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			lib := library
			if lib == "" {
				lib = cfg.LibraryDir
			}
			bundles, issues := skill.ListLibrary(lib)
			if issues == nil {
				issues = []skill.Issue{}
			}
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
			for _, issue := range issues {
				if issue.Path != "" {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s: %s\n", issue.Path, issue.Message)
				} else {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", issue.Message)
				}
			}
			if strict && len(issues) > 0 {
				return exitcodes.Wrap(fmt.Errorf("library load issues detected"), exitcodes.ExitData, exitcodes.KindValidation, "list library")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&library, "library", "", "Library directory")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON")
	cmd.Flags().BoolVar(&strict, "strict", false, "Exit non-zero when library load issues exist")
	return cmd
}

func isSkillDir(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "SKILL.md"))
	return err == nil
}

func resolveSkillDir(args []string, requiredMsg string, libraryDir string) (string, error) {
	if len(args) == 1 {
		dir := args[0]
		if isSkillDir(dir) {
			return dir, nil
		}
		if libraryDir != "" {
			libPath := filepath.Join(libraryDir, dir)
			if isSkillDir(libPath) {
				return libPath, nil
			}
		}
		if libraryDir != "" {
			return "", fmt.Errorf("skill %q not found locally at %q nor in library at %q", dir, dir, filepath.Join(libraryDir, dir))
		}
		return "", fmt.Errorf("skill %q not found locally at %q", dir, dir)
	}
	if isSkillDir(".") {
		return ".", nil
	}
	return "", fmt.Errorf("%s", requiredMsg)
}

func newInspectCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "inspect [skill-dir]",
		Short: "Inspect a skill directory",
		Args:  cobra.RangeArgs(0, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			dir, err := resolveSkillDir(args, "skill-dir is required", cfg.LibraryDir)
			if err != nil {
				return exitcodes.Wrap(err, exitcodes.ExitData, exitcodes.KindValidation, "inspect skill")
			}
			bundle, err := skill.LoadBundle(dir)
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
		Use:   "validate [skill-dir]",
		Short: "Validate a skill directory",
		Args:  cobra.RangeArgs(0, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			dir, err := resolveSkillDir(args, "skill-dir is required", cfg.LibraryDir)
			if err != nil {
				return exitcodes.Wrap(err, exitcodes.ExitData, exitcodes.KindValidation, "load skill")
			}
			bundle, err := skill.LoadBundle(dir)
			if err != nil {
				return exitcodes.Wrap(err, exitcodes.ExitData, exitcodes.KindValidation, "load skill")
			}
			issues := skill.Validate(bundle)
			if issues == nil {
				issues = []skill.Issue{}
			}
			if len(issues) > 0 {
				messages := make([]string, 0, len(issues))
				for _, issue := range issues {
					messages = append(messages, issue.Message)
				}
				newEventLogger().Record(events.Event{
					Event:   events.EventValidateFailure,
					Skill:   bundle.Frontmatter.Name,
					Path:    dir,
					Outcome: events.OutcomeError,
					Error:   strings.Join(messages, "; "),
					Actor:   events.ActorCLI,
				})
			}
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
	var targetName, output, profileName string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "render [skill-dir]",
		Short: "Render a skill or profile for supported harness targets",
		Args:  cobra.RangeArgs(0, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			out := output
			if out == "" {
				out = cfg.RenderDir
			}
			targets, err := targetsFromFlag(targetName)
			if err != nil {
				return exitcodes.Wrap(err, exitcodes.ExitConfig, exitcodes.KindValidation, "parse target")
			}
			if profileName != "" {
				if len(args) > 0 {
					return exitcodes.Wrap(fmt.Errorf("skill-dir is not used with --profile"), exitcodes.ExitConfig, exitcodes.KindValidation, "render profile")
				}
				return renderProfile(cmd, cfg, out, targets, profileName, jsonOut)
			}
			dir, err := resolveSkillDir(args, "skill-dir is required without --profile", cfg.LibraryDir)
			if err != nil {
				return exitcodes.Wrap(err, exitcodes.ExitConfig, exitcodes.KindValidation, "render skill")
			}
			bundle, err := skill.LoadBundle(dir)
			if err != nil {
				return exitcodes.Wrap(err, exitcodes.ExitData, exitcodes.KindValidation, "load skill")
			}
			results, errs := render.RenderAll(bundle, out, targets)
			logger := newEventLogger()
			if len(errs) > 0 {
				logger.Record(events.Event{Event: events.EventRender, Skill: bundle.Frontmatter.Name, Target: targetName, Path: out, Outcome: events.OutcomeError, Error: errs[0].Error(), Actor: events.ActorCLI})
				return exitcodes.Wrap(errs[0], exitcodes.ExitSoftware, exitcodes.KindInternal, "render skill")
			}
			logger.Record(events.Event{Event: events.EventRender, Skill: bundle.Frontmatter.Name, SkillVersion: bundle.Frontmatter.Version, Target: targetName, Path: out, Outcome: events.OutcomeOK, Actor: events.ActorCLI})
			return printRenderResults(cmd, results, jsonOut)
		},
	}
	cmd.Flags().StringVar(&targetName, "target", "all", "Target harness: all, opencode, claude, codex, hermes, antigravity, openclaw")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Render output directory")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON")
	cmd.Flags().StringVar(&profileName, "profile", "", "Render all skills from a context profile")
	return cmd
}

func printRenderResults(cmd *cobra.Command, results []render.Rendered, jsonOut bool) error {
	if jsonOut {
		return printJSON(cmd, results)
	}
	for _, result := range results {
		fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\n", result.Target, result.Name, result.Source, result.Path)
	}
	return nil
}

func renderProfile(cmd *cobra.Command, cfg *config.Config, output string, targets []render.Target, profileName string, jsonOut bool) error {
	results, issues, err := profile.RenderProfile(cfg.LibraryDir, cfg.ProfilesDir, ".", output, targets, profileName)
	logger := newEventLogger()
	if err != nil {
		logger.Record(events.Event{Event: events.EventRender, Outcome: events.OutcomeError, Error: err.Error(), Actor: events.ActorCLI})
		return exitcodes.Wrap(err, exitcodes.ExitData, exitcodes.KindValidation, "resolve profile")
	}
	if len(issues) > 0 {
		for _, issue := range issues {
			fmt.Fprintf(cmd.ErrOrStderr(), "%s	%s	%s\n", issue.Severity, issue.Code, issue.Message)
		}
		messages := make([]string, 0, len(issues))
		for _, issue := range issues {
			messages = append(messages, issue.Message)
		}
		logger.Record(events.Event{Event: events.EventRender, Outcome: events.OutcomeError, Error: strings.Join(messages, "; "), Actor: events.ActorCLI})
		return exitcodes.Wrap(fmt.Errorf("profile has unresolved issues"), exitcodes.ExitData, exitcodes.KindValidation, "resolve profile")
	}
	if len(results) == 0 {
		if jsonOut {
			return printJSON(cmd, []render.Rendered{})
		}
		fmt.Fprintln(cmd.OutOrStdout(), "No skills in profile")
		return nil
	}
	for _, result := range results {
		logger.Record(events.Event{Event: events.EventRender, Skill: result.Name, SkillVersion: result.Frontmatter.Version, Target: string(result.Target), Path: result.Path, Outcome: events.OutcomeOK, Actor: events.ActorCLI})
	}
	return printRenderResults(cmd, results, jsonOut)
}

func newDiffCmd() *cobra.Command {
	var targetName string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "diff [skill-dir]",
		Short: "Compare rendered skill output with the installed target path",
		Args:  cobra.RangeArgs(0, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := render.ParseTarget(targetName)
			if err != nil {
				return err
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			// Render to a temp directory so diff never touches the
			// real render cache or a symlinked install target (#86).
			tempDir, err := os.MkdirTemp("", "symskills-diff-")
			if err != nil {
				return err
			}
			defer os.RemoveAll(tempDir)

			dir, err := resolveSkillDir(args, "skill-dir is required", cfg.LibraryDir)
			if err != nil {
				return exitcodes.Wrap(err, exitcodes.ExitData, exitcodes.KindValidation, "diff skill")
			}
			bundle, err := skill.LoadBundle(dir)
			if err != nil {
				return err
			}
			rendered, errs := render.RenderAll(bundle, tempDir, []render.Target{target})
			if len(rendered) == 0 {
				if len(errs) > 0 {
					return exitcodes.Wrap(errs[0], exitcodes.ExitSoftware, exitcodes.KindInternal, "render target")
				}
				return exitcodes.Wrap(fmt.Errorf("target %s produced no render output", target), exitcodes.ExitSoftware, exitcodes.KindInternal, "render target")
			}
			installedPath, err := install.InstallPath(target, rendered[0].Name, install.Options{Scope: render.ScopeUser})
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
			if len(changes) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No changes detected.")
				return nil
			}
			for _, change := range changes {
				fmt.Fprintf(cmd.OutOrStdout(), "%s	%s\n", change.Status, change.Path)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&targetName, "target", string(render.TargetOpenCode), "Target harness")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON")
	return cmd
}

func newInstallCmd() *cobra.Command {
	var targetName, output, scopeName, modeName, profileName string
	var jsonOut, dryRun, force bool
	cmd := &cobra.Command{
		Use:   "install [skill-dir]",
		Short: "Render and install a skill or profile into a supported harness",
		Args:  cobra.RangeArgs(0, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := render.ParseTarget(targetName)
			if err != nil {
				return err
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			out := output
			if out == "" {
				out = cfg.RenderDir
			}
			opts := install.Options{Scope: render.Scope(scopeName), Mode: install.Mode(modeName), DryRun: dryRun, Force: force}
			if profileName != "" {
				if len(args) > 0 {
					return exitcodes.Wrap(fmt.Errorf("skill-dir is not used with --profile"), exitcodes.ExitConfig, exitcodes.KindValidation, "install profile")
				}
				return installProfile(cmd, cfg, out, target, profileName, opts, jsonOut)
			}
			dir, err := resolveSkillDir(args, "skill-dir is required without --profile", cfg.LibraryDir)
			if err != nil {
				return exitcodes.Wrap(err, exitcodes.ExitConfig, exitcodes.KindValidation, "install skill")
			}
			bundle, err := skill.LoadBundle(dir)
			if err != nil {
				return err
			}
			rendered, errs := render.RenderAll(bundle, out, []render.Target{target})
			if len(rendered) == 0 {
				if len(errs) > 0 {
					newEventLogger().Record(events.Event{Event: events.EventInstall, Skill: bundle.Frontmatter.Name, Target: string(target), Outcome: events.OutcomeError, Error: errs[0].Error(), Actor: events.ActorCLI})
					return exitcodes.Wrap(errs[0], exitcodes.ExitSoftware, exitcodes.KindInternal, "render target")
				}
				return exitcodes.Wrap(fmt.Errorf("target %s produced no render output", target), exitcodes.ExitSoftware, exitcodes.KindInternal, "render target")
			}
			result, err := install.Install(install.RenderedSkill{Target: target, Name: rendered[0].Name, Path: rendered[0].Path}, opts)
			logger := newEventLogger()
			if err != nil {
				logger.Record(events.Event{Event: events.EventInstall, Skill: rendered[0].Name, Target: string(target), Outcome: events.OutcomeError, Error: err.Error(), Actor: events.ActorCLI})
				return exitcodes.Wrap(err, exitcodes.ExitConflict, exitcodes.KindConflict, "install skill")
			}
			if !dryRun {
				logger.Record(events.Event{Event: events.EventInstall, Skill: result.Name, SkillVersion: bundle.Frontmatter.Version, Target: string(target), Scope: string(opts.Scope), Mode: string(result.Mode), Path: result.Path, Outcome: events.OutcomeOK, Actor: events.ActorCLI})
			}
			if jsonOut {
				return printJSON(cmd, result)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s at %s\n", result.Action, result.Name, result.Path)
			if result.BackupPath != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "previous unmanaged skill moved to %s\n", result.BackupPath)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&targetName, "target", string(render.TargetOpenCode), "Target harness")
	cmd.Flags().StringVar(&scopeName, "scope", string(render.ScopeUser), "Install scope: user or project")
	cmd.Flags().StringVar(&modeName, "mode", string(install.ModeSymlink), "Install mode: symlink or copy")
	cmd.Flags().BoolVar(&force, "force", false, "Adopt an unmanaged skill at the target path, moving the existing one to a backup directory")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Render output directory")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Plan install without writing")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON")
	cmd.Flags().StringVar(&profileName, "profile", "", "Install all skills from a context profile")
	return cmd
}

func installProfile(cmd *cobra.Command, cfg *config.Config, output string, target render.Target, profileName string, opts install.Options, jsonOut bool) error {
	results, issues, err := profile.InstallProfile(cfg.LibraryDir, cfg.ProfilesDir, ".", output, target, profileName, opts)
	logger := newEventLogger()
	if err != nil {
		logger.Record(events.Event{Event: events.EventProfileInstall, Target: string(target), Outcome: events.OutcomeError, Error: err.Error(), Actor: events.ActorCLI})
		return exitcodes.Wrap(err, exitcodes.ExitConflict, exitcodes.KindConflict, "install profile")
	}
	if len(issues) > 0 {
		for _, issue := range issues {
			fmt.Fprintf(cmd.ErrOrStderr(), "%s	%s	%s\n", issue.Severity, issue.Code, issue.Message)
		}
		messages := make([]string, 0, len(issues))
		for _, issue := range issues {
			messages = append(messages, issue.Message)
		}
		logger.Record(events.Event{Event: events.EventProfileInstall, Target: string(target), Outcome: events.OutcomeError, Error: strings.Join(messages, "; "), Actor: events.ActorCLI})
		return exitcodes.Wrap(fmt.Errorf("profile has unresolved issues"), exitcodes.ExitData, exitcodes.KindValidation, "resolve profile")
	}
	if len(results) == 0 {
		if jsonOut {
			return printJSON(cmd, []install.Result{})
		}
		fmt.Fprintln(cmd.OutOrStdout(), "No skills in profile")
		return nil
	}
	if !opts.DryRun {
		for _, result := range results {
			logger.Record(events.Event{Event: events.EventProfileInstall, Skill: result.Name, Target: string(result.Target), Scope: string(opts.Scope), Mode: string(result.Mode), Path: result.Path, Outcome: events.OutcomeOK, Actor: events.ActorCLI})
		}
	}
	if jsonOut {
		return printJSON(cmd, results)
	}
	for _, result := range results {
		fmt.Fprintf(cmd.OutOrStdout(), "%s %s at %s\n", result.Action, result.Name, result.Path)
		if result.BackupPath != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "previous unmanaged skill moved to %s\n", result.BackupPath)
		}
	}
	return nil
}

func newUninstallCmd() *cobra.Command {
	var targetName, scopeName string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "uninstall <name>",
		Short: "Remove a managed installed skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := render.ParseTarget(targetName)
			if err != nil {
				return err
			}
			opts := install.Options{Scope: render.Scope(scopeName)}
			removed, err := install.Uninstall(target, args[0], opts)
			logger := newEventLogger()
			path, _ := install.InstallPath(target, args[0], opts)
			if err != nil {
				logger.Record(events.Event{Event: events.EventUninstall, Skill: args[0], Target: string(target), Scope: string(opts.Scope), Path: path, Outcome: events.OutcomeError, Error: err.Error(), Actor: events.ActorCLI})
				return exitcodes.Wrap(err, exitcodes.ExitConflict, exitcodes.KindConflict, "uninstall skill")
			}
			logger.Record(events.Event{Event: events.EventUninstall, Skill: args[0], Target: string(target), Scope: string(opts.Scope), Path: path, Outcome: events.OutcomeOK, Actor: events.ActorCLI})
			if jsonOut {
				return printJSON(cmd, map[string]any{
					"name":    args[0],
					"target":  string(target),
					"removed": removed,
				})
			}
			if !removed {
				fmt.Fprintf(cmd.OutOrStdout(), "%s was not installed for %s\n", args[0], target)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Uninstalled %s from %s\n", args[0], target)
			return nil
		},
	}
	cmd.Flags().StringVar(&targetName, "target", string(render.TargetOpenCode), "Target harness")
	cmd.Flags().StringVar(&scopeName, "scope", string(render.ScopeUser), "Install scope: user or project")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON")
	return cmd
}

func newProfileCmd() *cobra.Command {
	profileCmd := &cobra.Command{
		Use:   "profile",
		Short: "Manage inherited context profiles for skill discovery",
	}
	profileCmd.AddCommand(
		newProfileListCmd(),
		newProfileResolveCmd(),
		newProfileValidateCmd(),
	)
	return profileCmd
}

func newProfileListCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available context profiles",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			refs, err := profile.List(cfg.ProfilesDir, ".")
			if err != nil {
				return exitcodes.Wrap(err, exitcodes.ExitData, exitcodes.KindValidation, "list profiles")
			}
			if jsonOut {
				return printJSON(cmd, refs)
			}
			for _, ref := range refs {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", ref.Name, ref.Source, ref.Path)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON")
	return cmd
}

func newProfileResolveCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "resolve <profile-name>",
		Short: "Resolve a profile and print its merged skill set",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			resolved, issues, err := profile.Resolve(cfg.LibraryDir, cfg.ProfilesDir, ".", args[0])
			if err != nil {
				return exitcodes.Wrap(err, exitcodes.ExitData, exitcodes.KindValidation, "resolve profile")
			}
			if len(issues) > 0 {
				for _, issue := range issues {
					fmt.Fprintf(cmd.ErrOrStderr(), "%s\t%s\t%s\n", issue.Severity, issue.Code, issue.Message)
				}
			}
			if jsonOut {
				return printJSON(cmd, map[string]any{"skills": resolved, "issues": issues})
			}
			for _, rs := range resolved {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\n", rs.Name, rs.Skill, rs.Source, rs.Profile)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON")
	return cmd
}

func newProfileValidateCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "validate <profile-name>",
		Short: "Validate a profile's structure and linked skill bundles",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			resolved, issues, err := profile.Resolve(cfg.LibraryDir, cfg.ProfilesDir, ".", args[0])
			if err != nil {
				return exitcodes.Wrap(err, exitcodes.ExitData, exitcodes.KindValidation, "validate profile")
			}

			// Validate each linked skill bundle
			for _, rs := range resolved {
				if rs.Skill == "" {
					continue
				}
				skillPath := filepath.Join(cfg.LibraryDir, rs.Skill)
				bundle, err := skill.LoadBundle(skillPath)
				if err != nil {
					issues = append(issues, skill.Issue{
						Code:     "profile_invalid_bundle",
						Severity: "error",
						Message:  fmt.Sprintf("profile %q links skill %q: %v", args[0], rs.Skill, err),
						Path:     rs.Name,
					})
					continue
				}
				bundleIssues := skill.Validate(bundle)
				for _, bi := range bundleIssues {
					issues = append(issues, skill.Issue{
						Code:     bi.Code,
						Severity: bi.Severity,
						Message:  fmt.Sprintf("profile %q links skill %q: %s", args[0], rs.Skill, bi.Message),
						Path:     rs.Name,
					})
				}
			}

			result := map[string]any{"valid": len(issues) == 0, "issues": issues}
			if jsonOut {
				return printJSON(cmd, result)
			}
			if len(issues) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "valid")
				return nil
			}
			for _, issue := range issues {
				fmt.Fprintf(cmd.OutOrStdout(), "%s	%s	%s\n", issue.Severity, issue.Code, issue.Message)
			}
			return exitcodes.Wrap(fmt.Errorf("validation failed"), exitcodes.ExitData, exitcodes.KindValidation, "validate profile")
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON")
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
			for _, target := range render.DefaultTargets() {
				dir, err := install.TargetDir(target, install.Options{Scope: render.ScopeUser})
				if err != nil {
					return err
				}
				paths = append(paths, targetPath{Target: target, User: dir})
			}
			result := map[string]any{
				"config_path":  config.ConfigPath(),
				"config":       cfg,
				"targets":      paths,
				"profiles_dir": cfg.ProfilesDir,
				"project_dir":  ".",
				"log_path":     events.DefaultPath(),
			}
			if jsonOut {
				return printJSON(cmd, result)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "config: %s\nlibrary: %s\nrendered: %s\nprofiles: %s\nlog: %s\n", config.ConfigPath(), cfg.LibraryDir, cfg.RenderDir, cfg.ProfilesDir, events.DefaultPath())
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
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve symskills MCP tools over stdio",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			// stdout stays reserved for JSON-RPC frames; all diagnostics go
			// through slog to stderr. The operation log is a file whose
			// location is passed explicitly.
			return mcptools.Serve(version, mcptools.Options{LibraryDir: cfg.LibraryDir, RenderDir: cfg.RenderDir, ProfilesDir: cfg.ProfilesDir, EventsPath: events.DefaultPath(), Version: version})
		},
	}
	// stdio is the only transport, so it is always enabled. The flag is
	// kept as a no-op alias so existing MCP client configs that pass
	// --stdio keep working unchanged.
	cmd.Flags().Bool("stdio", true, "Serve over stdio (default; flag kept for backward compatibility)")
	return cmd
}

func targetsFromFlag(name string) ([]render.Target, error) {
	if name == "" || name == "all" {
		return render.DefaultTargets(), nil
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

func newTargetsCmd() *cobra.Command {
	var jsonOutput bool
	var scope string
	cmd := &cobra.Command{
		Use:   "targets",
		Short: "Display read-only inventory and readiness status for AI-agent harnesses",
		RunE: func(cmd *cobra.Command, _ []string) error {
			sc := render.ScopeUser
			if scope == string(render.ScopeProject) {
				sc = render.ScopeProject
			}
			statuses := harness.ListStatus(harness.Options{
				Scope: sc,
			})
			if jsonOutput {
				return printJSON(cmd, map[string]any{"targets": statuses})
			}
			for _, st := range statuses {
				fmt.Fprintf(cmd.OutOrStdout(), "Target: %s (%s)\n", st.DisplayName, st.Target)
				fmt.Fprintf(cmd.OutOrStdout(), "  Status:      %s (verification: %s, evidence: %s)\n", st.InstallState, st.VerificationStatus, st.Evidence)
				fmt.Fprintf(cmd.OutOrStdout(), "  Skill Root:  %s (exists: %t, readable: %t)\n", st.EffectiveSkillRoot, st.SkillRootExists, st.SkillRootReadable)
				fmt.Fprintf(cmd.OutOrStdout(), "  Skills:      %d managed, %d unmanaged\n", st.ManagedSkillsCount, st.UnmanagedSkillsCount)
				fmt.Fprintf(cmd.OutOrStdout(), "  Setup Hint:  %s\n\n", st.SetupHint)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output status as JSON")
	cmd.Flags().StringVar(&scope, "scope", "user", "Scope to check (user or project)")
	return cmd
}

func newDiscoverCmd() *cobra.Command {
	var jsonOutput bool
	var scope string
	var paths []string
	cmd := &cobra.Command{
		Use:   "discover [paths...]",
		Short: "Discover unmanaged skill sources in harness roots or explicit paths",
		RunE: func(cmd *cobra.Command, args []string) error {
			sc := render.ScopeUser
			if scope == string(render.ScopeProject) {
				sc = render.ScopeProject
			}
			allPaths := append(paths, args...)
			candidates, err := discover.DiscoverScanned(discover.Options{
				Scope: sc,
				Paths: allPaths,
			})
			if err != nil {
				return err
			}
			if jsonOutput {
				return printJSON(cmd, map[string]any{"schema_version": 1, "candidates": candidates})
			}
			if len(candidates) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No skill candidates discovered.")
				return nil
			}
			for _, c := range candidates {
				managedStr := "unmanaged"
				if c.Managed {
					managedStr = "managed"
				}
				validStr := "valid"
				if !c.Valid {
					validStr = "invalid"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "[%s] %s (%s, %s)\n", c.SourceID, c.DisplayName, managedStr, validStr)
				fmt.Fprintf(cmd.OutOrStdout(), "  Location: %s\n", c.Location)
				if c.Target != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "  Harness:  %s\n", c.Target)
				}
				if len(c.Diagnostics) > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "  Diagnostics: %s\n", strings.Join(c.Diagnostics, "; "))
				}
				fmt.Fprintln(cmd.OutOrStdout())
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output discovery results as JSON")
	cmd.Flags().StringVar(&scope, "scope", "user", "Scope to check (user or project)")
	cmd.Flags().StringSliceVar(&paths, "path", nil, "Explicit paths to scan for skills")
	return cmd
}

func newLogCmd() *cobra.Command {
	var skillName, targetName, since string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "log",
		Short: "Show the local operation history",
		Long: `Show the append-only operation log written for every skill-mutating
operation (import, render, install, uninstall, profile install, validate
failure). Records live at ~/.local/share/symskills/events.jsonl and rotate
to events.1.jsonl when the file grows too large; the log is local-only and
never transmitted anywhere.

Filters narrow the records shown; --json emits the raw JSON records in
chronological order.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var filter events.Filter
			filter.Skill = skillName
			filter.Target = targetName
			if since != "" {
				ts, err := time.Parse(time.RFC3339Nano, since)
				if err != nil {
					return exitcodes.Wrap(fmt.Errorf("invalid --since %q: want an RFC3339 timestamp like 2026-08-01T00:00:00Z", since), exitcodes.ExitData, exitcodes.KindValidation, "parse since")
				}
				filter.Since = ts
			}
			records, err := newEventLogger().Read(filter)
			if err != nil {
				return exitcodes.Wrap(err, exitcodes.ExitData, exitcodes.KindInternal, "read event log")
			}
			if jsonOut {
				if records == nil {
					records = []events.Event{}
				}
				return printJSON(cmd, records)
			}
			for _, ev := range records {
				fmt.Fprintf(cmd.OutOrStdout(), "%s	%s	%s	%s	%s	%s	%s	%s", ev.TS, ev.Event, ev.Skill, ev.Target, ev.Scope, ev.Mode, ev.Outcome, ev.Path)
				if ev.Error != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "	%s", ev.Error)
				}
				fmt.Fprintln(cmd.OutOrStdout())
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&skillName, "skill", "", "Only show records for this skill")
	cmd.Flags().StringVar(&targetName, "target", "", "Only show records for this harness target")
	cmd.Flags().StringVar(&since, "since", "", "Only show records at or after this RFC3339 timestamp")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit raw JSON records")
	return cmd
}
