# Harness Variants

Most differences between agent harnesses are paths and metadata, and
`symskills` has always handled those. Some differences are *content*: the
worker-dispatch contract of a Hermes skill names `delegate_task` and a tier
label, the Claude Code equivalent names the Agent tool and a git worktree.
Shipping the Hermes sentence to Claude produces an install that passes every
check and then refuses itself at use time.

Harness variants let one canonical source carry those differences without
becoming two skills. The portable `SKILL.md` stays complete and readable on
its own; each harness's deviation is a named delta keyed to it.

Three constructs, resolved at render time for the target being rendered:

| Construct | Use it for | Lives in |
|---|---|---|
| **Block** | Replacing a paragraph or section wholesale | `SKILL.md` / references + `overlays/<target>/blocks/<id>.md` |
| **only / except** | Keeping or dropping a region, no second file | `SKILL.md` / references |
| **Term** | A single word or phrase that differs per harness | `{{term:name}}` + `[terms]` in `symskills.toml` |

## Blocks

Mark a replaceable region in the canonical text:

```markdown
<!-- symskills:block worker-isolation -->
Hermes only. Dispatch each worker with `delegate_task` at the configured tier.
<!-- /symskills:block -->
```

Give a target its own version of that region:

```text
overlays/claude/blocks/worker-isolation.md
```

```markdown
Dispatch each worker with the Agent tool in its own git worktree.
```

Rendering `claude` emits the override; every other target emits the canonical
text. The markers themselves never reach a rendered skill — for any target,
including the ones that change nothing.

Rules:

- Ids are lowercase alphanumeric segments joined by single dashes
  (`worker-isolation`), and are **unique across the whole skill**: an id
  addresses a block, not a file, so the same id may not appear in both
  `SKILL.md` and a reference.
- Regions do not nest. A flat structure keeps both the parser and a human
  diff unambiguous.
- An **empty override drops the region** for that target. That is how a
  skill says "this section does not apply here" without inventing prose.
- An override must name a block the source defines. `overlays/claude/blocks/
  invented.md` with no matching `symskills:block invented` is a validation
  **error** and refuses the render. This is the rule that keeps an overlay a
  delta instead of a second, drifting copy.

## only / except

For a sentence or two, a second file is overkill:

```markdown
<!-- symskills:only hermes,codex -->
Never invoke an external coding-agent executable.
<!-- /symskills:only -->

<!-- symskills:except hermes -->
Run workers through your harness's own subagent mechanism.
<!-- /symskills:except -->
```

`only` keeps the region for the listed targets; `except` drops it for them.
A target name that no registered harness matches is reported as a warning —
a typo there would silently drop the content for every harness.

## Terms

Many harness differences are one noun. A term table avoids one overlay file
per sentence:

```toml
# symskills.toml
[terms.report_dir]
default = "~/.local/state/symskills/reports"
hermes  = "~/.hermes/reports"

[terms.subagent_dispatch]
default = "run each worker as an isolated subagent"
hermes  = "dispatch each worker with `delegate_task` at the configured tier"
claude  = "dispatch each worker with the Agent tool in its own worktree"
```

Reference a term anywhere in `SKILL.md` or a markdown reference:

```markdown
Persist the run report under {{term:report_dir}}/issue-sweep/.
```

Rules:

- `default` is **required**. It is the harness-neutral text, so the canonical
  source always states something true even for a target with no override.
- Resolution is `terms.<name>.<target>`, falling back to
  `terms.<name>.default`.
- A referenced term that is not defined, or defined without a `default`, is a
  validation error and refuses the render.
- Placeholders are explicit tokens, so substitution is safe everywhere in the
  document — including inside fenced code blocks. No heuristics are involved
  and no prose is rewritten by accident.

## What gets resolved

- `SKILL.md` — body, including overlay `prepend.md` / `append.md` fragments.
- Every **markdown** resource in the bundle (`references/*.md` and any other
  `.md` / `.markdown` file outside `overlays/`).
- Nothing else. Scripts, assets, and data files travel byte-identical, so a
  placeholder-looking string in a shell script is never rewritten.

## Inspecting the result

```bash
symskills render --target claude --explain ~/.local/share/symskills/library/issue-sweep
```

`--explain` reports, per target, the block ids substituted, the term values
resolved, the markdown files whose content changed, and a divergence figure
(how many bytes of the canonical text this harness replaces). The same data
is in `render --json` under `variants`.

Divergence is reported, never enforced. A skill whose Claude render replaces
most of its body is telling you something: it is probably **two skills**, not
one skill with an overlay. Splitting it is a human editorial decision, and
the number exists to make that decision visible early.

## When not to use this

A variant mechanism can adapt a skill; it cannot invent a capability the
harness does not have. When a skill is architecture-bound — Hermes provider
chains, tier escalation, durable lanes — the honest output for a harness that
cannot run it is no install at all, not a degraded best-effort. Disable the
target in `symskills.toml`:

```toml
[targets.claude]
enabled = false
```

rather than overlaying the skill into something that only looks portable.

## Validation codes

| Code | Severity | Meaning |
|---|---|---|
| `variant_marker_malformed` | error | A `symskills:` marker that is not a recognised form (usually a typo). |
| `block_id_invalid` | error | Block id is not lowercase-dash form. |
| `block_nested` | error | A region opens inside another region. |
| `block_unclosed` | error | An opening marker is never closed. |
| `block_unmatched_close` | error | A closing marker with no opening marker. |
| `block_close_mismatch` | error | A closing marker of a different kind than the open region. |
| `block_duplicate_id` | error | The same block id is defined twice in the skill. |
| `block_target_list_empty` | error | `only` / `except` lists no target names. |
| `block_target_unknown` | warning | `only` / `except` names a target no harness matches. |
| `block_override_unknown` | error | An overlay override addresses a block the source does not define. |
| `block_override_unused` | warning | A disabled target ships block overrides. |
| `term_unknown` | error | A `{{term:...}}` reference has no `[terms]` entry. |
| `term_name_invalid` | error | A term name or reference is not lowercase-dash/underscore form. |
| `term_default_required` | error | A term has no `default` value. |

Every error-severity code above **refuses the render**: the alternative is an
installed skill that misrepresents its source, which is the failure this
feature exists to prevent. Warnings are reported by `symskills validate` and
render normally.

## Compatibility

Blocks and terms are opt-in. A skill that uses neither renders byte-identical
to before, keeps the same `source_hash`, and reports no new validation
findings — no existing install is made stale by adopting this version.
