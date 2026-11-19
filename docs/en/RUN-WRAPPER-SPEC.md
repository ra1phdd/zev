<div align="center">

**English** | [Русский](../ru/RUN-WRAPPER-SPEC.md)

</div>

# Specification: standalone zev run — execution, failure analysis, and correction

**Status:** literal argv execution, bounded output capture, and interactive failure
recovery are implemented. Each explicit run performs one command attempt. Analyze and fix
request provider suggestions only after selection; injected commands restore the exact linked
conversation, while direct runs send a standalone failure query. Query injection wrapping is
controlled by `command_wrapper_enabled` (default `true`). Suggestions are never executed
automatically. Run events, retained artifacts, several advanced flags, and independent review
remain outstanding.

## 1. Objective and approved decisions

Zev has two equally prominent user entry points:

| Entry | User intent | Result |
|---|---|---|
| `zev query <request>` (or root query) | “What should I run?” | Commands generated from natural language |
| `zev run -- <command> [args...]` | “Run my command; help me understand a failure” | Real execution, visible log, analysis/correction choices |

`run` is a first-class feature, not just an internal query wrapper. Users may bring any
explicitly selected executable command; no prior query, conversation ID, or LLM history is
required. Automatic LLM-command wrapping is an additional integration of the same runner.
It is not a global shell hook, sandbox, or file-editing agent.

Mandatory user journey:

1. On failure, **always show the log first**, before allowing further actions.
2. Interactive mode offers four primary actions: **“Retry command”**, **“Analyze error”**,
   **“Fix command”**, **“Do nothing”**. Initial selection is **“Do nothing”**.
3. Without interaction, do nothing by default. Automatic actions require explicit flags
   for the current invocation.
4. “Fix command” opens existing **Clarify request**. Clarification is optional: empty/
   whitespace-only Enter still sends error and log to the LLM. Escape/EOF/cancellation
   are not submission. Separate Analyze explains errors and gives recommendations without
   entering correction or requiring Clarify request.
5. Default wrapping applies **only to LLM commands**. Aliases/scripts and query
   `--print`/`--json` do not automatically execute through the wrapper. This restricts
   automatic integration, not explicit run of commands from any source.
6. Every built-in root command name and synonym is reserved against new aliases.
7. Approved defaults: `original` exit policy, 1 MiB log per attempt, 7-day / 100 MiB retention.
   Executing a correction requires separate consent.
8. Ordinary `zev continue` does not attach a log automatically: the user does that manually.
   Internal wrapper handoff does not change this manual contract.

Normative MUST/MUST NOT identify testable requirements. Engineering details below do not
authorize implementation, dependencies, migrations, publication, or a commit.

## 2. Existing state and scope

Source-verified integration points:

- [query.go](../../internal/cli/query.go): flag parsing, LLM command delivery, selection/refinement.
- [continue.go](../../internal/cli/continue.go): currently restores the latest conversation,
  asks for logs, and creates refinement even from fallback text. Run recovery cannot reuse
  this flow unchanged.
- [terminal](../../pkg/terminal/inject.go): shell text executes through `$SHELL -c`, fallback `sh`.
- [childprocess](../../internal/core/childprocess/run.go): shared process-group/job ownership;
  the existing API alone is not a full PTY session and cannot bound arbitrary blocking
  Readers/Writers.
- [provider](../../internal/core/domain/provider.go): command alternatives, not proof of correction.
- [tool executor](../../internal/entities/tool/repository/executor.go): restricted read-only path.
- [history](../../internal/entities/history/service/types.go): conversation/command IDs and one
  mutable execution summary, not an independent run-attempt journal.
- [filestore](../../internal/core/filestore/filestore.go): atomic replacement of one file;
  `ErrCommitted` means visible rename with uncertain crash durability.

Version one includes explicit argv execution, default wrapping, failure UI/Clarify request,
bounded retries/logs, Analyze without correction, run-bound internal handoff, private artifacts,
and separate events. README/help present run alongside query with standalone examples/use
cases, not only hidden inside query integration documentation.
Excluded: intercepting all shell commands, new write tools, autonomous correction execution,
automatic AI/regex failure-trigger classification, run search/dashboard, new SQLite schemas,
or full-project snapshots. Observe the command explicitly executed through run, not shell
history/previous terminal sessions; do not promise to reconstruct their stderr.
All runtime contracts below are targets; an existing package does not prove implementation.

## 3. CLI and parsing order

### 3.1. Explicit run

```bash
zev run -- go test ./...
zev run --on-failure=none -- go test ./...
zev run --no-interactive --on-failure=analyze -- go test ./...
zev run --no-interactive --on-failure=fix -- go test ./...
zev run -- sh -c 'go test ./... && go vet ./...'
```

`run` has a separate parser, never the query parser. Execution requires `--`; its own
`zev run --help` needs no separator and launches nothing. Pass post-separator argv exactly: no join, `eval`, glob
expansion, or second shell interpretation. Users explicitly launch a shell for a shell
program. `cd` is not an executable in argv mode.

