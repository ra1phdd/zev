<div align="center">

**English** | [Русский](../ru/CONFIGURATION.md)

</div>

# Configuration

Zev reads its configuration from `~/.zevrc` — a plain `KEY=VALUE` file loaded with
[godotenv](https://github.com/joho/godotenv) and parsed with
[caarlos0/env](https://github.com/caarlos0/env/v11).

The file is created and updated by `zev setup`:

```bash
zev setup              # interactive wizard
zev setup mode         # inject | execute
zev setup headers      # custom HTTP headers
```

## Defaults

Values below are applied by `config.DefaultConfig()` when the corresponding key is
absent from `~/.zevrc`.

| Key | Default |
|---|---|
| `LLM_PROVIDER` | `openai` |
| `OpenAIModel` | `gpt-5.6-luna` |
| `OPENAI_BASE_URL` | `https://api.openai.com/v1` |
| `GEMINI_BASE_URL` | `https://generativelanguage.googleapis.com` |
| `GeminiModel` | `gemini-3.1-flash-lite` |
| `ANTHROPIC_BASE_URL` | `https://api.anthropic.com/v1` |
| `AnthropicModel` | `claude-haiku-5.5` |
| `COMMAND_EXECUTION_MODE` | `inject` |
| `ScriptEditor` | `nano` |

## Variables

### Provider selection

| Variable | Description |
|---|---|
| `LLM_PROVIDER` | `openai`, `gemini`, `anthropic`, `ollama` or `opencode` |

### OpenAI / OpenCode

| Variable | Description |
|---|---|
| `OPENAI_API_KEY` | API key. Also used by the `opencode` provider |
| `OPENAI_MODEL` | Model name |
| `OPENAI_BASE_URL` | Base URL — point it at any OpenAI-compatible router |

### Gemini

| Variable | Description |
|---|---|
| `GEMINI_API_KEY` | API key |
| `GEMINI_MODEL` | Model name |
| `GEMINI_BASE_URL` | Base URL |

### Anthropic

| Variable | Description |
|---|---|
| `ANTHROPIC_API_KEY` | API key |
| `ANTHROPIC_MODEL` | Model name |
| `ANTHROPIC_BASE_URL` | Base URL |

### Ollama

| Variable | Description |
|---|---|
| `OLLAMA_BASE_URL` | Local Ollama endpoint, e.g. `http://localhost:11434/v1` |
| `OLLAMA_MODEL` | Model name |

### Behaviour

| Variable | Description |
|---|---|
| `COMMAND_EXECUTION_MODE` | `inject` copies the selected command to the clipboard-style prompt; `execute` runs it immediately |
| `SCRIPT_EDITOR` | Editor used by `zev script create` |
| `HTTP_HEADERS` | Comma-separated `Key:Value` pairs added to every provider request. Key case is preserved |

## Command execution mode

`zev setup mode` controls what happens after you pick a suggestion in the selector:

- `inject` (default) — the command is placed in your terminal, you review it and press Enter.
- `execute` — the command is executed right away. Use it only if you fully trust the
  suggestions; dangerous commands are flagged with a warning either way.

## Editor resolution for `zev script create`

1. `$SCRIPT_EDITOR`
2. `$EDITOR`
3. first available binary from `nano`, `vim`, `vi`

If no editor is found, Zev prints the path of the created file instead of failing.

## Files on disk

| Path | Contents |
|---|---|
| `~/.zevrc` | Configuration, `KEY=VALUE` lines |
| `~/.zevhistory` | Query history, JSONL, rotated at 100 entries |
| `~/.zev/scripts` | Script metadata, YAML |
| `~/.zev/script_files` | Script bodies |

`~/.zev/scripts` is a file and `~/.zev/script_files` is a directory on purpose: a file
and a directory with the same name cannot coexist.
