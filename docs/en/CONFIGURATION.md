<div align="center">

**English** | [Русский](../ru/CONFIGURATION.md)

</div>

# Configuration

Zev reads its settings from `~/.zev/config.yaml`. Environment variables do not supply
missing settings or override YAML. Run `zev setup` to create or update the file:

```bash
zev setup              # interactive wizard
zev setup mode         # inject | execute
zev setup headers      # custom HTTP headers
```

## Provider profiles

`providers` maps profile names to settings. `active_provider` must name an entry in
that map. The name is yours to choose; `type` selects the adapter. Two profiles can
use the same type with different accounts, models or endpoints. For example:

```yaml
version: 1
active_provider: local
providers:
  local:
    type: ollama
    model: llama3.2
    base_url: http://localhost:11434/v1
  work:
    type: openai
    api_key: "<your-api-key>"
    model: gpt-5.6-luna
    base_url: https://api.openai.com/v1
    http_headers:
      X-Project: work
language: auto
http_headers:
  User-Agent: zev/1.0
```

Here `local` is active. Only requests using `work` get the `X-Project` header.
Keep credentials under the profile that needs them, not in global headers.

| Command | Effect |
|---|---|
| `zev provider list` | Lists names and types in name order. `*` marks the active profile; keys and URLs are not shown. |
| `zev provider add NAME` | Interactively creates a profile. Refuses an existing name and leaves the active profile unchanged. |
| `zev provider use NAME` | Saves an existing profile as `active_provider` for subsequent runs. |
| `zev provider remove NAME` | Removes an inactive profile. To remove the active one, switch first. If it is the last profile, add another and switch to it. |

`zev setup` updates the active profile if its type matches the type you select.
Otherwise it configures the profile named after the selected type and makes it active.

## Migration compatibility

The current configuration reader accepts only the current versioned YAML document.
Migration is orchestrated before configuration is used; `internal/core/config` does not
parse legacy formats or run migrations itself.

The supported legacy source is the v1.0.0 dotenv file `~/.zev/config`. Zev converts
it directly to `config.yaml` with `version: 1`. Unreleased development formats,
including named-profile YAML without a version, are not migration inputs.
Future transitions are retained only as release-to-release steps after a format is
released; compatibility must not be inferred from a development schema.

## Migration from the legacy file

If the v1.0.0 dotenv file `~/.zev/config` exists, Zev converts it automatically on the
next full configuration load. It reads settings from that file, not from the shell. For
example, `LLM_PROVIDER` selects the initial active profile and `ZEV_LANG` becomes
`language`. If you used to export settings but never saved them, enter them in setup or
YAML.

Zev deletes the old file **only after** writing and verifying the YAML, syncing it to
disk and rechecking that the source has not changed. It creates no archive. If the
process stops midway, leave `~/.zev/config.yaml.migration` in place and retry; Zev uses
the marker to resume. If both config files exist without a matching pending marker,
Zev reports a conflict and leaves both intact. Compare them privately and reconcile
them before retrying; Zev will not guess which one to keep.

Recovery is limited to a matching pending journal from this migration protocol. Zev does
not interpret or recover markerless or unpublished archives/journals from development
snapshots, and it does not sweep unknown files or user-owned copies.

## Defaults

With no configuration file, Zev starts with an `openai` profile. In an existing YAML
file, specify `active_provider` and `providers`; omitted global settings use defaults:

| Global field | Default |
|---|---|
| `language` | `auto` |
| `http_headers` | `User-Agent: zev/1.0` |
| `command_execution_mode` | `inject` |
| `command_wrapper_enabled` | `true` |
| `script_editor` | `nano` |
| `history_enabled` | `true` |
| `history_semantic_enabled` | `false` |
| `history_embedding_max_bytes` | `32768` |
| `context_enabled` | `true` |
| `tools_enabled` | `true` |
| `tool_approval_mode` | `manual` |
| `security.mode` | `auto` |
| `security.web_fetch_enabled` | `false` |