| Run flag | Default | Contract |
|---|---|---|
| `--on-failure=ask\|none\|analyze\|fix` | `ask` | Menu / no action / analysis without correction / LLM correction |
| `--no-interactive` | `false` | Prohibit all run/recovery prompts |
| `--log-limit=BYTES` | `1048576` | Combined encoded sanitized tail per attempt, 1024–16777216 bytes |
| `--timeout=D` | `0s` | Per-attempt deadline; 0 adds none |
| `--events-file=PATH` | unset | Separate private JSONL metadata sink |
| `--session=ID` | unset | Exactly 10-character local registry key for the exact conversation and selected command; never latest |
| `--no-history` | `false` | Do not open SQLite or create run artifacts/conversation |
| `--no-context` | `false` | Do not attach automatic environment/Git context to the LLM |
| `--tools` / `--no-tools` | tools disabled | Explicitly permit / prohibit recovery tools |
| `--tool-approval=manual\|auto` | `manual` | Policy for explicitly enabled recovery tools only; bypass forbidden |

The session ID is an opaque 10-character key, not a reversible encoding of two UUIDs. Query injection registers the exact conversation/command UUID pair in the private `run-sessions.json` registry using an atomic locked update; recovery resolves the key and rejects missing or malformed entries. This preserves exact addressing without exposing two UUIDs in the shell line and without accepting a hash collision as another session.

Validate arguments before child launch or provider setup. Usage errors return `2`: empty
command, missing `--`, unknown pre-separator options, invalid values, negative durations,
invalid session token, tools/no-tools conflicts, or explicit
approval without tools. Timeout is capped at 24 hours. Validate ID syntax before launch.
With effective history disabled, ignore a supplied session key with a warning, without SQLite or
dialogue loading, and still execute the command. This rule is identical for explicit and
generated invocations. An active in-memory link is allowed without SQLite.

Each invocation performs one attempt; any rerun requires a new explicit `zev run` invocation.
`analyze` and `fix` authorize disclosure of the visible sanitized diagnostic bundle to
the configured provider, not executing a correction. Help MUST explain both consequences,
possible terminal echo, and lack of guaranteed arbitrary-secret removal. Persistent
“always repair” configuration cannot enable an autonomous policy. Previously proposed
`agent` policy was never shipped and is not retained as another alias: it is a usage error.

**Local execution is LLM-independent.** Before explicit Analyze/Fix, runner neither creates
a provider nor validates API keys/models/endpoints, and itself sends no LLM/embedding requests.
Missing/invalid provider settings cannot block successful/none/retry runs. Load only necessary
valid runtime/privacy settings; do not suppress errors in those settings themselves. If a
provider is unavailable after selecting AI handling, retain command/log/exit policy and show
a safe error. This does not constrain network operations performed by the child command.

### 3.2. Query dispatch and argument compatibility

Order: built-in routing → root-prefix options and named lookup on original argv → opaque
matched-entity tail → query parsing only after alias/script not-found. Preserve the
[named-dispatch contract](ALIASES-SCRIPTS-SPEC.md): query options such as `--wrap`, `--json`,
and `--no-history` after an entity name are literal argv, except existing entity-help/first
`--` rules. Root machine flags apply only before the entity name. Explicit root-prefix
`--wrap`/`--no-wrap` before a matched entity is inapplicable and rejected, not an execution
modifier. Neither lookup nor help calls the LLM.

New dotenv setting `COMMAND_WRAPPER_ENABLED=true`; precedence:
`--wrap`/`--no-wrap` → environment → `~/.zev/config` → `true`. Flags are mutually exclusive.
Machine query returns original command/JSON; explicit wrap flags there are usage errors.

```bash
zev 'run the tests'
zev --no-wrap 'run the tests'
zev --wrap --execute 'run the tests'
```

## 4. Default delivery and command names

### 4.1. Inject and execute

- `execute`: call the shared runner in-process with typed parameters; no second executable
  or repeated shell-text parsing.
- `inject`: invoke the **absolute path of the current Zev executable**, not bare `zev`
  resolved from PATH. Quote path, argv, and original shell program for the receiving dialect;
  the entire original shell program remains one `-c` argument.
- Successful injection emits no selected-command warning; fallback after injection failure emits
  the same invocation without executing it.
- Current binary identity means the chosen executable path at delivery time, not immutability
  of that file at a later manual launch. Same-UID replacement is outside the threat model.
- Choose the command interpreter using the existing `$SHELL`, then `sh` rule, resolve an
  executable path before launch, and fix it across attempts. Do not add `pipefail`, `set -e`,
  login mode, or startup options.
- **Receiving dialect** and interpreter differ. Auto receiving dialect uses verified OS
  evidence of the invoking shell only; `$SHELL` is not such evidence. New query-only
  `--receiving-shell=auto|posix` (default auto) explicitly declares a POSIX receiving dialect
  for wrapped inject/printed fallback. Initial grammar is POSIX quoting; auto accepts
  verified sh/bash/dash/zsh. Unknown/unsupported receiving shells fail before delivery;
  raw fallback is forbidden. The flag is inapplicable to machine query, entity, and execute
  and returns usage error there. It does not select the interpreter.
- `cd`, `export`, shell functions, and job control inside the child do not change the parent
  shell; explicit `--no-wrap` is required. Setup/help MUST disclose this limitation.

**Effective privacy restrictions must cross the inject boundary.** If query disabled history
or context through flags/config, its invocation includes `--no-history`/`--no-context` before
the separator. Later configuration changes cannot remove those prohibitions. If enabled,
stricter new runner settings win. Pass IDs only when history is available. If subsequently
disabled, apply the uniform §3.1 rule: ignore IDs with a warning, no SQLite, execute without
a link. Generated invocations omit IDs when history is already disabled.

