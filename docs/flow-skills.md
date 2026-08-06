# Flow Skills (`flow` skill type)

A flow skill is a portable `SKILL.md` bundle whose body is **executed by
a tool** rather than consumed by an agent harness. The first consumer is
`symbrowse` (browser automation); the contract is coordinated with
`symbrowse#56` and `symbrowse#55` (flow record). Discovery, versioning,
and distribution remain in `symskills` — symbrowse keeps no second
registry (SSOT rule).

## Layout conventions

A flow skill has `SKILL.md` plus flow documents in exactly one of two
layouts:

1. **`flows/` subdirectory** — every `*.yaml` file under `flows/` is a
   flow document. Preferred for skills carrying more than one flow.
2. **Root `*.flow.yaml` file** — a single flow document named
   `<anything>.flow.yaml` in the skill root, for skills with exactly one
   flow.

Flow documents conform to
https://symaira.dev/schemas/symbrowse-flow.json (validated by the
executing tool, `symbrowse` — not by symskills).

## Resource model

Flow documents are YAML data files. `symskills` treats them as ordinary
resources:

- `LoadBundle` inventories them in `Bundle.Resources` (relative path,
  size, mode, executable flag) like any other non-`SKILL.md` file.
- Skill-level validation accepts them as-is; they are data files, not
  scripts, so they carry no `resource_executable` warning.
- `import` copies them into the managed library; `render` copies them
  into every target's rendered tree; `install` ships them into the
  installed skill directory byte-identical.

No executable bit is required (and none is granted): the default install
policy strips executable bits unless `--allow-executable` (or
`[skill] allow_executable = true` in `symskills.toml`) is set — which is
irrelevant for flow documents, read as YAML, never executed.

## Discovery contract (symbrowse)

`symbrowse flow list` discovers symskills-hosted flows at runtime:

1. Run `symskills list --json`.
2. For every entry, read the `path` field (absolute library path of the
   skill directory).
3. Scan each path for flow documents in the two layouts above.

There is no compile-time import and no second registry in symbrowse.
The `path` field of `symskills list --json` is therefore a locked
contract: it must be absolute, non-empty, and point at the skill
directory in the managed library.

## Precedence

When the same flow name exists in several places, symbrowse resolves in
this order:

1. Project-local: `./.symbrowse/flows/`
2. Global config: `~/.config/symbrowse/flows/`
3. symskills library flows (via `symskills list --json`)

## Verification

`symbrowse flow list` shows symskills-hosted flows with origin
`symskills`. In this repo the contract is locked by:

- `internal/skill/flow_test.go` — bundle load, clean skill-level
  validation, and resource inventory of flow documents for both layouts
  (fixtures: `testdata/flow/browser-flows`, `testdata/flow/browser-root-flow`)
- `internal/render/flow_test.go` — rendered trees ship the flow
  documents byte-identical for every target
- `cmd/symskills/main_test.go` — import → `list --json` → install E2E
  (`TestFlowSkillImportListAndInstall`) and the `list --json` `path`
  contract symbrowse depends on (`TestListJSONPathContractForSymbrowse`)
