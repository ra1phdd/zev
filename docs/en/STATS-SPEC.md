<div align="center">

**English** | [Русский](../ru/STATS-SPEC.md)

</div>

# Specification: zev stats — tokens, activity, and context

**Status:** draft for approval. The feature is not implemented; this document does not authorize implementation, migration of user data, new dependencies, or a commit.

## 1. Objective and basis

The user wants `zev stats`, similar to Claude Code statistics: an attractive terminal interface with charts and understandable context consumption.

Success: the user sees token consumption over a period, model distribution, and the input size of the last request in a selected conversation without confusing these metrics. Data is local; viewing requires neither API credentials nor a network connection.

### Verified current state

- [InferenceProvider](../../internal/core/domain/provider.go) returns `OptionsResponse` without usage; [ToolTurn](../../internal/core/domain/tool.go) also has no usage.
- [OpenAI](../../internal/entities/query/repository/openai/provider.go), [Anthropic](../../internal/entities/query/repository/anthropic/provider.go), and [Gemini](../../internal/entities/query/repository/gemini/provider.go) adapters do not propagate usage to their consumer. [Ollama](../../internal/entities/query/repository/ollama/provider.go) uses the OpenAI adapter.
- [OpenCode](../../internal/entities/query/repository/opencode/provider.go) requests `include_usage`, but its current SSE parser extracts text only.
- The [tool service](../../internal/entities/tool/service/service.go) performs multiple model turns and a separate safety review. The final response does not represent all consumption.
- The [history lifecycle](../../internal/entities/history/service/lifecycle.go) persists conversations and responses, but not a usage journal. Disabled history does not open SQLite.
- The entry point is [cmd/zev/main.go](../../cmd/zev/main.go); routing/composition lives in [internal/cli](../../internal/cli/root.go). Architecture: [vertical slices + ports/adapters](ARCHITECTURE.md).

These are checkout observations, not a compatibility promise for concurrent unfinished changes. Recheck integration points before implementation.

### Proposed defaults requiring approval

1. One `stats` capability: usage collection, local aggregation, and presentation are parts of the same feature, not separate products.
2. Static dashboard by default; interactivity only with `--interactive`.
3. Default period: the last 30 calendar days including today; the process timezone.
4. Statistics follow the existing history policy; no independent hidden storage.
5. Version one excludes cost, subscription limits, and an approximate tokenizer.
6. Context means **the input of the last observed primary request**, not the guaranteed size of the next request.

MUST/MUST NOT below are testable requirements of the proposed contract, not descriptions of implemented behavior.

## 2. Version-one scope

Included: all five existing query providers; ordinary query, machine query, and `continue`; intermediate tool turns and safety reviews; a local observation journal; calendar; daily/weekly chart; models; cache/reasoning breakdown; selected conversation; JSON; Russian/English UI.

Excluded: cost and forecasts, account quotas, importing other CLIs' statistics, cloud sync, web dashboard, project filters, achievements/streaks, a full interactive history browser, automatic context reduction, prompt changes, and safety-policy changes. Embeddings, metadata generation, and unimplemented capabilities from other specifications are excluded from version-one totals. UI/JSON call this scope `query_llm`, not “all Zev spending.”

Direct alias/script execution without an LLM does not increase calls/tokens. Neither does a named machine lookup without an LLM. Independent run/update/metadata features are not prerequisites for stats.

## 3. Terms and numeric contract

| Field | Meaning |
|---|---|
| `input_tokens` | Full provider input, including cache reads/writes where included in that protocol's input |
| `output_tokens` | Full generated output, including reasoning where included by the protocol or reliably addable |
| `cache_read_tokens` | Subset of `input_tokens` read from cache |
| `cache_write_tokens` | Subset of `input_tokens` written to cache |
| `reasoning_tokens` | Subset of `output_tokens` used for reasoning |
| `total_tokens` | `input_tokens + output_tokens`, only when both are known |
| `provider_total_tokens` | Separate original numeric provider total for consistency checks, not another summand |
| `calls` | Registered logical invocations of a query adapter, not prompts, tools, or HTTP retries |
| `conversations` | Distinct conversations with registered calls in the range |

All counters are nonnegative `int64` integers; unavailable is `null`, observed zero is `0`. Absence, JSON null, negative/fractional values, overflow, and invalid types MUST NOT become known zero. An invalid metric becomes `null` with a safe diagnostic code; valid independent metrics remain available.