History retains the original model command. History write failure does not block first
execution, but must not invent a link. With history-disabled inject, the original query
process cannot transfer its in-memory dialogue to a second process without another transport;
Analyze/Fix starts a separate in-memory diagnostic conversation.
Explicit tools intent crosses the boundary only under §8.3; persistent permission does not.

### 4.2. Reserved names

Reserve every built-in **root** name, command alias, hidden, and automatically added command:
for example query/continue/history/setup/doctor/alias/script/version, new run, help, completion.
This list is not the source of truth. Comparison follows exact case-sensitive routing;
nested-only subcommand names are not reserved.

One shared validator MUST cover create/save-from-history/generate/rename/edit-name and
loaded-data paths, including direct service calls. Pre-write rejection names the conflict
without data changes. Registry coverage tests include all actual root commands, synonyms,
hidden/help/completion without importing CLI into domain. Adding a command updates the
shared contract, not a wrapper-only blacklist. An existing conflict is not silently deleted,
renamed, or executed instead of a built-in: report it with explicit repair instructions;
automatic migration is not authorized.

## 5. Outcomes, actions, and mandatory log display

An eligible failure is a nonzero exit code from normally exited child or a definite startup
failure with no launched process. Stderr with exit 0 is success, not a recovery trigger.
Timeout, parent cancellation, child-only signal, and unknown supervision/capture/cleanup
outcomes show diagnostics but **never open recovery**, even under retry/analyze/fix policy.

```text
validate → execute → finalize capture/cleanup → outcome
  success → finish
  cancelled/timed_out/signalled/unknown → show available log → finish
  eligible failure → show log barrier → resolve policy
    none → finish
    retry → next attempt → same outcome handling
    analyze → provider → analysis_ready → report → finish
    fix → optional Clarify request → provider → correction_ready → finish or consented correction
```

**Display barrier:** after child/cleanup completion, finalize the retained log, then fully
write/render it to the diagnostic sink (stderr/UI), and only then start retry/provider work.
This means delivery to the sink, not proof a human read it. An artifact without log display
is insufficient. A broken diagnostic sink prohibits recovery and returns `125`, even with
explicit headless analyze/fix. Emit `failure_log_shown` after the barrier, not before it. Use
`display_failed` if the event sink still works. On crashes only best-effort display of the
available log is possible.

The screen contains a masked command, cwd, attempt number/duration/outcome, expanded scrollable
log initially positioned at its end, truncation, and artifact path when present.
Explicitly show empty output together with the startup error/code.

```text
Command exited with code 1 · attempt 1
$ go test ./...
--- log ---
... output ...
--- end log ---
  [Retry command]
  [Analyze error]
  [Fix command]
> [Do nothing]
```

| Policy | UI available | UI unavailable |
|---|---|---|
| ask | Log, menu with default none | Log, none without reading input |
| none | Log, finish | Log, finish |
| retry | Log, bounded retry | Log, bounded retry |
| analyze | Log, analysis request, report without correction | Log, analysis request, report without correction |
| fix | Log, Clarify request, correction request | Log, correction request with empty clarification |

Enter with initial menu selection, Escape/EOF/close/UI error mean none and preserve command-result
policy. UI initialization failure permits explicit headless policy only **before** interaction
begins. Once Clarify request opens, its error/cancellation never becomes automatic submission.
UI failure differs from display-sink failure.

After another failure, manual Retry under ask returns to a menu with default none. Disable
retry after budget exhaustion. Automatic retry finishes at budget exhaustion, without analyze/fix
escalation. Retry reuses argv/cwd/environment/I/O snapshots, not a frozen filesystem/external
state, and does not roll back partial effects. No next attempt before confirmed previous
cleanup. AI handoff (analyze or fix) occurs at most once per invocation. An analysis report
does not enter Fix without a new explicit user request.

## 6. Analysis without correction and the separate Fix flow

### 6.1. Analyze error

Choosing **“Analyze error”** after the display barrier explicitly authorizes sending the shown
sanitized bundle to the LLM in `analyze` mode. No additional Clarify request is required:
return explanation and recommendations without generating an executable correction.
`--on-failure=analyze` does this without the menu, including headless; defaults are unchanged.

Analysis returns a separate bounded version-1 `FailureAnalysis` type: a short explanation,
probable causes tied to observed lines/codes, diagnostic/remediation recommendations, and
explicit uncertainty limits. Fields: `schema_version=1`, nonempty `summary`, `causes`
(objects with string `reason` and string-list `evidence`), `recommendations` (strings),
`uncertainty` (string).
When the cause is unknown, causes may be empty and uncertainty explains the missing evidence.
Encoded JSON totals ≤32 KiB with valid UTF-8 text; reject
unknown/invalid fields and overflow before rendering. HTTP body retains the existing
256 KiB limit. Do not print invalid raw responses as reports; apply the same terminal
escaping/redaction. History stores mode/outcome/reference, not report text or invented
command alternatives; the analysis text remains ephemeral. This is neither OptionsResponse with
a fabricated invalid-request status nor an executable Command list. Incomplete logs must
produce explicit unknowns, not assumptions presented as established causes.

