# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
make build        # build ./bin/quartr, version baked in via -ldflags
make test         # go test ./...
make install      # go install ./cmd/quartr; if QUARTR_API_KEY is in env,
                  # also runs `quartr auth login` to persist the key
                  # to ~/.config/quartr/config.json (mode 0600)
make dist         # cross-compile release archives + checksums into ./dist
make clean        # remove ./bin and ./dist

go test ./internal/cli -run TestBuildConfigPrecedence   # run a single test

pre-commit run --all-files    # lint+test against the whole tree
~/go/bin/golangci-lint run    # lint without pre-commit (needs v2.12+)
```

`pre-commit install` was already run in this clone — every commit runs golangci-lint (with `--fix`) and `go test ./...`. The lint hook shells out to the `golangci-lint` on `PATH` instead of the upstream pre-commit repo, which builds the linter from source with whatever Go it finds; a linter built with Go < 1.26 refuses to load this config. Install the matching binary once:

```bash
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.0
```

CI pins the same version through `golangci/golangci-lint-action@v7`.

## Architecture

Three internal packages, no external deps (Go stdlib only):

- **`internal/quartr`** — HTTP client and config persistence. `Client.GetBytes` retries 429/5xx with backoff (honors `Retry-After`). `LoadConfig`/`SaveConfig` handle the JSON file at `~/.config/quartr/config.json`.
- **`internal/cli`** — command dispatch and request shaping. The whole CLI surface is driven by a single `resources` map in `resources.go` keyed by command name; each entry describes the API path templates, allowed query params (`paramSet`), and download/stream URL fields. `handlers.go` dispatches the operations (list/get/summary/pages/chapters/download/stream) against any resource by reading from that map. Adding a new resource = one map entry, no per-command handler code.
- **`internal/output`** — formats results as `table` / `json` / `csv` / `raw`. `--fields` supports dotted paths (`event.title`) via `getPath` recursive traversal.

`cmd/quartr/main.go` is a 3-line entry point that calls `cli.Run`.

### Cross-cutting design choices to preserve

- **Auth precedence is `flag > env > file > defaults`** — implemented in `internal/cli/config.go` `buildConfig`. Tests must `t.Setenv` to clear `QUARTR_*` env vars before asserting defaults; otherwise the user's shell env leaks in.
- **Companies API uses `ids`, not `companyIds`** — handled by `listFlags.toParams` taking a `companyEndpoint` flag. Don't generalize this away; Quartr's API is genuinely asymmetric.
- **`--all` auto-bumps `--limit` to 500** unless the user passed `--limit` explicitly. Detection lives in `flagWasPassed` (string-scan over the raw args, since the `flag` package can't distinguish "default" from "explicitly default").
- **`parseInterspersed`** in `flags.go` lets users write `cmd <id> --flag value`. The stdlib `flag` package stops at the first positional, so we shuffle flags before positionals before delegating.
- **Downloads do NOT send `x-api-key` by default** — the Quartr `fileUrl` is publicly fetchable. `--with-api-key` is the opt-in.

## Lint config notes

- golangci-lint v2 syntax (config has `version: "2"` at top). v1 is built with Go 1.24 and rejects this repo's Go 1.26 target — never downgrade.
- `gomodguard` is referenced as `gomodguard_v2` after the v2.12 deprecation rename.
- `gocritic.hugeParam` is intentionally disabled — passing `resource` (200B) by value is the design, not a perf bug.
- `gosec G304/G602` excluded globally — file paths from CLI args and bounds-checked slice indexes are inherent to the tool.
- `gosec G117` is suppressed via `//nolint:gosec` on the single line where it fires (`json.MarshalIndent(cfg, ...)` in `internal/quartr/config.go`) — that file IS the credential storage, by design.
- `varnamelen` / `goconst` / `tagliatelle` / `canonicalheader` are disabled because they fire heavily on idiomatic short-scope vars, output-format string literals, Quartr's camelCase JSON tags, and Quartr's lowercase headers (`x-api-key`).

## Repo / branch hygiene

- Remote: `git@github.com:TJC-LP/quartr-cli.git` (public).
- Module path is `github.com/TJC-LP/quartr-cli`, which must keep matching the
  repo URL — that is what makes `go install github.com/TJC-LP/quartr-cli/cmd/quartr@latest`
  resolve. Renaming the repo means renaming the module.
- `main` is protected — push to a feature branch and open a PR with `gh pr create`. Direct pushes to `main` are rejected.
- `.env`, `bin/`, and `dist/` are gitignored; the `.env.example` exception is preserved.

## Releases

Cutting a release is one push:

```bash
git tag -a v0.2.0 -m "v0.2.0"
git push origin v0.2.0
```

`.github/workflows/release.yml` fires on `v*` tags, runs the tests, calls
`make dist VERSION=${tag#v}`, and publishes the archives with `gh release
create --generate-notes`. A tag containing a hyphen (`v0.2.0-rc1`) publishes as
a prerelease.

- **Tags carry the `v`; the reported version does not.** `v0.1.0` produces
  `quartr 0.1.0`. The workflow asserts this before publishing, so a mismatch
  fails the release rather than shipping a mislabeled binary.
- **`make dist` is the single build recipe** — CI calls it rather than
  reimplementing the matrix, and the `dist` job in `ci.yml` runs it on every PR
  so a broken cross-compile surfaces before a tag exists.
- **Version resolution lives in `internal/quartr/version.go`**, not in `cli`,
  so `--version` and the `User-Agent` header cannot drift. Order: `-ldflags
  -X ...quartr.buildVersion`, then `debug.ReadBuildInfo` (module version for
  `go install pkg@version`, VCS revision for a checkout build), then `dev`.
  If you move or rename `buildVersion`, update `VERSION_LDFLAGS` in the Makefile.
- Release archives bundle `README.md` and `LICENSE` alongside the binary and
  ship with a `SHA256SUMS` file.

## Skill

The `.claude/skills/quartr/` skill (loaded automatically by the harness when working in this repo) documents how to drive the `quartr` binary at runtime — commands, flags, recipes, and the type-id lookup tables for events and document forms (10-K = 11, etc.). Reference it instead of re-deriving CLI usage from the source.
