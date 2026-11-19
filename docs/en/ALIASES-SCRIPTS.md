<div align="center">

**English** | [Русский](../ru/ALIASES-SCRIPTS.md)

</div>

# Managed aliases and scripts

Aliases and scripts are local, named artifacts stored below `~/.zev`. They are resolved before an unmatched natural-language request is sent to an LLM. Built-in CLI commands take precedence over managed names.

## Names and storage

Names must match `[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}` and must not be a built-in name such as `query`, `run`, `script`, `history`, `setup`, `update`, `version`, `help`, or `tool`. Zev rejects collisions across the alias store, script store, and built-in commands.

| Path | Contents | Protection |
|---|---|---|
| `~/.zev/aliases` | Version 2 YAML alias metadata | Private `0600` file, cooperative lock, atomic replacement |
| `~/.zev/scripts` | Version 2 YAML script metadata | Private `0600` file, cooperative transaction lock |
| `~/.zev/script_files/` | Script bodies | Directory `0700`; new bodies `0700` on Unix |
| `~/.zev/.<name>-scripts.lock` | Cross-process script-store lock | Persistent identity; do not delete or replace |

Metadata is strict and versioned. Unknown fields, invalid types, duplicate argument names, malformed names, invalid UTF-8, control characters and oversized documents are rejected. Current readers accept version 2 only; legacy migration runs before the store is opened.

## Aliases

An alias is an argv-safe template for one simple command. It may contain standalone placeholders:

- `{{.arg1}}`, `{{.arg2}}`, … for positional values;
- `{{.args}}` for all supplied values.

Numbered placeholders must be contiguous. Arguments are rendered as POSIX single-quoted words, so an argument remains one shell word even when it contains spaces or shell metacharacters. The template itself cannot contain pipelines, redirects, command substitutions, quotes, backslashes, globbing, assignments, shell control words, or shell re-evaluation such as `sh -c`. Use a script for shell syntax rather than trying to encode it in an alias.

Aliases can have ordered argument metadata. Required arguments must precede optional arguments; missing required values and extra values are rejected before execution. The maximum is 32 metadata arguments, with bounded names/descriptions.

Examples:

```bash
zev alias add dps "docker service ps {{.arg1}} --no-trunc"
zev dps my-service
zev alias list
zev alias last recent
zev recent
zev alias edit dps
zev alias remove dps
```

`zev --print NAME ...` expands an alias without executing it. `--json` returns the normal machine-mode command schema. `-h` on an alias prints its local usage and does not call an LLM. The aliases `rm` and `delete` are accepted for `alias remove`.

## Scripts

A script has a name, immutable record ID, description, ordered argument metadata and a relative body path under `script_files`. The metadata path must be relative, normalized, free of traversal and platform-specific absolute-path syntax. Application-owned path components and script leaves must not be symlinks; script files must be regular and singly linked.

Create or edit a script with an editor or bounded non-interactive input:

```bash
zev script create deploy "Deploy the application"
zev script create deploy --body-file deploy.sh
zev script create deploy --body-stdin
zev script edit deploy
zev script show deploy
zev deploy staging
zev script remove deploy
```

`script show` reads the exact registered body and never executes it. Scripts are not printed by query machine mode automatically; use `script show` explicitly. The `rm` and `delete` aliases are accepted for `script remove`.

The editor receives a private temporary copy. On a successful save, Zev validates the original record, path, file identity and content hash again before publishing. A stale editor returns a conflict instead of overwriting a newer body. Failed or cancelled editing leaves the registered script unchanged and removes the temporary directory.

## Publication and recovery

Alias updates use a cooperative read-modify-write lock and same-directory atomic replacement. Script create, replace and remove coordinate metadata and body publication with a private signed journal under `.scripts-journal`. Recovery advances explicit phases and verifies SHA-256 hashes before moving a staged body or metadata file. It can distinguish a committed publication from a pre-publication failure.

This is not a transaction spanning all files: a post-rename directory-sync error can mean the new version is already visible with uncertain crash durability. Callers must inspect current state before retrying an operation reported with `ErrCommitted`. Zev does not create secure-erasure backups and does not sweep user-created copies.

## Metadata generation

`--generate-description` and `--generate-arguments` are explicit opt-in network operations. They are not required for successful CRUD. In interactive mode Zev previews the destination, payload and whether it is local or remote, then requires literal `yes`. Generated fields are validated with the same metadata rules; a failed field generation produces a warning and does not make the artifact invalid. Script body input is bounded before it is sent for generation.

Do not put credentials in descriptions, templates, script bodies or prompts. Provider requests and generated metadata can disclose the selected artifact to the configured endpoint.

## Execution and security

Aliases and scripts execute directly when invoked by name; they do not use `command_execution_mode`. Before execution, Zev applies the centralized security policy and a one-time grant:

- aliases are checked with bounded shell analysis;
- scripts are treated as opaque code and require confirmation in `security.mode: auto`;
- the grant binds the exact command or script bytes, arguments, policy, working-directory identity and filtered environment;
- a changed body, policy, working directory, environment or input invalidates the grant;
- the grant is consumed once, so a retry requires a new assessment.

`security.mode: bypass` skips permission confirmation within its configured scope but is not a sandbox. Host execution can modify files, access the network and read resources available to the user. Managed child processes use the filtered environment and process cleanup described in [Security](SECURITY.md). Windows runtime support for these execution paths is intentionally limited and must not be inferred from cross-compilation.

## Compatibility and testing

The current format is version 2. Supported legacy migration inputs are released formats only; development snapshots are not compatibility targets. Relevant checks include metadata codec/validation tests, alias rendering tests, store locking and cancellation tests, path-security tests, editor stale-save tests, journal durability/recovery tests and built-binary CRUD scenarios. Run:

```bash
make test
make test-race
make test-e2e
```

`make test-e2e` is the black-box gate and may require the documented deployment prerequisites. See [Configuration](CONFIGURATION.md), [CLI reference](CLI.md), [Security](SECURITY.md), and [Development](DEVELOPMENT.md).