Recommendations may contain command examples as **inert text**. Analyze never offers an
execution selector, invokes Execute/Inject, modifies files, or automatically enters Fix.
This also applies to command-like text, mistakenly returned corrective objects, and hostile
instructions. Host-side dispatch fixes the mode before provider calls; model output cannot
change it.

After validation, print the report to stderr and finish with the unchanged command-result
policy. Successful analysis neither makes the original failure successful nor establishes
remediation. Explicit read-only tools remain subject to §8.3; they grant no modification rights.

### 6.2. Fix command

Fix takes a typed bundle and optional clarification, not the generic empty-prompt refinement
path. **“Fix command”** opens existing Clarify request. Empty/whitespace submission is valid:
provider receives a standard instruction to correct using the error. Error/log are already
attached; no repeated paste-log/context-file questionnaire. Escape/EOF/input error/cancel are
not submit and never call the LLM. `--on-failure=fix` skips the menu; retain Clarify with UI,
or send the bundle with empty clarification without UI.

Blank clarification creates no historical refinement. The standard instruction must not
impersonate user text: add it only while constructing the provider request. Persist actual
nonempty clarification as user text. Structured exit/start errors suffice for an empty-log
failure; do not invent nonempty logs. Ordinary query/refine/continue retain their rules.

After validated Fix response, enter **correction_ready**: corrected alternatives preview
and a separate conscious choice to execute a specific alternative. Before consent, prohibit
Execute, Inject, and automatic first-option selection from configured execute mode.
Confirmation shows the exact command/warnings before execution; default is refusal.
After consent, execute the selected correction as a separate supervised correction run with
`on-failure=none`, without automatic retry/analyze/fix. It has its own ID/outcome and does not
replace original command-result policy. Refusal/cancellation neither executes nor injects.
Without UI, print correction/explanation to stderr without execution/injection. Do not
automatically rerun the original command after correction. A model answer is a proposal, not
proof of successful correction; verification requires a new explicit execution.

### 6.3. Shared AI handling constraints

Both modes use the same failure bundle, redaction, trust boundary, and tools policy.
Create providers only after explicit selection/flag and a successful display barrier.
Neither analyze nor fix is a local classifier of arbitrary stderr text. Preserve selected
`analyze|fix` mode in the typed request, event metadata, and original run manifest; never infer
it from model output, prompt wording, or presence of a commands field.

The entire provider/tool loop has one recovery deadline of 120 seconds from its start.
This excludes waiting for human input; all five adapters, HTTP retries, and tool turns share
one cancellable bounded context without resetting its deadline between turns. Existing
stricter tool/network timeouts remain. Recovery deadline expiry is provider failure: no
correction execution or new recovery, preserving command-result policy. Parent SIGINT/SIGTERM
and cleanup failures take priority under §10. Cancel/join owned network work and cooperative
workers; uninterruptible external/kernel calls are not claimed bounded and follow §7.2 limits.

## 7. I/O, process ownership, and diagnostics

### 7.1. Execution streams are separate from UI

| stdin / stdout / stderr | Execution I/O |
|---|---|
| All three TTY, same verified terminal session | PTY/platform equivalent; merged terminal stream allowed |
| Any descriptor redirected, including stdout pipe or stderr-only redirect | Pipes/pass-through; stdout/stderr remain separate byte streams |

A controlling terminal for UI is a separate check. Stderr is the diagnostic sink; interactive
TUI requires stderr to be a TTY and a separate controlling-terminal input handle. Otherwise
print a plaintext log, interaction is unavailable, and headless policy applies. Never read
redirected stdin/data stdout as UI. `--no-interactive` prohibits prompts. Do not record stdin
as a separate stream.

**Byte preservation applies to forwarded child bytes, not the entire stderr descriptor.**
In pipe/pass-through mode, child stdout/stderr preserve bytes/CRLF and are never merged.
Stdout contains child data only. Stderr additionally contains clearly separated wrapper
diagnostics and replayed failure logs, including with `2>file`; that resulting file is not
promised to be a byte-for-byte copy of child stderr. Do not add events there. Raw child
payload can be tested separately in capture/memory from replay diagnostics. PTY terminal
processing/CRLF changes and merging are allowed and marked `stream=terminal`,
`stderr_present=unknown`. Do not sanitize live output; transform the diagnostic copy only.

Do not replay piped/redirected stdin: disable retry; reject explicit retry policy before
launch. The exception is positively identified DEVNULL/known-empty stdin without consuming
data to “check”. Never await pipe EOF to establish emptiness.

### 7.2. Terminal session and cleanup

A terminal-session port owns PTY master/slave, controlling session/foreground group, resize,
termios restoration, and handle closure in the process owner's lifecycle. Passing a PTY
slave into the old foreground-stdin algorithm is not sufficient. Require native fixtures
before enabling a platform: interactive input, resize, Ctrl-C, mode restoration, descendant
termination, and echo. Background job control/daemon detachment is unsupported. External
brokers and detached Unix sessions are outside the ownership boundary.

