# Porting `symskills` to a New Harness

This document enumerates every touchpoint required to add a new AI-agent harness target to `symskills`. Follow these steps in order.

## Overview

Each harness target is represented by a `render.TargetSpec` entry in the single registry at `internal/render/render.go`. Adding a new target means adding one spec entry, updating the discovery and capability files, and committing a golden test fixture.

## Step-by-step

### 1. Choose a target name

Pick a short, lowercase, unique identifier (e.g. `cursor`, `gemini`, `kimi`). This becomes the `Target` constant and the directory name inside the user's config/home.

### 2. Add the `Target` constant

In `internal/render/render.go`, add a constant:

```go
const (
    // ... existing targets ...
    TargetMyHarness Target = "my-harness"
)
```

### 3. Add the `TargetSpec` entry

In the same file, add a spec to the `Targets` registry slice. Follow one of the existing entries (opencode, claude, codex, hermes) using the paths and conventions that harness expects:

```go
{
    Name:        TargetMyHarness,
    DisplayName: "My Harness",   // human-readable name for CLI output
    BinaryName:  "my-harness",   // the installed binary users invoke
    ConfigDir: func(home, project string, scope Scope) string {
        if scope == ScopeProject && project != "" {
            return filepath.Join(project, ".my-harness")
        }
        return filepath.Join(home, ".config", "my-harness")
    },
    SkillRoot: func(home, project string, scope Scope) string {
        if scope == ScopeProject && project != "" {
            return filepath.Join(project, ".my-harness", "skills")
        }
        return filepath.Join(home, ".config", "my-harness", "skills")
    },
    Quirks: "",  // note any special handling (e.g. Codex needs YAML frontmatter)
},
```

**Important:** Verify the exact install path the harness expects — some harnesses use `~/.config/<harness>/skills`, others use `~/.<harness>/skills`, and Hermes uses `~/.hermes/skills/symaira/<name>`.

### 4. Add overlay fragments (optional)

If the target needs target-specific content prepended or appended to the skill frontmatter or body, create overlay files under the source skill's `overlays/<target>/` directory. The convention is documented in `internal/render/render.go`.

For differences *inside* the body — a worker-dispatch contract, a report path, a paragraph that only applies to some harnesses — use harness variants instead of prepend/append: a skill marks the region with `<!-- symskills:block <id> -->` and the target supplies `overlays/<target>/blocks/<id>.md`, or uses a `{{term:...}}` placeholder resolved from the `[terms]` table. Appending contradicting text under an absolute instruction does not work; replacing the region does. See [harness-variants.md](harness-variants.md).

The overlay directory name defaults to the target name, so a new target needs no registration for either mechanism. A user-defined target may point elsewhere with `overlay_dir`.

### 4b. Declare the harness capabilities

Add a `Capabilities` map to the spec for anything the harness runtime offers a skill and that is evidenced by its own documented tooling:

```go
Capabilities: map[string]bool{
    render.CapSubagents: true,
},
```

Declare only what you can point at. A capability left out of the map is *unknown*, which renders with a warning; declaring `false` refuses the render for skills that require it, so a wrong `false` is worse than silence. Users complete the picture for their own builds via `[capabilities.<target>]` in `config.toml`. See [harness-capabilities.md](harness-capabilities.md).

### 5. Update install-path verification

`internal/install/install.go` reads from `render.LookupSpec` automatically — no switch or case statement needs editing. Run the golden tests (step 8) to confirm paths match expectations.

### 6. Update harness descriptors

`internal/harness/harness.go` builds its `Descriptors` from `render.Targets` automatically — no per-target edit is needed. The harness descriptor picks up `DisplayName`, `BinaryName`, `ConfigDir`, and `SkillRoot` from the spec.

### 7. Render a golden test fixture

Add a golden tree under `internal/render/testdata/golden/<target>/`:

1. Create the directory: `internal/render/testdata/golden/<target>/`
2. Run the golden test with the `-update` flag to seed the golden:

   ```bash
   go test ./internal/render/... -run TestGolden -update
   ```

3. Verify the generated tree contains the expected `SKILL.md`, `.symskills.json`, and any target-specific files.
4. Review the diff to ensure the rendered output matches the harness contract.
5. Commit the golden files.

Each golden directory must include a `README.md` (or a comment in the golden fixture) stating where the expected layout comes from (vendor docs, harness README, or reverse-engineering).

### 8. Run the full test suite

```bash
make test
```

The golden tests, path tests, and all existing tests must pass.

### 9. Update CLI discoverability

The `symskills targets` and `symskills discover` commands automatically pick up new targets from the registry — no code changes needed.

### 10. Update documentation

- Update `README.md` to list the new target in the supported harnesses section.
- Document any special install prerequisites or known quirks.

## Acceptance checklist

- [ ] `Target` constant defined in `internal/render/render.go`
- [ ] `TargetSpec` entry added to the `Targets` registry
- [ ] All existing tests pass (`make test`)
- [ ] Golden test fixture committed under `internal/render/testdata/golden/<target>/`
- [ ] `go test ./internal/render/... -run TestGolden` passes (byte-identical output)
- [ ] `make build` and `make fmt-check` pass
- [ ] `README.md` updated with new target
