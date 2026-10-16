<div align="center">

**English** | [Русский](../ru/DEVELOPMENT.md)

</div>

# Development

## Requirements

- Go 1.27.2 or newer
- [golangci-lint](https://golangci-lint.run/) for linting
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
| `make test` | `go test ./... -race` |
| `make lint` | `golangci-lint run --fix` |
| `make check` | Lint + tests |
| `make run` | Build and start interactive mode |
| `make setup` | Build and run the setup wizard |
| `make dev` | Run with debug logs |
| `make watch` | Restart on file changes (requires `entr`) |
| `make build-all` | Cross-compile for Linux, macOS and Windows |
| `make clean` | Remove build artifacts |

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
go test -tags integration ./...
```

## Project conventions

The full rules live in [AGENTS.md](../../AGENTS.md) and [CLAUDE.md](../../CLAUDE.md).
The short version:

- `cmd/*` is the composition root only — business logic goes into
  `internal/entities/*/service`.
- Vertical slices plus ports and adapters; interfaces are declared by consumers.
- No one-line wrapper functions and no speculative abstractions.
- Functions with more than three arguments take a named parameter struct, with
  `context.Context` always first and never inside the struct.
- All user-facing text goes through `internal/core/i18n` (not yet wired for the CLI
  itself, but the rule stands for new packages).

## Architecture reference

See [ARCHITECTURE.md](ARCHITECTURE.md) for the package map and the query flow.
