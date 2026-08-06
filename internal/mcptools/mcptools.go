// Package mcptools exposes symskills workflows over MCP.
package mcptools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/danieljustus/symaira-corekit/exitcodes"
	"github.com/danieljustus/symaira-corekit/mcpserver"
	"github.com/danieljustus/symaira-skills/internal/config"
	"github.com/danieljustus/symaira-skills/internal/discover"
	"github.com/danieljustus/symaira-skills/internal/events"
	"github.com/danieljustus/symaira-skills/internal/harness"
	"github.com/danieljustus/symaira-skills/internal/install"
	"github.com/danieljustus/symaira-skills/internal/metadata"
	"github.com/danieljustus/symaira-skills/internal/profile"
	"github.com/danieljustus/symaira-skills/internal/render"
	"github.com/danieljustus/symaira-skills/internal/skill"
	"github.com/danieljustus/symaira-skills/internal/vcs"
)

const emptyObject = `{"type":"object","properties":{}}`

type Options struct {
	LibraryDir  string
	RenderDir   string
	ProfilesDir string
	HomeDir     string
	ProjectDir  string
	// Version is the symskills build version stamped into event log
	// records as tool_version.
	Version string
	// EventsPath overrides the operation-log location. When empty, the log
	// lives under HomeDir (or the default home when HomeDir is empty too).
	EventsPath string
	// VCSEnabled mirrors the vcs.enabled config toggle: skills_restore
	// refuses to write to the per-skill repositories while it is false.
	// Nil means enabled (the default), so bare Options{} in tests keeps
	// working.
	VCSEnabled *bool
}

// mcpJSON marshals v to JSON and returns it as a string suitable for
// TextContent.text. The MCP protocol requires the text field to be a
// JSON string, not a raw JSON object; this helper ensures structured
// results are always serialized to a JSON string before the mcpserver
// wire layer embeds them.
func mcpJSON(v any) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("serialize result: %w", err)
	}
	return string(data), nil
}

// skillListItem is one skills_list row: the frontmatter summary plus the
// per-skill metadata record (same snake_case fields as `symskills list
// --json`).
type skillListItem struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Root        string `json:"root"`
	metadata.Record
}