A separate cleanup budget is 5 seconds per attempt/correction after cancellation/leader
exit, not the canceled request context. Required evidence: leader reaped, platform-supported
confirmation of owned-descendant termination, PTY/pipe endpoints closed, capture workers
joined, terminal restored. Sending a signal/successful Kill alone does not prove cleanup.
Unknown required evidence means cleanup_unknown/125, no next launch/recovery.
This guarantee excludes uninterruptible kernel I/O: do not promise a wall-clock bound for
a stalled filesystem syscall or removal of an unkillable kernel task. Explicitly report
those limitations and unresolved cleanup; timeout does not release ownership. Do not accept
arbitrary blocking Reader/Writer callbacks as if this budget bounded them.

### 7.3. Capture and privacy

Diagnostic copy: bounded tail, best-effort redaction of known credential patterns/configured
Zev secrets, escaping of terminal controls/invalid UTF-8. Encoded retained bytes after
escaping/redaction stay within log-limit; raw discarded-byte counts are separate.
Independent stdout/stderr ordering is observed capture order, not a global causal order.
Redaction spans chunk boundaries with bounded lookbehind; arbitrary-secret detection is
not guaranteed. Do not persist a separate raw copy.

**PTY echo:** not recording a separate stdin channel does not exclude input from terminal
output. PTY artifacts have `echo_possible=true`; visible logs and Clarify request warn that
input/secrets may appear and will be sent to the provider with Analyze/Fix. Choosing Analyze,
or Fix followed by submission even empty, authorizes disclosure of **the displayed sanitized
log**. Explicit headless analyze/fix flags authorize the same disclosure without UI, as warned
in help. Do not promise arbitrary passwords are absent from artifacts/bundles. This does not
protect against hostile same-UID processes or disclosure in original live output.

Capture/memory is bounded and does not block due to ring overflow. Known capture loss differs
from intentional truncation: the former disables recovery/returns 125; the latter is visible
and permits recovery. Do not duplicate full capture into slog, SQLite, FTS, or embeddings.
Terminal-escape application diagnostics and paths too.

## 8. Continuation, provenance, tools, and recursion

### 8.1. Wrapper handoff and ordinary continue are separate entry points

Ordinary `zev continue` retains its existing contract: the user **attaches the log manually**.
It does not search run artifacts, select a failed attempt, or automatically send a saved log.
Version one adds no public continuation-by-run-ID entry point, attempt selector, or saved-log
submission flag. This feature does not change standalone continue exit codes. Manually attached
logs follow existing continue/history rules, not the prohibition on automatic capture copies.

Automatic attachment applies only to the **current wrapper's internal handoff**, after
Analyze, Fix with Clarify request submission, or explicit `--on-failure=analyze`/
`--on-failure=fix`. It takes the selected mode and an immutable bounded
snapshot of the **exact current failed attempt** and IDs already known to the runner. It never
rereads a log pathname, calls LastConversation, or selects the latest attempt. No handoff after
a successful retry, signal, timeout, or unknown outcome. Do not automatically resume it after
process termination, a crash, or manual continue.

Bundle v1: run/attempt IDs/ordinal; UTC start/duration; started; outcome/error_source;
nullable exit_code and separate signal; sanitized command; bounded log with labels/truncation/
redaction/echo metadata; linked conversation/command IDs; analyze/fix mode; optional
clarification (for Fix).
Manifest fixes command-snapshot digest (not authorization), IDs, and local cwd snapshot.
Keep raw argv/environment in memory only for retry. Artifact stores sanitized command,
not a separate raw argv/env copy. When history is available, compare original-command digest
and ID ownership; mismatch fails closed. Checking does not authorize execution. History
rows are not attempt-outcome authority and are not overwritten by concurrent runs.

Standalone run without a conversation link uses its own diagnostic context from the
**memory bundle**, requires no existing LLM history, and does not warn about a link that
never existed. If a link was supplied but the source conversation is unavailable, explain
why and create a separate context without loading another conversation. Even in that new
dialogue, history stores only a reference/summary, not the full log; provider receives it only
from the bounded memory snapshot. History/storage warnings cannot invent provenance. Active
in-process handoff can use its exact in-memory dialogue; history-disabled inject cannot recover
that memory after its original query process exits.

### 8.2. Trust boundary and no historical log copies

`command_error` Kind alone is not a wire boundary: existing builders forward Content.
Require a shared recovery serializer: JSON-encoded fixed-type/version data envelope with
string command/error/log fields, proper escaping, and a trusted instruction that these fields
are data, not instructions. All five provider paths, including tool-enabled builders/Ollama,
MUST preserve framing; hostile logs cannot close the envelope or become system messages.
Framing does not make an LLM immune to prompt injection; enforce side-effect policy through
host-side tools approval, not a promise that the model never follows log instructions.

History stores a command_error **reference/summary** (run/attempt IDs, outcome, digest), not
the full log; provider receives the log from the validated memory snapshot. Blank clarification
is not user refinement; synthetic instructions exist only during request construction.
Restoring history does not automatically expand run references into logs; missing artifacts
are never replaced with unrelated data. Do not leak logs into embeddings/FTS through copied
history metadata.

### 8.3. Recovery tools policy

- Default tools disabled, independent of TOOLS_ENABLED/TOOL_APPROVAL_MODE configuration.
- Explicit current `--tools` permits existing read-only manual/auto operations only;
  enabled default is manual. `--no-tools` prohibits; conflicting flags are usage errors.
- Explicit bypass on run is rejected before launch/provider; configured bypass is ignored.
  This feature does not change ordinary standalone continue policy.
