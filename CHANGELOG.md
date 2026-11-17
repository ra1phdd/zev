# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Add `zev alias edit`, `zev script edit`, and safe `zev script show` commands using the shared editor resolver.
- Add repeated `--context-file`/`-cf` query inputs with bounded, sectioned project context,
  fail-closed path and sensitive-file checks, and machine-mode stdout preservation.
- Add `zev doctor` with local-only provider/configuration diagnostics, stable JSON checks,
  explicit actionable messages, secret-safe output, and opt-in confirmed provider verification.
- Add persistent SQLite history with incremental conversation lifecycle records and
  no automatic count-based retention.
- Add automatic repository-level legacy JSONL import and verified archival, local lexical
  search with morphology and bounded typo tolerance, and CLI semantic/hybrid search
  whose vector component is limited to user messages and refinements.
- Add history inspection/export/deletion commands, request-bound `--cursor` pagination,
  index status and explicit lexical/vector rebuilds; vector transmission displays
  provider/model/eligible count and requires `--yes` consent.
- Add machine-readable `--print`, `--json` and `--option N` query output with stable
  exit codes and no selector, terminal injection, shell execution or command execution.
- Add `zev alias last <alias-name>` to save the latest successfully selected history
  command into the existing YAML alias store without invoking an LLM or network.
- Add bilingual user documentation for history storage, search, migration, privacy
  and embedding configuration.
- Add a demo animation to the English and Russian READMEs.
- Document the repository's actual architecture, configuration, storage, release
  workflow and operational limitations in `PROJECT.md`.
- Add explicit formatting, race-test and tagged-test Make targets, with manual-test
  authorization and CI guards, formatter/tool preflight checks and deadcode reporting.

### Changed

- Add opt-in `HISTORY_SEMANTIC_ENABLED=false` and separate OpenAI-compatible embedding
  settings, including the 32768-byte input limit and remote-text disclosure. Both query
  recording and explicit history commands use automatic legacy migration. No automatic
  vector backfill or history-to-prompt RAG is enabled.
- Synchronize bilingual project, architecture, configuration and README documentation
  with usable SQLite history commands and verified implementation limits.
- Add opt-out privacy controls for provider context and query history, mask setup
  secrets and custom header values, and redact them from setup output.
- Make Unix and PowerShell release installers fail closed when SHA-256 metadata or
  verification tools are unavailable, malformed, duplicated or mismatched.
- Run tests, lint and build before the release publishing step; publish checksums
  in a draft before the release artifacts are made public. The shared local/CI
  release path also runs formatting and race tests before tag creation/push and
  refuses tracked source changes after gates or packaging hooks.
- Reduce the README demo display width to 346 pixels.
- Use `-s -w` for local Make builds to omit symbol tables and DWARF debug information,
  matching GoReleaser and reducing executable size while retaining the build version;
  remove local paths with `-trimpath` and omit the linker build ID.
- Separate `go vet` and default unit tests from the optional race-test target;
  limit cleanup to the selected binary and its cross-compiled variants.

### Fixed

- Preserve immutable creation retries, observed execution outcomes, command timestamps
  during FTS rebuilds, and pagination after semantic fallback; protect existing SQLite
  sidecars and migration archives with private POSIX permissions or Windows DACLs.
- Apply history document filters before bounded FTS candidate limits, preserve source
  UUIDs in search hits, require all query terms across morphology/typo expansion, and
  retain exact-match priority with deterministic ranking and derived-index cleanup.
- Validate provider responses and persisted history before command selection or
  delivery, with fixed byte/count limits and bounded provider response bodies;
  reject terminal control characters before injecting any input.
- Serialize configuration and alias read-modify-write operations with cooperative
  cross-process locks and atomic file replacement; use SQLite WAL transactions for
  history. Preserve malformed existing data and distinguish post-replacement durability
  errors from failures before commit.
- Confine saved-script paths, reject unsafe links, execute pinned script files and
  edit private temporary copies; atomically replace edited files and reject stale
  editor saves rather than overwriting a newer cooperating edit. Serialize script
  metadata/body changes with rollback for pre-commit failures, without claiming
  crash-atomic multi-file transactions.
- Propagate CLI signal cancellation through provider calls, retry waits, TUI input,
  editors and command execution; clean up owned process groups/jobs and preserve
  child exit errors. Detached Unix children and externally brokered processes are
  outside the ownership boundary; native Windows runtime validation remains needed.
