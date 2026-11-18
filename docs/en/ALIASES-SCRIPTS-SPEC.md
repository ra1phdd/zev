<div align="center">

**English** | [Русский](../ru/ALIASES-SCRIPTS-SPEC.md)

</div>

# Specification: Managed Aliases and Scripts

**Status:** draft, revised after adversarial review

## 1. Objective and scope

Zev must provide one predictable mechanism for storing and running command aliases and
Bash scripts. Each entity may have a description and an ordered positional-argument
contract. These fields must be consumed from one validated model by lists, the TUI, help,
launch errors, and machine mode.

The scope includes:

- optional `description` and `arguments` for aliases and scripts;
- interactive and fully command-driven CRUD;
- explicit generation of missing metadata through a separate LLM port;
- `zev <name> --help` without execution or network calls;
- required-argument validation before launch;
- preservation of original `argv` boundaries;
- canonical YAML, strict validation, and legacy migration;
- recovery after publication failures and stale-edit protection.

Changing ordinary query `inject`/`execute` rules, generating script bodies, adding a shell
sandbox, and remote synchronization between computers are out of scope.

## 2. Capability map and delivery order

This request contains several independently testable capabilities. Deliver them as vertical
slices:

| Slice | Result | Depends on |
|---|---|---|
| `metadata-model` | shared model, codec, validator, version-2 read/write | — |
| `alias-crud` | alias CRUD, help, uniform argv-safe execution | `metadata-model` |
| `script-crud` | script metadata/body CRUD, help, exact argv | `metadata-model` |
| `metadata-generation` | separate LLM port, consent, fallback, provider adapters | `metadata-model`, CRUD |
| `migration-recovery` | v1.0.0 migration without generated archives, journal recovery, concurrency tests | `metadata-model`, CRUD |
| `native-acceptance` | shell/Bash runtime checks on Linux, macOS, and Windows | all previous slices |

Each slice has focused tests and a checkpoint. The complete acceptance gate is not green
until every slice is checked; green host tests do not replace native runtime verification.

## 3. Terms and model

**MUST**, **MUST NOT**, **SHOULD**, **SHOULD NOT**, and **MAY** have their RFC 2119 meaning.

- **Alias** — a named shell-command template.
- **Script** — a registered Bash file and its metadata.
- **Metadata** — `description` and ordered `arguments`.
- **Positional argument** — one original `argv` element passed after the entity name.
- **Legacy source** — an old-format aliases/scripts file before migration.
- **Argv alias** — every canonical alias after migration, using the restricted template dialect
  and exact data substitution.

Compatibility is release-scoped. The supported legacy sources are the unversioned
v1.0.0 aliases/scripts metadata files; current repositories read only the current v2
envelopes. Development snapshots and their intermediate schemas are not migration
inputs and have no backwards-compatibility promise. Future steps are appended only
when a new format is released.

### 3.1. Shared metadata

```yaml
arguments:
  - name: service
    description: Service name or ID
    required: true
```

- `name` is required, unique within one entity, an ASCII identifier of 1–64 bytes;
- `description` is optional and may be empty;
- `required` defaults to `true`; omission must not erase an explicit `false`;
- positions start at 1 and cannot be sparse;
- required positions precede optional positions;
- duplicate names, nulls, wrong types, invalid UTF-8, and unknown fields are rejected;
- metadata does not contain the script body and is not a second source of body content.

New names match `[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}` and are case-sensitive. Built-in Cobra
command names (`alias`, `script`, `query`, `history`, `setup`, `doctor`, `version`,
`continue`, `help`, and future reserved names) are forbidden. Aliases and scripts use
separate namespaces; when names collide, root invocation chooses the alias, preserving
current precedence. Explicit `zev script ...` and `zev alias ...` remain unambiguous.

Legacy records are not silently renamed. An invalid legacy name blocks activation and
remains unchanged in the legacy source. The diagnostic names it and instructs the user to
stop Zev writers, back up the file, manually repair the legacy key, and retry. Because normal
CRUD requires a readable store, it MUST NOT be used to repair a blocked migration. An
optional future `zev alias repair rename` / `zev script repair rename` command may provide
this operation only as an offline, locked repair; it is not part of the first implementation.

### 3.2. Alias

An alias has `name`, `template`, `description`, and `arguments`. All aliases use one
argv-safe execution contract. There is no persisted `mode` field, mode-selection flag, alternative
renderer, or legacy execution path. Old and new records use the same model after migration.

