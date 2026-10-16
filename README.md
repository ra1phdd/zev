<div align="center">
<h1>Zev</h1>
<p>Terminal command suggestions from natural language — describe what you want, get the command</p>

<p>
    <img src="https://img.shields.io/badge/Go-1.27+-00ADD8?style=flat&logo=go&logoColor=white" alt="Go">
    <img src="https://img.shields.io/badge/platform-linux%20%7C%20macOS%20%7C%20Windows-lightgrey?style=flat" alt="Platforms">
    <img src="https://img.shields.io/badge/license-MIT-green" alt="License">
</p>

**English** | [Русский](README.ru.md)
</div>

## Features

- Natural language in, terminal commands out — up to three options with a short
  explanation for each.
- Dangerous operations (`rm -rf`, `DROP TABLE`, `kubectl delete`) are flagged with a
  warning explaining exactly what the command will do.
- Five LLM providers: OpenAI, Gemini, Anthropic, Ollama and OpenCode.
- Custom base URLs — works with any OpenAI-compatible router.
- Interactive TUI selector: pick a suggestion with arrow keys, get it injected into your
  terminal or executed immediately.
- Command aliases with positional argument substitution — `zev dps my-service` runs your
  saved long command.
- Bash scripts with metadata, created straight in `$EDITOR` from the CLI.
- Query history (last 100 requests) available offline in `~/.zevhistory`.
- Verbosity control with `-v`, `-vv`, `-vvv`.

## Quick start

```bash
# Clone and build
git clone https://github.com/ra1phdd/zev-go.git
cd zev-go
make build

# Configure your LLM provider (interactive)
./bin/zev setup

# Ask for a command
./bin/zev "find all files modified in the last 24 hours"
```

Without the `query` subcommand, `zev` starts its interactive prompt.

## Usage

### Query

```bash
zev "kill the process listening on port 8080"
zev query "list all docker containers by size"
```

### Choose what happens to the suggestion

```bash
zev --inject "compress the logs"   # put it in the terminal for review (default)
zev --execute "restart nginx"      # run it right away
```

The default comes from `COMMAND_EXECUTION_MODE` and can be changed with `zev setup mode`.

### History

```bash
zev history          # browse previous queries
```

### Aliases

Store a long command once and call it by name. Extra arguments are appended
automatically, or substituted into the template:

```bash
zev alias add dps "docker service ps {{.arg1}} --no-trunc"
zev dps my-service

zev alias list
zev alias remove dps
```

Template variables: `{{.arg1}}`, `{{.arg2}}`, … for positional arguments and `{{.args}}`
for all arguments joined with spaces.

### Scripts

```bash
zev script create deploy "Deploy the application to production"
zev script list
zev script remove deploy
```

Metadata is stored in `~/.zev/scripts`, the script bodies — in `~/.zev/script_files`.
`zev script create` opens the file in `$SCRIPT_EDITOR`, `$EDITOR`, or the first available
of `nano`, `vim`, `vi`.

### Other

```bash
zev version
zev -vv "tail the logs in real time"   # debug logs
```

## Installation

### Precompiled binaries

Precompiled binaries will be published on the [releases page](https://github.com/ra1phdd/zev-go/releases)
once the release pipeline is in place. Until then, build from source.

### Building from source

Requirements: Go 1.27.2 or newer.

```bash
git clone https://github.com/ra1phdd/zev-go.git
cd zev-go

# Local binary
make build            # -> bin/zev

# Or install into $GOPATH/bin
make install
```

## Providers

| Provider | `LLM_PROVIDER` | Required variables |
|---|---|---|
| OpenAI | `openai` | `OPENAI_API_KEY`, `OPENAI_MODEL`, `OPENAI_BASE_URL` |
| Gemini | `gemini` | `GEMINI_API_KEY`, `GEMINI_MODEL` |
| Anthropic | `anthropic` | `ANTHROPIC_API_KEY`, `ANTHROPIC_MODEL` |
| Ollama | `ollama` | `OLLAMA_BASE_URL`, `OLLAMA_MODEL` |
| OpenCode | `opencode` | `OPENAI_API_KEY`, `OPENAI_MODEL`, `OPENAI_BASE_URL` |

Ollama runs fully locally:

```bash
export LLM_PROVIDER=ollama
export OLLAMA_BASE_URL=http://localhost:11434/v1
export OLLAMA_MODEL=qwen2.5-coder
```

## Documentation

- [Configuration](docs/en/CONFIGURATION.md) — `~/.zevrc`, environment variables, execution
  modes, files on disk
- [Architecture](docs/en/ARCHITECTURE.md) — package map, query flow, provider contracts
- [Development](docs/en/DEVELOPMENT.md) — make targets, TDD workflow, conventions
- [Contributing](docs/en/CONTRIBUTING.md) — how to propose and land a change

## Technology stack

- Go 1.27+, Cobra for the CLI
- Bubble Tea + Lipgloss for the terminal UI
- [go-openai](https://github.com/sashabaranov/go-openai) for OpenAI-compatible providers,
  raw HTTP for Gemini, Anthropic and OpenCode (SSE streaming)
- `godotenv` + `caarlos0/env` for configuration
- `golangci-lint` and `go test` in CI

## Contributing

Contributions are welcome — read the [Contributing Guide](docs/en/CONTRIBUTING.md).

## License

This project is licensed under the MIT License — see the [LICENSE](LICENSE) file.
