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
			return map[string]any{"skills": items, "issues": issues}, nil
		},
	})
	srv.RegisterTool(&mcpserver.Tool{
		Name:        "skills_inspect",
		Description: "Inspect one skill by path or library name.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"name":{"type":"string"}}}`),
		Handler: func(ctx context.Context, in json.RawMessage) (any, error) {
			return callInspect(ctx, srv, opts, in)
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
			return map[string]any{"issues": skill.Validate(result)}, nil
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
			return map[string]any{"profiles": refs}, nil
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
			return map[string]any{"skills": resolved, "issues": issues}, nil
		},
	})
	srv.RegisterTool(&mcpserver.Tool{
		Name:        "skills_render_plan",
		Description: "Render a skill or profile to the managed artifact directory and return planned target paths.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"name":{"type":"string"},"target":{"type":"string"},"profile":{"type":"string"}}}`),
		Handler: func(ctx context.Context, in json.RawMessage) (any, error) {
			var args struct {
				Target  string `json:"target"`
				Profile string `json:"profile"`
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
			if args.Profile != "" {
				return renderProfile(opts, cfg, targets, args.Profile)
			}
			bundle, err := callInspect(ctx, srv, opts, in)
			if err != nil {
				return nil, err
			}
			rendered, errs := render.RenderAll(bundle, opts.RenderDir, targets)
			if len(rendered) == 0 && len(errs) > 0 {
				return nil, errs[0]
			}
			return rendered, nil
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
				return installProfile(opts, cfg, target, args.Profile, installOpts)
			}
			bundle, err := callInspect(ctx, srv, opts, in)
			if err != nil {
				return nil, err
			}
			rendered, errs := render.RenderAll(bundle, opts.RenderDir, []render.Target{target})
			if len(rendered) == 0 {
				if len(errs) > 0 {
					return nil, errs[0]
				}
				return nil, fmt.Errorf("target %s produced no render output", target)
			}
			return install.Install(install.RenderedSkill{
				Target: target,
				Name:   rendered[0].Name,
				Path:   rendered[0].Path,
			}, installOpts)
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

func renderProfile(opts Options, cfg *config.Config, targets []render.Target, profileName string) (any, error) {
	results, issues, err := profile.RenderProfile(opts.LibraryDir, opts.ProfilesDir, opts.ProjectDir, opts.RenderDir, targets, profileName)
	if err != nil {
		return nil, exitcodes.Wrap(err, exitcodes.ExitData, exitcodes.KindValidation, "resolve profile")
	}
	if len(issues) > 0 {
		return map[string]any{"skills": []render.Rendered{}, "issues": issues}, nil
	}
	return results, nil
}

func installProfile(opts Options, cfg *config.Config, target render.Target, profileName string, installOpts install.Options) (any, error) {
	results, issues, err := profile.InstallProfile(opts.LibraryDir, opts.ProfilesDir, opts.ProjectDir, opts.RenderDir, target, profileName, installOpts)
	if err != nil {
		return nil, exitcodes.Wrap(err, exitcodes.ExitData, exitcodes.KindValidation, "resolve profile")
	}
	if len(issues) > 0 {
		return map[string]any{"results": []install.Result{}, "issues": issues}, nil
	}
	return results, nil
}

func Serve(version string, opts Options) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	srv := mcpserver.New("symskills", version)
	Register(srv, opts)
	return srv.ServeStdio(ctx)
}