- `--no-context` removes automatic environment context, not explicitly authorized tools.
  Help warns that explicit tools may disclose the data they read.
- Headless manual requests receive denial, never wait or become auto-approved.
- Explicit query tools intent crosses into generated run only with safe manual/auto policy.
  A bypass query retains its existing contract, but its recovery tools are disabled with
  a warning; bypass does not cross the wrapper boundary.

### 8.4. Recursion

Typed execution context limits in-process recovery depth to one handoff. Across processes,
runner passes an internal depth marker in child environment. Cooperative nested run, including
shell-launched run, suppresses its retry/menu/provider and returns command outcome to the outer
owner. This is a correctness protocol, not a security boundary. If a child explicitly removes
the marker/environment, new Zev is an independent invocation; do not promise absolute protection
against deliberate context removal. Invalid/exhausted markers suppress recovery and grant no
permissions. Provider/tool/editor/cleanup paths are not wrapped. The owner explicitly creates
a consented correction run as a distinct ID with recovery disabled.

## 9. Durable artifacts, quota, and crash recovery

Enabled history permits `~/.zev/runs/<run_id>/`; history-disabled/no-history prohibits
**creating** SQLite/run artifacts. ID is a UUID, not a path. Unix directories 0700/files 0600;
Windows ACLs are a native gate. Validate nofollow/regular-file identity/link count, size,
version, and safe components on read/publication; trusted HOME ancestor aliases are allowed.

**Attempt publication unit:** one immutable versioned JSON envelope containing metadata and
sanitized log. Write temp → fsync → rename → directory fsync. A separate atomic manifest
references already committed attempts and their digests only. Attempt first, manifest second;
no multi-file atomic transaction promise. Readers accept only completed manifests with valid
references. Unreferenced committed attempts after crashes are not adopted as latest.
`ErrCommitted` is not rollback: retain visible state with uncertain durability, disable recovery,
return wrapper failure/125. SQLite-summary errors are separate warnings, not capture/artifact
failures or reasons to repeat a child.

Manifest lifecycle: active → finished; owner disappearance before finished → crashed/unknown.
Owner holds an OS-backed exclusive per-run lock, reader a shared lease until snapshot copy,
and GC attempts a nonblocking exclusive lock. Coordinate run creation and reader-lease
acquisition under the same budget lock so GC cannot delete between lookup and lock.
PID/time/heartbeat cannot prove liveness.
Kernel lock release after crash permits reclaim without PID-reuse heuristics. Never automatically
resume crashed-run recovery; reclaim incomplete staging/orphan attempts under lock. Do not
unlink/recreate lock inodes while they may identify a live lease.

**Quota:** 100 MiB bounds logical payload bytes/reservations inside runs, not filesystem block
allocation/metadata. Count committed/staging attempts and manifests; exclude external events-file.
Attempt metadata ≤64 KiB, manifest ≤64 KiB. Before first launch, reserve under a cross-process
budget lock:
`2 × ((max_retries + 1) × (log_limit + 65536) + 65536)` bytes.
The factor 2 covers coexisting staging/committed generations. All allocation/update/GC obey
reservations; the **budget lock** never spans child/provider I/O. The owner holds its
**per-run lock continuously** through child/provider work until finalization and completion
of all its artifact writes. Short budget operations never wait for
per-run locks: GC uses try-lock. Reconcile stale-owner reservations under budget lock using
kernel-lock evidence. Minimum reservation also bounds concurrent persisted-run count; there
is no separate unbounded registry. Charge an active run its reservation, not reservation
plus bytes already covered by it. After completed finalization, replace that charge under
the budget lock with actual retained-file size, releasing unused reservation; GC releases
the remainder.

If reservation does not fit, first GC unlocked finished runs oldest-finished first, then
choose **ephemeral mode for the entire invocation before first launch**, with a warning.
Do not switch a partially persistent run after write failure. For example, 16 MiB × 11 attempts
cannot fit and becomes ephemeral up front, but in-memory recovery remains available. Actual
ENOSPC/write/fsync failure despite reservation returns 125 and prohibits recovery.
Enforce 7-day retention on startup/access/GC, not through a background daemon; active/read-leased
runs survive until lock release. Reclaim is allowed after expiry and lease release; retention
is not an automatic saved-log disclosure mechanism. Deletion does not guarantee forensic
erase, backup removal, or removal of
text already sent to a provider.

## 10. Exit codes and independent outcomes

First compute **command result**: original = first attempt, last = last actually executed
original-command attempt. None, analyze/fix response/failure, correction refusal, and consented
correction outcome do not change that policy. Startup errors count as attempts with code
126/127; cleanup/capture are not ordinary command exits.

Returned-result priority, highest first:

| Condition | Code |
|---|---|
| Supervision/capture/artifact/display/events failure or cleanup_unknown | 125 |
| Wrapper SIGINT / SIGTERM, including during menu/delay/provider | 130 / 143 on Unix |
| Expired per-attempt wrapper timeout | 124 |
| Child-only signal without parent cancellation | 128 + signal on Unix |
| Pre-launch argument error | 2 |
| Otherwise | original/last command result; 0 on initial success |

