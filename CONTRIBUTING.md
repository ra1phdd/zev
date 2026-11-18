<div align="center">

**English** | [Русский](CONTRIBUTING.ru.md)

</div>

# Contributing to Zev

Thank you for taking the time to improve Zev. Contributions of code, tests,
documentation, issue reports, examples, and product feedback are welcome.

## Before you start

1. Search existing issues and pull requests before opening a new discussion.
2. For a bug, use the bug report template and include a minimal safe reproduction.
3. For a new feature, describe the user problem, proposed behaviour, and alternatives.
4. Open an issue before implementing a non-trivial feature or a change to providers,
   the TUI, configuration, storage, command execution, security, or architecture.
   This avoids investing in an approach that cannot fit the project.
5. Small documentation and typo fixes may go directly to a pull request.

Never include API keys, tokens, private command history, personal data, or other
secrets in an issue, test fixture, commit, or pull request. If you believe you have
found a security vulnerability, do not disclose the details publicly; contact the
maintainer through a private GitHub channel first.

## Development setup

Zev is a Go 1.27.2+ CLI application. The command reference is in
[`docs/en/DEVELOPMENT.md`](docs/en/DEVELOPMENT.md); repository facts and known
limitations are in [`PROJECT.md`](PROJECT.md).

Required tools for the full local gate:

- Go 1.27.2 or newer;
- `gofmt` (included with Go), `gofumpt`, and `goimports`;
- `golangci-lint` v2 and `deadcode`;
- Git and Bash for the repository scripts.

Optional tools include GoReleaser v2 for release checks and `entr` for `make watch`.
Provider credentials and network access are not required for unit tests.

```bash
git clone https://github.com/ra1phdd/zev.git
cd zev
make build
make run ARGS=version
```

## Branches and commits

- Fork the repository and branch from `main`.
- Keep a branch short-lived and focused on one logical change.
- Use a descriptive branch name, for example:
  - `feat/add-provider`
  - `fix/history-migration`
  - `docs/improve-installation`
  - `refactor/query-service`
- Keep commits small and reviewable. Separate refactoring, behaviour changes, and
  documentation-only changes when they can be reviewed independently.
- Use Conventional Commits with an English message, for example:
  `fix: preserve provider headers across retries`.
- Do not commit generated binaries, `dist/`, local configuration, provider keys, or
  unrelated formatting changes.

## Project conventions

Please read [`docs/en/ARCHITECTURE.md`](docs/en/ARCHITECTURE.md) before making a
structural change. The most important rules are:

- `cmd/*` is the composition root; business logic belongs in the relevant
  `internal/entities/*/service` package.
- Use vertical slices with ports and adapters. Define small interfaces on the
  consumer side.
- Do not add one-line wrappers or speculative abstractions.
- Functions with more than three meaningful arguments should use a named parameter
  struct. `context.Context` is always the first argument and is not put in that struct.
- Wrap errors with `%w` and handle returned errors explicitly.
- Keep CLI routing and presentation in `internal/cli`; keep `cmd/zev` as a thin launcher.
- Use named localization keys from `internal/core/locale/keys.go`; translated text
  belongs in the English and Russian TOML catalogs.
- Comments in source code are written in English. Documentation outside source code
  may be written in either supported language.

## Tests and local checks

New or changed behaviour needs tests. Prefer a test-first workflow and table-driven
tests for related scenarios. Provider tests use local `httptest` servers; unit tests
must not call real provider endpoints.

Run the focused checks while developing, then the complete local gate before opening
a PR:

```bash
# Apply Go formatting; inspect the diff afterwards.
make fmt

# Formatting, golangci-lint, deadcode report, go vet and unit tests.
make check

# Concurrency checks.
make test-race

# Build the CLI into bin/zev.
make build

# Optional black-box checks with an isolated HOME and local fixtures.
make test-e2e

# Remove the binaries created by local builds.
make clean
```

`make lint` runs `golangci-lint run --fix`, so it can change Go files. Always review
`git diff` after it runs. The `deadcode` output is a report to investigate, not an
automatic instruction to delete exported or platform-specific code.

Integration, e2e, and manual tests are separate from the default unit suite. The repository's
black-box integration scenarios live under `e2e/` and use the `e2e` build tag; the legacy
`make test-integration` target is a compatibility alias for `make test-e2e`. Manual tests
require explicit authorization and must not run in CI. If no tests carry the `manual`
tag, the target fails clearly instead of reporting a false green result:

```bash
make test-integration
make test-e2e
make test-manual ALLOW_MANUAL_TESTS=1
```

A successful cross-compilation does not prove native Windows runtime behaviour.
Document any platform-specific verification that you could not perform.

## Documentation changes

User-facing documentation is maintained in both languages:

- English: `docs/en/`
- Russian: `docs/ru/`

Use matching uppercase filenames, keep the language switcher at the top, and update
both versions in the same pull request. Update `README.md`, `README.ru.md`, or
`CHANGELOG.md` when the change affects discoverability or user-visible behaviour.

## Opening a pull request

1. Rebase or update your branch from `main` if it has become stale.
2. Review the complete diff, including untracked files and generated files.
3. Confirm that the PR contains one logical change and no secrets.
4. Run the checks listed above and record the exact commands in the PR description.
5. Open the PR against `main` and fill in every applicable section of the template.
6. Link the issue with `Fixes #123` or `Refs #123` where applicable.

A useful PR description answers:

- What problem does this solve?
- What changed, and why was this approach chosen?
- Which tests and manual checks were run?
- Does it change configuration, stored data, CLI output, provider behaviour, or
  security boundaries?
- Are there compatibility concerns, migration steps, screenshots, or known limits?

For user-facing CLI or TUI changes, include before/after examples or screenshots when
that makes the result easier to review. For a breaking change, call it out explicitly
and explain how users can migrate.

## Review process

Maintainers may ask for a smaller scope, tests, documentation, or a design discussion
before reviewing implementation details. Keep review replies focused on the code and
update the PR when feedback is addressed. A passing local check is necessary but does
not replace review of correctness, security, compatibility, and user experience.

Do not force-push after review if it would make the discussion difficult to follow;
when a force-push is useful, explain what changed. Maintainers may squash commits when
merging to keep the main history readable.

## License

By contributing, you agree that your work is provided under the [MIT License](LICENSE).
