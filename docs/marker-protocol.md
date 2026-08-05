# `.symskills.json` Marker Protocol

`symskills` marks every directory it manages with a `.symskills.json` marker.
The marker is the contract between the Go CLI and the macOS client: both
sides must agree on its shape, and neither may silently rewrite a marker it
does not understand.

## Location

- Every managed skill install directory contains `.symskills.json`.
- Symlink-mode installs point at the render cache, so the marker lives in the
  rendered tree itself and is shared with every symlinked destination.
- Copy-mode installs carry a copy of the marker together with the skill
  content.

## Fields

| Field            | Type   | Required (writers) | Meaning                                        |
| ---------------- | ------ | ------------------ | ---------------------------------------------- |
| `schema_version` | int    | yes                | Marker format version; see versioning rules.   |
| `managed_by`     | string | yes                | Always `"symskills"`.                          |
| `target`         | string | yes                | Harness target (e.g. `opencode`).              |
| `name`           | string | yes                | Installed skill name.                          |
| `rendered_at`    | string | yes                | Path of the rendered source tree.              |
| `mode`           | string | yes                | `symlink` or `copy`.                           |
| `installed`      | string | yes                | RFC 3339 install timestamp (UTC).              |
| `source_hash`    | string | no                 | Content hash used by the render freshness check. |

Keys are `snake_case`. Unknown fields are preserved by the render pipeline
(rewrites merge into the existing marker instead of replacing it) and must be
ignored by readers.

## Versioning rules

- `schema_version` is a positive integer; the current version is `1`.
- **Readers** must treat a missing `schema_version` as version `1` (markers
  written before versioning existed carry no field).
- **Writers** must refuse to overwrite a marker whose `schema_version` is
  greater than the version they support. `symskills` returns an error instead
  of clobbering it.
- `symskills` also refuses to install over a destination whose marker is one
  version ahead, even with `--force`.
- Bump `schema_version` only when the marker shape changes incompatibly.
  Additive fields (such as `source_hash`) do not require a bump.

## macOS client obligations

- Write markers with `schema_version: 1` and `managed_by: "symskills"`.
- Never delete or rewrite a marker with a newer `schema_version`.
- Treat a marker with an unknown `schema_version` as "do not touch": surface
  it to the user instead of installing over it.