First registered parent cancellation fixes SIGINT/SIGTERM reason; later signals do not change
it. Cleanup failure still promotes it to 125. Retry timeout overrides original failure.
Parent cancellation during recovery returns cancellation code; ordinary provider failure keeps
command-result policy. Preserve secondary causes in metadata; do not choose by errors.Join order.
After run_finished finalization, the result is immutable; later events do not belong to that
finished run. Publish/native-test Windows mapping separately, never copy Unix formulas.
Distinguish identical child/wrapper numeric codes by outcome/error_source, not inference.

## 11. Events, architecture, and platform gates

`--events-file` is an explicit no-history exception: permit metadata-only JSONL even with
`--no-history`, without SQLite/run artifacts/log copies. Help documents the combination.
Exclusive create 0600, no overwrite/symlink/nonregular; one writer, ordered sequence.
Validate **every** external events-path component: directory-relative traversal from pinned
cwd/chosen absolute anchor, existing real parent directories, exclusive nofollow leaf, and
opened-identity verification. Reject symlinks in the selected path and parent replacement
between validation/open; never continue writing through an unsafe path. Trusted OS/HOME
aliases may be canonicalized beforehand in composition root, not used as fallback for
unvalidated path components. Do not automatically create parents.
Each encoded line ≤8 KiB, entire events-file ≤1 MiB; limit/short-write errors return 125 and
prohibit recovery. Blocking kernel filesystem I/O has the §7.2 limitation.

Fields: schema_version=1, run_id, attempt_id when present, sequence, timestamp, type.
Types: run_started, attempt_started, attempt_finished, failure_log_shown, display_failed,
action_selected, recovery_started, recovery_finished, run_finished.
Action includes none/retry/analyze/fix and reason (user_submit/cancel/eof/ui_error/headless_default/
explicit_policy/budget_exhausted). Terminal result includes command-result policy, initial/
last command outcome, separate correction/recovery outcome, returned code, secondary causes.
No argv/cwd/log contents/secrets. Events are not the authoritative recovery journal.

On normal termination with a healthy sink, **emit run_finished once**. Crashes may omit a
terminal event/leave a partial final line: readers ignore only the incomplete tail, never
infer success from it. No fsync durability, event replay, or exactly-once delivery across
crashes is promised. Broken sink diagnostics are best-effort stderr. Slog includes stable
names/IDs only. Localize EN/RU UI, not machine keys/IDs/outcomes.

Run/service owns policies/consumer ports; repository/client own adapters; CLI wires them,
cmd/zev stays a launcher. Childprocess remains the shared owner; terminal-session adapters
extend required ownership contracts rather than create another supervisor. Extract a shared
typed AI-handling use case instead of another agent loop. Analyze has a separate consumer-side
provider contract/validator for FailureAnalysis and its own instruction/schema in all five
adapters; it reuses transport/serialization, not a disguised Command-generation response.
Instantiate provider factories lazily only for explicit AI handling. Existing query
OptionsResponse API and ordinary continue remain unchanged. SQLite schema stays unchanged;
existing history summaries are not attempt stores. New terminal dependencies need separate
approval and official-documentation/version verification.

Initial native acceptance: Linux and macOS. Do not enable default wrapping on Windows until
native TTY/process/ACL gates pass; requested wrapped delivery returns unsupported before
execution, explicit no-wrap retains the existing path. Cross-build is not native evidence.
Implementation fixes the precise support matrix/platform mapping before claiming support.

## 12. Acceptance criteria and verification