`cache_read_tokens`, `cache_write_tokens`, and `reasoning_tokens` MUST NOT be added to input/output that already include them. Do not invent missing fields by subtracting inconsistent totals. A known subset/total invariant violation excludes the affected metric set from exact accounting and marks it invalid; command options do not become invalid because of it.

### Protocol normalization

| Protocol | Normalized input | Normalized output |
|---|---|---|
| OpenAI Chat Completions | `prompt_tokens`; cached tokens are a subset | `completion_tokens`; reasoning is a subset |
| Anthropic Messages | `input_tokens + cache_read_input_tokens + cache_creation_input_tokens` when required components are known | `output_tokens`, without adding thinking twice |
| Gemini generateContent | `promptTokenCount`, already including cached content | `candidatesTokenCount + thoughtsTokenCount` when component semantics and presence are confirmed |
| OpenCode SSE | OpenAI-compatible usage normalization from the final usage event | Same; events without `choices` must also be processed |
| Ollama compatible endpoint | OpenAI-compatible usage from the actual response | Same, without assuming endpoint completeness |

A missing optional field can mean zero only when the official contract of the specific protocol guarantees it; a compatible URL alone is not such a guarantee. During implementation, check semantics against pinned SDKs and official documentation. Additional Gemini categories not covered by the chosen formula are not hidden: a provider-total mismatch is marked inconsistent, not forcibly allocated.

### Consumption versus context

- Period/conversation consumption is the sum of **known normalized usage for individual calls**. Resent history counts on every invocation.
- `last_request_input_tokens` is the input of the conversation's last primary call. If unavailable, display unavailable rather than an older known value.
- `context_window_tokens` is a trusted window limit for that same model/endpoint at call time. Persist its provenance. Do not infer it from a model name or output maximum. In version one, the limit may be unavailable for every existing adapter.
- `input_window_percent = 100 × last_request_input_tokens / context_window_tokens`; valid only when both are known and the limit is positive. Label: “Last request input / window.” This is not the remaining generation budget; output and reserve need a separate policy.
- Do not hide values above 100%: cap the bar width, show the actual number and a warning. Unavailable context must not appear as 100% free.
- Safety reviews increase consumption but never replace the primary context snapshot. Primary conversation tool turns may replace it.
- After process exit, the snapshot is historical. Persisted history contains compact responses and cannot exactly reconstruct the next wire request. Display snapshot time and model.

## 4. CLI

```bash
zev stats
zev stats --days 7
zev stats --year 2026
zev stats --all
zev stats --provider anthropic --model claude-example
zev stats --session last
zev stats --session <conversation-id>
zev stats --days 30 --json
zev stats --interactive
```

`claude-example` is a placeholder identifier, not a model availability promise.

| Flag | Default | Contract |
|---|---|---|
| `--days N` | `30` | Integer 1–366; N calendar dates through today |
| `--year YYYY` | unset | Four-digit year 1970–9999; no later than the current year |
| `--all` | `false` | Entire persisted query-usage scope |
| `--provider ID` | unset | Exact configured provider ID: openai/gemini/anthropic/ollama/opencode |
| `--model ID` | unset | Exact persisted request model ID, case preserved |
| `--session ID\|last` | unset | One conversation; incompatible with period and provider/model filters |
| `--json` | `false` | One schema-version-1 JSON document, no ANSI/TUI |
| `--interactive` | `false` | Fullscreen TUI, only with TTY stdin/stdout |

Explicit `--days`, `--year`, and `--all` are mutually exclusive. `--json` and `--interactive` are incompatible. Positional arguments, unknown flags, empty IDs/filters, repeated value flags, and out-of-range numbers are usage errors before opening the database. A nonexistent provider/model combination yields an empty report, not a provider lookup. `--interactive=false` does not enable TUI.

`stats` is a built-in taking precedence over named entities. Prohibit the new name for new/renamed aliases/scripts through shared write-time validation, with a root-command consistency test; current [metadata validation](../../internal/core/metadata/validation.go) uses a static list, so do not assume a ready-made dynamic registry. Reading an existing alias/script named `stats` MUST NOT make the whole store unreadable: preserve the record and allow list/edit/remove, but root `zev stats` now belongs to the built-in. On discovering a collision, issue a safe warning with instructions to change the name; never automatically delete/rename it. This precedence change is proposed and requires approval with the draft. Preserve other aliases/scripts' opaque argv-tail contract.

### Ranges and last selection

