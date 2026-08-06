package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/danieljustus/symaira-corekit/exitcodes"
	"github.com/spf13/cobra"

	"github.com/danieljustus/symaira-skills/internal/config"
	"github.com/danieljustus/symaira-skills/internal/events"
	"github.com/danieljustus/symaira-skills/internal/install"
	"github.com/danieljustus/symaira-skills/internal/render"
	"github.com/danieljustus/symaira-skills/internal/skill"
	"github.com/danieljustus/symaira-skills/internal/vcs"
)

// skillDisplayName returns the frontmatter name of the skill at dir when
// it loads, falling back to the directory base name.
func skillDisplayName(dir string) string {
	if b, err := skill.LoadBundle(dir); err == nil && b.Frontmatter.Name != "" {
		return b.Frontmatter.Name
	}
	return filepath.Base(dir)
}

func newHistoryCmd() *cobra.Command {
	var limit int
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "history <skill>",
		Short: "List the versioned commit history of a library skill",
		Long: `List the commits of a skill's per-skill git repository (#118), newest
first: revision, timestamp, the operation that produced the commit (import,
update, restore; "unknown" for hand-made commits) and the files it changed.

The skill must be versioned (imported with per-skill versioning enabled and
git available). Read-only: history never modifies the repository.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if limit < 1 {
				return exitcodes.Wrap(fmt.Errorf("invalid --limit %d: must be at least 1", limit), exitcodes.ExitData, exitcodes.KindValidation, "history")
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			dir, err := resolveSkillDir(args, "skill is required", cfg.LibraryDir)
			if err != nil {
				return exitcodes.Wrap(err, exitcodes.ExitData, exitcodes.KindValidation, "history")
			}
			name := skillDisplayName(dir)
			if !vcs.IsRepo(dir) {
				return exitcodes.Wrap(fmt.Errorf("skill %q is not versioned: no git repository at %s (import it with versioning enabled)", name, dir), exitcodes.ExitData, exitcodes.KindValidation, "history")
			}
			history, err := vcs.History(dir, limit)
			if err != nil {
				return exitcodes.Wrap(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "read history")
			}
			if jsonOut {
				return printJSON(cmd, map[string]any{"name": name, "history": history})
			}
			if len(history) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "%s: no commits yet\n", name)
				return nil
			}
			for _, entry := range history {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\n", entry.Hash, entry.Timestamp, entry.Operation, strings.Join(entry.Files, ", "))
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 20, "Maximum number of commits to show")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON")
	return cmd
}

func newShowCmd() *cobra.Command {
	var rev string
	var diff bool
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "show <skill> --rev <rev>",
		Short: "Show a skill's state at a past revision",
		Long: `Print the skill's SKILL.md and resource tree at a revision of its
per-skill git repository, exactly as it was committed. With --diff, also
print a unified diff of that revision against the current working tree.

Read-only: show never modifies the repository or the skill files.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if rev == "" {
				return exitcodes.Wrap(fmt.Errorf("--rev is required"), exitcodes.ExitData, exitcodes.KindValidation, "show skill")
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			dir, err := resolveSkillDir(args, "skill is required", cfg.LibraryDir)
			if err != nil {
				return exitcodes.Wrap(err, exitcodes.ExitData, exitcodes.KindValidation, "show skill")
			}
			name := skillDisplayName(dir)
			if !vcs.IsRepo(dir) {
				return exitcodes.Wrap(fmt.Errorf("skill %q is not versioned: no git repository at %s", name, dir), exitcodes.ExitData, exitcodes.KindValidation, "show skill")
			}
			resolved, err := vcs.Resolve(dir, rev)
			if err != nil {
				return exitcodes.Wrap(fmt.Errorf("revision %q not found: %v", rev, err), exitcodes.ExitData, exitcodes.KindValidation, "show skill")
			}
			skillMD, err := vcs.ShowFile(dir, resolved, "SKILL.md")
			if err != nil {
				return exitcodes.Wrap(err, exitcodes.ExitData, exitcodes.KindValidation, "read SKILL.md at revision")
			}
			files, err := vcs.TreeFiles(dir, resolved)
			if err != nil {
				return exitcodes.Wrap(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "list tree at revision")
			}
			diffText := ""
			if diff {
				if diffText, err = vcs.Diff(dir, resolved); err != nil {
					return exitcodes.Wrap(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "diff against current state")
				}
			}
			if jsonOut {
				return printJSON(cmd, map[string]any{"name": name, "rev": resolved, "skill_md": skillMD, "files": files, "diff": diffText})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s at %s\n\n", name, resolved)
			fmt.Fprint(cmd.OutOrStdout(), skillMD)
			if !strings.HasSuffix(skillMD, "\n") {
				fmt.Fprintln(cmd.OutOrStdout())
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\nFiles at %s:\n", resolved)
			if len(files) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "(none)")
			}
			for _, f := range files {
				fmt.Fprintln(cmd.OutOrStdout(), f)
			}
			if diffText != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "\nDiff against current state:\n%s", diffText)
				if !strings.HasSuffix(diffText, "\n") {
					fmt.Fprintln(cmd.OutOrStdout())
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&rev, "rev", "", "Revision to show (hash, prefix, HEAD, HEAD~1, ...)")
	cmd.Flags().BoolVar(&diff, "diff", false, "Also print a unified diff against the current state")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON")
	return cmd
}

