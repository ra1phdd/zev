<div align="center">

**English** | [Русский](../ru/HISTORY-SPEC.md)

</div>

# Specification: Persistent History, FTS, and Vector Search

**Status:** draft

## 1. Objective and scope

The new Zev history must persist the complete lifecycle of a dialogue locally, without a last-N retention limit, while providing exact full-text search and an additional semantic search over the meaning of user requests.

This specification covers:

- SQLite as the persistent history store;
- persistence of every message in the current dialogue, including every user refinement;
- stable identifiers for conversations, messages, responses, and suggested commands;
- FTS search over words, message text, explanations, and shell commands, including Russian morphology and typo tolerance;
- a vector index **only for user messages and refinements**;
- import of the existing JSONL file;
- CLI commands for search, inspection, export, and deletion;
- privacy, transactions, migrations, and recovery after an incomplete request.

The vector index MUST NOT be created for commands, assistant explanations, system prompts, provider metadata, or any other text that was not a user message. Vector search is not automatic RAG: matching records are not added to a new model request unless a separately defined explicit mode requests that behavior.

## 2. Normative terms

The terms **MUST**, **MUST NOT**, **SHOULD**, **SHOULD NOT**, and **MAY** have their usual RFC 2119 meaning.

- **Conversation** — one query history containing the initial message, refinements, model responses, and selection/execution result.
- **Message** — an ordered conversation item with role `user`, `assistant`, or `system`.
- **Refinement** — a separate user message after the initial request. It does not replace the initial request.
- **Response** — the structured `OptionsResponse` returned by a provider.
- **Command** — one shell-command option from a model response.
- **Lexical search** — word/token search through SQLite FTS5.
- **Semantic search** — search over embeddings of user messages and refinements.

## 3. Data requirements

### 3.1. Identifiers

Every entity MUST have its own identifier:

- `conversation_id` — conversation identifier;
- `message_id` — message identifier;
- `response_id` — model-response identifier;
- `command_id` — suggested-command identifier.

Identifiers SHOULD be UUIDv4 values generated from a cryptographically secure random source. They are stored as TEXT in canonical lowercase form. Short values shown in the TUI are only display representations and MUST NOT be primary keys.

An identifier MUST be created before the first conversation record is written. Retrying a write with the same ID MUST NOT create a new entity.

### 3.2. Conversations

The `conversations` table MUST contain at least:

| Field | Type | Requirement |
|---|---|---|
| `id` | `TEXT PRIMARY KEY` | conversation UUID |
| `created_at` | `TEXT` | RFC3339 UTC |
| `updated_at` | `TEXT` | RFC3339 UTC |
| `status` | `TEXT` | `started`, `completed`, `cancelled`, `failed` |
| `kind` | `TEXT` | launch type, for example `query` |
| `provider` | `TEXT` | provider name, nullable before first response |
| `model` | `TEXT` | model name, nullable before first response |
| `selected_command_id` | `TEXT` | nullable |
| `metadata_json` | `TEXT` | optional extensible metadata |

History MUST retain conversations without a count limit. Deleting old records after a fixed count is prohibited. If retention is added later, it must be an explicit setting and cannot be hidden rotation.

### 3.3. Messages

The `messages` table MUST contain:

| Field | Type | Requirement |
|---|---|---|
| `id` | `TEXT PRIMARY KEY` | message UUID |
| `conversation_id` | `TEXT NOT NULL` | FK to `conversations` with `ON DELETE CASCADE` |
| `sequence` | `INTEGER NOT NULL` | monotonic order within a conversation |
| `role` | `TEXT NOT NULL` | `user`, `assistant`, `system` |
| `kind` | `TEXT NOT NULL` | `initial_prompt`, `refinement`, `response`, `system_prompt`, `error` |
| `content` | `TEXT NOT NULL` | complete message text |
| `created_at` | `TEXT NOT NULL` | RFC3339 UTC |
| `metadata_json` | `TEXT` | optional metadata |

A unique constraint on `(conversation_id, sequence)` MUST prohibit duplicate positions. Sequence allocation is atomic within the write transaction; timestamps do not replace ordering. Stored messages are immutable. Retrying the same ID and payload returns the existing entity; a different payload returns a conflict without overwriting it.

Every user refinement MUST be stored as its own row with `role = 'user'` and `kind = 'refinement'`. It MUST NOT replace the `initial_prompt` or be reduced to only the last text in a conversation field.