func Register(srv *mcpserver.Server, opts Options) {
	cfg := config.Defaults()
	if opts.LibraryDir == "" {
		opts.LibraryDir = cfg.LibraryDir
	}
	if opts.RenderDir == "" {
		opts.RenderDir = cfg.RenderDir
	}
	if opts.ProfilesDir == "" {
		opts.ProfilesDir = cfg.ProfilesDir
	}
	if opts.VCSEnabled == nil {
		enabled := cfg.VCSEnabled()
		opts.VCSEnabled = &enabled
	}
	// The operation log is a file, never console output: stdout stays
	// reserved for JSON-RPC frames while serving MCP (AGENTS.md). The
	// logger is only created when a log location is known; the CLI serve
	// command passes EventsPath explicitly, so a bare Options{} (as used
	// by tests) never writes anywhere.
	logPath := opts.EventsPath
	if logPath == "" && opts.HomeDir != "" {
		logPath = filepath.Join(opts.HomeDir, ".local", "share", "symskills", "events.jsonl")
	}
	var logger *events.Logger
	if logPath != "" {
		logger = events.New(logPath, opts.Version)
	}

	srv.RegisterTool(&mcpserver.Tool{
		Name:        "skills_list",
		Description: "List skills in the symskills library.",
		InputSchema: json.RawMessage(emptyObject),
		Handler: func(_ context.Context, _ json.RawMessage) (any, error) {
			bundles, issues := skill.ListLibrary(opts.LibraryDir)
			items := make([]skillListItem, 0, len(bundles))
			metaOpts := metadata.Options{
				LogPath:    logPath,
				InstallOpt: install.Options{HomeDir: opts.HomeDir, Scope: render.ScopeUser},
			}
			for _, bundle := range bundles {
				rec := metadata.Collect(bundle.Root, bundle.Frontmatter.Name, metaOpts)
				items = append(items, skillListItem{
					Name:        bundle.Frontmatter.Name,
					Description: bundle.Frontmatter.Description,
					Root:        bundle.Root,
					Record:      rec,
				})
			}
			return mcpJSON(map[string]any{"skills": items, "issues": issues})
		},
	})
	srv.RegisterTool(&mcpserver.Tool{
		Name:        "skills_inspect",
		Description: "Inspect one skill by path or library name.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"name":{"type":"string"}}}`),
		Handler: func(ctx context.Context, in json.RawMessage) (any, error) {
			bundle, err := callInspect(ctx, srv, opts, in)
			if err != nil {
				return nil, err
			}
			rec := metadata.Collect(bundle.Root, bundle.Frontmatter.Name, metadata.Options{
				LogPath:    logPath,
				InstallOpt: install.Options{HomeDir: opts.HomeDir, Scope: render.ScopeUser},
			})
			return mcpJSON(struct {
				*skill.Bundle
				metadata.Record
			}{bundle, rec})
		},
	})
	srv.RegisterTool(&mcpserver.Tool{
		Name:        "skills_validate",
		Description: "Validate one skill by path or library name.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"name":{"type":"string"}}}`),
		Handler: func(ctx context.Context, in json.RawMessage) (any, error) {
			result, err := callInspect(ctx, srv, opts, in)
			if err != nil {
				return nil, err
			}
			issues := skill.Validate(result)
			if len(issues) > 0 {
				messages := make([]string, 0, len(issues))
				for _, issue := range issues {
					messages = append(messages, issue.Message)
				}
				logger.Record(events.Event{
					Event:   events.EventValidateFailure,
					Skill:   result.Frontmatter.Name,
					Path:    result.Root,
					Outcome: events.OutcomeError,
					Error:   strings.Join(messages, "; "),
					Actor:   events.ActorMCP,
				})
			}
			return mcpJSON(map[string]any{"issues": issues})
		},
	})
	srv.RegisterTool(&mcpserver.Tool{
		Name:        "skills_profile_list",
		Description: "List available context profiles (global and project).",
		InputSchema: json.RawMessage(emptyObject),
		Handler: func(_ context.Context, _ json.RawMessage) (any, error) {
			refs, err := profile.List(opts.ProfilesDir, opts.ProjectDir)
			if err != nil {
				return nil, exitcodes.Wrap(err, exitcodes.ExitData, exitcodes.KindValidation, "list profiles")
			}
			return mcpJSON(map[string]any{"profiles": refs})
		},
	})
	srv.RegisterTool(&mcpserver.Tool{
		Name:        "skills_profile_resolve",
		Description: "Resolve a context profile and return the merged skill set.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`),
		Handler: func(_ context.Context, in json.RawMessage) (any, error) {
			var args struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal(in, &args); err != nil {
				return nil, exitcodes.Wrap(err, exitcodes.ExitData, exitcodes.KindValidation, "parse arguments")
			}
			resolved, issues, err := profile.Resolve(opts.LibraryDir, opts.ProfilesDir, opts.ProjectDir, args.Name)
			if err != nil {
				return nil, exitcodes.Wrap(err, exitcodes.ExitData, exitcodes.KindValidation, "resolve profile")
			}
			return mcpJSON(map[string]any{"skills": resolved, "issues": issues})
		},
	})
	srv.RegisterTool(&mcpserver.Tool{
		Name:        "skills_render_plan",
		Description: "Render a skill or profile to the managed artifact directory and return planned target paths. Pass dry_run=true to preview without writing.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"name":{"type":"string"},"target":{"type":"string"},"profile":{"type":"string"},"dry_run":{"type":"boolean"}}}`),
		Handler: func(ctx context.Context, in json.RawMessage) (any, error) {
			var args struct {
				Target  string `json:"target"`
				Profile string `json:"profile"`
				DryRun  *bool  `json:"dry_run"`
			}
			if err := json.Unmarshal(in, &args); err != nil {
				return nil, exitcodes.Wrap(err, exitcodes.ExitData, exitcodes.KindValidation, "parse arguments")
			}
			targets := render.DefaultTargets()
			if args.Target != "" {
				target, err := render.ParseTarget(args.Target)
				if err != nil {
					return nil, err
				}
				targets = []render.Target{target}
			}
			dryRun := false
			if args.DryRun != nil {
				dryRun = *args.DryRun
			}

			if args.Profile != "" {
				if dryRun {
					return renderProfileDryRun(opts, targets, args.Profile)
				}
				return renderProfile(opts, cfg, targets, args.Profile, dryRun, logger)
			}
			bundle, err := callInspect(ctx, srv, opts, in)
			if err != nil {
				return nil, err
			}

			if dryRun {
				// Plan only: no writes to render directory.
				planned := make([]render.Rendered, 0, len(targets))
				var errs []error
				for _, target := range targets {
					item, err := render.RenderTarget(bundle, target)
					if err != nil {
						errs = append(errs, fmt.Errorf("target %s: %w", target, err))
						continue
					}
					item.Path = filepath.Join(opts.RenderDir, string(target), item.Name)
					planned = append(planned, item)
				}
				if len(planned) == 0 && len(errs) > 0 {
					return nil, errs[0]
				}
				return mcpJSON(planned)
			}

			rendered, errs := render.RenderAll(bundle, opts.RenderDir, targets)
			if len(rendered) == 0 && len(errs) > 0 {
				logger.Record(events.Event{Event: events.EventRender, Skill: bundle.Frontmatter.Name, Target: args.Target, Outcome: events.OutcomeError, Error: errs[0].Error(), Actor: events.ActorMCP})
				return nil, errs[0]
			}
			targetLabel := args.Target
			if targetLabel == "" {
				targetLabel = "all"
			}
			logger.Record(events.Event{Event: events.EventRender, Skill: bundle.Frontmatter.Name, SkillVersion: bundle.Frontmatter.Version, Target: targetLabel, Path: opts.RenderDir, Outcome: events.OutcomeOK, Actor: events.ActorMCP})
			return mcpJSON(rendered)
		},
	})
	srv.RegisterTool(&mcpserver.Tool{
		Name:        "skills_install",
		Description: "Render and install a skill or profile. Dry-run defaults to true; pass dry_run=false for writes.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"name":{"type":"string"},"target":{"type":"string"},"scope":{"type":"string"},"dry_run":{"type":"boolean"},"profile":{"type":"string"}}}`),
		Handler: func(ctx context.Context, in json.RawMessage) (any, error) {
			var args struct {
				Target  string `json:"target"`
				Scope   string `json:"scope"`
				DryRun  *bool  `json:"dry_run"`
				Profile string `json:"profile"`
			}
			if err := json.Unmarshal(in, &args); err != nil {
				return nil, exitcodes.Wrap(err, exitcodes.ExitData, exitcodes.KindValidation, "parse arguments")
			}
			target := render.TargetOpenCode
			if args.Target != "" {
				parsed, err := render.ParseTarget(args.Target)
				if err != nil {
					return nil, err
				}
				target = parsed
			}
			scope := render.ScopeUser
			if args.Scope == string(render.ScopeProject) {
				scope = render.ScopeProject
			}
			dryRun := true
			if args.DryRun != nil {
				dryRun = *args.DryRun
			}
			installOpts := install.Options{HomeDir: opts.HomeDir, ProjectDir: opts.ProjectDir, Scope: scope, DryRun: dryRun}

			if args.Profile != "" {
				if dryRun {
					return installProfileDryRun(opts, target, args.Profile, installOpts)
				}
				return installProfile(opts, cfg, target, args.Profile, installOpts, logger)
			}
			bundle, err := callInspect(ctx, srv, opts, in)
			if err != nil {
				return nil, err
			}

			if dryRun {
				// Plan only: compute render target without writing to render dir.
				item, err := render.RenderTarget(bundle, target)
				if err != nil {
					return nil, err
				}
				item.Path = filepath.Join(opts.RenderDir, string(target), item.Name)
				dest, err := install.InstallPath(target, item.Name, installOpts)
				if err != nil {
					return nil, err
				}
				return mcpJSON(install.Result{
					Action: "planned",
					Target: target,
					Name:   item.Name,
					Path:   dest,
					Mode:   installOpts.Mode,
				})
			}

			rendered, errs := render.RenderAll(bundle, opts.RenderDir, []render.Target{target})
			if len(rendered) == 0 {
				if len(errs) > 0 {
					logger.Record(events.Event{Event: events.EventInstall, Skill: bundle.Frontmatter.Name, Target: string(target), Outcome: events.OutcomeError, Error: errs[0].Error(), Actor: events.ActorMCP})
					return nil, errs[0]
				}
				return nil, fmt.Errorf("target %s produced no render output", target)
			}
			result, err := install.Install(install.RenderedSkill{
				Target: target,
				Name:   rendered[0].Name,
				Path:   rendered[0].Path,
			}, installOpts)
			if err != nil {
				logger.Record(events.Event{Event: events.EventInstall, Skill: rendered[0].Name, Target: string(target), Outcome: events.OutcomeError, Error: err.Error(), Actor: events.ActorMCP})
				return nil, err
			}
			logger.Record(events.Event{Event: events.EventInstall, Skill: result.Name, Target: string(result.Target), Scope: string(installOpts.Scope), Mode: string(result.Mode), Path: result.Path, Outcome: events.OutcomeOK, Actor: events.ActorMCP})
			return mcpJSON(result)
		},
	})
	srv.RegisterTool(&mcpserver.Tool{
		Name:        "skills_targets_status",
		Description: "Read-only inventory and readiness status for supported AI-agent harnesses.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"scope":{"type":"string"}}}`),
		Handler: func(_ context.Context, in json.RawMessage) (any, error) {
			var args struct {
				Scope string `json:"scope"`
			}
			if err := json.Unmarshal(in, &args); err != nil {
				return nil, exitcodes.Wrap(err, exitcodes.ExitData, exitcodes.KindValidation, "parse arguments")
			}
			scope := render.ScopeUser
			if args.Scope == string(render.ScopeProject) {
				scope = render.ScopeProject
			}
			statuses := harness.ListStatus(harness.Options{
				HomeDir:    opts.HomeDir,
				ProjectDir: opts.ProjectDir,
				Scope:      scope,
			})
			return mcpJSON(map[string]any{"targets": statuses})
		},
	})
	srv.RegisterTool(&mcpserver.Tool{
		Name:        "skills_discover_sources",
		Description: "Discover unmanaged skill sources in known harness roots or explicit paths.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"paths":{"type":"array","items":{"type":"string"}},"scope":{"type":"string"}}}`),
		Handler: func(_ context.Context, in json.RawMessage) (any, error) {
			var args struct {
				Paths []string `json:"paths"`
				Scope string   `json:"scope"`
			}
			if err := json.Unmarshal(in, &args); err != nil {
				return nil, exitcodes.Wrap(err, exitcodes.ExitData, exitcodes.KindValidation, "parse arguments")
			}
			scope := render.ScopeUser
			if args.Scope == string(render.ScopeProject) {
				scope = render.ScopeProject
			}
			candidates, err := discover.DiscoverScanned(discover.Options{
				HomeDir:    opts.HomeDir,
				ProjectDir: opts.ProjectDir,
				Scope:      scope,
				Paths:      args.Paths,
			})
			if err != nil {
				return nil, exitcodes.Wrap(err, exitcodes.ExitData, exitcodes.KindValidation, "discover sources")
			}
			return mcpJSON(map[string]any{"candidates": candidates})
		},
	})
	srv.RegisterTool(&mcpserver.Tool{
		Name:        "skills_history",
		Description: "List the versioned commit history of a library skill: revision, timestamp, operation (import/update/restore/unknown) and changed files.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"},"limit":{"type":"integer"}},"required":["name"]}`),
		Handler: func(_ context.Context, in json.RawMessage) (any, error) {
			var args struct {
				Name  string `json:"name"`
				Limit int    `json:"limit"`
			}
			if err := json.Unmarshal(in, &args); err != nil {
				return nil, exitcodes.Wrap(err, exitcodes.ExitData, exitcodes.KindValidation, "parse arguments")
			}
			if args.Name == "" {
				return nil, exitcodes.Wrap(fmt.Errorf("name is required"), exitcodes.ExitData, exitcodes.KindValidation, "history")
			}
			limit := args.Limit
			if limit <= 0 {
				limit = 20
			}
			dir := filepath.Join(opts.LibraryDir, args.Name)
			if !vcs.IsRepo(dir) {
				return nil, exitcodes.Wrap(fmt.Errorf("skill %q is not versioned: no git repository at %s", args.Name, dir), exitcodes.ExitData, exitcodes.KindValidation, "history")
			}
			history, err := vcs.History(dir, limit)
			if err != nil {
				return nil, exitcodes.Wrap(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "read history")
			}
			if history == nil {
				history = []vcs.CommitInfo{}
			}
			return mcpJSON(map[string]any{"name": args.Name, "history": history})
		},
	})
	srv.RegisterTool(&mcpserver.Tool{
		Name:        "skills_restore",
		Description: "Roll a library skill's files back to a previous revision by forward commit. Dry-run defaults to true; pass dry_run=false to write. Refuses invalid restored states and never discards uncommitted changes (pass allow_dirty=true to snapshot them first); sync=true re-installs stale targets.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"},"rev":{"type":"string"},"dry_run":{"type":"boolean"},"allow_dirty":{"type":"boolean"},"sync":{"type":"boolean"}},"required":["name","rev"]}`),
		Handler: func(_ context.Context, in json.RawMessage) (any, error) {
			var args struct {
				Name       string `json:"name"`
				Rev        string `json:"rev"`
				DryRun     *bool  `json:"dry_run"`
				AllowDirty bool   `json:"allow_dirty"`
				Sync       bool   `json:"sync"`
			}
			if err := json.Unmarshal(in, &args); err != nil {
				return nil, exitcodes.Wrap(err, exitcodes.ExitData, exitcodes.KindValidation, "parse arguments")
			}
			if args.Name == "" || args.Rev == "" {
				return nil, exitcodes.Wrap(fmt.Errorf("name and rev are required"), exitcodes.ExitData, exitcodes.KindValidation, "restore skill")
			}
			dryRun := true
			if args.DryRun != nil {
				dryRun = *args.DryRun
			}
			dir := filepath.Join(opts.LibraryDir, args.Name)
			if !vcs.IsRepo(dir) {
				return nil, exitcodes.Wrap(fmt.Errorf("skill %q is not versioned: no git repository at %s", args.Name, dir), exitcodes.ExitData, exitcodes.KindValidation, "restore skill")
			}
			if opts.VCSEnabled != nil && !*opts.VCSEnabled {
				return nil, exitcodes.Wrap(fmt.Errorf("per-skill versioning is disabled (vcs.enabled = false); restore cannot write to %s", dir), exitcodes.ExitConfig, exitcodes.KindConfig, "restore skill")
			}
			resolved, err := vcs.Resolve(dir, args.Rev)
			if err != nil {
				return nil, exitcodes.Wrap(fmt.Errorf("revision %q not found: %v", args.Rev, err), exitcodes.ExitData, exitcodes.KindValidation, "restore skill")
			}
			dirty, err := vcs.Dirty(dir)
			if err != nil {
				return nil, exitcodes.Wrap(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "check working tree")
			}
			if dirty && !args.AllowDirty {
				return nil, exitcodes.Wrap(fmt.Errorf("skill %q has uncommitted changes; they are never discarded — pass allow_dirty=true to snapshot them into a pre-restore commit first", args.Name), exitcodes.ExitConflict, exitcodes.KindConflict, "restore skill")
			}
			// Materialize the target revision and validate it before any
			// write. The tree is extracted into <tmp>/<name> so the restored
			// frontmatter name is compared against the library directory name
			// by the normal validation rules.
			tmp, err := os.MkdirTemp("", "symskills-restore-*")
			if err != nil {
				return nil, exitcodes.Wrap(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "restore skill")
			}
			defer os.RemoveAll(tmp)
			restoredTree := filepath.Join(tmp, args.Name)
			if err := vcs.ExtractRev(dir, resolved, restoredTree); err != nil {
				return nil, exitcodes.Wrap(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "extract revision")
			}
			if err := validateRestoredMCP(resolved, restoredTree); err != nil {
				return nil, err
			}
			changed, err := vcs.ChangedFiles(dir, resolved)
			if err != nil {
				return nil, exitcodes.Wrap(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "compare revision")
			}
			result := map[string]any{
				"name":          args.Name,
				"rev":           resolved,
				"dry_run":       dryRun,
				"action":        "planned",
				"changed_files": changed,
			}
			if dirty {
				result["notes"] = []string{"uncommitted changes would be committed as a pre-restore snapshot"}
			}
			if dryRun {
				return mcpJSON(result)
			}
			if dirty {
				if _, err := vcs.Commit(dir, fmt.Sprintf("restore: snapshot uncommitted changes before restore to %s", resolved)); err != nil {
					return nil, exitcodes.Wrap(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "snapshot uncommitted changes")
				}
			}
			head, err := vcs.Restore(dir, restoredTree, fmt.Sprintf("restore: skill %s to %s", args.Name, resolved))
			if err != nil {
				return nil, exitcodes.Wrap(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "restore skill")
			}
			if head == "" {
				result["action"] = "already_at_rev"
			} else {
				result["action"] = "restored"
				result["commit"] = head
			}
			// Report installed copies the library change made stale (#115);
			// the scan never fails the restore, which already succeeded.
			statuses, err := install.Status(install.StatusOptions{
				HomeDir:    opts.HomeDir,
				ProjectDir: opts.ProjectDir,
				Scope:      render.ScopeUser,
				LibraryDir: opts.LibraryDir,
				Targets:    render.DefaultTargets(),
				Skills:     []string{args.Name},
			})
			if err != nil {
				return nil, exitcodes.Wrap(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "scan installed targets")
			}
			var staleTargets []map[string]string
			var stale []install.InstallStatus
			for _, st := range statuses {
				if st.Status != install.StatusStale {
					continue
				}
				stale = append(stale, st)
				staleTargets = append(staleTargets, map[string]string{"target": string(st.Target), "path": st.Path})
			}
			if len(staleTargets) > 0 {
				result["stale_targets"] = staleTargets
			}
			if args.Sync && len(stale) > 0 {
				result["sync_results"] = resyncStaleMCP(opts, stale, logger)
			}
			return mcpJSON(result)
		},
	})
}