Security policy schema is versioned independently under `security.version` (currently
`1`). `security.default_decision` accepts only `ask` or `deny`; `allow` is never a
valid global fallback. `probe` and `classifier` accept only `mode: local`,
`on_failure: ask|deny`. Legacy `security.mode: normal` is read as `auto` and saved as
`auto`. Legacy `security.sandbox.mode: off` is discarded and is not written back;
other legacy sandbox values are rejected. No sandbox setting is supported.
Optional `limits` fields are bounded review durations/call counts. `rules.deny`,
`rules.ask`, and `rules.allow` use unique IDs and non-empty conjunctive matches for
`sources`, `deliveries`, `effects`, `paths`, `executables`, `argv_prefix`, `certainty`,
and `risk`. Matching priority is always deny, then ask, then allow, regardless of
YAML order; an allow must cover every requested effect. Invalid, duplicate, null, or
unknown policy values fail closed before dispatch.

The user home (`HOME` on Unix, `USERPROFILE` on Windows) must be an absolute path. A missing or relative home is an
error; Zev never falls back to `.zev` inside the current project.

## Privacy controls

`history_enabled: false` prevents this query from opening `~/.zev/history.db`, storing
messages or running JSONL migration. Ordinary queries never generate background
embeddings, even with history enabled. See [History](HISTORY.md) for the incremental
lifecycle, absence of count-based rotation and search/privacy limits.
`context_enabled: false` prevents local OS, shell, user, home and working-directory details
from being sent to the provider. The command-line overrides `--no-history` and
`--no-context` disable the corresponding behavior for one query. Disabling history
leaves existing entries untouched; it does not disable the active in-memory dialogue
sent to the provider. Disabling context does not remove anything explicitly included
in your prompt. Never include credentials in prompts or generated commands: those
contents can otherwise reach the provider, terminal output and enabled history.

API keys and custom header values are entered with masked input and are redacted from
setup output. Provider diagnostics omit raw request/response bodies, credentials and
endpoint URLs; this does not redact ordinary command output or enabled history.
Installers fail closed unless the downloaded archive has exactly one
matching SHA-256 entry and the computed digest matches it.

## Model tools

`security.mode` controls command-delivery policy. `auto` and `bypass` are peer modes:
`auto` applies the configured local policy, while `bypass` permits otherwise technically
valid host `execute` actions without that confirmation. This setting does not select or
rewrite the separate model-tool approval mode, is never enabled by model output or a
repository file, and does not provide a sandbox. `zev doctor` reports the selected mode.

`security.web_fetch_enabled: true` separately enables the model's bounded `web_fetch`
read capability. It permits only public HTTP(S) GET requests with destination checks,
redirect limits, response limits, no cookies or credentials, and local untrusted-output
probing. It does not enable arbitrary network access or change command delivery.

The persisted `~/.zev/config.yaml` must be a regular file owned by the current user
with no group or other permission bits. Zev rejects symlinks, hard-linked files,
special files, and group/world-readable or writable configuration before parsing it.

`command_wrapper_enabled: true` wraps generated `inject` commands with `zev run --` so
the user can inspect a bounded failure log and request analysis or correction suggestions.
Set it to `false` to inject the original command without the wrapper. This setting affects
only generated injection, not explicit `zev run` calls. `zev run -- <command> [args...]`
passes literal argv without joining it into shell text and does not use `execute`-specific
authorization. On failure, `--on-failure=analyze|fix` sends the bounded diagnostic context
to the configured provider only after that action is selected and Zev warns that it may
contain secrets; injected commands continue
their linked conversation, while direct runs start a standalone recovery query. Suggestions
are displayed as inert text and are never executed automatically. `retry`, `--max-retries`,
`--retry-delay`, `--timeout`, `--log-limit`, and `--exit-code=original|last` control execution.
Zev does not provide an OS sandbox; commands run with host privileges and environment policy.


Model tools are enabled by default through `tools_enabled: true`. `tool_approval_mode`
accepts `manual`, `auto`, or `bypass`; the value is validated even when tools are disabled.
The `--tools`, `--no-tools`, and `--tool-approval` flags override these settings for one run.

Tools are disabled for a query when `--no-context` is used or `context_enabled: false`,
so local working-directory information cannot leak through tool execution. An explicit
`--tools` flag overrides that privacy default. In `--print` and `--json` modes, `--tools`
must be supplied explicitly; these modes never open the TUI, even on a terminal. A manual
tool request in machine mode is returned to the model as denied with an explanation to
choose `auto` or `bypass` explicitly.

The model receives a native `exec` function with `command`, `args` and `reason`. Arguments
are an exact argv array, not an implicitly evaluated shell string. Native tool-capable
models/endpoints are required; use `--no-tools` when an endpoint rejects function calling.
OpenAI-compatible/Ollama, OpenCode, Anthropic and Gemini adapters support this exchange.

