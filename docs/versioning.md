# Versioning Policy

This document defines how version numbers are selected and propagated for
`symaira-skills` releases.

## Single source of truth: the git tag

Every release is tagged `vX.Y.Z` on `main`. The tag is the single source of
truth for version numbers. Both distribution artifacts derive their version
from that tag — there is no intentionally independent version scheme for the
macOS app.

- **CLI binary (`symskills`)** — version injected at build time:
  - Releases: GoReleaser injects `{{.Version}}` (the tag) via
    `.goreleaser.yml` `ldflags`.
  - Local/dev builds: `make build` injects
    `git describe --tags --always --dirty`.
- **macOS app (`Symskills.app`)** — marketing version derived from the
  release tag:
  - `.github/workflows/release-app.yml` exports the tag (without the leading
    `v`) as `APP_VERSION`.
  - `client/package.sh` passes it to `xcodebuild` as `MARKETING_VERSION` and,
  after archiving, **verifies** that the built app's
  `CFBundleShortVersionString` matches the tag version. A mismatch fails the
  build before the DMG can be published.
  - `client/project.yml` holds only the dev default (`MARKETING_VERSION`),
    kept aligned with the latest release; it is used for local builds that
    do not run through the release path. Local packaging overrides it only
    on an exact release tag.

## Version scheme

- [Semantic Versioning](https://semver.org/spec/v2.0.0.html): `MAJOR.MINOR.PATCH`.
- Patch: bug fixes and documentation.
- Minor: new features (backwards compatible).
- Major: breaking changes.
- Pre-releases use a `-prerelease` suffix on the tag where needed.

## Release steps

1. Merge the release PRs and confirm CI is green on `main`.
2. Run the prerelease gate: checks pass, coverage above the repository gate,
   and `CHANGELOG.md` has an entry for the upcoming version (see the update
   convention in `CHANGELOG.md`).
3. Tag `vX.Y.Z` on `main` and push the tag.
4. GitHub Actions publishes automatically:
   - `release.yml` — GoReleaser CLI binaries, checksums, provenance,
     Homebrew formula.
   - `release-app.yml` — signed and notarized `Symskills.dmg`, Homebrew cask.
     `package.sh` has already asserted the visible app version matches the
     tag before the DMG is uploaded.
5. Verify the published assets: the Homebrew formula/cask version equals the
   tag, and the downloaded app shows the tagged version in *About*.

## Divergence is a bug

If the visible app version ever differs from the CLI/repository release
version, treat it as a release-blocking defect: fix the derivation or the
verification step, not the displayed number.
