<div align="center">

**English** | [Русский](../ru/HISTORY.md)

</div>

# History

Zev stores requests, refinements, model responses and command alternatives locally in
`~/.zev/history.db`. There is no automatic count-based rotation. History and embeddings
are sensitive data, not encrypted storage: do not put credentials in requests.

> History is available through both query recording and the explicit history commands. The
> automatic legacy migration runs before the repository is returned; semantic search is
> opt-in and uses the separately configured embedding client. Use `zev history --help`
> for the commands available in the installed version.

## Storage and lifecycle

When history opens successfully, the CLI records a conversation incrementally:

1. the initial user request before the first provider call;
2. every refinement as a separate ordered user message, before the next call;
3. each structured response and all its command alternatives in one transaction;
4. the selected command and the observed delivery/execution outcome;
5. completion, cancellation or failure without discarding previously saved messages.

A selected, printed or injected command is **not** proof of execution. An abrupt process
termination may leave a conversation `started` and its execution outcome unknown. History
write failures produce warnings; they do not repeat an already executed command.

Conversations, messages, responses and commands have separate full IDs. Creation/append
operations with the same ID and unchanged payload are idempotent; conflicting payloads
are rejected. System prompts and automatically collected environment details are not
persisted by the query recorder. Diagnostic text explicitly included in a user message
is part of that message, including for embedding eligibility.

SQLite uses foreign keys, WAL, a five-second busy timeout and related-record transactions.
The database is secured to POSIX mode `0600`; new application directories use `0700`. On Windows
an explicit DACL grants full access only to the current user and SYSTEM, including database
sidecars and migration archives. Protect WAL/SHM sidecars, exports and backups too. This is
not isolation from hostile same-user processes.

`HISTORY_ENABLED=false` and `--no-history` skip history storage opening: no database,
import or embeddings are created for that query. They do not delete existing history or
remove the active in-memory dialogue sent to the command provider. Explicit history
commands are separate operations, not disabled by this per-query opt-out.

## Automatic upgrade from legacy JSONL

Opening the repository automatically imports an existing `~/.zev/history` before
returning the active store. Both enabled queries and explicit history commands use this
path. With no source and no unfinished import, opening simply initializes/opens SQLite.

The import runs under the legacy file lock, verifies an open-file snapshot using identity,
size, modification time and SHA-256, and reads JSONL line by line. It creates local search
documents, not embeddings, and makes no network calls. Each line becomes a conversation;
missing refinements or execution results are not invented. After transaction commit and
count verification, the source is renamed without overwriting an existing archive:

```text
~/.zev/history.migrated.<sha256-prefix>.<UTC-timestamp>.<UUID>.jsonl
```

The ledger stores the intended archive path before rename; activation completes after
archive verification. A retry can finish a verified import or recover a rename followed
by interrupted activation. Fingerprints and line identity prevent duplicate imports;
two identical source lines remain two records. Invalid JSON, invalid responses or
inconsistent selections abort with a line number/error code, not the payload. A changed
source is rejected; recovery may discard only an unfinished run's imported rows and retry.
A source differing from an already completed import is a conflict, not an automatic merge.

Keep the source/archive for recovery; do not manually delete it to bypass an error. Stop
old JSONL-writing clients before upgrading and do not restart them after cutover: there
is no dual-write mode. The archive is not the active store and is not removed by `clear`.
Legacy files must be regular files with private permissions (no wider than `0600` on Unix).

## Browse, inspect and manage

```bash
zev history
zev history show <conversation-id>
zev history show --message <message-id>
zev history export --format json
zev history export --format jsonl
zev history delete <conversation-id>
zev history clear --yes
```

`show` emits JSON. Export writes complete conversation views to stdout; shell redirection
controls file permissions. Use a private directory and `umask 077`. Deletion cascades to
messages, responses, commands, search documents and embeddings; `clear` requires `--yes`.
Deletion is logical, not forensic erasure of WAL/free pages, exports, backups, legacy
archives or remote copies. Back up an active WAL database using a consistent SQLite
snapshot/backup mechanism, not a copy of only `history.db`.

## Search

Local lexical search is the default and needs no embedding provider:

```bash
zev history search "large files" --limit 20 --json
zev history search "network error" --from 2026-01-01 --to 2027-01-01
zev history search "docker" --type command --provider ollama
zev history search --exact-command 'git status --short'
zev history search --literal -- '--dry-run'
```

- `--mode lexical|semantic|hybrid`: lexical combines surface, stemmed and typo lists;
  semantic uses user-message cosine similarity; hybrid merges lexical/vector lists with
  reciprocal rank fusion (RRF). Enable and configure the separate embedding client below
  and explicitly rebuild vectors for saved messages. If embeddings are disabled or the
  query embedding fails, search visibly falls back to lexical; an empty vector index
  instead returns no semantic hits, with coverage reported in JSON.
- `--fuzzy auto|always|off`: `auto` expands when the initial surface/morphology candidate
  set is smaller than the requested limit; `always` requests expansion; `off` disables it.
- `--literal`: case-sensitive exact substring of canonical text, retaining punctuation;
  bypasses morphology, typo expansion and embeddings. It may scan stored text.
- `--exact-command`: complete command comparison byte-for-byte after UTF-8 validation;
  bypasses expansion/embeddings and takes precedence over search text. Never executes a hit.
- Filters: `--provider`, `--model`, `--status started|completed|cancelled|failed`,
  `--type user|assistant|command|explanation`, `--dangerous` / `--dangerous=false`,
  and `--from` / `--to`. Dates use a half-open UTC interval `[from, to)`; dates without
  a time mean UTC midnight. Danger filters apply to command-linked documents.