- Capture current time once on report opening; database times are UTC, calendar boundaries use the system timezone and DST, not N × 24 hours.
- Interval: `[from, to)`; `to` is captured now. For a past year, `to` is January 1 of the following year. Exclude future records.
- Filter calls by `started_at`; a call finishing after midnight belongs to its starting date. An unfinished registered call counts as a call, not as a known token total.
- `last` selects the latest persisted query conversation by `(created_at DESC, id DESC)`, even without usage. An old conversation shows “usage was not recorded,” not an older conversation with numbers. A continuation means the conversation actually created by existing `continue`; inherited messages are not new calls.
- Explicit stats may read existing history with `HISTORY_ENABLED=false`: the setting disables query collection, not explicit local viewing. It does not retroactively enable recording.

### Streams and errors

Stdout contains the report or JSON; stderr contains safe diagnostics. No progress, provider setup, or network calls. Build JSON completely before writing; stdout stays empty on argument/read errors. A stdout write failure may leave an incomplete document and returns failure.

Exit codes: `0` — report, including empty; `1` — storage/I/O/unsupported schema/session not found; `2` — arguments or unsuitable TTY for explicit interactivity; `130` — Ctrl-C cancellation. TUI `q`/Escape exits normally with `0`. `--help` neither opens history nor runs migrations. Changing other commands' exit codes is outside scope.

## 5. Presentation

### Static overview

Block order: period/timezone/scope heading → summary cards → heatmap → token bars → models → coverage/limitations. Cards: conversations, calls, known total/input/output. Cache/reasoning are labeled “included,” not extra total summands.

- Default heatmap metric: known tokens/day; legend explicitly names the unit. Interactive mode can switch to calls/day.
- Heatmap levels: 0, then four equally sized intervals relative to the maximum positive value in the visible range (`ceil(4 × value / max)`). Do not clip an isolated peak. Expose thresholds in legend/JSON; changing the period changes the scale.
- Dates without calls have a distinct empty state. Calls with entirely unavailable tokens show `?`, not zero consumption. Partial data shows the known sum with a visible marker. Future dates are blank, not zero activity.
- `--days` bars are daily; `--year`/`--all` bars are weekly starting Monday. Incomplete weeks do not include data outside the range. Heatmap and weekly bars show at most the last 52 weeks; cards retain the full requested period. A caption discloses chart versus total range differences.
- Models: group by `(configured provider, request model)`, sort by known token total DESC then provider/model ASC. Show the top 5 and an others row, preserving sums. Groups without known tokens follow known groups. Escape terminal control sequences in names.
- Cache share = known cache-read tokens / known input tokens **only for calls where both fields are known**. A zero denominator yields `n/a`; show covered sample size. This is a token share, not a percentage of requests with a cache hit.

Snapshot-test widths: 120, 80, 40, and 20 columns. At >=100, cards may share a row; at 60–99 use compact layout; below 60 stack metrics and show the latest fitting weeks with a caption. Below 30 replace the calendar with a text summary. Never exceed available width, split UTF-8, or convey meaning through color alone.

One accent color, subdued labels, aligned numbers, minimal borders. `NO_COLOR`, `TERM=dumb`, and non-TTY disable ANSI. Non-TTY never starts TUI and emits text tables instead of a graphical grid. Russian/English follows existing `ZEV_LANG`; flags, JSON keys, and user model IDs are not translated.

### Conversation

Show ID, model(s), date, and coverage first; then last primary call input/limit and a horizontal bar when the limit is known; then cumulative consumption and input evolution across primary calls in start order. Display safety-review consumption separately. Missing chart samples are not connected as zeros. A conversation without calls has an empty state.

### Interactivity

`Tab` switches overview/models; `m` switches tokens/calls; `Left`/`Right` select a date; `Enter` opens local numeric day details; `Escape` closes details, then exits; `q` exits. `7`/`3`/`y`/`a` select 7 days/30 days/current year/all time; period switches do not apply in session view. Terminal resize preserves the selected date if it fits the new range.

The initial screen uses a consistent storage snapshot. No automatic polling; `r` opens a fresh snapshot and updates now. Changing the period may reread the database, never invoke a provider. Navigation never runs commands, exposes prompts, or changes history. TUI restores terminal state after exit/error/cancellation.

## 6. Collection, storage, and compatibility

### Call journal

Propose an additive migration in the existing SQLite history, not a separate database/JSONL. Choose the next migration number from the current checkout. SQL DDL and Go signatures follow specification approval; this section defines the mandatory logical contract.

