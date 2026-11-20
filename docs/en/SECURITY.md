<div align="center">

**English** | [Русский](../ru/SECURITY.md)

</div>

# Security behavior and limits

This guide describes controls present in the current Zev implementation. Security checks are application policy, not an operating-system sandbox. Read [Configuration](CONFIGURATION.md) for the YAML schema and [CLI reference](CLI.md) for flags.

## Security boundaries

Zev handles provider responses, tool requests/results, project files, saved aliases and scripts, command output, and fetched web pages as untrusted input. These values can supply task data, but cannot change local policy or grant approval. The user’s current command-line request and an explicit approval prompt are the authority for the operation being requested; a provider’s explanation or “dangerous” flag is not authorization.

Zev does not defend against a compromised OS/kernel, administrator, or hostile process running as the same user. File locks coordinate cooperating Zev processes only. Approval, command analysis, path checks, and capability descriptors do not isolate arbitrary host commands from files, network, credentials, or other host resources.

## Command delivery

### `inject` (default)

Zev selects a command and attempts to insert it into terminal input. It never presses Enter. The command warning and exact selected command are shown before insertion when a warning is present. Injection is **warning-only**: the user's shell will later execute the text if the user submits it, and Zev cannot control or observe that later execution. On unsupported/denied injection, Zev prints the command instead. If injection was partial, do not submit the terminal input; inspect it first.

When `command_wrapper_enabled: true` (default), generated injection is wrapped with `zev run --` so the later shell invocation can capture bounded failure output and offer recovery. Disable the wrapper with `command_wrapper_enabled: false`; explicit `zev run` remains available. The wrapper is unsupported for injection on Windows. Wrapping does not turn injection into a sandbox or authorize execution by Zev.

### `execute`

`execute` runs the first provider suggestion through the selected host shell. Local analysis uses a bounded structural subset for the resolved POSIX `sh`, Bash, or Zsh. It does not run the command during analysis. Known dangerous operations and incomplete/unsupported analysis require confirmation in `security.mode: auto`; a configured deny rule refuses the action. For an unrecognized command, automatic execution first sends the exact command, original user request, and generated short explanation to a separate, bounded safety-review request to the configured provider. A timeout, provider error, malformed/unclear response, or unsafe verdict stops execution. This remote review does not override a local dangerous finding or a matching deny rule.

An explicit confirmation approves only the current operation. It does not make a command safe, and it is not remembered as a standing permission. The `security.mode: bypass` setting skips permission prompts for otherwise valid host `execute` delivery; matching deny rules still apply to that path. `inject` deliberately remains warning-only and does not use those permission decisions because execution is handed to the user's shell. `bypass` is not a security guarantee and is not a sandbox.

### Managed aliases and scripts

Aliases and scripts execute directly, independently of `command_execution_mode`. Alias commands are structurally assessed; dangerous or unknown aliases require confirmation in `auto`. Saved script bodies are treated as opaque/unanalysed: the user sees a warning including the SHA-256 digest of the exact body, and `auto` requires confirmation. A deny rule still refuses either action; `bypass` skips the confirmation but does not change the host privilege boundary.

Scripts are read into a bounded immutable snapshot before authorization and that snapshot is executed, so a pathname replacement cannot silently substitute different script bytes after approval. On Unix, scripts run through Bash using an inherited descriptor and literal arguments. Script execution is unavailable on Windows until a verified handle-safe Bash transport exists. Alias execution is also unavailable on Windows; it requires an allowlisted POSIX-compatible shell on Unix.

## Policy configuration

The `security` section has its own schema version (`1`). Defaults are `mode: auto`, `default_decision: ask`, local probe/classifier, and failure behavior `ask`. Version 1 does not enable model-based probe or classifier modes and does not provide an OS sandbox. `security.mode: normal` is accepted as a legacy spelling of `auto` and normalized when saved; legacy sandbox configuration is not supported.

Rules are grouped as `deny`, `ask`, or `allow`. Each rule has a unique non-empty ID and a non-empty `match`. Selectors include sources, deliveries, effects, paths, executables, argv prefixes, analysis certainty, and risk. Fields within a match are conjunctive; values within one selector list are alternatives. Path selectors support `${workspace}` and `${home}` substitutions and component-aware `*`/`**` matching. Matching priority is always **deny > ask > allow**, independent of YAML order. An allow must cover all requested effects; partial effect coverage does not grant permission. Global default may be `ask` or `deny`, never `allow`. Invalid or unknown values fail configuration validation before dispatch.

Example:

```yaml
security:
  version: 1
  mode: auto
  default_decision: ask
  rules:
    deny:
      - id: deny-policy-edits
        match:
          effects: [security_policy_change]
    ask:
      - id: confirm-network
        match:
          effects: [network]
```

Use the actual supported selector strings documented in [Configuration](CONFIGURATION.md); this sample is illustrative and does not add policy capabilities.

