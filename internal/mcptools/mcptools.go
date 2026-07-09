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
	"github.com/danieljustus/symaira-skills/internal/render"
	"github.com/danieljustus/symaira-skills/internal/skill"
)

const emptyObject = `{"type":"object","properties":{}}`

type Options struct {
	LibraryDir string
	RenderDir  string
	HomeDir    string
	ProjectDir string
}

func Register(srv *mcpserver.Server, opts Options) {
	cfg := config.Defaults()
	if opts.LibraryDir == "" {
		opts.LibraryDir = cfg.LibraryDir
	}
	if opts.RenderDir == "" {
		opts.RenderDir = cfg.RenderDir
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
		Name:        "skills_render_plan",
		Description: "Render a skill to the managed artifact directory and return planned target paths.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"name":{"type":"string"},"target":{"type":"string"}}}`),
		Handler: func(ctx context.Context, in json.RawMessage) (any, error) {
			var args struct {
				Target string `json:"target"`
			}
			if err := json.Unmarshal(in, &args); err != nil {
				return nil, exitcodes.Wrap(err, exitcodes.ExitData, exitcodes.KindValidation, "parse arguments")
			}
			bundle, err := callInspect(ctx, srv, opts, in)
			if err != nil {
				return nil, err
			}
			targets := render.DefaultTargets
			if args.Target != "" {
				target, err := render.ParseTarget(args.Target)
				if err != nil {
					return nil, err
				}
				targets = []render.Target{target}
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
		Description: "Render and install a skill. Dry-run defaults to true; pass dry_run=false for writes.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"name":{"type":"string"},"target":{"type":"string"},"scope":{"type":"string"},"dry_run":{"type":"boolean"}}}`),
		Handler: func(ctx context.Context, in json.RawMessage) (any, error) {
			var args struct {
				Target string `json:"target"`
				Scope  string `json:"scope"`
				DryRun *bool  `json:"dry_run"`
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
			dryRun := true
			if args.DryRun != nil {
				dryRun = *args.DryRun
			}
			scope := install.ScopeUser
			if args.Scope == string(install.ScopeProject) {
				scope = install.ScopeProject
			}
			return install.Install(install.RenderedSkill{
				Target: target,
				Name:   rendered[0].Name,
				Path:   rendered[0].Path,
			}, install.Options{HomeDir: opts.HomeDir, ProjectDir: opts.ProjectDir, Scope: scope, DryRun: dryRun})
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

func Serve(version string, opts Options) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	srv := mcpserver.New("symskills", version)
	Register(srv, opts)
	return srv.ServeStdio(ctx)
}