- `manual` shows every request and the fixed working directory on stderr and requires
  the literal `yes`. Empty input, refusal, cancellation, or an unavailable terminal never
  authorizes execution. Approval applies to this invocation only.
- `auto` first enforces the local read-only policy, then makes a **separate request to the
  configured provider/model** for every admitted invocation. The reviewer has no tools,
  primary conversation, or file output; it sees only the command arguments, reason and
  root. Only an unambiguous JSON approval permits execution. Unsafe verdicts, network
  errors and malformed responses deny the call and return an explanation to the main
  model so it can adapt. A model verdict alone is not a security boundary.
- `bypass` is a peer tool-approval mode: it skips the per-call prompt and reviewer but
  keeps the same read-only builtin capability and technical checks as the other tool
  modes. It cannot run arbitrary executables, shell commands, writes or network calls
  through the model context-tool path. Host query `execute` is a separate delivery mode
  controlled by `security.mode` and is **not a sandbox**.

### Local tool adapter contract

Caller authorization and isolated model review belong to the consuming service, not the
filesystem/process adapter. In `auto`, the service performs the separate reviewer request
described above; in all modes, `Execute` revalidates the complete request. A prior lexical
check or reviewer decision is never durable authority.

The `auto` mode uses these built-ins, not executables found through `PATH`. The restricted
adapter accepts only these exact argv forms:

```text
pwd                  # no arguments
ls                   # root, one level only
ls PATH              # one directory, one level only
cat PATH             # one regular, singly linked file
head PATH            # first 10 LF-delimited lines
head -n N PATH       # canonical decimal N in [1, 1000]
```

There is no PATH lookup, subprocess, shell, stdin, globbing, expansion, option parser,
recursion, multi-path mode, or support for other flags (including `--`). Shell
metacharacters in a path are literal filename bytes, never instructions. `ls` emits ASCII
Go-quoted filenames in filesystem order, without metadata; it omits symlinks, sensitive
names and platform-ambiguous names.

Relative paths start at the configured root; absolute paths are accepted only inside it.
Every lexical `..` component is rejected, even when the result would remain inside the
root, and every relative component is checked before as well as after cleaning. Names
beginning with `-`, containing a colon or a non-native backslash, or ending in a dot or
space are rejected (except `.`). Symlinks in every component—including the configured
root and its ancestors—are rejected. `New` does not canonicalize aliases: a composition
root may canonicalize a trusted cwd before construction, but must never canonicalize an
untrusted tool path to bypass these checks.

Sensitive matching is case-insensitive, including root ancestors. It rejects `.env*`,
any name containing `credential`, `secret`, `password`, `token`, `privatekey`,
`private_key`, `private-key` or `keys`; exact `.git`, `.ssh`, `.aws`, `.kube`, `.azure`,
`.gnupg` and `.zev`; `id_*`; and `.pem`, `.key`, `.p12` or `.pfx` suffixes. This
intentionally overblocks some innocent names and cannot detect secrets stored under
innocent names. Files must be regular and singly linked.

The root is pinned until close, and each operation verifies that the current path still
names that identity. Descendant directories are opened one component at a time through
`os.Root`; opened identities must match pre-open, non-symlink metadata. File handles are
verified before reading. Unix uses `O_NOFOLLOW|O_NONBLOCK` so a replacement FIFO cannot
block open; Windows verifies link counts and reparse attributes on the opened handle.

Fixed adapter budgets are: 32 KiB combined stdout/stderr (including escaped output),
32,769 bytes consumed per file read (`head` may read that bounded prefix even when fewer
lines suffice), at most 1,024 `ls` entries including omitted entries,
4,096 bytes and 64 components for each path and the root, 64 arguments, 4,096 bytes per
executable/argument, 16,384 bytes for executable plus arguments, and 2,048 bytes for
`reason`. `reason` is never executed. `ls` does not sort or recurse; reaching its entry
budget marks `Truncated` conservatively even at exact EOF. `Truncated` means a budget was
encountered, not that the data is a consistent filesystem snapshot. C0/C1 controls except
LF and tab, CR, and Unicode format controls are escaped; invalid UTF-8 is replaced. No
child output is written to the application's stdout/stderr; callers should JSON-quote
adapter output before terminal rendering.