## Model context tools

Tools have a separate `tool_approval_mode` (`manual`, `auto`, or `bypass`); it is independent of `security.mode`.

- `manual` displays the exact request and fixed working directory on stderr; `y` or Enter approves, `n` refuses, and Esc cancels. No terminal, cancellation, or refusal means denial.
- `auto` allows only the restricted local builtins `pwd`, `ls`, `cat`, and `head` in the configured working root. It checks the request locally and sends a separate tools-disabled review request before each call. Execution repeats the checks. Review/network errors and ambiguous responses deny the call.
- `bypass` skips the prompt/reviewer, but still uses the same restricted builtins and checks. It does not grant arbitrary process, shell, write, or network access to `exec`.

`--print`/`--json` do not enable tools implicitly; use `--tools` to opt in. `--no-context` or disabled `context_enabled` turns tools off unless `--tools` explicitly overrides that privacy default. Tool data is ephemeral and not saved as intermediate history, but the provider may receive tool results and include them in its final answer. Known credential-shaped values are redacted before probing/disclosure; this redactor is not comprehensive secret detection. After suspicious or incomplete tool output, automatic follow-up calls are blocked.

The restricted local adapter is not a complete content-secret scanner. It conservatively rejects sensitive path components, symlinks, special files, hard-linked files, traversal, and ambiguous names. Its exact allowlisted arguments, file/path limits and time/output budgets are in [Configuration](CONFIGURATION.md). A kernel/filesystem call may outlive the in-process deadline.

## Public web fetch

`security.web_fetch_enabled` is a separate opt-in capability; it is disabled by default. It permits bounded HTTP(S) GET only, with no userinfo, non-default ports, cookies, ambient credentials, uploads, or arbitrary methods. It validates literal IPs and (when locally resolved) every destination address, pins validated addresses for direct connections, limits redirects to three and keeps redirects on the original origin. Responses are capped at 256 KiB and 15 seconds; URL input is capped at 2048 bytes. Common credential-shaped response values are redacted and the bounded content is locally probed as untrusted data.

When an HTTP(S) or remote-DNS SOCKS proxy resolves the destination, that proxy is part of Zev's trusted computing base: Zev cannot independently inspect the proxy's DNS answer. With `socks5://`, name resolution is local and destination IPs are checked; `socks5h://` delegates name resolution to the proxy. A selected proxy is not silently bypassed. See proxy routing details in [Configuration](CONFIGURATION.md).

## Files, credentials, and child processes

The configuration file must be a private regular file owned by the current user. Symlinks, hard links, special files and group/world permission bits are rejected before YAML parsing. On Windows, native owner/ACL checks fail closed if the security descriptor cannot be verified; cross-compilation is not a Windows runtime test. On Unix, Zev creates its data directory with mode `0700` and data files with `0600`; permissions do not encrypt plaintext provider credentials or local history.

Default child-process environments are filtered to an explicit allowlist, removing provider keys and unrelated ambient variables. This is defense in depth, not a secret boundary: a host command can still read accessible files or use host network access. Managed child processes are cancelled and their owned Unix process group/Windows job is cleaned up; detached processes and external brokers are outside this boundary.

Provider diagnostics omit raw request/response bodies, credentials, and endpoint URLs. Ordinary command output, prompts, enabled history, explicit context files, and opted-in recovery payloads are not guaranteed to be secret-free. `zev run` warns before sending captured failure output for provider analysis; review that output before opting in. Common credential redaction is pattern-based and incomplete.

## Data and privacy

History is local SQLite data, not encrypted storage. It can contain requests, refinements, provider responses and command alternatives; there is no count-based retention. `--no-history` prevents opening history for that query but neither deletes earlier data nor disables the in-memory conversation sent to the provider. Logical deletion does not guarantee erasure from WAL/free pages, exports, backups or remote copies.

Project context files are bounded, confined to the current working directory and checked for symlinks and sensitive paths before network access. `--allow-sensitive-context` is an explicit opt-in for a selected sensitive path; it does not permit symlinks. Text explicitly placed in a prompt can still be sent to the provider.

Semantic history embeddings are off by default. When enabled, only initial/refinement user messages are eligible; remote embedding requests disclose that text, and semantic/hybrid searches disclose the search query. Vector rebuild requires explicit `--yes` and reports provider, model and eligible count.

## Verification and known limits

Tests cover policy priority, grant binding/replay rejection, bounded shell/argv analysis, tool path checks, web-fetch destinations/redirects, redaction/probing and CLI approval flows. They do not establish that all shell syntax or all secrets are detected, that application policy is OS isolation, or that native Windows runtime behavior works. Do not infer safety from the absence of a warning. See [Development](DEVELOPMENT.md) for available test targets and [Project profile](../../PROJECT.md) for current platform limitations.