- `--limit` is 1–1000 (default 20). Empty search text is allowed only with `--exact-command`.
  Text is escaped/tokenized, not executed as a raw FTS5 expression.

Lexical ordering puts exact all-term matches before morphology and typo-only matches.
Surface/morphology tiers use RRF score, newest timestamp, then source ID; typo-only
matches are first ordered by verified all-term similarity and converted to an RRF rank
list. RRF uses `sum(1 / (60 + rank))`, with one-based ranks, not raw BM25 plus cosine
scores. The hybrid service separately fuses lexical/vector lists; its ordering does not
retain lexical quality tiers. Every query term must match through the selected lexical
path; typo expansion is not an any-term search.

Document filters are applied before each bounded FTS candidate limit. Each FTS list
retains at most 5000 documents; typo dictionary lookup is bounded to 256 candidates per
lookup/expansion path, and hybrid fusion uses at most 1000 hits per lexical/vector list.
Literal and exact-command paths do not use these FTS candidate caps. Semantic search
scans all current compatible vectors but retains only a page plus lookahead in memory.
These are fixed implementation limits, not user-configurable settings.

`--json` includes full source IDs (`documentId`, `conversationId`, `messageId` and, when
applicable, `commandId`), conversation, document type, timestamp, snippet,
`score`, `scoreKind`, `matchSources`, `requestedMode`, actual `mode`, `degraded`,
`embeddingCoverage`, `truncated` and, when available, `nextCursor`. Pass the returned
token with `--cursor` and otherwise repeat the same request, including limit and filters:

```bash
zev history search "large files" --limit 20 --json --cursor '<nextCursor>'
```

Cursors bind the request, search profile and dataset revision; changed data/indexes
invalidate them, so restart the search. `nextCursor` continues the current ranked set;
it does not bypass candidate caps. `truncated` reports observed candidate/retained-window
bounds, not ordinary page completion. Neither `truncated=false` nor an absent cursor
promises unrestricted typo recall or complete embedding coverage; inspect
`embeddingCoverage` and `degraded` as well. Search hits retain the actual source message
or command UUID, not an internal FTS row number; command/explanation hits also carry
their distinct document type. Searching never executes a matching command.

### Morphology and typos

The profile `snowball-v0.10.0-nfkc-casefold-yo2e-v1` applies NFKC, Unicode case folding,
whitespace and `ё`/`е` normalization, then Russian/English Snowball stemming per token.
Examples include `файл`/`файла`/`файлы`/`файлов`, `file`/`files` and
`connect`/`connected`/`connecting`. Other scripts retain surface tokens. This is bounded
stemming, not full lemmatization: `go`/`went`/`gone` and synonyms are not promised.

Typo lookup uses local token/deletion-key indexes and Unicode adjacent-transposition edit
distance. Query tokens shorter than four runes are not expanded; four–five allow one edit;
longer tokens allow at most two and 30% of the longer token. Expansion is limited to
32-rune query tokens, up to 600 deletion keys and bounded candidate sets (256 per
lookup/expansion path). A profile change requires rebuilding derived indexes rather
than mixing old and new tokens.

## Semantic configuration and index maintenance

Semantic search is disabled by default and configured separately from the command LLM:

```dotenv
HISTORY_SEMANTIC_ENABLED=false
HISTORY_EMBEDDING_PROVIDER=openai-compatible
HISTORY_EMBEDDING_BASE_URL=https://embedding.example/v1
HISTORY_EMBEDDING_MODEL=text-embedding-model
HISTORY_EMBEDDING_DIMENSIONS=1536
HISTORY_EMBEDDING_API_KEY=replace-me
HISTORY_EMBEDDING_MAX_BYTES=32768
```

Use an actual model/dimension pair supported by your endpoint. The client appends
`/embeddings`; command-provider keys/headers are not inherited. Oversized UTF-8 input is
rejected, not silently truncated, and its saved message remains searchable lexically.
Only `user` messages with kind `initial_prompt` or `refinement` are eligible; never
commands, assistant/system text or metadata. Vectors use model/dimensions/profile/content
hash compatibility. A remote request discloses that user text; semantic/hybrid search
also discloses its search query. `localhost` may be a forwarding proxy; use HTTPS for
remote endpoints. Set `HISTORY_SEMANTIC_ENABLED=true` with valid endpoint settings to
activate the CLI embedding client; the `false` example above shows the default.

Ordinary queries do not generate embeddings or backfill old messages, and do not add
retrieved history to new LLM prompts (no automatic RAG). Index maintenance is explicit:

```text
zev history index status --json
zev history index rebuild --fts
zev history index rebuild --vectors --yes
zev history index rebuild --vectors --retry-failed --yes
```

`status` reports lexical readiness and eligible/ready/failed/stale embedding counts for
the selected profile. Choose exactly one of `--fts` (local lexical rebuild, no embedding
requests) or `--vectors`. Vector rebuild shows provider, model and total eligible-message
count on stderr before requiring explicit `--yes`, including non-interactive use. The
count is the eligible population, not a promise that every message needs transmission:
current ready vectors are skipped; failed messages are retried only with `--retry-failed`,
which requires `--vectors`. Pending/stale messages are processed in bounded pages.
Embedding failures retain canonical messages and record controlled failure status.
When a query embedding is unavailable, search reports lexical fallback via `degraded`
and a warning; JSON carries the warning rather than printing a separate text warning.
Full cosine scan costs O(N × dimensions), uses streaming vectors and cancellation,
and is not an ANN index or a constant-latency guarantee.

## Related documentation

- [Configuration](CONFIGURATION.md) — history and embedding settings.
- [Architecture](ARCHITECTURE.md) — repository, service, lexical and client boundaries.
- [Project profile](../../PROJECT.md) — implementation facts and limitations.