Migration compiles every old template into the argv-safe dialect. It converts supported
numbered placeholders, converts old implicit-append behavior to an explicit `{{.args}}`,
and validates the resulting closed grammar. Migration never preserves raw legacy shell
semantics. An old template that cannot be converted without changing its argument boundaries
blocks migration with a named diagnostic; it is not assigned a legacy mode or silently
rewritten.

the argv-safe dialect supports only standalone placeholders:

- `{{.arg1}}`, `{{.arg2}}`, … — one shell word;
- `{{.args}}` — zero or more shell words in original order.

The dialect is a closed grammar: one simple command consisting of literal, unquoted
words and standalone placeholders. It has no pipes, redirections, comments, assignments,
quoting around placeholders, escaped newlines, command/arithmetic substitution, heredocs,
grouping, control operators, or other shell syntax. A placeholder inside a quoted or
concatenated word (`prefix{{.arg1}}`) is rejected before save and before launch. The
interpreter is the configured POSIX-compatible shell, invoked with the rendered command;
an unsupported shell returns an explicit error. Each value is rendered with the dialect's
POSIX single-quoted-word algorithm. There is no fallback to a wider template grammar.

the argv-safe dialect has one computable effective argument contract:

- numbered placeholders MUST form the contiguous range `{{.arg1}}` through `{{.argK}}`,
  with `K` between 1 and 32;
- metadata entries are merged by position; an uncovered numbered position defaults to
  `required: true` and display name `argN`;
- metadata positions outside `1..K` are rejected; explicit `required: false` makes that
  numbered position optional;
- required positions precede optional positions, and supplied empty values count as present;
- each numbered placeholder emits zero or one word; an omitted optional placeholder emits
  zero words;
- `{{.args}}` emits every supplied argv element, including elements also referenced by
  numbered placeholders, and imposes no positional minimum;
- without `{{.args}}`, supplied values beyond `K` are rejected before shell launch;
- a template without placeholders MUST have empty arguments metadata and accepts no argv.

`{{.args}}` cannot be combined with implicit append. Requiredness is determined only by this
metadata/placeholder contract, never by heuristics over arbitrary Go templates.

All canonical aliases, including migrated aliases and aliases created by `alias last`, use
the argv-safe dialect. Help never displays a legacy warning. Existing shell syntax that is outside
the closed grammar is a migration error requiring manual repair of the source before retry;
there is no compatibility escape hatch in the data model.

### 3.3. Script

A script has `name`, immutable `recordID`, `script` (a relative path inside `script_files`),
`description`, and `arguments`. `recordID` is generated on creation and migration and is
required in version 2; manual removal or alteration is a conflict. The body is not stored
in YAML: YAML owns metadata and the validated path, while a separate regular file owns the
body. Execution and generation use an already-open, validated file descriptor and do not
reopen a replaced pathname.

Scripts are not analyzed to infer an argument contract. `arguments` is the only source of
requiredness; the body receives exact `argv` and `$@`. Without metadata, a script accepts
any number of arguments, but help makes no promise about named parameters. Existing rooted
path, no-symlink, regular-file, single-hard-link, and pinned-file checks remain in force.

## 4. Canonical YAML and strict codec

### 4.1. Version 2

`~/.zev/aliases`:

```yaml
version: 2
aliases:
  dps:
    template: docker service ps {{.arg1}} --no-trunc
    description: Show Docker Swarm service tasks without truncating output
    arguments:
      - name: service
        description: Service name or ID
        required: true
```

`~/.zev/scripts`:

```yaml
version: 2
scripts:
  deploy:
    recordID: 550e8400-e29b-41d4-a716-446655440000
    script: deploy.sh
    description: Deploy the application to production
    arguments:
      - name: environment
        description: Target environment
        required: true
```

A scalar integer top-level `version` denotes an envelope: `version: 2` requires the
corresponding `aliases`/`scripts` map, and an incomplete or invalid envelope is an error.
A legacy map with an entity named `version` is accepted only when its value has the legacy
record shape; otherwise classification is ambiguous and fails closed. Duplicate YAML keys,
multiple documents, null roots, wrong types, and unknown fields are errors; malformed v2
must not fall back to legacy. Unknown newer versions are not downgraded or replaced with
an empty file.

The persistence-neutral metadata model and semantic validator live in a neutral internal
package. One versioned codec serves both repositories. Alias/script repositories own only
paths, locks, and publication; the CLI does not parse YAML.

