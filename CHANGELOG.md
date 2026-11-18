# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Add managed aliases and scripts with strict v2 metadata, argv-safe execution, descriptions,
  ordered positional arguments, CRUD/help/machine flows, and crash-recoverable migration/publication.
  Legacy data is archived only after verified migration; keep a backup before upgrading and do not
  downgrade to a version that cannot read the v2 store. Optional metadata generation is explicit and
  never required for successful migration. Post-migration bulk enrichment and its one-time
  startup consent are deferred to a follow-up release slice.

- Add contributor onboarding guides in English and Russian, a pull request template,
  issue templates and a GitHub Actions verification workflow.

- Add English and Russian CLI/TUI localization using embedded TOML catalogs in `locale/` and `go-i18n`.

- Add `zev continue` and `zev query --continue` to restore the latest usable conversation with optional project context and command-error input.
- Add `zev alias edit`, `zev script edit`, and safe `zev script show` commands using the shared editor resolver.
- Add repeated `--context-file`/`-cf` query inputs with bounded, sectioned project context,
  fail-closed path and sensitive-file checks, and machine-mode stdout preservation.
- Add `zev doctor` with local-only provider/configuration diagnostics, stable JSON checks,
  explicit actionable messages, secret-safe output, and opt-in confirmed provider verification.
- Add persistent SQLite history with incremental conversation lifecycle records and
  no automatic count-based retention.
- Add explicit legacy JSONL detection and migration through `history/migrator`, with
  verified archival, recovery, and no migration side effects in `repository.Open`.
  Local lexical search and CLI semantic/hybrid search remain available; vector input is
  limited to user messages and refinements.
- Add history inspection/export/deletion commands, request-bound `--cursor` pagination,
  index status and explicit lexical/vector rebuilds; vector transmission displays
  provider/model/eligible count and requires `--yes` consent.
- Add machine-readable `--print`, `--json` and `--option N` query output with stable
  exit codes and no selector, terminal injection, shell execution or command execution.
- Add `zev alias last <alias-name>` to save the latest successfully selected history
  command into the existing YAML alias store without invoking an LLM or network.
- Add bilingual user documentation for history storage, search, migration, privacy
  and embedding configuration.
- Add native model `exec` tools with per-call manual approval, root-confined read-only
  auto execution plus isolated model safety review, and explicit unrestricted bypass.
  Configure with `TOOLS_ENABLED`/`TOOL_APPROVAL_MODE` or CLI flags; machine output
  requires explicit `--tools` and retains its stdout and final-command delivery contracts.
- Add a demo animation to the English and Russian READMEs.
- Add an explicit uninstall prompt for removing the user `.zev` configuration and data directory;
  default uninstall keeps it, while `ZEV_REMOVE_CONFIG=1` opts into deletion.
- Document the repository's actual architecture, configuration, storage, release
  workflow and operational limitations in `PROJECT.md`.
- Add explicit formatting, race-test and tagged-test Make targets, with manual-test
  authorization and CI guards, formatter/tool preflight checks and deadcode reporting.
- Harden tests against ambient configuration and deployment environment, make Windows
  permission assertions platform-aware, strengthen YAML/lexical/security boundaries,
  remove tautological domain tests, and fail the manual gate when no manual tests exist.
- Add offline metadata boundary/provider tests, persisted setup coverage for all five providers,
  and built-binary metadata generation/fallback and Unix alias execution checks.

### Changed

- Move localization Go code into `internal/core/locale`, keep only TOML catalogs in `locale/`,
  and rename the query catalogs to `query.en.toml` and `query.ru.toml`.
- Refresh the README demos with separate English and Russian GIFs, unified JetBrains Mono Bold
  typography (16 pt commands and 14 pt secondary text), macOS-style window controls,
  non-overlapping command prompts and full command-description selection.
- Detect query UI language locally with `ZEV_LANG=auto` (Russian or English fallback),
  with persistent `ZEV_LANG=ru|en` overrides and unchanged machine output.
- Repair legacy history permissions silently before import and archive recovery;
  fail only when repair or subsequent validation cannot complete.
- Stop treating pipeline/chain separators alone as dangerous; retain operation checks
  inside every stage and inspect nested `find -exec` actions.
- Keep `cmd/zev` as a thin launcher and move CLI routing/presentation into `internal/cli`.
- Use named localization keys instead of English source strings; preserve existing
  translations and machine-readable output.
- Run legacy JSONL migration explicitly from CLI history startup using a separate
  migrator; opening the SQLite repository no longer imports or archives legacy data.
- Move black-box CLI and migration-concurrency tests into `e2e/`; `make test-e2e`
  builds and exercises the actual executable. Move the tool adapter guide into docs.
- Move deployment-script integration tests into `e2e/deploy/` and include them in
  `make test-e2e`; preserve `make test-integration` as an alias for that gate.
  Deployment fixtures require Bash and PowerShell explicitly, outside the unit suite.
- Add opt-in `HISTORY_SEMANTIC_ENABLED=false` and separate OpenAI-compatible embedding
  settings, including the 32768-byte input limit and remote-text disclosure. Both query
  recording and explicit history commands invoke the explicit legacy migrator when needed.
  No automatic vector backfill or history-to-prompt RAG is enabled.
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

### Removed

- Remove unused Go TUI wrappers `list.Select` and `prompt.Input`; Go callers should
  pass their context to `list.SelectContext` and `prompt.InputContext` instead.
  The CLI commands and machine-output contracts are unchanged.

### Fixed

- Fix terminal command injection on macOS: the kernel encodes the ioctl argument size
  in the request, so TIOCSTI is `_IOW('t', 114, char)` (0x80017472) on Darwin instead
  of the Linux value; the Linux encoding failed with ENOTTY and every `inject` fell
  back to printing the command.
- Keep provider redirects on the initial origin so configured authentication headers
  cannot be reattached to another host, port, or scheme; snapshot custom header maps.
- Fail closed when the home directory is missing or relative instead of trusting a
  project-local `.zev` configuration, alias store, or script store.
- Pin context-file reads beneath the allowed root and validate opened identities and
  byte budgets; require sensitive-context opt-in for credential directory components.
- Reject stale alias editor saves and return alias lists in stable name order.
- Keep multiline script descriptions as comments, preserve UTF-8 in narrow terminal
  rendering, avoid duplicate prompts, and display literal `::` commands accurately.
- Reject conflicting doctor output formats before configuration I/O, validate search
  limits and whitespace-only exact commands before opening history, and expose the
  context-file flags in localized query help.
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
- Classify migration archive publication marker failures as committed-but-unknown,
  remove completed migration markers during recovery, and reject shell re-evaluation
  and Bash reserved-word alias templates before persistence.
- Refuse Windows script execution until a verified handle-safe Bash transport exists;
  do not pass script bodies through a shell command-line fallback.
- Build SQLite file URIs correctly for Windows drive and UNC paths so migrated
  history databases open and persist query history on native Windows.
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
