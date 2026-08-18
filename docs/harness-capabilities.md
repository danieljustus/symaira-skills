# Harness Capabilities

[Harness variants](harness-variants.md) let a skill *adapt* to a harness. They
cannot make a harness do something it has no mechanism for. When a skill is
architecture-bound — it needs child agents, or work that survives between
turns — the honest output for a harness that cannot run it is **no install at
all**, not a degraded best-effort that passes every check and then refuses
itself at use time.

Capabilities are how a skill says what it needs and a target says what it
offers.

## Declaring what a skill needs

```toml
# symskills.toml
[skill]
name = "issue-sweep"
requires = ["subagents"]
```

Rendering for a target that declares it lacks `subagents` is refused, naming
the skill, the target and the capability. Nothing is written.

## The vocabulary

A closed list, deliberately about the harness *runtime* rather than the
machine. Anything a shell can do — running git, creating a worktree, calling
an HTTP API — is not a capability here, because every harness that can run
commands has it.

| Capability | Meaning |
|---|---|
| `subagents` | The harness dispatches work to a child agent it manages itself (Claude Code's Agent tool, Hermes' `delegate_task`). |
| `background_tasks` | Work continues between turns and reports back, rather than only running to completion inside one turn. |
| `mcp` | The harness connects to Model Context Protocol servers. |
| `slash_commands` | Skills are invocable by an explicit user-typed command, not only by model-side discovery. |
| `hooks` | The harness runs user-configured hooks around tool calls. |
| `scheduled_tasks` | The harness can run a skill on a schedule without an interactive session. |

## Three states, not two

A capability a target has not declared is **unknown**, not absent.

`symskills` does not observe harness runtimes — it writes skill folders and
the harness loads them out of process. Treating silence as "unsupported"
would refuse renders on a guess, which is exactly the kind of confident wrong
answer this feature exists to remove.

| State | Render behaviour |
|---|---|
| `supported` | Renders silently. |
| `unsupported` | **Refused.** `--ignore-capabilities` overrides; see below. |
| `unknown` | Renders, with a warning naming the capability and the config fix. |

`symskills targets` shows all three per harness:

```console
$ symskills targets
Target: Claude Code (claude)
  …
  Runtime:     subagents ?background_tasks ?mcp ?slash_commands ?hooks ?scheduled_tasks   (! = unsupported, ? = undeclared)
```

`symskills targets --json` reports the same under `runtime_capabilities` as
`"supported" | "unsupported" | "unknown"`.

## Declaring what a target offers

The built-in registry is **deliberately sparse**. It declares only what is
evidenced by a harness's own documented skill-facing tooling, and leaves
everything else unknown rather than guessing on your behalf. Today that means
`claude` and `hermes` declare `subagents`; nothing else is declared.

You complete the picture for the harness builds you actually run:

```toml
# ~/.config/symskills/config.toml  (or ./.symskills.toml, which overrides it)
[capabilities.codex]
subagents = true
mcp = true

[capabilities.opencode]
background_tasks = false
```

`true` declares support, `false` declares its absence, and a capability you
omit stays unknown. A misspelled target or capability name is an error, not a
silent no-op. User-defined targets declare theirs inline:

```toml
[[targets]]
name = "myagent"
skill_root_user = "/home/u/.myagent/skills"

[targets.capabilities]
subagents = true
```

## Overriding a refusal

```bash
symskills render --target codex --ignore-capabilities ./issue-sweep
symskills install --target codex --ignore-capabilities ./issue-sweep
```

The render proceeds, warns on stderr, and — this is the point — the result
**declares no `compatibility`**. A forced render is an experiment, and it does
not get to assert a compatibility the target itself denies.

`--ignore-capabilities` is refused together with `--profile`: forcing a whole
profile would turn a deliberate per-skill override into a blanket one.

## The coupling warning

A skill can also be harness-bound by accident, in prose. `symskills validate`
reports `harness_coupling` when the body or a markdown reference names a
registered harness in text every target renders:

```console
warning  harness_coupling  SKILL.md  line 12: names harness "hermes" outside any
symskills:only region; every other target renders this text too — scope it with
a region, move the differing part into a {{term:...}}, or disable the other
targets in symskills.toml
```

Two mentions are exempt, because they are legitimate:

- inside a `symskills:only` region, the construct whose purpose is scoping
  text to named harnesses;
- inside a fenced code block, where a harness name is usually a command being
  demonstrated rather than an instruction being given.

Being inside a `symskills:block` is **not** an exemption: the canonical text
of a block is what every target without an override receives, so a
harness-bound default is real coupling.

The check is skipped for a skill that enables exactly one target — a
deliberately single-harness skill may name its harness freely.

## Metadata namespacing

Frontmatter metadata namespaced under a harness name is that harness's
convention and now travels only to it:

```yaml
metadata:
  hermes:
    tags: [GitHub, Issues]
  workflow: github
```

The Hermes render keeps `metadata.hermes`; every other target's render drops
it. Keys that are not target names (`workflow` above) are untouched.

## Compatibility

Everything here is opt-in. A skill with no `requires` renders exactly as
before, including its `compatibility` field, and no existing install is made
stale. The only unconditional change is metadata namespacing, which affects a
skill only if it uses a harness name as a metadata key.
