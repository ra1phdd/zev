<div align="center">

**English** | [Русский](../ru/DEVELOPMENT.md)

</div>

# Development

## Requirements

- Go 1.27.2 or newer
- `gofmt` (bundled with Go), `gofumpt`, `goimports`,
  [golangci-lint](https://golangci-lint.run/) v2 and `deadcode` for `make lint`
- GoReleaser v2 for release checks and snapshot builds
- Optionally `entr` for `make watch`

## Setup

```bash
git clone https://github.com/ra1phdd/zev.git
cd zev
make build
```

## Make targets

| Target | Description |
|---|---|
| `make build` | Build into `bin/zev` |
| `make install` | `go install` into `$GOPATH/bin` |
| `make test` | `go vet ./...` followed by `go test ./...` |
| `make test-race` | `go test -race ./...` |
| `make test-integration` | Tests with the `integration` build tag |
| `make test-e2e` | Tests with the `e2e` build tag |
| `make test-manual ALLOW_MANUAL_TESTS=1` | Explicitly authorized manual tests, refused in CI |
| `make lint` | Check gofmt/gofumpt/goimports, run golangci-lint with fixes, report dead code for `APP` |
| `make fmt` | Apply gofmt, goimports and gofumpt |
| `make check` | Lint + tests |
| `make run` | Build and start interactive mode |
| `make setup` | Build and run the setup wizard |
| `make dev` | Run with debug logs |
| `make watch` | Restart on file changes (requires `entr`) |
| `make build-all` | Cross-compile for Linux, macOS and Windows |
| `make clean` | Remove `BIN` and its four cross-compiled variants, preserving other files |
| `make release-check` | Validate `.goreleaser.yaml` without publishing |
| `make release-snapshot` | Build a GoReleaser snapshot without publishing |
| `make release` | Run release gates, then tag, package and publish (explicit authorization required) |
| `make version` | Print the build version (`dev` by default) |
| `make info` | Print the app package, binary path, version and Go platform |

`GO`, `APP`, `BIN`, `FLAGS` and `VERSION` can be overridden. `BIN` must name a file
inside `bin/`, without `..` path components. `make clean` validates the same path.
`ARGS` forwards arguments to `run`, `setup` and `dev`:

```bash
make run ARGS=version
make build VERSION=1.0.0 BIN=bin/zev-local
make clean BIN=bin/zev-local
```

`make install` uses `go install` directly, respecting `GOBIN` or `GOPATH/bin`;
it does not create a redundant local binary. `make dev` enables debug logging
with `-vv`. Local `make build-all` builds Linux amd64, macOS amd64/arm64 and
Windows amd64. GoReleaser additionally builds Linux arm64/armv7 and Windows
arm64; its archives and snapshot output currently go to `dist/`.

Local builds pass `-s -w` to the Go linker, removing the symbol table and DWARF
debug information to reduce binary size, while preserving `-X main.version`.
`FLAGS` also contains `-buildid=` to omit the Go linker build ID and `-trimpath`
to remove local filesystem paths. GoReleaser already uses `-s -w` and `-trimpath`.
These flags do not enable additional CPU
optimizations; source-level debugging requires a build without stripping:

```bash
make build FLAGS=
```

`make lint` runs `golangci-lint run --fix` and can modify Go sources.
`deadcode -test $(APP)` reports reachability for the selected application and
its tests, not a cross-platform proof that reported functions can be deleted.

## Release ordering and verification

`VERSION` defaults to `dev` for local builds; the `VERSION` file is the release
source of truth. Both `make release` and the [GitHub release workflow](../../.github/workflows/release.yml)
call [deploy/release.sh](../../deploy/release.sh). The workflow starts on a `VERSION`
change on `main` or manually, with an optional version override.

The shared release path runs `make fmt`, `make lint`, `make test`, `make test-race`
and `make build VERSION=<version>` **before creating or pushing a tag**. A failed
gate stops the release; an existing version tag must point at `HEAD` and is not
force-replaced. GoReleaser then packages with `--skip=publish`, and
[deploy/publish-release.sh](../../deploy/publish-release.sh) verifies the local
archives against `dist/checksums.txt` before the tag is pushed. It creates a draft
release with checksums first, uploads archives, then makes the release public.
A publishing failure can leave a pushed tag or draft; inspect remote state before
retrying. The workflow pins its release-tool versions; local release execution
requires those tools plus `gh`, `sha256sum`, Git and Bash.

`make release` is a mutating publishing command and requires explicit authorization.
Formatting/linting and GoReleaser hooks can change the working tree; the release
script rejects tracked changes against `HEAD` after the gates and again after
packaging, rather than publishing uncommitted fixes. Snapshot and release builds
use `--clean` and clear `dist/`. `make release-check` only validates
GoReleaser configuration; it neither exercises the release gates nor publishes.
A successful cross-build is not native Windows runtime verification.

## TDD

Every feature follows red-green-refactor:

1. Write a test describing the required behaviour — it must fail.
2. Write the minimum code that makes it pass.
3. Refactor while keeping the tests green.

```bash
make check
```

Committing with open linter findings is not allowed.

## Tests

- Unit tests are fast and isolated, and live next to the code they cover.
- Integration tests use the `integration` build tag:

```bash
make test-integration
```

Unit tests live next to their packages and do not require provider credentials or a
running LLM. Black-box tests live in `e2e/` behind the `e2e` tag; `make test-e2e`
builds and exercises the actual CLI with isolated homes and local HTTP fixtures.
Use `make test-race` separately for concurrency checks. Do not run manual tests
without explicit authorization.

Hardening tests cover provider response limits, history/terminal validation,
cooperative file updates and post-commit errors, confined script paths and stale
editor saves, plus cancellation through the real CLI-to-HTTP path. Unix child tests
also exercise cleanup after normal leader exit, not only cancellation. Windows
uses suspended-start job assignment before resume, but a cross-build does not
exercise native process jobs, reparse-point handling or installer behavior. Unix
children that detach into another group/session and externally brokered processes
remain outside supervision. See [CONFIGURATION.md](CONFIGURATION.md) for the
storage, execution and recovery boundaries.

## Project conventions

The repository profile is in [PROJECT.md](../../PROJECT.md). Local agent rules
are in `AGENTS.md` and `CLAUDE.md`. The short version:

- `cmd/*` is the composition root only — business logic goes into
  `internal/entities/*/service`.
- Vertical slices plus ports and adapters; interfaces are declared by consumers.
- No one-line wrapper functions and no speculative abstractions.
- Functions with more than three arguments take a named parameter struct, with
  `context.Context` always first and never inside the struct.
- CLI routing and presentation live in `internal/cli`; `cmd/zev` is a thin launcher.
- Locale call sites use named constants from `internal/core/locale/keys.go`; translated text lives
  in the English/Russian TOML catalogs. Technical guides belong in `docs/`.

## Architecture reference

See [ARCHITECTURE.md](ARCHITECTURE.md) for the package map and the query flow.