The `content` of a user message must be the exact text formed for that message and sent to the model. If diagnostic output was added to the user prompt, that output is stored inside the same `content`; history does not create a separate log table for it. `metadata_json` is not a substitute for message content and is not semantically embedded.

The message model MUST allow additional typed parts (`parts_json` with a format version) as an extension point for future input kinds. User-message text remains in `content`; additional parts are not separate messages, are not automatically indexed by FTS, and are not embedded without a separate decision.

System prompts and automatically collected environment details are not persisted by default. The current code has no dedicated system-prompt persistence setting; this specification does not introduce one. The `system` type is reserved for extension and index-exclusion tests.

### 3.4. Responses and commands

The `responses` table MUST store the complete structured model response:

- `id TEXT PRIMARY KEY`;
- `conversation_id TEXT NOT NULL`;
- `message_id TEXT NOT NULL` — assistant message;
- `provider TEXT NOT NULL`;
- `model TEXT NOT NULL`;
- `payload_json TEXT NOT NULL`;
- `is_valid INTEGER NOT NULL`;
- `created_at TEXT NOT NULL`.

The `commands` table MUST store alternatives separately from the response JSON:

- `id TEXT PRIMARY KEY`;
- `response_id TEXT NOT NULL`;
- `conversation_id TEXT NOT NULL`;
- `ordinal INTEGER NOT NULL`;
- `command TEXT NOT NULL`;
- `short_explanation TEXT`;
- `is_dangerous INTEGER NOT NULL`;
- `dangerous_explanation TEXT`;
- `selected_at TEXT`;
- `executed_at TEXT`;
- `execution_exit_code INTEGER`;
- `execution_error TEXT`.

A unique constraint on `(response_id, ordinal)` MUST preserve option order. Commands are not vector documents. They MAY be indexed for FTS and searched through exact/structured predicates.

## 4. SQLite storage

### 4.1. File and operating mode

The primary file MUST be `~/.zev/history.db`. When the history module is first opened, an existing `~/.zev/history` MUST be migrated to SQLite automatically. The source file is not overwritten: after successful verification it is automatically renamed to an archive file beside the database. After migration, `~/.zev/history` is no longer the active store.

SQLite MUST use:

- `PRAGMA foreign_keys = ON`;
- WAL during normal operation;
- a bounded `busy_timeout`;
- transactions for related records;
- atomic migrations through a `schema_migrations` table;
- file permissions no wider than `0600` when created.

The repository MUST NOT read and rewrite the entire database for every new record. Adding a message, response, or command must be an append/update operation inside a short transaction.

The SQLite driver must preserve Zev's claimed CGO-free cross-compilation capability. A driver or SQLite extension that violates this requirement is not accepted without a separate decision and verification on all target platforms.

### 4.2. Transactions and partial conversations

Records must be written as the query progresses:

1. create the conversation and store the initial user message;
2. append a separate user message after every refinement;
3. append an assistant message, response, and commands after every model response;
4. update the selected command when selection occurs;
5. record the execution result and final conversation status;
6. preserve available messages and set `cancelled` or `failed` on cancellation or error.

A failure after the user message is stored MUST NOT lose the request. A failure after a response is stored MUST NOT remove earlier messages. Repeating an operation with the same ID MUST be idempotent or return a controlled conflict error.

### 4.3. Automatic JSONL → SQLite migrator

The migrator runs automatically when the history module is first opened if `~/.zev/history` exists and `~/.zev/history.db` does not contain a completed migration for that source. The user must not need to run a separate migration command for the normal upgrade path. Migration runs before ordinary history reads or writes and prevents a second migrator from running concurrently.

The migrator follows this algorithm:

1. Validate paths, permissions, and conflicts. `~/.zev/history` must be a regular file; `history.db` must either be an SQLite database at a supported schema version or not exist.
2. Create `~/.zev/history.db` and the base schema if needed. Schema and FTS5 are created through normal versioned SQLite migrations.
3. Compute a stable source fingerprint before reading: size, `mtime`, and SHA-256 of the content. Read through an open descriptor and verify after reading that the source did not change.
4. Create a `migration_runs`/import-ledger row with status `running`, source fingerprint, and format version.
5. Read JSONL line by line without loading the whole file into memory. Each line becomes one conversation with `metadata_json.source = "legacy_jsonl"`, `source_line`, and a line fingerprint. The old `prompt`, `response`, `provider`, `model`, `selected_command`, and `selected_index` fields map to `conversations`, `messages`, `responses`, and `commands`.
6. The old format contains no refinements, so the migrator must not invent them. The imported `prompt` becomes one `initial_prompt`; no refinements are added.
7. Create FTS documents for each imported record in the same transaction. Do not create embeddings for old records during migration; build them only through an explicit indexing operation and only for imported user messages.
8. Handle valid and corrupt lines according to a consistent policy: by default, any format error aborts migration before the result is activated, and the report contains the line number, error code, and processed count without printing the payload. A partial import must not be considered complete.
9. Commit the import in one transaction or resumable transactions with a ledger. Persist the line count, imported-conversation count, skipped/error count, and fingerprint in the ledger. Status remains `running` until final verification, then becomes `verified`; `completed` is only allowed after archiving. Records from an unfinished run are not exposed to normal history operations; these are blocked until migration finishes, with no fallback to JSONL reads/writes.
10. Re-check the source fingerprint. If the file changed during migration, migration fails, its result is not activated, and the user receives an error instructing them to retry.
11. After a successful commit and count verification, atomically rename the source to an archive path such as `~/.zev/history.migrated.<fingerprint-prefix>.<timestamp>.jsonl`. The archive is created with permissions no wider than `0600`; an existing archive path is never overwritten.
12. Only after the rename succeeds is the SQLite history considered active, and the ledger records `archived_path` and status `completed`. If rename fails, the database remains available for verification, but the active legacy file remains; retrying must not duplicate the import because of the fingerprint and line identity.

The intended archive path MUST be recorded in the ledger before rename so a crash between rename and final status commit is recoverable. A retry resumes a `verified`/unfinished run: if the source is absent and the recorded archive has the expected SHA-256, activation finishes without reimport. An archive hash mismatch or both files being absent is an error, not grounds for creating empty history. A changed source for an unfinished run requires discarding only that run’s imported rows and reimporting a verified snapshot; user records in an existing database are not removed.

The migration lock MUST cover import and archiving and use the legacy store’s locking mechanism for compatible old processes. An old version must not continue using JSONL during/after cutover; running outdated clients after migration is unsupported and is not a dual-write mode. The migrator sends nothing to an LLM or embedding provider. Queries using `--no-history`/`HISTORY_ENABLED=false` do not open history storage or trigger migration.

The migrator must not silently delete, truncate, or overwrite `~/.zev/history`. Archiving after verified migration is a mandatory part of automatically replacing the old store; the user must not manually delete the old JSONL. Normal history commands use only `history.db` after migration, while the archive remains available for manual recovery and audit.

If `history.db` already contains a completed migration for this fingerprint, a retry imports nothing. If the database contains a completed migration for a different fingerprint, changes to the old file are not imported automatically into the active database: the migrator preserves the new source as a separate conflict and reports it, preventing duplicates or silently mixed histories.

## 5. FTS5

### 5.1. Indexed scope

SQLite FTS5 MUST index these document types:

- `user_message` — initial requests and refinements;
- `assistant_message` — textual representations of model responses, if persisted;
- `command` — shell commands;
- `explanation` — command explanations and danger explanations.

Minimum document fields:

- `document_id` — message, response, or command ID;
- `conversation_id`;
- `document_type`;
- `content`;
- `command`;
- `explanation`;
- `created_at` as an `UNINDEXED` value for output and filters.

System prompts and optional metadata MUST NOT enter FTS unless separately enabled by a privacy setting. The index must be updated in the same transaction as the source record or have a verifiable rebuild mechanism.

### 5.2. Russian and English morphology and typo tolerance

Ordinary text search MUST have at least three lexical representations of a document:

1. `surface` — original tokens for exact search;
2. `normalized` — Unicode NFKC, case folding, `ё/е` normalization according to the selected profile, and repeated-separator cleanup;
3. `morphology` — language-specific lemmas or stems produced by a versioned analyzer port.

`morphology` MUST make inflected forms of one word searchable together. The Russian acceptance set includes `файл`, `файла`, `файлы`, and `файлов`; the English set includes at least `file`, `files`, `connect`, `connected`, and `connecting`. The English analyzer MUST handle English morphology rather than falling back to Unicode normalization and surface tokens. Unknown languages use a language-neutral fallback: Unicode normalization plus surface tokens; it must not invent Russian or English lemmas.

SQLite FTS5 does not provide a ready Russian or English lemmatizer. The implementation must therefore choose and pin a language-aware analyzer after a small benchmark/spike, rather than pretending that `unicode61` solves morphology. A lemmatizer supporting Russian and English forms, including irregular forms, is preferred; a stemmer is allowed as a fallback only for an explicitly bounded acceptance set. The analyzer must have a versioned `profile_id`, be deterministic, and be applied identically to indexed documents and queries. Changing the analyzer makes the `morphology` index stale and requires a rebuild; old and new tokens must not be silently mixed.