Every `Execute` receives a ten-second deadline, shortened by its caller. Native processes
use `childprocess.Run`, including process-group/job cleanup and bounded pipe draining;
cleanup can extend wall-clock time beyond the deadline. Ordinary filesystem APIs cannot
cancel an in-flight kernel call: the adapter checks cancellation around bounded I/O, but
a stalled filesystem, FUSE or kernel call has no hard wall-clock bound and must not be
advertised as one.

With `restricted=false`, a separate caller has already approved the complete request. `os/exec`
receives literal argv and `cwd=root`; a shell runs only when explicitly requested. Stdin
is the null device. The environment contains only `PATH`, `LANG=C`, `LC_ALL=C`, and on
Windows `SystemRoot`; provider secrets, `HOME`, loader hooks and shell-startup variables
are not inherited. Bare executables are searched only in `/usr/bin:/bin`, or
`SystemRoot/System32` on Windows and only as `.exe`; explicit absolute paths or relative
paths containing separators may select other tools.

The model context-tool path never uses `restricted=false`. Host query execution is not a
sandbox: approved commands may modify files, access the network, read credentials from
disk or intentionally detach descendants. `os.Root`
does not prevent mount crossings, and the adapter does not protect against hostile
same-UID writers, mount changes or concurrent mutation of regular-file data. Filename
filtering is not a secret-content detector; do not enable tools in an untrusted or
secret-bearing project without considering what may be sent to the provider.

```bash
zev query --tool-approval manual "inspect this project"
zev query --tool-approval auto "find the right build command"
zev query --print --tools --tool-approval auto "find the right test command"
zev continue --tool-approval auto "look into the error"
zev query --no-tools "show disk usage"
# Skip the per-call approval prompt and isolated reviewer while retaining read-only checks:
zev query --tools --tool-approval bypass "inspect this project"
```

To set the tool policy persistently, use top-level fields in `~/.zev/config.yaml`:

```yaml
tools_enabled: true
tool_approval_mode: manual
```

Fixed budgets: **8 calls per response, 10 seconds per local execution, 30 seconds per
review, 2 minutes per tool loop, 32 KiB combined stdout/stderr per call**, and 256 KiB of
aggregate serialized tool results. These are cancellation deadlines, not hard wall-clock
limits: process cleanup may take longer, and a blocked kernel/filesystem call cannot be
interrupted by this in-process adapter. On the call limit the model is asked to finish without
new tools. Output may be truncated; owned child processes are cleaned up on cancellation.
Tool calls add tokens/cost. Intermediate tool/review data is ephemeral, not written to
query history, although a final model answer can repeat inspected information.
`--print`/`--json` never execute the final suggested command; only an explicit `--tools`
opt-in permits context tool execution. `doctor`, alias resolution and script inspection
never invoke model tools. Native Windows runtime behavior has not been verified;
cross-compilation alone is not evidence of native correctness.

## YAML field reference

### Fields in each provider profile

| Field | Meaning |
|---|---|
| `type` | Adapter: `openai`, `gemini`, `anthropic`, `ollama` or `opencode`. |
| `api_key` | Credential for this profile when required. |
| `model` | Model identifier accepted by this endpoint. |
| `base_url` | API root. For an OpenAI-compatible router, use its URL with `type: openai`; for local Ollama, for example, use `http://localhost:11434/v1`. |
| `http_headers` | Optional map of header names to string values. Profile headers override global headers of the same name, ignoring case. |

The setup wizard offers `gpt-5.6-luna` at `https://api.openai.com/v1` for OpenAI,
`gemini-3.1-flash-lite` at `https://generativelanguage.googleapis.com` for Gemini,
and `claude-haiku-5.5` at `https://api.anthropic.com/v1` for Anthropic.

### History embeddings (explicit opt-in)

These are top-level fields, not a provider profile or a nested history section.

| Field | Default / requirement |
|---|---|
| `history_semantic_enabled` | `false`; explicitly enable semantic history search |
| `history_embedding_provider` | Empty; when enabled, must be `openai-compatible` |
| `history_embedding_base_url` | Empty; required HTTP(S) API root, e.g. `https://embedding.example/v1` |
| `history_embedding_model` | Empty; required embedding model |
| `history_embedding_dimensions` | `0`; must be positive when enabled and match the model |
| `history_embedding_api_key` | Empty; separate credential, optional for endpoints without authentication |
| `history_embedding_max_bytes` | `32768`; positive UTF-8 input byte limit, not a token count |

