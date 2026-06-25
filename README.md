# symaira-skills

`symskills` is a local-first SSOT manager for Agent Skills. It lets users keep one portable skill source and render/install harness-specific variants for OpenCode, Claude Code, Codex, and Hermes.

The repository ships empty. It contains the tool, schema conventions, and test fixtures only. Users bring their own skill repositories.

## Why

Most modern agent harnesses can consume a `SKILL.md`-style bundle, but they disagree on discovery paths, optional metadata, invocation policies, and install workflows. `symskills` keeps the portable source in one place and generates normal harness-readable skill folders.

## Install From Source

```bash
go install github.com/danieljustus/symaira-skills/cmd/symskills@latest
```

During local development:

```bash
make build
./symskills --help
```

## Quick Start

```bash
symskills init
symskills import /path/to/my-skill
symskills list
symskills validate ~/.local/share/symskills/library/my-skill
symskills render --target all ~/.local/share/symskills/library/my-skill
symskills install --target opencode ~/.local/share/symskills/library/my-skill
```

Use `--json` on inspect/list/validate/render/install-style commands for machine-readable output.

## Skill Source Layout

```text
my-skill/
  SKILL.md
  symskills.toml              # optional
  references/                 # optional portable support files
  scripts/                    # optional portable support files
  assets/                     # optional portable support files
  overlays/
    opencode/
      prepend.md              # optional
      append.md               # optional
      frontmatter.toml        # optional
    claude/
    codex/
    hermes/
```

`SKILL.md` is the canonical portable source. `symskills.toml` enables target-specific aliases and install/render preferences:

```toml
[skill]
name = "repo-review"
version = "1.0.0"
source = "https://example.test/repo-review"

[targets.opencode]
enabled = true
alias = "repo-review-opencode"
description = "OpenCode-optimized repository review workflow."

[targets.claude]
enabled = true

[targets.codex]
enabled = true

[targets.hermes]
enabled = true
category = "developer-tools"
```

An overlay `frontmatter.toml` can add target metadata:

```toml
[metadata]
workflow = "github"
audience = "maintainers"
```

## Commands

| Command | Purpose |
|---------|---------|
| `init` | Create XDG config and data directories |
| `import <skill-dir>` | Copy an existing skill into the managed library |
| `list` | List managed skills |
| `inspect <skill-dir>` | Show parsed SKILL.md + symskills metadata |
| `validate <skill-dir>` | Validate portable skill metadata and references |
| `render <skill-dir>` | Render target-specific skill folders |
| `diff <skill-dir>` | Compare rendered output with installed target |
| `install <skill-dir>` | Render and install a target-specific skill |
| `uninstall <name>` | Remove a managed installed skill |
| `doctor` | Print config, library, render, and target paths |
| `serve --stdio` | Serve MCP tools over stdio |

## Install Safety

`symskills` renders into `~/.local/share/symskills/rendered` first, then installs into the target harness. It refuses to overwrite or remove an install path unless that path contains a `.symskills.json` marker.

Default user install paths:

| Target | Path |
|--------|------|
| OpenCode | `~/.config/opencode/skills/<name>` |
| Claude Code | `~/.claude/skills/<name>` |
| Codex | `~/.agents/skills/<name>` |
| Hermes | `~/.hermes/skills/symaira/<name>` |

## MCP Tools

```bash
symskills serve --stdio
```

Exposes:

- `skills_list`
- `skills_inspect`
- `skills_validate`
- `skills_render_plan`
- `skills_install`

`skills_install` defaults to dry-run mode. Pass `dry_run=false` to perform writes.

## Development

```bash
make test
make lint
make build
```

## License

Apache-2.0 - Daniel Justus