The morphology index applies to user messages and refinements, assistant text, and explanations. Language is selected from the language profile and/or token script: Cyrillic tokens use the Russian profile, Latin tokens use the English profile by default, and mixed text is processed per token. Command tokens must not accidentally switch the language of the whole message. For `command`, the canonical value remains the original shell text: morphology processing must not change, normalize, or substitute the full command. Exact command search always compares the original value.

Typos MUST be handled by a separate fuzzy layer, not only by embeddings:

1. build an additional FTS5 trigram index for text-search fields;
2. retrieve a bounded candidate set by trigram overlap;
3. calculate Unicode-aware Damerau–Levenshtein or equivalent edit distance in Go for candidate tokens;
4. accept a match only with a length-dependent threshold: tokens shorter than 4 characters require an exact match, 4–5 characters allow at most 1 edit, and 6+ characters allow at most 2 edits and at most 30% of token length;
5. rank fuzzy hits by `typo_similarity`, not as ordinary BM25.

Distance is calculated over Unicode code points after normalization, not UTF-8 bytes. A transposition of adjacent characters counts as one error. The 30% limit uses the maximum length of the two tokens. Trigram `MATCH` alone is not fuzzy search: the entire misspelled string is not used as the only pattern; the query combines escaped individual trigrams with OR. Matching all trigrams must not be required.

Trigrams alone are insufficient: `файл` and `фалй` share no trigrams despite differing by one transposition. A supplementary local surface-token dictionary with a deletion-key index (up to two deletions), or another indexed token edit-distance lookup, MUST therefore exist. This path is required for short words and candidates with no shared trigrams; scanning all messages is prohibited. Candidates are verified by actual edit distance, so a deletion-key collision is not a match. If candidate limits apply, the response exposes `truncated` rather than promising complete coverage. Expansion/token-length/candidate-count limits are fixed in the analyzer profile and benchmarked.

To combine inflection and typos, the pipeline expands a misspelled token into surface-dictionary candidates, then applies the same Russian analyzer and searches the morphology index. This is query expansion, not mutation of stored user messages. `ё → е` normalization is enabled by default only for derived text indexes. Stemming (for example, Snowball Russian or Snowball English) is the minimum morphological-search option, not full lemmatization: suppletive forms and synonyms are not guaranteed; irregular forms such as `go`/`went`/`gone` require a lemmatizer rather than a stemmer alone. Supported Russian and English groups are fixed in acceptance fixtures. Morphology and typo search run locally without an LLM, remote API, embeddings, or mandatory native dependency. An exact all-term match outranks a result based only on typo expansion; RRF merges lists within a documented quality tier.

The fuzzy layer may search commands for history purposes, but it must never select or execute a found command automatically. `--exact-command` disables morphology/fuzzy expansion and compares the stored shell text byte-for-byte after validating UTF-8.

Hybrid lexical ranking MUST merge separate lists through RRF rather than adding incompatible BM25, typo, and vector scores. Results expose `match_sources` (`exact`, `morphology`, `typo`, `semantic`) and `score_kind`. If the analyzer or trigram index is not built, the mode is explicitly marked `degraded`; silent fallback is prohibited.

### 5.3. Indexes and lexical pipeline

The minimum implementation must have separate FTS5 virtual tables or equivalent versioned index views:

- `history_fts_surface` — original/normalized tokens, tokenizer `unicode61`;
- `history_fts_morphology` — analyzer-generated lemma/stem tokens, with no additional morphological logic in the tokenizer;
- `history_fts_trigram` — `tokenize = 'trigram'` for bounded candidate retrieval on typos.

All three indexes use the same `document_id`, `conversation_id`, `document_type`, and version/profile metadata. They may be contentless/external-content tables, but rebuilding from canonical tables MUST be supported. Changes to canonical text, analyzer profile, or tokenizer profile make the corresponding index rows stale and require an atomic update or a verifiable later rebuild.

`--mode lexical` means a hybrid inside lexical search: exact/surface, morphology, and typo candidates are combined through RRF. By default fuzzy search operates in `auto` mode: it is added when exact/morphology candidates are insufficient for the requested limit; `--fuzzy always` enables it explicitly, while `--fuzzy off` keeps only exact/morphology. Semantic vector search is not part of lexical mode.

