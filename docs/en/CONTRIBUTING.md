<div align="center">

**English** | [Русский](../ru/CONTRIBUTING.md)

</div>

# Contributing

Thanks for taking the time to contribute.

## Before you start

1. Open an issue describing the problem or the feature, so the approach can be agreed on
   before you write code.
2. For anything touching the provider layer, the TUI or the config format, write a short
   design first. Large refactors that restructure how components fit together need a
   written spec, not a patch.

## Workflow

```bash
git clone https://github.com/ra1phdd/zev.git
cd zev
make check
```

- Branch from `main` with a descriptive name (`feat/add-anthropic-provider`).
- One logical change per commit; write tests first (TDD).
- Run `make check` before pushing: `golangci-lint run --fix` and `go test ./... -race`
  must both be clean.
- Open a PR describing what changed and why, and link the issue.

## Code style

The authoritative rules live in [AGENTS.md](../../AGENTS.md) and
[CLAUDE.md](../../CLAUDE.md). The essentials:

- Vertical slices with ports and adapters. Business logic belongs in
  `internal/entities/*/service`; `cmd/*` only wires things together.
- Define interfaces on the consumer side, keep them small.
- No one-line wrapper functions, no abstractions added "for later".
- Functions with more than three arguments take a named parameter struct;
  `context.Context` is always the first argument and never a struct field.
- Errors are wrapped with `fmt.Errorf("...: %w", err)`; no ignored errors.
- Documentation comments start with the entity name.
- All user-facing strings go through `internal/core/i18n`.

## Tests

- New behaviour needs a test. Use table-driven tests where several scenarios apply.
- Unit tests must be fast and isolated; integration tests carry the `integration` build
  tag.
- Providers are tested with `httptest` servers — no network calls in tests.

## Documentation

- Documentation is bilingual: `docs/en/` and `docs/ru/`, UPPERCASE filenames, with a
  language header and cross-links that stay inside one language.
- Update both languages in the same commit.

## License

By contributing you agree that your work is licensed under the MIT License.
