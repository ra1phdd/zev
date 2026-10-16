<div align="center">
<h1>Zev</h1>
<p>Terminal command suggestions from natural language — describe what you want, get the command</p>

<p>
    <img src="https://img.shields.io/badge/Go-1.27+-00ADD8?style=flat&logo=go&logoColor=white" alt="Go">
    <img src="https://img.shields.io/badge/platform-linux%20%7C%20macOS%20%7C%20Windows-lightgrey?style=flat" alt="Platforms">
    <img src="https://img.shields.io/badge/license-MIT-green" alt="License">
</p>

![Zev in action](assets/zev.gif)

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

### Linux and macOS

Install the latest release with one command — the script picks the archive for your OS
and architecture, verifies its SHA-256 checksum and installs it:

```bash
# curl
curl -fsSL https://raw.githubusercontent.com/ra1phdd/zev/main/deploy/install.sh | bash

# or wget
wget -qO- https://raw.githubusercontent.com/ra1phdd/zev/main/deploy/install.sh | bash
```

Both install into `/usr/local/bin` with `sudo` when you have it, or fall back to
`$HOME/.local/bin` without root. To choose the directory yourself, set `PREFIX`:

```bash
PREFIX=$HOME/.local/bin curl -fsSL https://raw.githubusercontent.com/ra1phdd/zev/main/deploy/install.sh | bash
```

The script uses `curl` when available and falls back to `wget`. To install a specific
version, pass `-v`:

```bash
curl -fsSL https://raw.githubusercontent.com/ra1phdd/zev/main/deploy/install.sh | bash -s -- -v 1.0.0
```

### Windows

```powershell
irm https://raw.githubusercontent.com/ra1phdd/zev/main/deploy/install.ps1 | iex
```

Or download [`install.ps1`](deploy/install.ps1) and run `.\install.ps1`. It installs into
`%LOCALAPPDATA%\Programs\zev` and needs no administrator rights.

### Then configure a provider

```bash
zev setup                          # interactive LLM provider setup
zev "find all files modified in the last 24 hours"
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

## Uninstallation

To remove zev, use the uninstaller scripts:

```bash
# Linux and macOS
curl -fsSL https://raw.githubusercontent.com/ra1phdd/zev/main/deploy/uninstall.sh | bash
```

```powershell
# Windows
irm https://raw.githubusercontent.com/ra1phdd/zev/main/deploy/uninstall.ps1 | iex
```

Or manually remove the binary:

```bash
rm /usr/local/bin/zev      # system-wide (Linux/macOS)
rm $HOME/.local/bin/zev    # user-local (Linux/macOS)
```

```cmd
REM Windows
del %LOCALAPPDATA%\Programs\zev\zev.exe
```

## Contributing

Contributions are welcome — read the [Contributing Guide](docs/en/CONTRIBUTING.md).

## License

This project is licensed under the MIT License — see the [LICENSE](LICENSE) file.
