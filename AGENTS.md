# Agent Instructions - symaira-skills

`symaira-skills` is the public Apache-2.0 Skill SSOT manager for local AI agent harnesses. It ships empty: users bring their own skill repositories, and `symskills` validates, renders, diffs, and installs harness-specific variants.

## Build & Test

```bash
make build              # -> symskills binary
make test               # go test -race ./...
make lint               # gofmt check + go vet
make fmt-check          # fail if gofmt would change files
make clean              # remove build/test artifacts
```

## Architecture & Boundaries

- Keep the tool standalone-first. It may use `symaira-corekit` as a versioned public library, but it must not import sibling Symaira tool repositories or require another Symaira binary at startup.
- The canonical source format is portable `SKILL.md` plus optional `symskills.toml` and `overlays/<target>/` fragments.
- V1 targets are `opencode`, `claude`, `codex`, and `hermes`.
- Do not add curated, private, or personal skill content to this repo. Tests may use tiny fixtures only.
- Public output remains normal harness-readable skill folders, not a proprietary runtime format.
- MCP transport runs over stdio. Never print logs or diagnostics to stdout while serving MCP; stdout is reserved for JSON-RPC frames.

## Storage

- Config: `~/.config/symskills/config.toml`
- Library: `~/.local/share/symskills/library`
- Rendered artifacts: `~/.local/share/symskills/rendered`
- Cache: `~/.cache/symskills`

## Key Packages

- `internal/skill` - SKILL.md parsing, `symskills.toml`, validation, import.
- `internal/render` - target-specific rendering and overlay application.
- `internal/install` - managed installs, markers, diff, uninstall safety.
- `internal/mcptools` - MCP tools backed by the same core workflows.

## Safety Rules

- Never overwrite or remove a skill install path unless it contains `.symskills.json`.
- Prefer symlink installs; fall back to copy only when needed or explicitly requested.
- Preserve script permissions when copying support files.
- Keep JSON fields snake_case for CLI/MCP payloads.