Lexical search must return not only the final score but also `match_sources`, so the user can see whether a match came from a word form or typo correction. Tie-breaking remains deterministic: score, then `created_at DESC`, then full ID.

### 5.4. Search behavior

The search command must support at least:

```text
zev history search <text>
zev history search <text> --limit 20
zev history search <text> --from 2026-01-01 --to 2026-12-31
zev history search <text> --provider ollama
zev history search <text> --type user
zev history show <conversation-id>
zev history show --message <message-id>
```

Search MUST:

- return conversations and the specific matching documents;
- show `conversation_id`, date, role/type, a short snippet, and score;
- use deterministic ordering: relevance first, then `created_at DESC`, then ID;
- support limit and pagination/cursor;
- handle empty queries and FTS5 special characters correctly;
- avoid executing raw FTS5 expressions from user input without escaping or an explicitly documented advanced-syntax mode.

Shell-command search must support:

- word and token search through FTS5;
- exact matching through `commands.command`;
- filters by provider, model, date, status, and danger flag.

FTS search must work without an embedding provider.

## 6. Vector index

### 6.1. Vectorization scope

Only `messages` rows satisfying both conditions may be embedded:

```text
role = 'user'
kind IN ('initial_prompt', 'refinement')
```

Embeddings are forbidden for:

- shell commands;
- assistant explanations and warnings;
- assistant messages;
- system prompts;
- provider/model metadata;
- diagnostic fields that are not part of the user message.

If a user log was included in the `content` of a user message, it is part of that message and may be included in its embedding. A separate embedding for the log MUST NOT be created.

### 6.2. Embedding storage

The `message_embeddings` table MUST contain:

- `message_id TEXT PRIMARY KEY`;
- `model TEXT NOT NULL`;
- `dimensions INTEGER NOT NULL`;
- `vector BLOB` — float32 in a fixed little-endian format;
- `content_hash TEXT NOT NULL`;
- `created_at TEXT NOT NULL`;
- `status TEXT NOT NULL` — `ready`, `failed`, `stale`;
- `error TEXT`.

`content_hash` is required to detect changed text and regenerate the embedding. An embedding MUST NOT be considered current based on message ID alone.

For `ready`, `vector` is required; for `failed`/`stale`, it may be NULL. A newly stored message without an embedding has no row yet. A vector must contain exactly `dimensions * 4` bytes; NaN, infinity, and zero norm are invalid.

Each row additionally stores `profile_id`: a profile identifier fixing provider, model/version, dimensions, preprocessing version, cosine metric, and encoding. Queries are compared only with `ready` vectors in their own profile. `content_hash` is SHA-256 of exact UTF-8 `content`; identical texts from different messages are not collapsed into one entity.

The baseline implementation must store embeddings in SQLite and calculate cosine similarity in Go. It MUST NOT require a native SQLite extension if that breaks CGO-free builds. At larger volumes an ANN adapter MAY be added without changing the history contract.

### 6.3. Generation and failure handling

Message persistence MUST NOT depend on successful embedding generation. Embeddings are generated after the message is committed or in a separate explicitly controlled step.

If the embedding provider is absent, unavailable, or returns an error:

- the message and FTS record are still stored;
- search falls back to FTS;
- the embedding failure is recorded in `message_embeddings` or diagnostic logging;
- the user receives a clear warning only when semantic/hybrid mode was requested.

The embedding model and dimensions are part of the index configuration. Embeddings from different models MUST NOT be compared: changing the model creates a new index version or marks existing rows `stale` for rebuilding.

Deleting a message or conversation MUST delete its embedding through a foreign key or transaction. Index repair must find all user messages without a current embedding.

### 6.4. Semantic and hybrid search

Semantic search returns matching user messages and their parent conversations. It MUST NOT replace the matched user wording with a command from another record.

Recommended hybrid behavior:

1. retrieve FTS candidates;
2. retrieve cosine-similarity candidates only from user/refinement embeddings;
3. merge and deduplicate by `message_id`/`conversation_id`;
4. apply normalized reciprocal-rank fusion or another documented formula;
5. return explainable match sources: `fts`, `semantic`, or `hybrid`.

The search command must explicitly choose a mode, for example `--mode lexical|semantic|hybrid`. The default is `lexical`, so search remains fast, predictable, and available without an embedding provider.

Vector matches may be used for history browsing, but automatic injection of matched conversations into a new LLM request is out of scope for this specification.

## 7. Public history-module interface

The implementation may differ, but the external interface must hide SQL, FTS parsing, serialization, and vector calculation. The minimum operation set is:

```go
type Store interface {
    StartConversation(ctx context.Context, input StartConversationInput) (Conversation, error)
    AppendMessage(ctx context.Context, input AppendMessageInput) (Message, error)
    RecordResponse(ctx context.Context, input RecordResponseInput) (Response, error)
    RecordSelection(ctx context.Context, input RecordSelectionInput) error
    CompleteConversation(ctx context.Context, input CompleteConversationInput) error
    GetConversation(ctx context.Context, id string) (ConversationView, error)
    Search(ctx context.Context, request SearchRequest) (SearchResult, error)
    DeleteConversation(ctx context.Context, id string) error
}
```

The service must receive its storage adapter from the composition root. The CLI must not open SQLite or assemble SQL statements directly. Search must return a structured result usable by both the TUI and `--json` output.

## 8. CLI contract

The existing `zev history` command remains an interactive browser, but now reads SQLite and displays conversations rather than only final records.

Minimum commands:

```text
zev history
zev history search <query>
zev history show <conversation-id>
zev history export [--format json|jsonl]
zev history delete <conversation-id>
zev history clear
```

`clear` MUST require explicit confirmation or a separate non-interactive flag. Deletion should be limited to specified IDs unless the user confirms clearing the entire database.

Scripts need `--json`. JSON output must include full IDs, timestamps, document type, score, snippet, and conversation ID. The TUI may display short IDs, but actions and copied references must use the full ID.

The existing `--no-history` semantics remain: the current conversation, messages, FTS documents, and embeddings are not persisted. Existing records are not deleted.

## 9. Privacy and security

- The database file, export files, and backups are created with permissions no wider than `0600`.
- Prompts, refinements, commands, and model responses are sensitive data.
- Application logs must not contain secrets.
- A remote embedding provider is optional and must be explicitly configured; sending user messages outside the machine must be visible in documentation and settings.
- Embeddings are also sensitive data: they must not be stored in a shared cache or sent to a provider without permission.
- Search and migration errors must not print the full payload unless the user explicitly requested an export.
- Deleting a conversation must remove its messages, responses, commands, FTS documents, and embeddings.

## 10. Compatibility and migrations

The schema must have a version and sequential migrations. SQLite structure must not be changed manually outside a migration.

The first migration must:

1. create `history.db` and the base schema;
2. enable FTS5;
3. automatically migrate the old JSONL when the history module is first opened;
4. verify the fingerprint, line count, imported-conversation count, and errors;
5. persist migration state and the import ledger;
6. after successful verification, rename the source JSONL to an archive file without overwriting it.

Migrations must be idempotent and safely resumable after interruption. Destructive operations require a backup or explicit user confirmation.

## 11. Testing and completion criteria

### Unit tests

- conversation creation and ID uniqueness;
- preserving multiple refinements without losing the initial request;
- partial/failed/cancelled conversation persistence;
- idempotent writes with the same ID;
- FTS search for Russian and English words;
- matching Russian inflected forms (`файл`/`файла`/`файлы`/`файлов`) and English forms (`file`/`files`, `connect`/`connected`/`connecting`);
- the selected lemmatizer handles tested irregular forms (`go`/`went`/`gone`) or explicitly reports that they are outside the stemmer profile;
- Cyrillic and Latin typo tolerance with Unicode Damerau–Levenshtein;
- no false fuzzy matches for short tokens;
- search for flags and shell commands;
- escaping FTS5 special characters;
- filters and deterministic pagination;
- cosine similarity and vector normalization;
- no embeddings for assistant/system/command documents;
- regeneration after `content_hash` changes;
- semantic/hybrid fallback to FTS when the embedding provider is unavailable;
- import of valid and corrupt JSONL lines;
- removal of all dependent records.

### Integration tests

- concurrent writes from two processes;
- FTS recovery after an interrupted transaction;
- automatic migration of a real old JSONL file without interactive confirmation;
- archiving only after verification, and failure on corrupt lines or source changes;
- two concurrent migrators, an existing database with user records, identical lines, and duplicate-free retries;
- crashes before commit, after commit, before/after rename, and before final activation;
- rename failure, occupied archive path, invalid archive hash, and ledger recovery;
- no migration with `--no-history`/`HISTORY_ENABLED=false` and no network calls during import;
- reading the database after a CLI restart;
- preserving `0600` permissions;
- operation on target platforms without mandatory CGO.

### Acceptance criteria

The feature is complete when:

- records older than the first 100 conversations remain available;
- the initial request and every refinement are visible in the correct order;
- every conversation, message, response, and command has its own stable ID;
- word, explanation, and shell-command search works without an embedding provider;
- lexical mode without networking or embeddings finds `файлов` for `файл`, `резервные копии` for `резервная копия`, and matches `ёлка` with `елка` in normalized search;
- `--fuzzy always` finds `файл` for `фалй`, `docker` for `dokcer`, and `резервная` for `резерная`; tests cover insertion, deletion, substitution, transposition, combined typo/inflection, and unrelated words outside the threshold;
- exact matches are not displaced by typo-only candidates; `--fuzzy off`, `--literal`, and `--exact-command` do not correct input; all derived indexes rebuild after migration/rebuild and are removed with their source records;
- semantic search embeds only user initial/refinement messages;
- the database contains no embeddings for commands or assistant messages;
- an incomplete or failed query does not lose already stored messages;
- the old `~/.zev/history` is automatically migrated to `~/.zev/history.db`, and after verification the source is archived by rename without overwriting it;
- `zev history`, `search`, `show`, `export`, and deletion use one history module;
- `make test`, `make test-race`, `make lint`, and `make check` pass after implementation.

## 12. Out of scope

- automatically adding retrieved history to a model prompt;
- changing provider response formats;
- changing shell-command execution or approval rules;
- storing raw command output as a separate history entity;
- a server database or mandatory external search service;
- automatic count-based history cleanup.

## 13. Contract clarifications and specification checks

### Integrity and lifecycle

All cross-table references MUST have foreign keys with cascading deletion, except `selected_command_id`, which uses `ON DELETE SET NULL`. The service additionally verifies that the selected command belongs to the same conversation, using composite constraints or transactional validation. Boolean fields are constrained to `0/1`, `sequence` and `ordinal` to non-negative values, and JSON to valid documents.

`completed` means the Zev interaction ended, not that command execution was proven. Delivery outcome separately distinguishes `selected`, `injected`, `printed`, `executed`, `failed`, and `unknown`. Injected terminal input receives neither `executed_at` nor an exit code. After a crash, `started` is not automatically converted to `failed`: another process must not close an active conversation without evidence that its owner ended. Uncertain execution is shown as `unknown`, not success.

Assistant responses are persisted completely, not only through `OptionsResponse.CompactJSON()`. Writing a response, its assistant message, and all command alternatives is atomic. No command strings from a full response become embedding rows.

### Lexical modes and ranking

The baseline FTS tokenizer is `unicode61` with Unicode case folding and diacritic normalization. It remains the exact lexical layer, but it is NOT the only search mechanism: Russian morphology and typo tolerance MUST be provided by separate indexes and ranking stages below. ID/type/date metadata is `UNINDEXED`; structured filters join ordinary tables. BM25 sorts ascending (lower is better); JSON includes `score_kind`, so raw BM25 is not presented as cosine similarity.

Additional explicit options:

```text
zev history search --mode lexical --exact-command 'git status --short'
zev history search --mode lexical --literal -- '--dry-run'
zev history search 'large files' --mode hybrid --json --limit 20
zev history search 'network error' --model MODEL --status failed --dangerous
```

`--exact-command` compares the entire command without normalization; `--literal` searches an exact substring preserving punctuation and disables morphology/typo/semantic expansion. Literal search MUST escape `%`, `_`, and the escape character as data; a parameterized substring scan with documented cost is acceptable. `--` separates text starting with a dash. FTS must not be advertised as exact shell-flag search: `--dry-run` and `dry run` may produce identical tokens. An empty positional query is allowed only with `--exact-command`; otherwise it is a validation error.

Canonical `--type` values are `user`, `assistant`, `command`, and `explanation`; the service maps them to index document types. Date ranges are half-open UTC intervals `[from, to)`; a date without time means UTC midnight. Filters apply before top-K and pagination in every mode. Grouping must not lose matches: each result includes its conversation and matching documents with message/command IDs. Identical text in response JSON and an extracted command must not artificially double its rank.

Hybrid uses RRF `sum(1 / (60 + rank))`, with ranks starting at 1, rather than summing incompatible BM25/cosine scores. Commands may participate in the lexical part of hybrid, never its vector part. Results expose actual mode, embedding coverage, and `degraded`; semantic → lexical fallback cannot be silent. Pages bind query/filter/profile and a dataset watermark; changed data/index invalidates the cursor with a clear error instead of falsely promising snapshot consistency.

### Vector configuration and maintenance

