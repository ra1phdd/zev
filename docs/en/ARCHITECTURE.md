<div align="center">

**English** | [Русский](../ru/ARCHITECTURE.md)

</div>

# Architecture

Zev is a small Go CLI built as **vertical slices with ports and adapters**.

```
cmd/zev/                        Cobra commands, composition and query orchestration
internal/core/
  domain/                       domain types, InferenceProvider interface, errors
  config/                       ~/.zev/config loading (godotenv + caarlos0/env)
  httpclient/                   shared HTTP client and transport
internal/entities/
  query/
    service/                    prompt validation and environment context
    repository/openai/          OpenAI + OpenAI-compatible providers
    repository/gemini/          Google Gemini (raw HTTP + JSON response schema)
    repository/anthropic/       Anthropic Claude
    repository/ollama/          Ollama (reuses the OpenAI-compatible adapter)
    repository/opencode/        OpenCode (SSE streaming, tool calls)
    repository/factory/         provider selection from config
    repository/prompt.go          embeds system_prompt.txt; user template is "%s"
  history/service/              JSONL history, 100 entries, automatic rotation
  alias/{service,repository}/   argument substitution and YAML persistence
  script/{service,repository}/  bash scripts, metadata, editor and execution
  setup/service/                interactive TUI setup wizard
pkg/tui/prompt/                 text input with wrapping
pkg/tui/list/                   generic lists and command warnings
pkg/terminal/                   shell execution and platform-specific input injection
```

## Query flow

1. `cmd/zev/query.go` manually parses root-command flags and loads configuration.
2. It resolves aliases first, then saved scripts. A match executes directly; only
   unmatched input reaches an LLM.
3. The CLI creates the HTTP client and calls `factory.NewProvider`. The query
   service collects OS, architecture, shell, user, home and cwd; the CLI renders the
   system prompt from embedded `system_prompt.txt` with that environment context and
   keeps the original query as the user message (`UserPromptTemplate = "%s"`). Installed
   tools are not scanned.
4. The CLI calls `InferenceProvider.GetOptionsWithHistory`. Providers decode an
   `OptionsResponse`; the query service validates it before selection and enforces up to
   three alternatives plus field and encoded-response size limits.
5. `inject` displays choices and supports refinement using the active in-memory
   conversation. The selected command is inserted into terminal input, or printed if
   injection is unsupported or denied.
6. `execute` runs the first suggestion without selection or confirmation. The selected
   command is validated again before delivery; its danger warning is printed after
   execution, not used as a safety gate.
7. When history is enabled, the CLI saves the response and selected command to
   `~/.zev/history`.

Conversation orchestration, selection, execution and history saving currently live
in the CLI, not in `query/service`. The root `signal.NotifyContext` cancellation
context is passed through provider/HTTP calls, retries, TUI, and command/editor
execution. `CONTEXT_ENABLED=false` / `--no-context` omits automatically added environment
context, not the user's query. The active conversation stays in memory even with
`HISTORY_ENABLED=false` / `--no-history`; only the query result is persisted to JSONL
history when history is enabled.

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

The factory imports concrete provider adapters. Alias and script services currently
use concrete repositories. History handles
file persistence inside its service rather than in a separate repository adapter.

## Provider specifics

- **OpenAI** — `github.com/sashabaranov/go-openai`, supports a custom base URL so any
  OpenAI-compatible router can be used.
- **Gemini** — raw HTTP with a JSON response schema.
- **Anthropic** — `github.com/liushuangls/go-anthropic/v2`.
- **Ollama** — reuses the OpenAI adapter against the local endpoint.
- **OpenCode** — direct HTTP client: `stream: true`, a `read` function tool,
  `stream_options.include_usage`, SSE parsing that accumulates `delta.content`, and
  arbitrary headers via `HTTP_HEADERS` (key case preserved).

## Adding a provider

1. Create `internal/entities/query/repository/<name>/` implementing
   `domain.InferenceProvider`.
2. Add config fields for the API key, model and base URL in `internal/core/config`.
3. Add a `case` to `factory.NewProvider`, update setup/provider selection and both
   configuration guides.
4. Cover it with unit tests using `httptest`.

## Tests

Tests live next to the code. `make check` runs formatting checks, the linter,
application deadcode reporting, vet and default unit tests; `make test-race` is
separate. Tagged-test targets exist, but tracked tests currently have no
`integration`, `e2e` or `manual` suites. See [DEVELOPMENT.md](DEVELOPMENT.md) for
commands and [PROJECT.md](../../PROJECT.md) for operational limitations.