- Correct the README HTML header layout and Russian language switcher, and
  document the legacy JSONL location as `~/.zev/history` (SQLite uses `~/.zev/history.db`).
- Complete the Makefile migration to `GO`, `APP` and `BIN`: repair run/setup and
  cross-platform build paths, honor overrides consistently, support forwarded CLI
  arguments, restore target help and phony declarations, and validate cleanup paths.
- Enable development debug logging with the supported `-vv` flag instead of the
  unused `LOG_LEVEL` environment variable.
- Synchronize bilingual development and configuration documentation with the
  Make targets and actual on-disk paths, environment keys and execution behavior.

## [1.0.0] - 2026-10-17

### Added

- Go CLI for natural-language terminal command suggestions, with explanations,
  LLM-provided danger warnings and an interactive setup wizard.
- OpenAI, Gemini, Anthropic, Ollama and OpenCode providers, including configurable
  API base URLs and support for OpenAI-compatible endpoints.
- Custom HTTP headers through `HTTP_HEADERS` and `zev setup headers`; a shared
  HTTP client with request timeout and diagnostic logging.
- OpenCode streaming support with SSE parsing, the required tool definition and
  usage-stream options, plus provider-specific setup defaults.
- Multi-turn query refinement with conversation context and clearer prompts when
  the provider returns no usable response.
- Interactive query-history browsing with the selected command highlighted;
  local JSONL storage retains the last 100 entries.
- Command aliases with positional templates, `{{.args}}` and automatic argument
  appending when a template does not consume arguments.
- Saved bash scripts with YAML metadata, CLI management, direct execution and
  configurable editor selection through `SCRIPT_EDITOR` or `EDITOR`.
- Terminal-input injection for reviewing suggestions before execution, a printed
  fallback when injection is unavailable, and configurable `inject`/`execute` modes.
- CLI version reporting and verbosity flags `-v`, `-vv` and `-vvv`.
- Make targets for development, tests, linting, installation and cross-platform builds;
  unit coverage for services, configuration, flag parsing, terminal execution,
  response parsing and TUI rendering.
- VERSION-driven GitHub releases through GoReleaser, producing checksummed archives
  for Linux amd64/arm64/armv7, macOS amd64/arm64 and Windows amd64/arm64.
- Unix and PowerShell installation/uninstallation scripts, with platform detection,
  version selection and SHA-256 verification support on installation.
- English and Russian documentation for usage, architecture, configuration,
  development and contribution, plus the MIT license.

### Changed

- Replace the Python implementation with Go and organize code into vertical slices
  with ports and adapters; publish the module as `github.com/ra1phdd/zev`.
- Load dotenv configuration with `godotenv` and `caarlos0/env`, preserving the
  priority of existing environment variables over file settings. Store configuration
  in `~/.zev/config` and history in `~/.zev/history` instead of the earlier
  `~/.zevrc` and `~/.zevhistory` paths; old files are not migrated automatically.
- Make the TUI compact and inline rather than full-screen, display all choices
  without pagination, and wrap lists and input prompts for narrow terminals.
- Improve system prompts to distinguish independent command alternatives from
  sequential steps and include the user's environment in conversation context.
- Execute resolved aliases and saved scripts directly rather than only printing them.
- Update default OpenAI and Gemini models to `gpt-5.6-luna` and `gemini-3.1-flash-lite`.

### Fixed

- Extract structured provider responses from Markdown fences and `<escape>` tags
  before JSON decoding across provider adapters.
- Restore manual verbosity and execution-flag parsing while preserving other
  arguments and flags passed to aliases.
- Parse custom HTTP header values containing colons and preserve header-name case.
- Separate script metadata in `~/.zev/scripts` from script bodies in
  `~/.zev/script_files`, avoiding a file/directory name collision.
- Correct OpenCode request headers and setup defaults for its streaming endpoint.
- Correct the default Anthropic base URL to include `/v1`.
- Fix Windows compilation by separating Unix input injection from the Windows
  implementation, which falls back to displaying the selected command.
- Handle user cancellation without misleading TUI errors; improve visible list
  sizing and provider/network error reporting.
- Correct GoReleaser footer templating and remove the redundant installation footer
  from generated release notes.

### Removed

- Remove the bundled legacy Python tree and its obsolete packaging/release assets.

[Unreleased]: https://github.com/ra1phdd/zev/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/ra1phdd/zev/releases/tag/v1.0.0