A call contains: local `call_id`, `conversation_id`, `sequence`, `purpose=primary|safety_review`, configured provider, request model, UTC started/completed timestamps, `status=started|succeeded|failed|cancelled`, nullable normalized counters, provider total, safe diagnostic codes, and nullable context limit with provenance. `sequence` totally orders calls within one conversation, including reviews. Do not persist endpoint URLs, API keys, headers, prompts, tool arguments/results, or raw responses in the journal.

- Preserve configured provider separately from adapter `Name()`: Ollama must not become openai-compatible. Request model is the actually sent ID, not viewing-time configuration.
- Register call start immediately before invoking the adapter when history is available. Denied tool execution is not itself an LLM call; a model request receiving the denial as a tool result is.
- Capture usage before options JSON decoding and subsequent business validation: a received, chargeable but invalid response still contributes observed consumption. Command outcome is not replaced by network-call status.
- Finalization updates the same record rather than inserting another. Repeating identical finalization is idempotent; conflicting finalization preserves the prior observation and emits safe diagnostics.
- Persist usage as it arrives, not only after the entire query succeeds. Failure/cancellation does not remove known usage; a crash leaves started/unknown.
- Repeated stream events MUST NOT repeatedly sum cumulative snapshots. Snapshot replacement/delta summation follows the protocol contract; final usage without content must be processed. An interrupted stream is partial/unknown according to actually observed fields.
- HTTP transport retries are attempts inside one logical call, not additional prompts. Consumption invisible to the client after retry/timeout cannot be reconstructed. Persist `transport_retried` when a retry occurred and do not promise complete provider billing. Usage capture must never repeat a request for statistics.
- Cover both the primary adapter path and ToolProvider path. Wrapping the final OptionsResponse is insufficient. Never record one primary call twice through different layers.

Journal errors do not block receiving/delivering a command: issue a safe stderr warning without network retry or machine-stdout contamination. Without a created durable conversation, the collector creates neither orphan usage nor alternative storage. Missing recording around a dispatched network request does not authorize automatic repetition; only a new independent user invocation can initiate new work.

### Coverage and accuracy boundaries

For registered calls, count `known_total_calls`, `partial_calls`, and `unknown_calls` as disjoint categories. Known total requires known input/output; optional details may remain unavailable. Partial has at least one known token counter but no total; unknown has no valid counters.

Each metric aggregate contains `known_sum` and `known_calls`; coverage fraction = known_calls / calls, or null when calls=0. The known sum of an empty set is 0, but with calls>0 and known_calls=0 UI shows `n/a`, not “0 used.” Input/output sums may have different coverage and MUST NOT be added to obtain total: sum total only across calls with both fields known.

Report old conversations without journal entries separately as `legacy_conversations_without_call_records`; the number of hidden tool/review calls cannot be estimated. In overview this means query conversations created within the range matching provider/model filters; the metric does not establish that newer conversations have no missing calls. `recording_started_at` is the collector-version activation time, not proof of continuous recording. Deleted data, disabled history, recording errors, crashes, and lost responses limit statistics. Even 100% coverage of registered calls does not mean a complete account bill.

### Migrations and reading

- The new schema does not change OptionsResponse JSON, the machine-query envelope, prompts, saved commands, or old records. Do not backfill historical usage from text length.
- `HISTORY_ENABLED=false`/`--no-history` does not open the database, create call records, or save counters elsewhere. `--no-context` does not disable measurement when history is enabled.
- Deleting a conversation removes its call records and changes totals; no hidden permanent aggregate. Inherited `continue` messages do not copy source-conversation usage.
- Use existing transactions/path checks/permissions for writes. Foreign keys, uniqueness, and indexes enforce relationships/order; preserve Unix 0600/0700 privacy and existing Windows guarantees.
- Read one consistent SQLite snapshot; do not retain its transaction throughout TUI lifetime. Memory is bounded by date/model aggregates, not all prompts/responses.
- An absent database without legacy data yields a successful empty report without creating a database/directory. With legacy data or older SQLite, use the established history migration protocol; this may modify local storage but never invokes a network. Do not advertise the command as strictly filesystem-read-only. Do not disguise migration failures as empty reports.
- Unknown future schema, corrupt database, denied permissions, and busy timeout are visible errors. Never reset/recover by replacing the database with an empty one. SQL belongs in repositories, not CLI/TUI.

## 7. Machine contract

`--json` emits one UTF-8 object with numeric `schema_version: 1`, `status: "ok"`, and `scope: "query_llm"`. Keys use snake_case; identifiers/statuses are not localized. Version-one consumers MUST use an integer-capable decoder: counters can exceed JavaScript Number precision.

