<div align="center">
<h1>Zev</h1>
<p>Terminal command suggestions from natural language — describe what you want, get the command</p>

<p>
    <img src="https://img.shields.io/badge/Go-1.27+-00ADD8?style=flat&logo=go&logoColor=white" alt="Go">
    <img src="https://img.shields.io/badge/platform-linux%20%7C%20macOS%20%7C%20Windows-lightgrey?style=flat" alt="Platforms">
    <img src="https://img.shields.io/badge/license-MIT-green" alt="License">
    <a href="https://github.com/ra1phdd/zev/actions/workflows/ci.yml"><img src="https://github.com/ra1phdd/zev/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
</p>

**English** | [Русский](README.ru.md)
</div>

## Features

- Turn natural-language requests into terminal command suggestions with explanations.
- Inject selected suggestions into the terminal for review without pressing Enter.
- Save frequently used commands as aliases, with positional arguments.
- Create and run reusable Bash scripts.
- Supports OpenAI, Gemini, Anthropic, Ollama, and OpenCode providers.

## Quick Start

Install Zev using the instructions below, then configure a provider:

```sh
zev setup
zev "find files modified in the last 24 hours"
```

By default, Zev injects the selected command into your terminal for review. It does not press Enter. To make injection explicit:

```sh
zev --inject "show running Docker containers"
```

## Installation

### Binaries

Download the latest release from [GitHub Releases](https://github.com/ra1phdd/zev/releases), or install on Linux/macOS with:

```sh
curl -fsSL https://raw.githubusercontent.com/ra1phdd/zev/main/deploy/install.sh | bash
```

On Windows, run in PowerShell:

```powershell
irm https://raw.githubusercontent.com/ra1phdd/zev/main/deploy/install.ps1 | iex
```

Install scripts require SHA-256 checksum verification for the downloaded archive.

### Docker

Build your own container image around the binary and mount the Zev configuration directory as needed to retain provider settings. Interactive terminal injection depends on the host terminal and is not provided by a container.

### Source

Requires the Go version listed in [`go.mod`](go.mod):

```sh
go install github.com/ra1phdd/zev/cmd/zev@latest
```

## Typical Usage

### Natural-language query and inject

```sh
zev "find files larger than 100 MB"
zev "show the current Git branch"
```

Review the suggestion before running it. Zev inserts the command but does not press Enter.

### Alias

Save a reusable command template, then invoke its name with its argument at the end:

```sh
zev alias add dps "docker service ps {{.arg1}} --no-trunc"
zev dps my-service
```

### Script

Create a named Bash script, edit its body, then invoke it by name:

```sh
zev script create deploy "Deploy the application"
zev deploy
```

## Documentation

- [Full CLI reference](docs/en/CLI.md) — command tree, arguments, every local and inherited flag, defaults, aliases, hidden flags, and shell completion.
- [Configuration](docs/en/CONFIGURATION.md)
- [Security](docs/en/SECURITY.md) — implemented policy, tool boundaries, web fetch and privacy limits.
- [History](docs/en/HISTORY.md)
- [Aliases and scripts](docs/en/ALIASES-SCRIPTS.md)
- [`zev run` wrapper](docs/en/RUN-WRAPPER.md)
- [Updating](docs/en/UPDATE.md)
- [Architecture](docs/en/ARCHITECTURE.md)
- [Development](docs/en/DEVELOPMENT.md)
- [Contributing](CONTRIBUTING.md)

For help, run `zev --help` or `zev COMMAND --help`. `zev help` is not a command; all command help and flag details are in the full CLI reference.

## Contributing

Contributions are welcome — read the [Contributing Guide](CONTRIBUTING.md).

## License

This project is licensed under the MIT License — see [LICENSE](LICENSE).