Validation also limits sizes: metadata files, migration journals, and staged payloads are at most 1 MiB, YAML depth is
at most 16 nodes and 10,000 nodes, templates are at most 16 KiB, and script paths are at
most 512 bytes. Name is limited to 64 bytes, description to 4096 bytes, argument name to
64 bytes, argument description to 1024 bytes, and arguments to 32 items. The decoder reads
through a bounded reader and rejects anchors, aliases, merge keys, custom tags, NUL, C0/C1,
ANSI escapes, invalid UTF-8, and trailing documents. Terminal renderers escape LF, backspace,
ESC, and other display-affecting controls; machine output JSON-encodes every user field.
The canonical YAML codec rejects anchors, aliases, merge keys, and custom tags.

### 4.2. Legacy migration and recovery

Legacy formats:

- aliases: YAML map `name: string-template`;
- scripts: the existing YAML map with `script` and an existing `description`.

Migration MUST preserve every existing script description byte-for-byte after UTF-8
validation and must not change script bodies. Missing fields become empty; existing fields
are never replaced with empty values. Every migrated alias is compiled and stored as
the argv-safe dialect; no migrated record receives a legacy mode.

Migration runs before the first operation that requires the store: list, show/help, CRUD,
and named execution. `zev --help`, `version`, and a query that ends before named lookup do
not create metadata files. A missing active file means an empty store; a read-only or
malformed file ends the operation without writing.

After a successful migration, migrated records remain usable without AI enrichment. The
post-migration one-time enrichment offer, durable offer state, and bulk consent flow are
deferred to a follow-up slice; this release does not claim to provide them.

The deferred enrichment flow is not shown, persisted, or attempted by this release.

Each migrator uses exactly the same canonical lock identity and lock order as its repository.
Guarantees apply to cooperating writers; arbitrary writes that bypass the lock are outside
this threat model. Metadata migration generates no archive, archive sidecar, or backup copy:
it publishes canonical v2 from the legacy source using private staging files and a durable
journal. Staging files and journals are recovery artifacts, not user-owned backups. Existing
user-owned backups, exports, and other historical copies are outside this protocol; Zev does
not sweep or promise to delete them, and migration provides no secure-erasure guarantee.

Before any metadata read or write, recovery runs under the repository lock. It reconciles a
pending journal with the source and active-file identities. A valid incomplete journal may
finish the pending publication or clean up its own staging artifact; an already-published
matching state may finish journal cleanup. A changed source, malformed journal, or
conflicting state fails closed without guessing. No unknown path is deleted or overwritten.
Markerless or unpublished journals from development snapshots are not interpreted, recovered,
or swept. Any error after a rename or publication is committed-but-durability-unknown
(`errors.Is(..., filestore.ErrCommitted)`), not evidence that nothing changed. The shared
filesystem primitives in `internal/migrations/filetxn` cover this migration
staging/publication/recovery boundary; they are not SQLite transaction/rollback machinery
and do not provide rollback for script CRUD.

### 4.3. Script body and metadata transactions

An operation changing both body and metadata runs under the script-store lock through staging
and a recoverable journal with phases `prepared`, `body-published`, `metadata-published`,
`done`. The journal records operation kind, immutable record ID, old/new metadata hashes,
old/new body hashes, path identities, staged/recovery paths, and a checksum-protected phase.
This is a script-CRUD publication journal, separate from migration recovery; it is not a
general rollback mechanism. Before any script-store read or write—including list, help, show,
generation, and execution—the process acquires the same lock and recovers every pending journal.
Recovery completes only a known publication or blocks on an unresolved mismatch; it does not
promise to restore an earlier user-owned backup. On platforms without this protocol, combined
body and metadata publication is unavailable rather than falsely reported as safe.

A script record has an immutable generated `recordID` retained across edits and assigned during
migration. A stale edit compares that ID, selected metadata bytes, script-path identity, and
body content hash/identity. Deletion/recreation is therefore a conflict; unrelated-record
changes do not cancel a field-level edit. Manual removal or alteration of `recordID` is a
conflict, not a new identity.

## 5. CLI contract and mode matrix

### 5.1. Aliases

```text
zev alias add [name template [description [arguments-json]]]
zev alias edit [name]
zev alias remove [name]
zev alias list [--json]
zev alias last <name>
zev <alias> [argv...]
zev <alias> --help
zev <alias> -- --help
```

Flags:

```text
--template TEXT
--description TEXT
--argument NAME[=DESCRIPTION]   # replacement ordered list, required=true
--arguments-json JSON           # replacement ordered list, supports required=false
--clear-description
--clear-arguments
--generate-description
--generate-arguments
--no-metadata
--json
--non-interactive
```