| Field | Required content |
|---|---|
| `generated_at` | RFC3339 UTC snapshot time |
| `range` | `mode=days|year|all|session`, nullable `from`, exclusive `to`, timezone name and applied filters; session ID for session |
| `summary` | conversations, calls, known_total_calls, partial_calls, unknown_calls, legacy_conversations_without_call_records |
| `tokens` | input/output/cache_read/cache_write/reasoning/total; each `{known_sum, known_calls}` |
| `daily` | Array ordered by date ASC: date, calls, token aggregates, unknown_calls, partial_calls; includes empty elapsed dates in a bounded range |
| `models` | All provider/model groups with calls and token aggregates; same order as UI, without top-5 truncation |
| `context` | null in overview; in session: last primary call ID/time/model/status, nullable input/limit/percent/provenance; null if there are no primary calls |
| `coverage` | recording_started_at, stable limitation codes, retry/invalid-usage warnings |
| `chart` | Visible range, metric, bucket unit, heatmap thresholds; chart range may be shorter than totals |

For `all`, daily starts at the first persisted call date; without calls it is empty. For session, daily covers its call dates. An unbounded all request may produce large JSON: it explicitly requests the complete daily series, not fixed-size output. Bounded overview ranges include empty dates through today, not future dates of the selected year. Session exports no prompt, command text, error payload, or raw provider response IDs.

Additional optional keys are compatible without a version change; semantic/type changes require a new version. Never disguise errors as `status: ok`; read errors occur before any JSON is written, with stderr/exit semantics from section 4. JSON contains no UI strings or preformatted numbers.

## 8. Architecture, style, and dependencies

Stack: Go 1.27.2; existing Cobra 1.10.2, Bubble Tea 1.3.10, Lipgloss 1.1.0, modernc SQLite 1.62.0. No new dependencies required; Context7 was unavailable during initial research, so verify library APIs against official documentation and pinned versions during implementation.

- `internal/core/domain`: provider-usage types without UI/persistence; do not add usage to model-generated JSON.
- `internal/entities/query/repository/*`: capture/validate/normalize usage for both provider paths.
- `internal/entities/history/repository`: current SQLite data access and journal storage; migration orchestration and the embedded baseline remain outside the entity package.
- Proposed `internal/entities/stats/service`: metrics, coverage, ranges, consumer-side storage port; a repository adapter to the existing history database without a second opening/permissions system.
- Proposed `internal/entities/stats/transport`: text/JSON/Bubble Tea presentation. Exact files belong to the subsequent plan, not approved existing symbols.
- `internal/cli`: routing, flags, registration, DI, and lifetime. Locale keys/catalogs follow the existing mechanism. Unit tests beside code; black-box tests under `e2e/` with the `e2e` build tag.

Style: context first, named parameter structs for long signatures, `%w` error wrapping, existing history errors.Is classifications, English-only source comments, GoDoc for exports. Nullable counters are explicit, not magic negative sentinels. This is a shape example, not a promise of an existing API:

```go
// Counter distinguishes unavailable usage from an observed zero.
type Counter struct {
    Value int64
    Known bool
}
```

## 9. Acceptance and verification

| ID | Testable condition |
|---|---|
| ST-01 | All five providers: ordinary and tool paths; absent/null/zero/invalid/overflow, cache/reasoning without double counting |
| ST-02 | Query with multiple tool turns and safety reviews: every call exactly once; review contributes total but not context |
| ST-03 | Invalid options JSON after valid usage, failure/cancellation, interrupted SSE, and final usage-only chunk preserve actually known consumption |
| ST-04 | Repeated snapshots/idempotent finalization do not double tokens; conflicting finalization preserves the record; retries disclose limitations |
| ST-05 | Old conversations, no usage, partial/zero usage, and no database yield distinct correct states; no invented backfill |
| ST-06 | No-history neither creates nor opens storage; journal failure does not break query/repeat external work; machine stdout stays clean |
| ST-07 | Day/year/DST, midnight crossing, `[from,to)` boundaries, filters, and stable ordering match the contract |
| ST-08 | Session last selects the latest conversation even without usage; continuation does not copy consumption; deletion removes related usage |
| ST-09 | Input snapshot never uses cumulative total or review; unavailable limit produces no percentage; historical timestamp is visible |
| ST-10 | Fixtures at 120/80/40/20 columns, Unicode/control characters, NO_COLOR, dumb, and pipe; legend/incomplete coverage remain visible without color |
| ST-11 | TUI keys, resize, q/Escape/Ctrl-C, refresh, and terminal restoration; reject headless interactivity without a prompt |
| ST-12 | JSON schema/nullable fields/order/sums/empty arrays; parser conflicts and exit codes; read errors leave stdout empty |
| ST-13 | Invalid LLM/embedding settings and a forbidden HTTP transport do not prevent local stats/help; no network/command execution |
| ST-14 | Atomic additive migration, failure rollback, future schema/corruption/busy, concurrent query writer/stats reader; no history loss |
| ST-15 | Built-in routing/name reservation preserves other entities' opaque argv and access to manage an existing alias/script stats |
| ST-16 | Synchronized en/ru text; docs/help explain query-only scope, history privacy, unknown coverage, and input versus consumption |