// validateRestoredMCP loads and validates the extracted tree, refusing
// the restore (naming every validation error) when the state is not a
// valid skill. The caller writes nothing on error.
func validateRestoredMCP(rev, restoredTree string) error {
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

// resyncStaleMCP re-installs the given stale installs, mirroring the
// reinstall path of `symskills sync` (the #115 resync surface).
func resyncStaleMCP(opts Options, stale []install.InstallStatus, logger *events.Logger) []map[string]any {
	results := []map[string]any{}
	for _, st := range stale {
		row := map[string]any{"target": string(st.Target), "name": st.Name}
		bundle, err := skill.LoadBundle(filepath.Join(opts.LibraryDir, st.Name))
		if err != nil {
			row["action"] = "failed"
			row["error"] = err.Error()
			results = append(results, row)
			continue
		}
		installOpts := install.Options{
			Scope:           render.ScopeUser,
			Mode:            st.Mode,
			AllowExecutable: bundle.Manifest.Skill.AllowExecutable,
		}
		rendered, errs := render.RenderAll(bundle, opts.RenderDir, []render.Target{st.Target})
		if len(rendered) == 0 {
			msg := fmt.Sprintf("target %s produced no render output", st.Target)
			if len(errs) > 0 {
				msg = errs[0].Error()
			}
			logger.Record(events.Event{Event: events.EventInstall, Skill: st.Name, Target: string(st.Target), Outcome: events.OutcomeError, Error: msg, Actor: events.ActorMCP})
			row["action"] = "failed"
			row["error"] = msg
			results = append(results, row)
			continue
		}
		lock, lockErr := install.AcquirePullLock(st.Target, rendered[0].Name, install.PullOptions{HomeDir: opts.HomeDir})
		if lockErr != nil {
			row["action"] = "skipped"
			row["error"] = lockErr.Error()
			results = append(results, row)
			continue
		}
		result, err := install.Install(install.RenderedSkill{Target: st.Target, Name: rendered[0].Name, Path: rendered[0].Path}, installOpts)
		_ = lock.Release()
		if err != nil {
			logger.Record(events.Event{Event: events.EventInstall, Skill: st.Name, Target: string(st.Target), Outcome: events.OutcomeError, Error: err.Error(), Actor: events.ActorMCP})
			row["action"] = "failed"
			row["error"] = err.Error()
			results = append(results, row)
			continue
		}
		logger.Record(events.Event{Event: events.EventInstall, Skill: result.Name, SkillVersion: bundle.Frontmatter.Version, Target: string(st.Target), Scope: string(installOpts.Scope), Mode: string(result.Mode), Path: result.Path, Outcome: events.OutcomeOK, Actor: events.ActorMCP})
		row["action"] = result.Action
		row["path"] = result.Path
		results = append(results, row)
	}
	return results
}

func callInspect(_ context.Context, _ *mcpserver.Server, opts Options, in json.RawMessage) (*skill.Bundle, error) {
	var args struct {
		Path string `json:"path"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(in, &args); err != nil {
		return nil, exitcodes.Wrap(err, exitcodes.ExitData, exitcodes.KindValidation, "parse arguments")
	}
	root := args.Path
	if root == "" && args.Name != "" {
		root = filepath.Join(opts.LibraryDir, args.Name)
	}
	if root == "" {
		return nil, exitcodes.Wrap(fmt.Errorf("path or name is required"), exitcodes.ExitData, exitcodes.KindValidation, "inspect skill")
	}
	return skill.LoadBundle(root)
}

func renderProfile(opts Options, cfg *config.Config, targets []render.Target, profileName string, dryRun bool, logger *events.Logger) (any, error) {
	if dryRun {
		return renderProfileDryRun(opts, targets, profileName)
	}
	results, issues, err := profile.RenderProfile(opts.LibraryDir, opts.ProfilesDir, opts.ProjectDir, opts.RenderDir, targets, profileName)
	if err != nil {
		return nil, exitcodes.Wrap(err, exitcodes.ExitData, exitcodes.KindValidation, "resolve profile")
	}
	if len(issues) > 0 {
		messages := make([]string, 0, len(issues))
		for _, issue := range issues {
			messages = append(messages, issue.Message)
		}
		logger.Record(events.Event{Event: events.EventRender, Outcome: events.OutcomeError, Error: strings.Join(messages, "; "), Actor: events.ActorMCP})
		return mcpJSON(map[string]any{"skills": []render.Rendered{}, "issues": issues})
	}
	for _, result := range results {
		logger.Record(events.Event{Event: events.EventRender, Skill: result.Name, SkillVersion: result.Frontmatter.Version, Target: string(result.Target), Path: result.Path, Outcome: events.OutcomeOK, Actor: events.ActorMCP})
	}
	return mcpJSON(results)
}

// renderProfileDryRun plans profile rendering without writing to disk.
func renderProfileDryRun(opts Options, targets []render.Target, profileName string) (any, error) {
	resolved, issues, err := profile.Resolve(opts.LibraryDir, opts.ProfilesDir, opts.ProjectDir, profileName)
	if err != nil {
		return nil, exitcodes.Wrap(err, exitcodes.ExitData, exitcodes.KindValidation, "resolve profile")
	}
	if len(issues) > 0 {
		return mcpJSON(map[string]any{"skills": []render.Rendered{}, "issues": issues})
	}
	var planned []render.Rendered
	for _, rs := range resolved {
		bundle, err := skill.LoadBundle(filepath.Join(opts.LibraryDir, rs.Skill))
		if err != nil {
			return nil, fmt.Errorf("profile link %q: %w", rs.Name, err)
		}
		for _, target := range targets {
			item, err := render.RenderTarget(bundle, target, render.RenderMeta{Source: rs.Source, Profile: rs.Profile})
			if err != nil {
				continue // skip targets this skill doesn't support
			}
			item.Path = filepath.Join(opts.RenderDir, string(target), item.Name)
			planned = append(planned, item)
		}
	}
	return mcpJSON(planned)
}

func installProfile(opts Options, cfg *config.Config, target render.Target, profileName string, installOpts install.Options, logger *events.Logger) (any, error) {
	results, issues, err := profile.InstallProfile(opts.LibraryDir, opts.ProfilesDir, opts.ProjectDir, opts.RenderDir, target, profileName, installOpts)
	if err != nil {
		logger.Record(events.Event{Event: events.EventProfileInstall, Target: string(target), Outcome: events.OutcomeError, Error: err.Error(), Actor: events.ActorMCP})
		return nil, exitcodes.Wrap(err, exitcodes.ExitData, exitcodes.KindValidation, "resolve profile")
	}
	if len(issues) > 0 {
		messages := make([]string, 0, len(issues))
		for _, issue := range issues {
			messages = append(messages, issue.Message)
		}
		logger.Record(events.Event{Event: events.EventProfileInstall, Target: string(target), Outcome: events.OutcomeError, Error: strings.Join(messages, "; "), Actor: events.ActorMCP})
		return mcpJSON(map[string]any{"results": []install.Result{}, "issues": issues})
	}
	for _, result := range results {
		logger.Record(events.Event{Event: events.EventProfileInstall, Skill: result.Name, Target: string(result.Target), Scope: string(installOpts.Scope), Mode: string(result.Mode), Path: result.Path, Outcome: events.OutcomeOK, Actor: events.ActorMCP})
	}
	return mcpJSON(results)
}

// installProfileDryRun plans profile install without writing to render or install directories.
func installProfileDryRun(opts Options, target render.Target, profileName string, installOpts install.Options) (any, error) {
	resolved, issues, err := profile.Resolve(opts.LibraryDir, opts.ProfilesDir, opts.ProjectDir, profileName)
	if err != nil {
		return nil, exitcodes.Wrap(err, exitcodes.ExitData, exitcodes.KindValidation, "resolve profile")
	}
	if len(issues) > 0 {
		return mcpJSON(map[string]any{"results": []install.Result{}, "issues": issues})
	}
	var results []install.Result
	for _, rs := range resolved {
		bundle, err := skill.LoadBundle(filepath.Join(opts.LibraryDir, rs.Skill))
		if err != nil {
			return nil, fmt.Errorf("profile link %q: %w", rs.Name, err)
		}
		item, err := render.RenderTarget(bundle, target, render.RenderMeta{Source: rs.Source, Profile: rs.Profile})
		if err != nil {
			continue
		}
		item.Path = filepath.Join(opts.RenderDir, string(target), item.Name)
		dest, err := install.InstallPath(target, item.Name, installOpts)
		if err != nil {
			return nil, err
		}
		results = append(results, install.Result{
			Action: "planned",
			Target: target,
			Name:   item.Name,
			Path:   dest,
			Mode:   installOpts.Mode,
		})
	}
	return mcpJSON(results)
}

func Serve(version string, opts Options) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	srv := mcpserver.New("symskills", version)
	Register(srv, opts)
	return srv.ServeStdio(ctx)
}