These settings are independent from the command LLM; command credentials and headers
are not implicitly inherited. Provider requirements are validated only when semantic search
is enabled; YAML values must still have the correct types. The client
appends `/embeddings` to the API root and rejects credentials in the URL, query strings
and fragments. Oversized input is rejected, never silently truncated.

Only initial/refinement user messages are eligible. Remote embedding requests disclose
that text, and semantic/hybrid search also sends the search query. A local proxy can forward
it externally. Enabling the setting does not automatically backfill old messages or add
retrieved history to LLM prompts. When enabled, the CLI uses the configured client for
semantic/hybrid search and explicit vector rebuilding; ordinary queries do not generate
vectors. `zev history index rebuild --vectors --yes` shows provider/model and the total
eligible-message count and requires explicit transmission consent. Use `--retry-failed`
to retry failures; `zev history index rebuild --fts` rebuilds local indexes without
embedding requests. See [History](HISTORY.md) for commands, coverage and candidate limits.

### Behaviour

| Field | Description |
|---|---|
| `command_execution_mode` | `inject` attempts to insert the command into terminal input; `execute` runs the first suggestion immediately |
| `script_editor` | Editor used by `zev script create` |
| `http_headers` | Map of headers shared across provider profiles |
| Retry policy | Provider requests retry up to 3 times for HTTP `429` and `5xx` responses. Backoff is exponential (`1s`, `2s`, `4s`); a valid `Retry-After` header can extend the delay, and the delay is capped at 30 seconds. Retries are logged at warning level; the final failure is logged at error level. |

The client does not follow provider redirects. It rejects cross-origin redirects
before credentials or custom headers can reach the other endpoint.
The header map is snapshotted when the client is created.

## Provider request retries

All five providers use the same retrying HTTP client for both single queries and
conversation history. There are at most four attempts: the original request plus
three retries. The existing 30-second client timeout covers all attempts, backoff
and response reading, so a timeout or cancellation can stop the operation earlier.
Other `4xx` responses and malformed successful JSON are not retried. A stream
that has already returned HTTP `200` is not restarted if it breaks while reading.
Retry warnings are visible without `-v`; URLs, API keys and payloads are not
included in retry logs. These defaults require no extra configuration.

## Response validation and limits

All five providers validate suggestions before selection, execution or saving to
history. Existing history is also validated before command selection. Fixed limits
are **3 alternatives**, **4096 UTF-8 bytes per command**, **2048 UTF-8 bytes per
explanation**, **32 KiB of structured response JSON**, and **256 KiB of provider HTTP
response body** (including SSE framing for OpenCode). These are byte limits, not
character counts, and are not configurable. Empty commands, invalid UTF-8, oversized
responses and inconsistent valid/invalid response shapes are rejected.

Response text rejects control characters except tab, LF and CR. Terminal injection
is stricter: it rejects **every control character**, including those three, before
writing any terminal input. A multiline suggestion can still be displayed as a
fallback or executed in `execute` mode. Validation is not shell sanitization, a
sandbox or a guarantee that a suggested command is safe.


## Interface language

The CLI and interactive prompts support English and Russian via `go-i18n`.
`language: auto` (default) detects the request language locally with `whatlanggo`: Russian
uses Russian UI; other or unidentified languages use English, regardless of system locale.
Detection examines at most 4096 runes and is statistical: very short, mixed or similar-language
texts can be misclassified. No extra network/model request is made. Refinements update the
UI language; continuation without new clarification uses the latest restored user prompt,
not attached files or command logs.

To fix the interface language regardless of the prompt, set `language: en` or
`language: ru` at the top level of `~/.zev/config.yaml`. Set `language: auto` to restore detection.

Without a request (help/setup and the initial input prompt), auto uses the first nonempty
`LC_ALL`, `LC_MESSAGES`, or `LANG` (including `ru_RU.UTF-8`); unsupported locales use English.
Invalid configured language values are rejected when loading the full configuration.

Safety checks inspect operations inside pipelines/chains; separators such as `|`, `&&`
or `;` alone do not mark a command dangerous. Warnings for destructive operations,
shell execution from pipeline input, substitutions and output redirection remain;
provider-supplied warnings are preserved. Shell text is analyzed by a bounded structural
analyzer selected from the resolved `sh`/Bash/Zsh interpreter; unsupported dialects and
over-budget or ambiguous syntax remain unknown and require confirmation. This is not a
complete shell AST or sandbox. Literal `run` argv is analyzed as separate argument bytes
and is never joined and reparsed as shell text. Absence of a warning is not a safety guarantee.

