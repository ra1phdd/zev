<div align="center">

**English** | [Русский](../ru/RUN-WRAPPER.md)

</div>

# `zev run`: shell execution and recovery

`zev run '<command>'` is a wrapper for one shell command. The command runs inside the configured `$SHELL`; `zev run` is not a retry loop or sandbox.

## Parsing contract

Wrapper options must appear before the command. Exactly one argument containing shell text must follow the options; Zev does not parse that text itself and passes it as the program to the configured shell.

```bash
zev run 'git status --short'
zev run --timeout=30s --log-limit=65536 './build/check --format json'
```

Supported options:

| Option | Contract |
|---|---|
| `--on-failure=ask|none|analyze|fix` | Failure policy; default `ask` |
| `--timeout=DURATION` | Per-attempt timeout from `0s` to `24h`; `0s` means no wrapper timeout |
| `--log-limit=BYTES` | Combined capture limit from 1 KiB to 16 MiB; default 1 MiB |
| `--session=ID` | Exactly 10 characters from the local session alphabet |
| `--proxy=URL` | Proxy for recovery/provider requests |
| `--no-interactive` | Disable recovery selection and clarification prompts |

Unknown options, options after `--`, an absent separator, an empty executable and missing option values are rejected before the child starts. Each invocation makes one attempt; there are no retry count, delay or retry-exit-code options.

## Process and output behavior

Zev forwards the shell's stdout and stderr to the caller while capturing bounded tails independently for recovery. Captured bytes are converted to valid UTF-8 and terminal controls are escaped in the failure log. The configured shell receives the command text and the wrapper's normal process boundary. Recovery output is written to stderr and does not replace the original exit result.

`--timeout` bounds the wrapper attempt through context cancellation. Process cleanup can take longer than the deadline, and detached descendants or processes started by external brokers are outside Zev’s ownership boundary. This is host execution, not OS isolation.

## Failure policies

- `none`: show the bounded failure log and keep the original failure.
- `ask`: in an interactive terminal, choose **Do nothing**, **Analyze error**, or **Fix command**. Without an interactive terminal the default remains no recovery.
- `analyze`: send a bounded diagnostic bundle to the provider for explanation.
- `fix`: optionally collect a clarification and ask for corrected command suggestions.

Recovery never reruns the failed command and never executes a suggested correction. It preserves the original failure and exit status. A cancellation, timeout, or uncertain process outcome skips recovery rather than sending a misleading diagnostic.

The diagnostic bundle is schema version 1 and contains the literal command (display-quoted), failure text, captured stdout/stderr tails and truncation flags. Treat it as sensitive: Zev warns before disclosure because output can contain secrets. The bundle is placed in an untrusted user-data section of the provider request; log text and model output cannot become execution instructions.

## Linked sessions

Generated injection can add an opaque `--session` key. The key maps locally to a canonical conversation ID and selected command ID in `~/.zev/run-sessions.json`. The registry is private, size-bounded, atomically written and protected by a cooperative lock. A session key is not a credential and does not authorize arbitrary history access: recovery verifies the mapped conversation and selected command before restoring it. Direct `zev run` without a session starts a standalone recovery query.

The registry is a local convenience, not a remote protocol. Invalid IDs, malformed records, non-canonical UUID references and oversized registries fail closed.

## Relationship to command security

`zev run` executes shell text, so it intentionally provides the configured shell's host capability and does not use the query `execute` authorization path or `security.mode` confirmation. The user explicitly invoked the wrapper; its technical boundaries still apply: bounded output, cancellation, filtered process handling where used, and no automatic retries. Shell metacharacters are interpreted by the configured shell.

This distinction is important:

- query `inject` inserts text and waits for the user's shell;
- query `execute` is a separate security-gated host delivery path;
- `zev run '<command>'` executes one shell command inside Zev and records bounded failure context;
- recovery suggestions are inert text only.

## Verification and limitations

Focused tests cover option parsing, exactly one shell-command argument, shell syntax execution, one attempt, output truncation, cancellation, session registry validation and linked conversation checks. They do not prove security of every shell command or shell dialect, containment of detached processes, or native Windows runtime behavior. Run `make test`, `make test-race` and the applicable `make test-e2e` gate. See [Security](SECURITY.md), [CLI reference](CLI.md), [Configuration](CONFIGURATION.md), and [Development](DEVELOPMENT.md).