Positional values and metadata flags are mutually exclusive for the same field. `--argument`
and `--arguments-json` are mutually exclusive. A value, clear, and generate option for one
field are mutually exclusive. Edit is a patch: an absent field is preserved, a supplied
field replaces, and `--clear-*` clears. `--no-metadata` is alias-add-only: it skips metadata prompts/generation and stores empty
metadata; it conflicts with metadata values and is rejected on edit. `--yes` on a named
remove is an accepted idempotent compatibility flag with no additional effect; it does not
select an unnamed record or bypass interactive confirmation.

`zev alias add` with no arguments opens an interactive form. `zev alias edit` without a
name lists all aliases and, after selection, walks through template, description, and
arguments. Each field is prefilled; Enter preserves it and new input replaces it.
`zev alias remove` without a name lists aliases and requires a separate confirmation.

### 5.2. Scripts

```text
zev script create [name [description [arguments-json]]]
zev script edit [name]
zev script remove [name]
zev script list [--json]
zev script show <name>
zev <script> [argv...]
zev <script> --help
```

Additional flags:

```text
--description TEXT
--argument NAME[=DESCRIPTION]
--arguments-json JSON
--body-file PATH
--body-stdin
--edit-body
--clear-description
--clear-arguments
--generate-description
--generate-arguments
--json
--yes
--non-interactive
```

`--body-file` and `--body-stdin` are mutually exclusive. `--body-stdin` exclusively owns
stdin and implies non-interactive mode. `--json` also implies non-interactive mode for every
management command and never opens prompts, a TUI, or an editor. A metadata-only edit does
not open an editor. `--edit-body` explicitly selects the interactive editor path, requires a
TTY, and conflicts with `--json`, `--non-interactive`, `--body-file`, and `--body-stdin`.
Conflicts fail with code 2 before stdin is read or storage changes.

### 5.3. Mode-selection matrix

| Invocation | TTY | Non-TTY/explicit non-interactive |
|---|---|---|
| `alias add` without args | interactive form | usage error, no prompt |
| `alias add name template` | command mode, missing metadata stays empty | command mode |
| `alias edit` without name | list → select → form | usage error |
| `alias edit name` without edit flags | form with prefilled values | usage error |
| `alias edit name --field ...` | command mode, no TUI | command mode |
| `alias remove` without name | list → Yes/No | usage error |
| `alias remove name` | command mode, no confirmation | command mode |
| `script create` without args | form + body editor | usage error |
| `script create name` without body source | existing editor journey | error before registration |
| `script edit name` without field flags | metadata form + optional body editor | usage error |
| `script remove` without name | list → Yes/No | usage error |
| `script remove name` | command mode, no confirmation | command mode |

Interactive edit shows current values directly; Enter preserves them. Interactive remove
confirms the selected record. `--yes` is a command-mode bypass only. Cancellation/EOF before registration writes nothing. After the default script and metadata
are committed, editor failure or cancellation leaves that committed skeleton registered and
returns a diagnostic; this preserves the current behavior. A future staged-editor change
would be a separate compatibility decision.

A named command with complete fields never opens the TUI/editor. Missing command-mode fields
are checked before reading body stdin or publishing any change.

### 5.4. Root dispatch, help, and machine mode

Named dispatch receives the original process `[]string` before anything is joined into a
natural-language prompt. Only after both stores return not-found may query call
`strings.Join(args, " ")`. `strings.Fields` and equivalent tokenizers are forbidden in
named dispatch. An interactive free-form query remains text, not claimed argv.

Built-in commands take precedence over entity lookup. For a matched entity the tail is
opaque except for the rules below: query flags, context files, history, and provider setup
are not applied. Root machine flags must appear before the entity name. `--` removes only the
first delimiter and passes all following argv verbatim, including later `--` tokens.

For a matched entity:

- before the first delimiter, a sole `-h` or `--help` requests entity help;
- before the first delimiter, either help token combined with other tail arguments is a usage error;
- after removing the first delimiter, every token is literal, including `-h`, `--help`, and later delimiters;
- `name -- --help` passes one literal `--help`, while `name -- value --help` passes two literal arguments.

Help shows type, description, argv-safe dialect, template/file, argument table, usage, and example. It
does not require an executable body and does not call the LLM. An existing entity returns 0;
missing/invalid usage returns 2.

Existing machine behavior is preserved:

- `zev --print <alias> [args]` validates/renders the alias and never executes;
- `zev --json <alias> [args]` returns the existing versioned query JSON contract and never executes;
- `zev --print/--json <script>` returns status 2 with empty stdout; use `script show` for the body;
- `<entity> --print` and `<entity> --json` after the name are literal entity argv.

Management JSON uses schema version 1. A successful list is
`{"schemaVersion":1,"status":"ok","type":"alias","items":[...]}` for aliases or
`type:"script"` for scripts; successful add/create/edit/remove is
`{"schemaVersion":1,"status":"ok","type":"alias","item":{...}}` (or `script`),
with remove returning the removed record. Records contain explicit boolean `required` values,
and alias `template` or script `script`. `--json` is non-interactive. Data and help go
to stdout; prompts, warnings, and diagnostics go to stderr. Failures produce empty stdout.
Exit codes are 0 for success, 2 for usage/validation/missing arguments, 1 for definite
storage/launch failure, 6 for `committed_unknown` (with operation ID and reconciliation
instructions), or the child's exit code after a child starts. Existing ordinary query
machine codes 3/4/5 remain unchanged. No automatic retry is performed for code 6.

### 5.5. Missing arguments

Validation occurs before shell/Bash launch:

```text
alias "dps" requires argument 1: service — Service name or ID
usage: zev dps <service>
help: zev dps --help
```

The message names the type, entity, position, name, and description. An empty argv value
counts as supplied. All aliases without `{{.args}}` reject values beyond `K`. Script metadata
establishes only minimum required positions; extra script values pass through as `$@`.

## 6. Argv boundaries and execution

The invoking shell removes outer quotes before Zev starts:

```bash
zev dps "service with spaces"
zev dps 'service with spaces'
```

Both produce one argv element. Zev must never split it again. Tests cover empty strings,
tabs, newlines, Unicode, quotes, `$`, `*`, `;`, `&&`, `$()`, and backticks.

In the argv-safe dialect, numbered placeholders render as zero or one shell word according to
presence/requiredness, while `{{.args}}` renders zero or more independently quoted words
from the original argv. Unsupported contexts are rejected; generic string replacement is
not advertised as safe. The same execution rules apply to new and migrated aliases;
there is no alternative renderer or compatibility execution mode.

Scripts pass exact argv to Bash without joining or reparsing. Runtime matrix:

| OS | Alias | Script | Acceptance |
|---|---|---|---|
| Linux/macOS | POSIX-compatible `$SHELL` | Bash in PATH, `/dev/fd` Unix path | native runtime tests |
| Windows | explicitly allowlisted compatible shell | handle-safe body transport; no pathname fallback; otherwise explicit unsupported error | native Windows gate |

On Windows, the implementation MUST define handle/descriptor transport, argv encoding,
reparse-point/hard-link checks, and shell allowlisting; falling back to a reopened pathname,
PowerShell, or `cmd.exe` is forbidden. Cross-compilation does not prove runtime behavior.
Child stdout/stderr remain child output;
Zev adds result text only to its own streams. Signal and non-zero exit propagation are tested
with a real built binary.

## 7. Metadata generation through the LLM

Generation uses a separate consumer-side port, not the query `InferenceProvider`:

```go
type MetadataGenerator interface {
    GenerateDescription(ctx context.Context, input DescriptionInput) (string, error)
    GenerateArguments(ctx context.Context, input ArgumentsInput) ([]Argument, error)
}
```

The request contains only the entity name, alias template or bounded pinned script body,
and already supplied metadata. It contains no history, ordinary system prompt, environment
context, tools, API keys, or unrelated files. Provider adapters use a dedicated metadata
prompt and validate the response schema.

Interactive generation discloses that the complete serialized request—including entity name,
existing metadata, template/body, destination, remote/local status, and byte size—may be
transmitted to the configured endpoint. Explicit confirmation authorizes that transmission
for the invocation. In command mode, the corresponding `--generate-*` flag is consent;
without it, network generation is forbidden. Neither mechanism creates persisted consent.
A local-only policy MAY disable remote generation and returns a clear error.

The same limits and validator apply to manual and generated input. The generation request
(including a pinned script body or template) is at most 65,536 UTF-8 bytes and is never
silently truncated. The provider response body is limited to 65,536 bytes before decoding,
with one JSON document, bounded nesting, at most 32 arguments, and no trailing data. Invalid
UTF-8, controls/ANSI, duplicate argument names, invalid booleans, unknown fields, and
oversized values are rejected. On add, generation failure leaves the field empty and saves the entity. On edit,
generation failure preserves the old field. A warning goes to stderr; an invalid candidate
is not a success. Generation is forbidden inside the migrator and ordinary help/list/remove/execution paths.
The one-time post-migration enrichment in §4.2 is a separate, explicitly authorized step;
it is not part of data conversion and cannot make migration fail.