| ID | Verifiable behavior |
|---|---|
| A01 | Exit 0 with stderr invokes no recovery |
| A02 | Publish failure log before menu/retry/provider; menu default none |
| A03 | Enter in initial action menu with Do nothing selected, Escape/EOF/that menu failure causes no recovery; this is not the Clarify-submit rule |
| A04 | Headless default reads no input and does nothing |
| A05 | Explicit headless retry/analyze/fix follows policy and display barrier |
| A06 | Failed manual retry returns ask-menu with none; auto budget never escalates to analyze/fix |
| A07 | Retry preserves launch snapshot and awaits cleanup; never replay input pipes |
| A08 | Bounded binary/large output, chunk redaction, escaped controls, visible truncation |
| A09 | Empty output/start errors are visible; distinguish pre-start failure from launched child |
| A10 | Cancel/timeout/child signal starts no recovery; cleanup failure returns 125 |
| A11 | Real subprocess checks original/last for [7,9], none/analyze/fix/provider failure |
| A12 | Exact run/attempt/conversation across two terminals, never latest fallback |
| A13 | No-history opens no SQLite/creates no artifacts; in-memory Analyze/Fix works |
| A14 | Repository rejects invalid/missing/incomplete/symlink artifacts; their presence never triggers automatic LLM disclosure |
| A15 | Configured execute/bypass grants no recovery execution/tool authority |
| A16 | Real nested/shell-nested runs suppress recovery; environment-removal boundary documented |
| A17 | Wrapped inject/execute/fallback preserve shell program as one argv and executable path |
| A18 | Raw machine query; no automatic entity wrapper; opaque alias tail is not query-parsed |
| A19 | Clean data stdout; ordered/bounded metadata events, terminal emission without crash guarantee |
| A20 | Concurrent quota reservation/GC stays within budget and preserves active/read-leased runs |
| A21 | EN/RU help/screens/defaults and parent-shell limitations agree |
| A22 | Native PTY input/resize/Ctrl-C/termios/descendant cleanup per supported OS |
| A23 | Root built-in names/aliases/hidden/help/completion rejected across alias create/rename paths |
| A24 | Fix opens Clarify request and attaches log/error automatically |
| A25 | Empty/whitespace submission calls provider without empty historical refinement |
| A26 | Fix Clarify cancel/EOF/UI error sends nothing; headless fix skips the screen; Analyze requires no Clarify |
| A27 | Registry coverage/existing name conflicts prevent silent shadowing/rename |
| A28 | Inject carries effective privacy restrictions across config changes without recovering lost memory |
| A29 | Ordinary continue uses manual log attachment; internal wrapper handoff uses neither LastConversation nor paste-log questionnaire |
| A30 | Correction preview neither executes nor injects; default refusal; consented run has no recovery |
| A31 | Broken stderr/display blocks recovery; distinguish UI-init from mid-Clarify failure |
| A32 | Redirected child payload stays unchanged; stdout is clean, stderr adds separated plaintext diagnostics; TTY stdin does not enable PTY/TUI for redirected stderr |
| A33 | Receiving dialect is not inferred from SHELL; quotes/$()/backticks/Unicode/LF preserve boundaries |
| A34 | Current Zev path works without PATH entry, with spaces/quotes/another zev in PATH |
| A35 | PTY echo_possible/disclosure warning; fixtures do not promise arbitrary-secret elimination |
| A36 | All provider/tool builders preserve escaped untrusted envelopes; default tools never execute log instructions |
| A37 | Recovery tools matrix config bypass/no-context/explicit tools/manual/auto/headless is fail-closed |
| A38 | Crashes before/after attempt/manifest rename, ErrCommitted, stale locks/reservations classified correctly |
| A39 | Internal handoff snapshots the current failed attempt, never a previous/latest history attempt; no automatic correction of success/unknown outcomes |
| A40 | No-history + explicit events-file writes bounded metadata only; short writes/partial crash tails handled |
| A41 | Mixed cancellation/deadline/capture/cleanup causes and returned code follow one priority table |
| A42 | Wrapper log is not copied to history/FTS/embeddings or a new missing-source diagnostic dialogue; manual continue attachments retain their contract |
| A43 | Budget lock is available during blocked child/provider work; per-run lock protects run/reservation from GC until finalization |
| A44 | Recovery deadline cancels provider/tool loop without execution, preserving command policy; parent cancellation/cleanup failure takes priority |
| A45 | Symlinked/replaced events-path parents cannot create files through unsafe paths; trusted anchor aliases never weaken suffix checks |
| A46 | Explicit run executes a user command without query, conversation IDs, or existing LLM history |
| A47 | Success/none/retry creates no provider and never validates missing/invalid LLM settings; provider call requires AI consent/display barrier |
| A48 | FailureAnalysis uses a strict separate bounded/escaped schema; causes cite evidence and uncertainty is explicit |
| A49 | Analyze command-like/hostile output never triggers Execute/Inject/selector, file changes, or entry into Fix |
| A50 | Four menu actions with default none; distinct analyze/fix flags, old agent rejected; Clarify only for Fix |
| A51 | Response cannot change request/event/manifest mode; all five provider paths support analysis without fabricated Command objects |
| A52 | Analysis success/failure/deadline preserves original/last command result; report is not copied into history as logs/commands |

Unit tests beside code: testing/table-driven and existing assertion style; RED before
implementation. Local provider fixtures, no real keys/LLM. Process tests use real children/
built binaries, not signals sent to the test runner. Native integration proves PTY/kernel
ownership separately from compile/host tests. Green mocks do not establish freedom from
arbitrary kernel hangs or cleanup of detached processes.

```bash
go test ./internal/cli ./internal/core/childprocess ./pkg/terminal
make test
make test-race
make lint
make test-integration
make test-e2e
make build
make clean
```

Focused run-slice tests are required once the package exists. Manual requires separate
permission. Before shipping, synchronize README, CONFIGURATION, ARCHITECTURE, PROJECT,
help/setup, locale catalogs, CHANGELOG. Prose-only review checks links/whitespace/CLI/default/
acceptance parity/fences; do not run autofix to verify prose.

## 13. Approval and review status

The user approved feature scope, Clarify flow, reserved names, and defaults. Two independent
read-only rounds ran: four reviewers in round one and three in round two, covering
UX/CLI/storage/security/process. Verified findings were resolved in both languages; round-two
fixes clarify stderr, lifetime locks, recovery deadlines, and events paths. The user explicitly
clarified manual continue semantics, so the unnecessary public run-continuation entry point
was removed. This eliminates its two product ambiguities rather than silently choosing defaults.

Final amendments were author-checked against the target contracts and documentation checks;
no separate third independent round followed them. Specification review does not verify future
runtime behavior or prove absence of all possible defects.

Specification-only work: no code, commit, migration, or dependency installation. New product
choices do not become approved merely because a reviewer suggested them.

After these rounds the user added Analyze without correction and positioned standalone run
alongside query. Four actions, analyze/fix policies, FailureAnalysis, and lazy provider setup
are synchronized in EN/RU and author-checked. This extension is not covered by the prior
version's completed independent reviews. Standalone run remains part of the execution/failure-
handling feature, not a promise to read shell history or perform fully autonomous correction.