Unit fixtures are synthetic, without real keys/history; HTTP uses httptest with controlled time/timezone. A test first reproduces the required behavior; UI golden snapshots do not replace numeric tests. Run the race detector for concurrent collector/store paths. E2E builds a real binary with isolated HOME and verifies stdout/stderr/exit/storage side effects.

Verification commands **after implementation**, not claims that they ran:

```bash
make test
make lint
make test-race
make test-e2e
make build
make clean
```

During iteration use focused `go test ./internal/entities/history/...` and new stats-package/CLI tests. Behavioral changes require CHANGELOG, both READMEs, and relevant HISTORY/ARCHITECTURE updates. This draft requires Markdown links, fences, parity, and whitespace checks only; do not run source-mutating `make lint` for a prose-only task.

## 10. Boundaries and approval

**Always:** preserve user changes; verify real SDKs/paths; keep collection within query_llm; distinguish unavailable from zero; reuse existing history lifecycle/privacy; check both locales.

**Ask first:** this draft and defaults; provider/storage contract changes and additive migration in the subsequent phase; new dependencies; cost, independent storage with disabled history, or additional LLM workflows. The exact plan must not expand approved scope.

**Never:** infer exact tokens from text length; present local counters as account quotas; repeat an LLM request to record usage; journal secrets/raw payloads; automatically delete data; run commands from the dashboard; change safety policy; commit or migrate real data merely because this draft was written.

Approval questions: accept static-by-default with explicit interactivity; tie retention to history without independent statistics; leave cost and approximate context breakdown outside version one. Recommendation: accept all three defaults. Context percentages become available only with a trusted limit source; its absence does not block the token/activity dashboard.

## 11. Research references

Sources are UX/accounting examples, not code to copy or commitments to reproduce entire products:

- [Claude Code commands](https://code.claude.com/docs/en/commands): stats/usage and context; CLI implementation was unavailable in the inspected public tree.
- [Codex last/total usage](https://github.com/openai/codex/blob/6257fbc4bd5339144e6ffb3398a011917d883a2e/codex-rs/protocol/src/protocol.rs#L2617-L2653), [activity charts](https://github.com/openai/codex/blob/6257fbc4bd5339144e6ffb3398a011917d883a2e/codex-rs/tui/src/analytics/activity_chart.rs).
- [OpenCode stats presentation](https://github.com/anomalyco/opencode/blob/0b87612685dad0bb961d4086e650eb51ec4692cc/packages/cli/src/commands/handlers/stats.ts), [aggregation](https://github.com/anomalyco/opencode/blob/0b87612685dad0bb961d4086e650eb51ec4692cc/packages/core/src/session/stats.ts). Its activity calendar measures steps, not tokens.
- [Gemini CLI heatmap](https://github.com/google-gemini/gemini-cli/blob/4f343cabc7b59ab990d7411e920094108524abff/packages/cli/src/ui/components/HeatmapComponent.tsx).
- [Aider context tokens](https://github.com/Aider-AI/aider/blob/5dc9490bb35f9729ef2c95d00a19ccd30c26339c/aider/commands.py#L445-L551).
- [ccusage daily aggregation](https://github.com/ryoppippi/ccusage/blob/7ac5f2329fe1cd6bed5645867b18703ad86aaba5/rust/adapters/claude/src/daily.rs).
- [Anthropic prompt caching](https://platform.claude.com/docs/en/build-with-claude/prompt-caching), [OpenAI prompt caching](https://platform.openai.com/docs/guides/prompt-caching/best-practices), [Gemini usage metadata](https://ai.google.dev/api/generate-content#UsageMetadata).
