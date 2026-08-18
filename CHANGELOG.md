# Changelog

All notable changes to symaira-skills are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## Update convention

- Every user-visible change (new commands, changed behavior, bug fixes,
  security fixes, breaking changes) gets an entry under
  `## [Unreleased]` in the same PR that ships the change.
- Group entries as `Added`, `Changed`, `Fixed`, `Removed`, `Security`,
  `Performance`, `Tests`, or `CI and project maintenance`.
- Reference the PR or issue number where helpful.
- During release preparation, move the `[Unreleased]` entries into a dated
  `## [vX.Y.Z]` section and add the corresponding GitHub comparison link.

## [Unreleased]

### Added

- Harness variants: one canonical skill source can now carry harness-specific
  *content*, not just harness-specific paths and metadata (#208).
  - `<!-- symskills:block <id> -->` marks a replaceable region in `SKILL.md`
    or any markdown reference; `overlays/<target>/blocks/<id>.md` supplies
    that target's version. An empty override drops the region.
  - `<!-- symskills:only a,b -->` and `<!-- symskills:except a,b -->` keep or
    drop a region without a second file.
  - `{{term:name}}` substitutes a phrase from a new `[terms]` table in
    `symskills.toml`; every term requires a `default` so the canonical source
    always states something harness-neutral.
  - `symskills render --explain` reports per target which blocks were
    substituted, which term values were resolved, which reference files
    changed, and the resulting divergence from the canonical text. The same
    data is in `render --json` under `variants`.
  - New validation codes, all reported by `symskills validate`:
    `variant_marker_malformed`, `block_id_invalid`, `block_nested`,
    `block_unclosed`, `block_unmatched_close`, `block_close_mismatch`,
    `block_duplicate_id`, `block_target_list_empty`, `block_target_unknown`,
    `block_override_unknown`, `block_override_unused`, `term_unknown`,
    `term_name_invalid`, `term_default_required`. The error-severity codes
    refuse the render rather than shipping output that misrepresents the
    source.
  - Contract documented in [docs/harness-variants.md](docs/harness-variants.md).

  Blocks and terms are opt-in. A skill that uses neither renders
  byte-identical, keeps the same `source_hash`, and reports no new
  validation findings.

## [v0.3.2] - 2026-08-15

### Changed

- Update `symaira-corekit` to v0.9.1, pulling in fixes for concurrent
  config reload, cosign signature verification and JSON-RPC notification
  handling (#198).

### CI and project maintenance

- Pin `codeql-action` to v4.37.6 (#204) and
  `actions/attest-build-provenance` to v4.2.2 (#201).

## [v0.3.1] - 2026-08-10

### Added

- Versioning policy for CLI releases and the macOS app; the macOS app
  marketing version is now derived from the release tag and verified
  before publishing (see `docs/versioning.md`).
- Maintained changelog with release history and update convention.

## [v0.3.0] - 2026-08-07

### Added

- Tighten skill validation per the agentskills spec and gate bundle
  resources (#158, closes #155, #156, #157).
- Append-only lifecycle event log and `symskills log` command (#159,
  closes #116).
- Base snapshot persistence, staged comparison renders, and symlink diff
  drift fixes (#160, closes #123, #124, #125).
- Per-skill metadata in `list`, `inspect`, and MCP tools (#161, closes #117).
- `symskills status` and `sync` for fleet-wide drift repair (#162,
  closes #115).
- Flow skill type for symbrowse discovery (#163, closes #129).
- Real content differences in `symskills diff` and the Diff dialog (#164,
  closes #110).
- Install drift classification: harness edits vs library pushes vs
  conflicts (#165, closes #126).
- `symskills pull` with overlay refusal and contained write-back (#166,
  closes #127, #128).
- Per-skill versioning in its own git repository (#167, closes #118).
- `symskills history`, `show`, and `restore` with MCP undo tools (#168,
  closes #119).

### Fixed

- Harden skill metadata, drift handling, and maintenance workflows (#177,
  closes #169, #171, #172, #174, #175).
- Cache status renders and parallelize restore sync (#178).

### Security

- Pin `codeql-action` to a full SHA (#170).

### Tests

- Error-path tests closing the coverage regression since v0.2.0 (#182,
  closes #179, #180, #181).

## [v0.2.0] - 2026-08-06

### Added

- `schema_version` on the `discover --json` envelope for compatible client
  handshakes (#154).
- Antigravity and OpenClaw targets plus user-defined targets from
  `symskills.toml` (#150).
- stdio as the `serve` default, source-tree hash reuse per bundle, and
  expanded CLI/MCP coverage (#146).
- Versioned managed-install markers with atomic replacement and rollback
  protection (#145).
- YAML string lists in `SKILL.md` frontmatter (#130, closes #112).

### Fixed

- Prevent self-referential installs; add a safe `--force` adopt path with
  backups (#131).
- Inject the git-derived version into builds; add the repository security
  policy (#143).
- Update `symaira-corekit` to v0.8.0 (#149).

### CI and project maintenance

- Run static analysis and vulnerability scanning in the PR gate (#144).
- Run the PR gate for every pull request, including docs-only changes (#151).
- Add CodeQL analysis for pull requests and the default branch (#152).
- Improve the README and add repository community templates (#147).
- Ignore local worktrees (#148).

## [v0.1.10] - 2026-07-30

- Add `govulncheck` CI job (issue #104, #105).
- Add opt-in headless smoke test for harness skill loading (#101, #109).
- Emit build-provenance attestation for release artifacts (#102, #106).
- Add harness inventory status and source discovery (#99).
- Add per-target golden tests for rendered skill bundles (#100, #107).
- Collapse five open-coded target lists into a single `TargetSpec`
  registry (#103, #108).

## [v0.1.9] - 2026-07-29

- CLI library skill name resolution, flag mutation fix, and profile
  validate enhancement (#83, #84, #85, #90).
- Pin release workflow actions to commit SHAs (#81, #88).
- Render performance: non-destructive diff and source-hash caching
  (#86, #87, #96).
- Serialize MCP tool results as JSON strings; honor `dry_run` before
  render (#89).
- Apply link alias on profile render and install (#82, #95).
- Harden overlay path containment against traversal escapes (#80, #94).

## [v0.1.8] - 2026-07-27

- Batch-import skills from a parent directory (#78, closes #77).

## [v0.1.7] - 2026-07-27

- Fix Telemetry Logs horizontal scrolling and Render Preview sheet
  editability (#73, closes #67, #68).
- Fix `doctor` reporting empty target install paths (#72, closes #66).
- Fix dashboard reveal buttons, sidebar width, and diff dialog title
  (#71, closes #59, #60, #61, #63).
- Fix null JSON arrays in `list`/`validate` output; report no-op
  uninstalls honestly (#70, closes #55, #58).
- Hide the broken Project install scope in the GUI and surface import
  failures (#69, closes #56, #57).
- Add Homebrew Cask publisher for `Symskills.app` (DMG) in the release-app
  workflow.

## [v0.1.6] - 2026-07-24

- Correct notarization auth from Apple ID/password to API-key.
- Add code signing, notarization, and stapling to the DMG workflow.

## [v0.1.5] - 2026-07-22

### Security

- Prevent symlink traversal and arbitrary host file reads during skill copy
  and import (#45, closes #37).

### Fixed

- Handle dangling symlinks correctly in install safety checks; stream file
  hashing in diff comparison (#46, closes #38, #44).
- Default skill directory to the current working directory when omitted;
  explicit diff feedback; deduplicate profile workflows (#47, closes
  #39, #40, #41).

### Performance

- Avoid redundant subprocess execution in the macOS client validation
  query (#48, closes #42).
- Avoid loading full skill bodies during library list operations (#49,
  closes #43).

### Tests

- Tests for extracted profile render/install workflows, install safety,
  CopyTree symlink guards, and the frontmatter-only fast path (#54, closes
  #50, #51, #52, #53).

## [v0.1.4] - 2026-07-10

- Add inherited context profiles for skills discovery (#25, closes #24).
- Profile-aware CLI command tests (#32, closes #28).
- Fix CI Go-version mismatch and harden the workflow (#33, closes #26).
- Tests for MCP profile tools (`renderProfile`/`installProfile`) (#34,
  closes #29).
- Document context profiles in the README (#35, closes #27, #30).
- Cover the `ProfilesDir` branch in `config.EnsureDirs` (#36, closes #31).

## [v0.1.3] - 2026-07-09

- Fix the macOS app release workflow by reading the Go version from
  `go.mod` instead of pinning an outdated Go release, so the DMG build
  compiles against the current toolchain.

## [v0.1.2] - 2026-07-09

### Added

- Native macOS Swift GUI for `symskills`, including automatic DMG packaging
  and a dedicated release workflow (#5).
- `versionkit` integration and a `--json` flag on the `version` command
  (#6).

### Fixed

- Harden render/install path safety, surface render errors, and deduplicate
  copy helpers (#13, closes #7, #9, #11).
- Print library load issues in text mode; return validation errors from MCP
  tool handlers (#15, closes #10, #12).

### CI and project maintenance

- Align the CI Go-version matrix with `go.mod` and pin actions to SHAs
  (#14, closes #8).
- Raise overall line coverage toward the 80% threshold (#20, closes #16).
- Add unit tests for target parsing, overlay safety, library directory
  listing, and install scope settings (#21, #22, #23, closes #17, #18, #19).

## [v0.1.1] - 2026-06-26

- Add tests for the `internal/config` package (#4).

## [v0.1.0] - 2026-06-26

- Initial release.

[Unreleased]: https://github.com/danieljustus/symaira-skills/compare/v0.3.2...HEAD
[v0.3.2]: https://github.com/danieljustus/symaira-skills/compare/v0.3.1...v0.3.2
[v0.3.1]: https://github.com/danieljustus/symaira-skills/compare/v0.3.0...v0.3.1
[v0.3.0]: https://github.com/danieljustus/symaira-skills/compare/v0.2.0...v0.3.0
[v0.2.0]: https://github.com/danieljustus/symaira-skills/compare/v0.1.10...v0.2.0
[v0.1.10]: https://github.com/danieljustus/symaira-skills/compare/v0.1.9...v0.1.10
[v0.1.9]: https://github.com/danieljustus/symaira-skills/compare/v0.1.8...v0.1.9
[v0.1.8]: https://github.com/danieljustus/symaira-skills/compare/v0.1.7...v0.1.8
[v0.1.7]: https://github.com/danieljustus/symaira-skills/compare/v0.1.6...v0.1.7
[v0.1.6]: https://github.com/danieljustus/symaira-skills/compare/v0.1.5...v0.1.6
[v0.1.5]: https://github.com/danieljustus/symaira-skills/compare/v0.1.4...v0.1.5
[v0.1.4]: https://github.com/danieljustus/symaira-skills/compare/v0.1.3...v0.1.4
[v0.1.3]: https://github.com/danieljustus/symaira-skills/compare/v0.1.2...v0.1.3
[v0.1.2]: https://github.com/danieljustus/symaira-skills/compare/v0.1.1...v0.1.2
[v0.1.1]: https://github.com/danieljustus/symaira-skills/compare/v0.1.0...v0.1.1
[v0.1.0]: https://github.com/danieljustus/symaira-skills/commits/v0.1.0