Translations live in root `locale/query.*.toml` and `locale/{cli,tools,ui}.*.toml` and are embedded into the binary. No locale
files are needed beside the installed executable. Add translations to the matching
English/Russian catalogs, keeping stable message IDs, printf placeholders, and CLI tokens intact.
Go code references named constants from `internal/core/locale/keys.go`, not English sentences; changing
an English translation does not change its message ID.
Machine JSON keys/statuses, suggested commands, script contents, provider responses, and
saved history are not translated. Lower-level system/library errors may remain English.

## Explicit project context files

A query may attach one or more bounded files to the model request:

```bash
zev --context-file README.md -cf docs/en/ARCHITECTURE.md "explain the startup path"
zev --print --context-file README.md "find the relevant command"
zev --json --context-file README.md "find the relevant command"
```

`--context-file` and `-cf` may be repeated. Paths are resolved relative to the current
working directory and must remain below it. Only regular files are accepted; directories,
symlinks, `.env` files, private keys and credential-like files or directories
(including `.aws`, `.docker`, `.ssh` and `.kube`) are rejected before any
provider request. Each file is limited to 64 KiB and the request total to 256 KiB.
Use `--allow-sensitive-context` only when a specific sensitive file is intentionally
selected; symlinks remain rejected. A context-file failure returns before network access,
and machine modes keep stdout reserved for their existing command/JSON contracts.

## Continue after a failed query

```bash
zev continue
zev query --continue
zev query --continue --print "retry after fixing the command"
```

`zev continue` is syntax sugar for `zev query --continue`. It restores the newest
completed or failed query that is eligible for continuation and preserves the complete
ordered transcript, not only the last prompt. In interactive mode Zev asks in order for
project context files, command/error log, and a clarification. Enter skips each step.
At least one file, log, or clarification is required. Machine modes use the same restored
history without opening the TUI; the existing stdout contracts remain unchanged.



```bash
zev alias edit <name>
zev script edit <name>
zev script show <name>
```

`alias edit` opens the stored alias template and saves it after the editor exits
successfully, provided its template has not changed since editing began; a stale
editor cannot overwrite a newer template. Alias listings are sorted by name. `script edit` opens the registered Bash script through the configured
editor (`script_editor`, `$EDITOR`, `nano`, `vim`, or `vi`). `script show` writes the
exact script body to stdout without executing it; it uses direct safe file reading rather
than launching an external shell command.
## Machine-readable output

For scripts and CI, machine mode never opens the selector, injects terminal input, starts a
shell, or executes a returned command:

```bash
zev --print "find large files"
zev --print --option 2 "find large files"
zev --json "find large files"
```

`--print` writes only the selected command to stdout. `--json` writes schema version 1:
`{"schema_version":1,"status":"ok","selected_option":1,"commands":[...]}`.
Warnings, progress and errors go to stderr. Option numbering is one-based and defaults to 1;
an unavailable option leaves stdout empty and returns an error. Stable machine-mode exit codes
are 0 for success, 2 for arguments/selection errors, 3 for provider/configuration errors,
4 for invalid provider responses, and 5 for local context-read errors. `--print` expands
aliases without execution. Saved scripts are not printed automatically; use `zev script show
<name>` to inspect their body.


## `zev doctor`

`zev doctor` first performs local-only checks: configuration loading, selected provider,
required model, API-key configuration, base URL syntax, execution mode, shell/editor
availability, private `~/.zev` permissions, read-only history access and CLI mode parsing.
It does not claim that a configured key works; only an explicitly confirmed network check can
verify provider functionality.

```bash
zev doctor             # static checks, then interactive yes/no confirmation
zev doctor --print     # static checks only, human-readable output
zev doctor --json      # static checks only, schema-versioned JSON
zev doctor --confirm   # static checks plus one real provider request
zev doctor --json --confirm
```

JSON uses `schema_version: 1`, top-level `status`, `checks` and `actionable_message`.
Doctor never prints API keys, custom header values, provider URLs or response bodies.

## Command execution mode

`zev setup mode` controls how suggestions are handled:

- `inject` (default) — select a suggestion, then review the inserted command and press
  Enter. If terminal injection is unsupported or denied, Zev prints the command instead.
