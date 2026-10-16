<div align="center">

**English** | [Русский](../ru/ARCHITECTURE.md)

</div>

# Architecture

Zev is a small Go CLI built as **vertical slices with ports and adapters**.

```
cmd/zev/                        composition root (Cobra commands)
internal/core/
  domain/                       domain types, InferenceProvider interface, errors
  config/                       ~/.zevrc loading (godotenv + caarlos0/env)
internal/entities/
  query/
    service/                    use case: build context, ask provider, save history
    repository/openai/          OpenAI + OpenAI-compatible providers
    repository/gemini/          Google Gemini (raw HTTP + JSON response schema)
    repository/anthropic/       Anthropic Claude
    repository/ollama/          Ollama (reuses the OpenAI-compatible adapter)
    repository/opencode/        OpenCode (SSE streaming, tool calls)
    repository/factory/         provider selection from config
    system_prompt.txt           system prompt (go:embed)
    user_prompt.txt             per-query prompt template (go:embed)
  history/service/              JSONL history, 100 entries, automatic rotation
  alias/service/                command aliases with argument substitution
  script/service/               bash script metadata + files
  setup/service/                interactive TUI setup wizard
pkg/tui/prompt/                 text input and choice (Bubble Tea)
pkg/tui/selector/               suggestion selector with danger warnings
pkg/tui/list/                   generic list renderer
pkg/terminal/                   clipboard / terminal helpers
```

## Query flow

1. `cmd/zev` parses the query (flag parsing is disabled on the root command so that
   aliases with flags still work).
2. `query/service` collects environment context (OS, shell, cwd, installed tools) and
   renders the user prompt from `user_prompt.txt`.
3. `factory.NewProvider` builds the `InferenceProvider` implementation chosen by
   `LLM_PROVIDER`.
4. The provider returns an `OptionsResponse` with up to three `Command` values, each
   carrying a short explanation and a `IsDangerous` flag.
5. The selector renders the options; the chosen command is injected into the terminal or
   executed, depending on `--execute` / `--inject` / `COMMAND_EXECUTION_MODE`.
6. The exchange is appended to `~/.zevhistory`.

## Ports and adapters

`domain.InferenceProvider` is the only port the query service depends on:

```go
type InferenceProvider interface {
    GetOptions(ctx context.Context, prompt string, envContext string) (*OptionsResponse, error)
    GetOptionsWithHistory(ctx context.Context, history *ConversationHistory) (*OptionsResponse, error)
    Name() string
    Model() string
}
```

Interfaces are declared on the consumer side; `cmd/zev` is the only place that knows
about concrete adapters. Adding a provider means adding one package under
`internal/entities/query/repository/` plus one `case` in the factory — nothing else moves.

## Provider specifics

- **OpenAI** — `github.com/sashabaranov/go-openai`, supports a custom base URL so any
  OpenAI-compatible router can be used.
- **Gemini** — raw HTTP with a JSON response schema.
- **Ollama** — reuses the OpenAI adapter against the local endpoint.
- **OpenCode** — direct HTTP client: `stream: true`, a `read` function tool,
  `stream_options.include_usage`, SSE parsing that accumulates `delta.content`, and
  arbitrary headers via `HTTP_HEADERS` (key case preserved).

## Adding a provider

1. Create `internal/entities/query/repository/<name>/` implementing
   `domain.InferenceProvider`.
2. Add config fields for the API key, model and base URL in `internal/core/config`.
3. Add a `case` to `factory.NewProvider`.
4. Cover it with unit tests using `httptest`.

## Tests

Unit tests live next to the code they cover. Integration tests use the `integration`
build tag and are run separately. `make check` runs the linter and the full test suite.