## 8. Drift, locking, and manual edits

Canonical YAML is the only persistent source of truth. There are no separate help/argument/
AI registries. All consumers reread through the repository and one validator.

Every change rereads the active file under the exact repository lock, validates the complete
set, changes only the selected record, and publishes atomically. Malformed data, unknown
versions, duplicate keys, or conflicts leave the previous active file unchanged.

List/help sees manual edits on the next invocation. Every script-store reader first performs
journal recovery under the repository lock; an unresolved journal returns a storage error and
never exposes or executes mixed state. Guarantees apply only to cooperating writers using the
same lock; direct unlocked writes are outside the protocol. Caches live only within one invocation.

## 9. Compatibility

Existing commands remain: `alias rm`, `script rm`, `alias last`, `script show`, existing
editor selection, and alias precedence. `alias last` uses the same argv-safe representation
as add and migration, stores empty metadata, does not call the LLM, and does not overwrite
an existing name. Commands outside the supported template grammar are rejected without
creating an alias. There is no legacy exception.

Migration changes storage, adds metadata, and converts the supported v1.0.0 records to the
single argv-safe representation. Unsupported raw shell semantics are not retained. This
release rejects schema versions newer than it supports without modifying them. Development
snapshot formats are not compatibility inputs. Future migration steps are retained only
for released format transitions. Running pre-v2 binaries against migrated stores is
unsupported; the new release cannot enforce their behavior. Before downgrade, stop all
Zev writers and manually restore a compatible legacy store only from a user-owned backup
you choose. Zev does not create, select, sweep, or delete those backups, and changes made
after that backup are not preserved.

## 10. Testing and acceptance

### Unit

- strict codec round trips for both entities, omitted/true/false `required`, null/wrong types,
  duplicate keys, unknown fields, multiple documents, and legacy names `version`/`aliases`;
- validator limits, controls, UTF-8, names, collisions, sparse/optional argument rules;
- argv-safe template parser and rejection of quoted/embedded/unsupported placeholders;
- exact argv substitution for spaces, empty values, quotes, shell metacharacters, Unicode, and LF;
- missing/extra argument diagnostics and help rendering;
- metadata-generator request/result validation and per-field fallback;
- stale metadata/body snapshot conflicts.

### Integration/e2e

- real root dispatch with aliases, scripts, built-ins, corrupt stores, and invalid provider config;
- `zev dps "a b"`, `zev dps -- --help`, `zev dps value --help`, and repeated `--`;
- root `--print/--json` never executes, while entity-tail flags remain literal;
- a real child captures exact argv and proves no marker file is created by metacharacters;
- interactive/non-TTY matrix, EOF/cancel, no partial writes, and editor-failure compatibility;
- management JSON schema, stdout/stderr, and exit statuses 0/1/2/7/127;
- a recording local provider proves consent, bounded body, and no history/tools/config leakage;
- legacy migration preserves descriptions, bodies and bytes; journal recovery handles
  interrupted publication without generating an archive;
- crash injection before/after staging, journal publication, body publication, metadata
  publication, rename, sync, and cleanup;
- concurrent cooperating CRUD/migration, stale edits, and same-name alias/script collisions;
- native Linux/macOS/Windows runtime gates, missing Bash, and signal cleanup.

### Completion

The feature is complete only when every capability-map slice passes its checkpoint, all
acceptance tests are green, `make test`, `make test-race`, `make lint` plus post-lint diff inspection, `make check`, and
`make build` pass. Because this repository's `make lint` can autofix files, the implementation
owner must inspect and report the resulting diff; native gaps are explicitly listed rather
than inferred from cross-compilation.

## 11. Implementation boundary

The migration policy is settled: every supported v1.0.0 record converts to the current
representation; there is no legacy execution mode. After successful migration, records
remain usable without descriptions or argument metadata. Optional AI enrichment is a
separate future slice, not a migration startup promise.

The remaining sections describe implementation requirements, not additional startup choices.
No extra migration wizard, mode selection, or per-record enrichment confirmation is introduced.

This task changes the specification only. Implementation, dependency installation,
migration of the user's actual files, and commits require a separate implementation request.
