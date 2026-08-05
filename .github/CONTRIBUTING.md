# Contributing to symaira-skills

Thanks for contributing! This project is small and intentionally focused — please open an issue to discuss larger changes before starting them.

## Build and test

```bash
make build       # build the symskills binary
make test        # go test -race ./...
make lint        # gofmt check + go vet
make fmt-check   # fail if gofmt would change files
make clean       # remove build/test artifacts
```

All tests and lint must pass before a pull request is merged.

## Pull request conventions

- One logical change per PR; keep the diff small and reviewable.
- Reference the issue you are fixing, e.g. `Fixes #123`.
- Use a descriptive title and summarize the change and the tests you ran in the description.
- Add screenshots for any UI changes (the `client/` app).
- Do not commit generated artifacts or build output.

## Commit messages

Use conventional commits (`feat:`, `fix:`, `docs:`, `chore:`, `test:`, `refactor:`) with a short imperative summary line.

## Scope

This repository ships empty on purpose: it contains the tool, schema conventions, and test fixtures only. Do not add curated or personal skill content, and keep public output as normal harness-readable skill folders.

## Code of conduct

Be respectful and constructive. Harassment or hostile behavior is not tolerated.