// staleTarget is one install the restore made stale, pointing at the
// #115 resync surface.
type staleTarget struct {
	Target render.Target `json:"target"`
	Path   string        `json:"path"`
}

// restoreOutcome is the `symskills restore` result (also used for the
// dry-run plan).
type restoreOutcome struct {
	Name   string `json:"name"`
	Rev    string `json:"rev"`
	DryRun bool   `json:"dry_run"`
	// Action is planned (dry-run), restored or already_at_rev.
	Action string `json:"action"`
	// Commit is the full hash of the forward restore commit; empty when
	// dry-running or when the skill was already at the revision.
	Commit string `json:"commit,omitempty"`
	// ChangedFiles lists the paths that differ between the revision and
	// the current working tree (the dry-run plan).
	ChangedFiles []string `json:"changed_files,omitempty"`
	// Notes carry plan details such as a pending dirty snapshot.
	Notes []string `json:"notes,omitempty"`
	// StaleTargets lists installed copies the library change made stale,
	// with the resync path from #115.
	StaleTargets []staleTarget `json:"stale_targets,omitempty"`
}

func newRestoreCmd() *cobra.Command {
	var to, scopeName string
	var dryRun, allowDirty, syncAfter, jsonOut bool
	cmd := &cobra.Command{
		Use:   "restore <skill> --to <rev>",
		Short: "Roll a library skill's files back to a previous revision",
		Long: `Restore the skill's files to the state of a past revision of its
per-skill git repository. The undo is a forward commit (git add -A +
commit), never a reset or history rewrite: the intermediate history stays
intact and the restore itself becomes the newest commit, so it can be
undone like any other change.

The restored state is validated before anything is written: restoring to a
revision whose SKILL.md is invalid is refused, naming the validation
errors.

Safety:
- Uncommitted local changes are never discarded: a dirty skill is refused
  unless --allow-dirty commits them first as a pre-restore snapshot.
- --dry-run prints the full plan (revision, validation result, files that
  would change, uncommitted changes that would be snapshotted) without
  writing anything.

After a restore the installed copies of the skill are stale relative to
the library. The command reports every stale target and points at
'symskills sync'; --sync re-installs them immediately.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if to == "" {
				return exitcodes.Wrap(fmt.Errorf("--to is required"), exitcodes.ExitData, exitcodes.KindValidation, "restore skill")
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			dir, err := resolveSkillDir(args, "skill is required", cfg.LibraryDir)
			if err != nil {
				return exitcodes.Wrap(err, exitcodes.ExitData, exitcodes.KindValidation, "restore skill")
			}
			name := skillDisplayName(dir)
			if !vcs.IsRepo(dir) {
				return exitcodes.Wrap(fmt.Errorf("skill %q is not versioned: no git repository at %s (restore needs a per-skill repository)", name, dir), exitcodes.ExitData, exitcodes.KindValidation, "restore skill")
			}
			resolved, err := vcs.Resolve(dir, to)
			if err != nil {
				return exitcodes.Wrap(fmt.Errorf("revision %q not found: %v", to, err), exitcodes.ExitData, exitcodes.KindValidation, "restore skill")
			}
			// A restore writes to the repository; refuse while versioning
			// is disabled rather than silently violating the toggle.
			if !cfg.VCSEnabled() {
				return exitcodes.Wrap(fmt.Errorf("per-skill versioning is disabled (vcs.enabled = false); restore cannot write to %s", dir), exitcodes.ExitConfig, exitcodes.KindConfig, "restore skill")
			}
			dirty, err := vcs.Dirty(dir)
			if err != nil {
				return exitcodes.Wrap(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "check working tree")
			}
			outcome := restoreOutcome{Name: name, Rev: resolved, DryRun: dryRun}
			if dirty && !allowDirty {
				return exitcodes.Wrap(fmt.Errorf("skill %q has uncommitted changes; they are never discarded — commit them first or pass --allow-dirty to snapshot them into a pre-restore commit", name), exitcodes.ExitConflict, exitcodes.KindConflict, "restore skill")
			}
			if dirty {
				outcome.Notes = append(outcome.Notes, "uncommitted changes would be committed as a pre-restore snapshot")
			}
			// Materialize the target revision and validate it before any
			// write. The tree is extracted into <tmp>/<name> so the
			// restored frontmatter name is compared against the library
			// directory name by the normal validation rules.
			tmp, err := os.MkdirTemp("", "symskills-restore-*")
			if err != nil {
				return exitcodes.Wrap(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "restore skill")
			}
			defer os.RemoveAll(tmp)
			restoredTree := filepath.Join(tmp, name)
			if err := vcs.ExtractRev(dir, resolved, restoredTree); err != nil {
				return exitcodes.Wrap(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "extract revision")
			}
			if err := validateRestoredState(resolved, restoredTree); err != nil {
				return err
			}
			if outcome.ChangedFiles, err = vcs.ChangedFiles(dir, resolved); err != nil {
				return exitcodes.Wrap(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "compare revision")
			}
			if dryRun {
				outcome.Action = "planned"
				if jsonOut {
					return printJSON(cmd, outcome)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "would restore %s to %s (validation: ok)\n", name, resolved)
				for _, f := range outcome.ChangedFiles {
					fmt.Fprintf(cmd.OutOrStdout(), "  would change %s\n", f)
				}
				for _, note := range outcome.Notes {
					fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", note)
				}
				return nil
			}
			if dirty {
				if _, err := vcs.Commit(dir, fmt.Sprintf("restore: snapshot uncommitted changes before restore to %s", resolved)); err != nil {
					return exitcodes.Wrap(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "snapshot uncommitted changes")
				}
			}
			head, err := vcs.Restore(dir, restoredTree, fmt.Sprintf("restore: skill %s to %s", name, resolved))
			if err != nil {
				return exitcodes.Wrap(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "restore skill")
			}
			if head == "" {
				outcome.Action = "already_at_rev"
			} else {
				outcome.Action = "restored"
				outcome.Commit = head
			}
			// Report installed copies the library change made stale (#115);
			// the scan never fails the restore, which already succeeded.
			stale, scanErr := staleInstalls(cfg, name, render.Scope(scopeName))
			if scanErr != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not scan installed targets: %v\n", scanErr)
			}
			for _, st := range stale {
				outcome.StaleTargets = append(outcome.StaleTargets, staleTarget{Target: st.Target, Path: st.Path})
			}
			if jsonOut {
				return printJSON(cmd, outcome)
			}
			if outcome.Action == "restored" {
				fmt.Fprintf(cmd.OutOrStdout(), "restored %s to %s (commit %s)\n", name, resolved, shortHead(outcome.Commit))
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "%s is already at %s\n", name, resolved)
			}
			for _, st := range outcome.StaleTargets {
				fmt.Fprintf(cmd.OutOrStdout(), "stale: %s at %s — run 'symskills sync --skill %s' to resync (or --sync)\n", st.Target, st.Path, name)
			}
			if syncAfter && len(outcome.StaleTargets) > 0 {
				for _, r := range resyncStaleTargets(cmd, cfg, name, stale, render.Scope(scopeName), newEventLogger()) {
					if r.Error != "" {
						fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s/%s: %s\n", r.Target, r.Name, r.Error)
						continue
					}
					fmt.Fprintf(cmd.OutOrStdout(), "synced %s: %s at %s\n", r.Target, r.Action, r.Path)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&to, "to", "", "Revision to restore to (hash, prefix, HEAD~1, ...)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print the restore plan without writing anything")
	cmd.Flags().BoolVar(&allowDirty, "allow-dirty", false, "Commit uncommitted changes as a pre-restore snapshot instead of refusing (never discards them)")
	cmd.Flags().BoolVar(&syncAfter, "sync", false, "Re-install every stale target after the restore")
	cmd.Flags().StringVar(&scopeName, "scope", string(render.ScopeUser), "Install scope for the stale-target scan: user or project")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON")
	return cmd
}

// validateRestoredState loads and validates the extracted tree at
// restoredTree, refusing the restore (naming every validation error) when
// the state is not a valid skill. The caller writes nothing on error.
func validateRestoredState(rev, restoredTree string) error {
	bundle, err := skill.LoadBundle(restoredTree)
	if err != nil {
		return exitcodes.Wrap(fmt.Errorf("refusing restore to %s: restored state is not a valid skill: %v", rev, err), exitcodes.ExitData, exitcodes.KindValidation, "restore skill")
	}
	var errors []string
	for _, issue := range skill.Validate(bundle) {
		if issue.Severity == "error" {
			errors = append(errors, issue.Message)
		}
	}
	if len(errors) > 0 {
		return exitcodes.Wrap(fmt.Errorf("refusing restore to %s: restored state fails validation: %s", rev, strings.Join(errors, "; ")), exitcodes.ExitData, exitcodes.KindValidation, "restore skill")
	}
	return nil
}

// staleInstalls scans every target for installs of name whose content no
// longer matches the library source (the #115 drift report).
func staleInstalls(cfg *config.Config, name string, scope render.Scope) ([]install.InstallStatus, error) {
	statuses, err := install.Status(install.StatusOptions{
		HomeDir:    userHomeDir(),
		Scope:      scope,
		LibraryDir: cfg.LibraryDir,
		BaseDir:    cfg.BaseDir,
		Targets:    render.DefaultTargets(),
		Skills:     []string{name},
	})
	if err != nil {
		return nil, err
	}
	stale := []install.InstallStatus{}
	for _, st := range statuses {
		if st.Status == install.StatusStale {
			stale = append(stale, st)
		}
	}
	return stale, nil
}

// resyncStaleTargets re-installs the given stale installs, mirroring the
// reinstall path of `symskills sync` (the #115 resync surface) for the
// listed entries. Returns one row per target.
func resyncStaleTargets(cmd *cobra.Command, cfg *config.Config, name string, stale []install.InstallStatus, scope render.Scope, logger *events.Logger) []syncResult {
	results := []syncResult{}
	for _, st := range stale {
		bundle, err := skill.LoadBundle(filepath.Join(cfg.LibraryDir, st.Name))
		if err != nil {
			results = append(results, syncResult{Target: st.Target, Name: st.Name, Path: st.Path, Action: "failed", Error: err.Error()})
			continue
		}
		opts := install.Options{
			Scope:           scope,
			Mode:            st.Mode,
			BaseDir:         cfg.BaseDir,
			AllowExecutable: bundle.Manifest.Skill.AllowExecutable,
		}
		rendered, errs := render.RenderAll(bundle, cfg.RenderDir, []render.Target{st.Target})
		if len(rendered) == 0 {
			msg := fmt.Sprintf("target %s produced no render output", st.Target)
			if len(errs) > 0 {
				msg = errs[0].Error()
			}
			logger.Record(events.Event{Event: events.EventInstall, Skill: st.Name, Target: string(st.Target), Outcome: events.OutcomeError, Error: msg, Actor: events.ActorCLI})
			results = append(results, syncResult{Target: st.Target, Name: st.Name, Path: st.Path, Action: "failed", Error: msg})
			continue
		}
		lock, lockErr := install.AcquirePullLock(st.Target, rendered[0].Name, install.PullOptions{HomeDir: userHomeDir()})
		if lockErr != nil {
			results = append(results, syncResult{Target: st.Target, Name: st.Name, Path: st.Path, Action: "skipped", Error: lockErr.Error()})
			continue
		}
		result, err := install.Install(install.RenderedSkill{Target: st.Target, Name: rendered[0].Name, Path: rendered[0].Path}, opts)
		_ = lock.Release()
		if err != nil {
			logger.Record(events.Event{Event: events.EventInstall, Skill: st.Name, Target: string(st.Target), Outcome: events.OutcomeError, Error: err.Error(), Actor: events.ActorCLI})
			results = append(results, syncResult{Target: st.Target, Name: st.Name, Path: st.Path, Action: "failed", Error: err.Error()})
			continue
		}
		logger.Record(events.Event{Event: events.EventInstall, Skill: result.Name, SkillVersion: bundle.Frontmatter.Version, Target: string(st.Target), Scope: string(opts.Scope), Mode: string(result.Mode), Path: result.Path, Outcome: events.OutcomeOK, Actor: events.ActorCLI})
		results = append(results, syncResult{Target: st.Target, Name: st.Name, Path: result.Path, Action: result.Action, Mode: result.Mode})
	}
	return results
}

// shortHead renders the first 8 characters of a commit hash.
func shortHead(hash string) string {
	if len(hash) > 8 {
		return hash[:8]
	}
	return hash
}