`HISTORY_SEMANTIC_ENABLED=false` by default. Enabling it and selecting an embedding provider/model is explicit opt-in, separate from the command LLM. Localhost does not guarantee privacy if the endpoint forwards data externally. Ordinary queries do not automatically backfill old history.

Target maintenance commands:

```text
zev history index status --json
zev history index rebuild --fts
zev history index rebuild --vectors
zev history index rebuild --vectors --retry-failed
```

Vector rebuild shows provider/model and user-message count before confirming transmission; non-interactive use requires explicit consent. Oversized messages must not be silently truncated: exceeding the embedding model limit records a controlled failure and leaves the message searchable through FTS. Embedding writes re-check message existence, hash, and profile within a transaction; concurrent deletion or profile changes must not resurrect data. External HTTP requests do not hold a SQLite write lock.

Full cosine scan costs O(N × dimensions); constant latency is not promised for unlimited history. The implementation MUST provide a fixed-dataset benchmark, cancellation through `ctx`, bounded memory, and no loading of every blob into memory. ANN requires measurements and a separate decision.

### Privacy, migration, and errors

`HISTORY_ENABLED=false` and `--no-history` also prohibit background embeddings, new storage files, and import during the disabled query. Explicit commands reading existing history do not erase it. `CONTEXT_ENABLED` controls sending environment details, not permission to store system prompts or attachments.

`0600` applies on POSIX; Windows requires equivalent ACL restrictions. The directory must exclude other users, and WAL/SHM sidecars and temporary files must be protected like the database. Logical DELETE does not guarantee forensic erasure from WAL, free pages, backups, or legacy JSONL. History deletion must not falsely promise removal of external copies.

JSONL migration is automatic when the history module is first opened; an empty database with no source file means ordinary first initialization. An import ledger records source fingerprint and line identity so retries do not duplicate records, while two identical source lines remain two records. The source is imported from a verified snapshot; changing it during import is rejected. Corrupt lines abort migration before activation by default; they are reported, not removed, and errors exclude payloads. An unknown newer schema version returns a clear error without downgrade. Backups of an active WAL database use SQLite backup API or a consistent snapshot, not a copy of only the `.db` file.

Typed module errors: `not_found`, `invalid_query`, `conflict`, `busy`, `cancelled`, `storage_failure`, `unsupported_schema`, `embedding_unavailable`. `errors.Is`/`errors.As` preserve classification. History-write failure gives an explicit warning, does not pretend persistence succeeded, and does not repeat an already executed shell command. The TUI treats stored text as data and escapes control/ANSI sequences.

### Architecture, verification, and open decisions

SQL and migrations belong in `internal/entities/history/repository`, history rules in `internal/entities/history/service`, and the network embedding adapter in `internal/entities/history/client`; `cmd/zev` remains composition root and CLI. A minimal consumer-side storage port and a separate embedding port must not make the repository responsible for TUI or network calls. Go conventions, `ctx` first, named input types, English comments, and adjacent tests follow project rules.

Examples above are **proposed**, not existing CLI commands. Implementation uses the existing verification commands:

```bash
make test
make test-race
make lint
make check
make build
make clean
```

Additional acceptance tests MUST prove: literal mode distinguishes shell flags, fallback is marked degraded, embedding failure does not lose records, profile/model vectors never mix, crashes do not mark uncertain execution successful, import/write retries are idempotent, and `--no-history` creates no history-storage side effects. English-specific tests cover plural, third-person singular, past tense, progressive, and irregular verb forms; typo tests cover Latin tokens as well as Cyrillic tokens.

The specific CGO-free SQLite driver, Russian and English morphology analyzer/library and its language profiles, embedding provider/model, and numerical benchmark budgets on agreed hardware remain open before implementation. Versions are pinned after a spike and target-architecture verification; this specification adds no invented dependencies to `go.mod`. Context7 is unavailable in the current client and SQLite is not pinned in `go.mod`, so technical requirements were checked against the [official FTS5 documentation](https://www.sqlite.org/fts5.html) and [PRAGMA reference](https://www.sqlite.org/pragma.html), [Snowball Russian stemming algorithm](https://snowballstem.org/algorithms/russian/stemmer.html) and [Snowball English stemming algorithm](https://snowballstem.org/algorithms/english/stemmer.html); actual driver and analyzer behavior must be verified during implementation.

This specification does not authorize implementation, runtime configuration changes, accepted ADRs, dependency installation, migration of the user's database, or committing unrelated edits. An implementation plan can follow document approval.
