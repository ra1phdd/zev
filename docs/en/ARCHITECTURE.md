<div align="center">

**English** | [Русский](../ru/ARCHITECTURE.md)

</div>

# Architecture

Zev is a small Go CLI built as **vertical slices with ports and adapters**.

```
cmd/zev/                        Thin process launcher
internal/cli/                    Cobra commands, composition and query orchestration
internal/core/
  domain/                       domain types, InferenceProvider interface, errors
  config/                       ~/.zev/config.yaml loading (current YAML only)
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
  history/service/              lifecycle rules, consumer ports, lexical/vector orchestration
  history/repository/           current SQLite transactions and derived indexes
  history/client/               opt-in OpenAI-compatible embedding HTTP adapter
  history/lexical/              local normalization, Snowball stemming and typo primitives
  alias/{service,repository}/   argument substitution and YAML persistence
  script/{service,repository}/  bash scripts, metadata, editor and execution
  setup/service/                interactive TUI setup wizard
internal/migrations/             ordered store migrators and registry
  filetxn/                       shared filesystem migration staging, publication and journal recovery
  history/                       v1.0.0 JSONL importer and embedded SQLite `.up.sql` baseline
  metadatafile/                  configuration/alias/script metadata migration
  config/                        v1.0.0 dotenv migration
  aliases/, scripts/              released metadata migration steps
pkg/tui/prompt/                 text input with wrapping
pkg/tui/list/                   generic lists and command warnings
pkg/terminal/                   shell execution and platform-specific input injection
```

## Migration boundary

Migration orchestration runs before the selected store is opened. The supported legacy
inputs are the v1.0.0 dotenv configuration, unversioned v1.0.0 aliases/scripts metadata,
and v1.0.0 JSONL history. The current readers in `internal/core` and `internal/entities`
accept current formats only; they do not detect legacy data or invoke migration while
reading or writing. Development snapshots are not compatibility inputs. Future steps are
retained only for transitions between released formats.

The history JSONL importer is separate from the one embedded SQLite `.up.sql` baseline.
It imports legacy records first; the current repository then opens the current SQLite
format. The baseline does not replay a development schema history.

## Query flow

1. `internal/cli/query.go` manually parses root-command flags and loads configuration.
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
7. When history is enabled, the CLI opens `~/.zev/history.db` and gives the store to
   the history lifecycle service. Initial input is recorded before the provider call;
   responses, command selection, execution results and final status are recorded as
   the interaction progresses. Every refinement is recorded separately before its provider
   call. History-write failures warn without preventing the command-provider request.

Conversation orchestration, selection and execution live in the CLI, not in
`query/service`; persistent recording is delegated to `history/service`.
The root `signal.NotifyContext` cancellation
context is passed through provider/HTTP calls, retries, TUI, and command/editor
execution. `CONTEXT_ENABLED=false` / `--no-context` omits automatically added environment
context, not the user's query. The active conversation stays in memory even with
`HISTORY_ENABLED=false` / `--no-history`; these switches skip opening history storage
and therefore also skip migration and embeddings for the query. Persistent history does
not automatically become context for a new model request.

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
use concrete repositories. History declares its persistence and embedding ports on the
consumer/service side. The composition root owns opening and closing the repository;
CLI handlers do not assemble SQL.

## History boundaries and limits

- `history/repository` owns the current SQLite store, related-record transactions,
  foreign keys, WAL and derived indexes. Opening it never imports legacy JSONL or
  interprets migration formats.
- `internal/migrations/filetxn` provides shared filesystem-migration staging, atomic
  publication, durability classification and recovery for recognized current-protocol
  journals. It does not implement SQLite transactions, database rollback, or rollback
  for alias/script CRUD. Migrations generate no archives or backup copies; pre-existing
  user-owned backups and other copies are outside its scope. Markerless or unpublished
  development artifacts are not swept or recovered.
- `internal/migrations/history` owns the supported v1.0.0 JSONL detection,
  snapshot verification, import ledger and source deletion/recovery. CLI orchestration
  checks it before opening the current store. Enabled queries and explicit history
  commands use this path; help, version, local doctor and history-disabled queries do
  not. Migration makes no network calls.
- `history/service` owns conversation lifecycle and search orchestration. Full responses
  and alternatives are atomic; selected does not mean executed, and a completed
  interaction is not proof that terminal input ran. A failed history write warns the
  user but must not repeat an already executed command.
- `history/lexical` applies the same versioned normalization and Russian/English Snowball
  profile to indexed text and queries. It is bounded stemming, not irregular-form
  lemmatization. The repository ranks surface matches before morphology and typo-only
  results, using RRF inside tiers. The service separately fuses lexical/vector lists
  through RRF; hybrid ordering does not retain those lexical tiers.
- `history/client` is a separate opt-in embedding adapter, not the command provider.
  Only initial/refinement user messages are eligible. No command, assistant or system
  text is embedded. Model, dimensions, profile and content hash control compatibility.

Semantic search is disabled by default, does not backfill old history automatically and
does not inject matches into new prompts. Full cosine search is O(N × dimensions), not
an ANN index or a constant-latency guarantee. Literal substring search may scan canonical
text. Deletion is logical rather than forensic erasure of WAL, backups or remote copies.

The CLI loads history configuration and supplies the opt-in embedding client to the
search service. `history index status` reports coverage; `history index rebuild --fts`
rebuilds local indexes, while `--vectors` shows provider/model/eligible count and requires
explicit `--yes` transmission consent. Ordinary queries do not generate vectors.
`history search --cursor` continues request/profile/revision-bound pages. Document
filters precede FTS top-K limits; bounded lexical/typo lists and hybrid rank windows
report observed truncation, not unrestricted recall. Semantic/hybrid search falls back
visibly to lexical when embeddings are disabled or query embedding fails.
See [History](HISTORY.md) for exact limits and recovery/privacy boundaries.
Cross-compilation does not validate native Windows runtime or ACL behavior, and this
description does not claim all release gates passed.

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

Unit tests live next to the code, including CLI tests in `internal/cli`.
Black-box scenarios live in `e2e/`, build the real binary and run with `make test-e2e`.
`make check` runs formatting checks, lint, deadcode, vet and unit tests;
`make test-race` is separate. See [DEVELOPMENT.md](DEVELOPMENT.md) for
commands and [PROJECT.md](../../PROJECT.md) for operational limitations.
