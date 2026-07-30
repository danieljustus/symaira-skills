// Package mcptools exposes symskills workflows over MCP.
package mcptools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/danieljustus/symaira-corekit/exitcodes"
	"github.com/danieljustus/symaira-corekit/mcpserver"
	"github.com/danieljustus/symaira-skills/internal/config"
	"github.com/danieljustus/symaira-skills/internal/discover"
	"github.com/danieljustus/symaira-skills/internal/harness"
	"github.com/danieljustus/symaira-skills/internal/install"
	"github.com/danieljustus/symaira-skills/internal/profile"
	"github.com/danieljustus/symaira-skills/internal/render"
	"github.com/danieljustus/symaira-skills/internal/skill"
)

const emptyObject = `{"type":"object","properties":{}}`

type Options struct {
	LibraryDir  string
	RenderDir   string
	ProfilesDir string
	HomeDir     string
	ProjectDir  string
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

	srv.RegisterTool(&mcpserver.Tool{
		Name:        "skills_list",
		Description: "List skills in the symskills library.",
		InputSchema: json.RawMessage(emptyObject),
		Handler: func(_ context.Context, _ json.RawMessage) (any, error) {
			bundles, issues := skill.ListLibrary(opts.LibraryDir)
			items := make([]map[string]any, 0, len(bundles))
			for _, bundle := range bundles {
				items = append(items, map[string]any{
					"name":        bundle.Frontmatter.Name,
					"description": bundle.Frontmatter.Description,
					"root":        bundle.Root,
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
			return mcpJSON(bundle)
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
			return mcpJSON(map[string]any{"issues": skill.Validate(result)})
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
			targets := render.DefaultTargets
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
				return renderProfile(opts, cfg, targets, args.Profile, dryRun)
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
				return nil, errs[0]
			}
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
			scope := install.ScopeUser
			if args.Scope == string(install.ScopeProject) {
				scope = install.ScopeProject
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
				return installProfile(opts, cfg, target, args.Profile, installOpts)
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
				return nil, err
			}
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
			scope := install.ScopeUser
			if args.Scope == string(install.ScopeProject) {
				scope = install.ScopeProject
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
			scope := install.ScopeUser
			if args.Scope == string(install.ScopeProject) {
				scope = install.ScopeProject
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

func renderProfile(opts Options, cfg *config.Config, targets []render.Target, profileName string, dryRun bool) (any, error) {
	if dryRun {
		return renderProfileDryRun(opts, targets, profileName)
	}
	results, issues, err := profile.RenderProfile(opts.LibraryDir, opts.ProfilesDir, opts.ProjectDir, opts.RenderDir, targets, profileName)
	if err != nil {
		return nil, exitcodes.Wrap(err, exitcodes.ExitData, exitcodes.KindValidation, "resolve profile")
	}
	if len(issues) > 0 {
		return mcpJSON(map[string]any{"skills": []render.Rendered{}, "issues": issues})
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

func installProfile(opts Options, cfg *config.Config, target render.Target, profileName string, installOpts install.Options) (any, error) {
	results, issues, err := profile.InstallProfile(opts.LibraryDir, opts.ProfilesDir, opts.ProjectDir, opts.RenderDir, target, profileName, installOpts)
	if err != nil {
		return nil, exitcodes.Wrap(err, exitcodes.ExitData, exitcodes.KindValidation, "resolve profile")
	}
	if len(issues) > 0 {
		return mcpJSON(map[string]any{"results": []install.Result{}, "issues": issues})
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