- `execute` — the **first suggestion** runs immediately, without a selector or
  confirmation. The danger warning is printed after execution; it is not a safety gate.

Aliases and saved scripts execute directly, independently of this setting.

The CLI propagates cancellation from Ctrl-C/SIGTERM through provider requests,
retry waits, interactive prompts and child execution. Child processes run in an
owned Unix process group or Windows kill-on-close job. Unix cleanup kills remaining
owned group descendants on cancellation or normal leader exit, before reaping the
leader. Windows starts the child suspended, assigns it to the job, then resumes it;
job cleanup terminates remaining owned processes. This is not containment: Unix
children that detach into another group/session and processes launched through
external brokers are outside that boundary. Arbitrary blocking caller-supplied
I/O is not bounded by child pipe cleanup. Windows runtime behavior needs native
Windows verification; cross-compilation alone does not prove it.

## Editor resolution for `zev script create`

1. `script_editor`
2. `$EDITOR`
3. first available binary from `nano`, `vim`, `vi`

Editors receive a private temporary working copy, not the registered script path.
A successful edit is staged, synced and atomically replaces the original only if
its file identity still matches; a stale editor cannot overwrite a cooperating
editor's newer replacement. Editors that save by rename are supported. Symlinks,
non-regular files and multiply linked script/working files are rejected.

The initial script is registered before the editor opens. If the editor cannot be
found, fails or is cancelled, the registered script remains, while the temporary
working copy is cleaned up. An error may mention that temporary path; it is not a
persistent recovery file.

## Files on disk

| Path | Contents |
|---|---|
| `~/.zev/config.yaml` | YAML settings and named provider profiles; credentials are stored in plaintext |
| `~/.zev/history.db` | Persistent SQLite conversation history without count-based rotation |
| `~/.zev/history` | Legacy JSONL source; imported once and deleted after verified migration |
| `~/.zev/aliases` | Alias templates, YAML |
| `~/.zev/scripts` | Script metadata, YAML |
| `~/.zev/script_files` | Script bodies |

`~/.zev/scripts` is a file and `~/.zev/script_files` is a directory on purpose: a file
and a directory with the same name cannot coexist.

### Storage safety and recovery boundaries

Configuration and aliases use same-directory temporary files followed by replacement,
with cooperative locks covering read-modify-write operations. SQLite history uses WAL
transactions instead of rewriting JSONL. Files are written with mode `0600`, new
application directories with `0700`, and new script bodies with `0700` on Unix. These
permissions do not encrypt API keys or
history; Windows validates configuration ownership and write-capable ACL entries through native security descriptors, while this checkout's Windows behavior is compile-checked but not runtime-tested here.

Persistent `*.lock` sidecars identify cross-process locks: **do not delete or replace
them**, even when idle. Data-file symlinks and a symlink at `~/.zev` are rejected;
lock sidecars must be regular singly linked files. Trusted ancestor aliases (such
as a symlink in the home-directory path) are allowed. This cooperative protocol is
not a sandbox against hostile same-user processes or writers that ignore locks.
Malformed existing storage is reported, not silently reset. A corrupt legacy history
source blocks migration activation instead of discarding earlier entries. Logical deletion
does not erase copies in WAL/free pages, exports, backups or remote copies. Back up an
active WAL database with a consistent SQLite snapshot, not a copy of only `history.db`.

Script metadata paths must remain relative beneath `script_files`: absolute paths,
traversal, symlinked application-owned components and multiply linked script files
are rejected. Execution reads the already-open validated script, rather than
reopening a checked pathname. Script creation/removal coordinates metadata and body
publication, but does not provide rollback or a crash-atomic transaction across both files.
The shared `internal/migrations/filetxn` primitives are for filesystem migrations, not
script CRUD. Zev does not sweep or delete user-owned backups or historical copies; logical
delete and migration provide no secure-erasure guarantee.

For an atomic replacement, a failure before rename leaves the previous destination
unchanged. A directory-sync error after rename means the new file is already visible
but crash durability is uncertain; it is not evidence that nothing changed. Go callers
can classify this with `errors.Is(err, filestore.ErrCommitted)` or the script
repository's `ErrCommitted`. Inspect the current state before retrying a file change.
Cleanup errors can also accompany a completed change. Windows
has no portable directory-fsync step here, and its crash durability/runtime behavior
must not be inferred from a successful cross-build.
