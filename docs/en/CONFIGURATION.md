<div align="center">

**English** | [Русский](../ru/CONFIGURATION.md)

</div>

# Configuration

Zev reads its configuration from `~/.zev/config` — a plain `KEY=VALUE` file loaded with
[godotenv](https://github.com/joho/godotenv) and parsed with
[caarlos0/env](https://github.com/caarlos0/env/v11).

Existing environment variables take precedence over the file; defaults apply
when neither supplies a value. The file is created and updated by `zev setup`:

```bash
zev setup              # interactive wizard
zev setup mode         # inject | execute
zev setup headers      # custom HTTP headers
```

## Defaults

Values below are applied by `config.DefaultConfig()` when the corresponding key is
absent from `~/.zev/config`.

| Key | Default |
|---|---|
| `LLM_PROVIDER` | `openai` |
| `OPENAI_MODEL` | `gpt-5.6-luna` |
| `OPENAI_BASE_URL` | `https://api.openai.com/v1` |
| `GEMINI_BASE_URL` | `https://generativelanguage.googleapis.com` |
| `GEMINI_MODEL` | `gemini-3.1-flash-lite` |
| `ANTHROPIC_BASE_URL` | `https://api.anthropic.com/v1` |
| `ANTHROPIC_MODEL` | `claude-haiku-5.5` |
| `COMMAND_EXECUTION_MODE` | `inject` |
| `SCRIPT_EDITOR` | `nano` |

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
| `COMMAND_EXECUTION_MODE` | `inject` attempts to insert the command into terminal input; `execute` runs the first suggestion immediately |
| `SCRIPT_EDITOR` | Editor used by `zev script create` |
| `HTTP_HEADERS` | Comma-separated `Key:Value` pairs added to every provider request. Key case is preserved |
| Retry policy | Provider requests retry up to 3 times for HTTP `429` and `5xx` responses. Backoff is exponential (`1s`, `2s`, `4s`); a valid `Retry-After` header can extend the delay, and the delay is capped at 30 seconds. Retries are logged at warning level; the final failure is logged at error level. |

## Provider request retries

All five providers use the same retrying HTTP client for both single queries and
conversation history. There are at most four attempts: the original request plus
three retries. The existing 30-second client timeout covers all attempts, backoff
and response reading, so a timeout or cancellation can stop the operation earlier.
Other `4xx` responses and malformed successful JSON are not retried. A stream
that has already returned HTTP `200` is not restarted if it breaks while reading.
Retry warnings are visible without `-v`; URLs, API keys and payloads are not
included in retry logs. These defaults require no extra configuration.

## Command execution mode

`zev setup mode` controls how suggestions are handled:

- `inject` (default) — select a suggestion, then review the inserted command and press
  Enter. If terminal injection is unsupported or denied, Zev prints the command instead.
- `execute` — the **first suggestion** runs immediately, without a selector or
  confirmation. The danger warning is printed after execution; it is not a safety gate.

Aliases and saved scripts execute directly, independently of this setting.

## Editor resolution for `zev script create`

1. `$SCRIPT_EDITOR`
2. `$EDITOR`
3. first available binary from `nano`, `vim`, `vi`

If no editor is found, Zev returns an error containing the created file path.

## Files on disk

| Path | Contents |
|---|---|
| `~/.zev/config` | Configuration, `KEY=VALUE` lines |
| `~/.zev/history` | Query history, JSONL, rotated at 100 entries |
| `~/.zev/aliases` | Alias templates, YAML |
| `~/.zev/scripts` | Script metadata, YAML |
| `~/.zev/script_files` | Script bodies |

`~/.zev/scripts` is a file and `~/.zev/script_files` is a directory on purpose: a file
and a directory with the same name cannot coexist.
