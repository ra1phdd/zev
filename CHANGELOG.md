# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Add a demo animation to the English and Russian READMEs.
- Document the repository's actual architecture, configuration, storage, release
  workflow and operational limitations in `PROJECT.md`.
- Add explicit formatting, race-test and tagged-test Make targets, with manual-test
  authorization and CI guards, formatter/tool preflight checks and deadcode reporting.

### Changed

- Reduce the README demo display width to 346 pixels.
- Use `-s -w` for local Make builds to omit symbol tables and DWARF debug information,
  matching GoReleaser and reducing executable size while retaining the build version;
  remove local paths with `-trimpath` and omit the linker build ID.
- Separate `go vet` and default unit tests from the optional race-test target;
  limit cleanup to the selected binary and its cross-compiled variants.

### Fixed

- Correct the README HTML header layout and Russian language switcher, and
  document the history location as `~/.zev/history`.
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
