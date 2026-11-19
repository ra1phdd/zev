<div align="center">

**English** | [Русский](../ru/CLI.md)

</div>

# CLI reference

This reference describes the complete command tree and each command's own flags. It is based on the Cobra command definitions and runtime `--help` output; hidden/internal flags are explicitly marked. `zev help` is not a supported command: request help with `zev --help`, `zev -h`, or append `--help`/`-h` to a command. The root `help` entry visible in older builds is removed in the documented CLI.

## Conventions and inherited flags

- `[value]` is optional; `<value>` is required. `...` means repeated/remaining arguments.
- Unless noted, booleans default to `false`; strings default to empty. `--help` (`-h`) is available on every Cobra command and prints help without performing the operation.
- **Inherited flags on ordinary subcommands:** `--proxy URL` (default empty; route Zev network requests through SOCKS5 or HTTP(S)) and `-v, --verbose` (count, default `0`; `-v` INFO, `-vv` DEBUG, `-vvv` TRACE). Each command entry lists these inherited flags explicitly.
- **Root query mode:** the root accepts an unparsed natural-language query, preserving flags not recognized by Zev for aliases/scripts. It also has query-tool flags `--tools`, `--no-tools`, and `--tool-approval MODE`; see `zev [query]` below. Root dispatch flags are manually parsed, so use `zev --help` for the root Cobra help and `zev query --help` for query flags.
- `zev run` has a separate literal-argv parser because it uses `DisableFlagParsing`; its `--proxy` is the run-specific recovery proxy and its options must precede `--`.

## Command tree

```text
zev [query]
├── alias
│   ├── add [name template [description [arguments-json]]]
│   ├── edit [name]
│   ├── last <alias-name>
│   ├── list
│   └── remove [name] (aliases: rm, delete)
├── completion
│   ├── bash
│   ├── fish
│   ├── powershell
│   └── zsh
├── continue [clarification]
├── doctor
├── history
│   ├── clear
│   ├── delete <id>
│   ├── export
│   ├── index
│   │   ├── rebuild
│   │   └── status
│   ├── search [query]
│   └── show [conversation-id]
├── provider
│   ├── add NAME
│   ├── list
│   ├── remove NAME
│   └── use NAME
├── query [query]
├── run -- command [args...]
├── script
│   ├── create [name [description [arguments-json]]]
│   ├── edit [name]
│   ├── list
│   ├── remove [name] (aliases: rm, delete)
│   └── show <name>
├── setup
│   ├── headers
│   └── mode [inject|execute]
├── update [stable|main]
└── version
```

The default `zev [query]` root route and explicit `zev query` route share query behavior. Built-in command names take precedence over managed aliases and scripts. Alias/script names are otherwise invoked directly, with arguments following the name; put arguments to an alias at the end of the invocation. For example: `zev list-containers --format json`.

## Root query: `zev [query]`

Natural-language request (zero or more positional tokens; joined as the request). With no request, Zev opens its interactive input. Query flags are manually parsed and can be placed around the query text.

**Local/root flags:**

| Flag | Values/default | Effect |
|---|---|---|
| `-h, --help` | — | Show root help. |
| `--execute`, `-e` | boolean; false | Execute the selected suggestion immediately; mutually exclusive with `--inject`. |
| `--inject` | boolean; false | Insert selected command into the terminal; default delivery mode unless config selects another. |
| `--no-history` | boolean; false | Do not persist this query. |
| `--no-context` | boolean; false | Do not send local environment context. |
| `--tools` | boolean; false | Enable model context tools. Explicitly required with `--print`/`--json`. |
| `--no-tools` | boolean; false | Disable model context tools. |
| `--tool-approval MODE` | `manual`, `auto`, `bypass`; config default unless overridden | Set tool-call approval policy. |
| `--print` | boolean; false | Print the selected command without executing it (machine mode). |
| `--json` | boolean; false | Emit schema-version 1 command options JSON (machine mode). |
| `--option N` | integer; default `1` | Choose a 1-based command option in machine mode. |
| `--context-file`, `-cf PATH` | repeatable; none | Attach a project context file. |
| `--allow-sensitive-context` | boolean; false | Allow explicitly requested sensitive context paths; symlinks remain forbidden. |
| `--proxy URL` | string; empty | Route provider/network requests through SOCKS5 or HTTP(S) proxy. |
| `-v, --verbose` | count; `0` | Increase logging (`-v` INFO, `-vv` DEBUG, `-vvv` TRACE). |

Unknown flags belong to the named alias/script and are preserved; `--` passes the remaining tokens literally. The parser specially recognizes `-h`/`--help`, verbosity, execution mode, tool options, proxy, machine-output options, and context-file options. Query behavior is therefore not equivalent to ordinary Cobra positional parsing.

**Typical examples:**

```sh
zev "find files larger than 100 MB"
zev query "show running containers"
zev --print --json "list current branch"
```

## `zev query [query]`

Explicit spelling of root query mode. Accepts zero or more query tokens, joined as a prompt; unmatched flags remain available to aliases when dispatching. Has the same local flags and defaults as `zev [query]` above, plus inherited `--proxy`, `-v, --verbose`, and help. `--execute` and `--inject` conflict; in machine mode `--option` is one-based and defaults to `1`. `--print` prints the chosen command; `--json` prints the options schema. Tool usage in machine modes requires explicit `--tools`.

## `zev continue [clarification]`

Continue the latest usable saved conversation. Remaining positional tokens are treated as a clarification (zero or more). This command intentionally bypasses Cobra flag parsing at runtime and uses the same query flag contract for help output and validation; positional text is the clarification. `--no-history` is accepted by the parser but rejected because continuation requires history.

**Local flags:** `--tools` (boolean, false), `--no-tools` (boolean, false), `--tool-approval MODE` (`manual|auto|bypass`, default config); plus `-h, --help`. **Inherited:** `--proxy URL` (empty), `-v, --verbose` (count, 0). The query parser also recognizes `--proxy`, `--verbose`, and `-h` in this special route.

## Aliases: `zev alias`

Manage command templates. An alias template accepts one simple command and supports `{{.arg1}}`, `{{.arg2}}`, … and `{{.args}}`; shell pipelines/redirections require a script. The command `zev alias last NAME` saves the last eligible command from history as a new alias.

### `zev alias add [name template [description [arguments-json]]]`

Create an alias; interactive form may collect omitted values. Positional argument count: 0–4. For explicit metadata, arguments JSON is an ordered JSON array; `--argument NAME[=DESCRIPTION]` may be repeated. **Aliases:** none. **Local flags:**

- `--argument NAME[=DESCRIPTION]` (string array, repeatable; no entries by default): declare required argument metadata.
- `--arguments-json JSON` (string; empty): replace ordered arguments from JSON.
- `--clear-arguments`, `--clear-description` (booleans, false): remove corresponding metadata.
- `--description TEXT` (string; empty): replace description.
- `--generate-arguments`, `--generate-description` (booleans, false): explicitly authorize AI metadata generation.
- `--json` (boolean, false): output schema-versioned JSON without prompts; implies non-interactive behavior.
- `--non-interactive` (boolean, false): disable prompts/editors.
- `--template ARGV-TEMPLATE` (string; empty): specify/replace argv-safe template when name is positional or collected interactively.
- `--no-metadata` (boolean, false): create with empty metadata.
- `-h, --help`.

Inherited: `--proxy URL` (empty), `-v, --verbose` (count, 0). Use one template source: positional template or `--template`.

### `zev alias edit [name]`

Edit the selected alias, or prompt for one; accepts 0–1 positional arguments. **Local flags:** `--argument NAME[=DESCRIPTION]` (repeatable string array, default empty), `--arguments-json JSON` (string, empty), `--clear-arguments` (boolean, false), `--clear-description` (boolean, false), `--description TEXT` (string, empty), `--generate-arguments` (boolean, false), `--generate-description` (boolean, false), `--json` (boolean, false), `--no-metadata` (boolean, false; create/edit with empty metadata), `--non-interactive` (boolean, false), `--template ARGV-TEMPLATE` (string, empty), `-h, --help`. Inherited: `--proxy URL` (default empty), `-v, --verbose` (count, default `0`; `-v` INFO, `-vv` DEBUG, `-vvv` TRACE), `-h, --help`. Template and metadata fields are explicit replacements; conflicting sources are rejected.

### `zev alias last <alias-name>`

Save the last used history command under the required alias name (exactly one positional argument). **Local flags:** `--argument NAME[=DESCRIPTION]` (repeatable string array, default empty), `--arguments-json JSON` (string, empty), `--clear-arguments` (boolean, false), `--clear-description` (boolean, false), `--description TEXT` (string, empty), `--generate-arguments` (boolean, false), `--generate-description` (boolean, false), `--json` (boolean, false), `--no-metadata` (boolean, false), `--non-interactive` (boolean, false), `--template ARGV-TEMPLATE` (string, empty), `-h, --help`. Inherited: `--proxy URL` (default empty), `-v, --verbose` (count, default `0`; `-v` INFO, `-vv` DEBUG, `-vvv` TRACE), `-h, --help`. History must contain a usable last command.

### `zev alias list`

List all aliases; no positional arguments. **Local flags:** `--json` (boolean, false; schema-versioned JSON without prompts), `--non-interactive` (boolean, false), help. **Inherited:** `--proxy URL` (default empty), `-v, --verbose` (count, default `0`; `-v` INFO, `-vv` DEBUG, `-vvv` TRACE), `-h, --help`.

### `zev alias remove [name]` (`rm`)

Remove an alias by optional name; if omitted, interactive selection is used. **Aliases:** `rm`, `delete`. **Local flags:** `--json` (boolean, false), `--non-interactive` (boolean, false), `--yes` (boolean, false; accept named command-mode removal), help. **Inherited:** `--proxy URL` (default empty), `-v, --verbose` (count, default `0`; `-v` INFO, `-vv` DEBUG, `-vvv` TRACE), `-h, --help`.

## Shell completion: `zev completion`

The parent `zev completion` accepts no positional arguments. **Local flags:** `-h, --help`. **Inherited:** `--proxy URL` (default empty), `-v, --verbose` (count, default `0`; `-v` INFO, `-vv` DEBUG, `-vvv` TRACE), `-h, --help`. Each shell command accepts no additional positional arguments and has these **local flags:** `--no-descriptions` (boolean, default false; omit completion descriptions), `-h, --help`. **Inherited on each shell path:** `--proxy URL` (default empty), `-v, --verbose` (count, default `0`; `-v` INFO, `-vv` DEBUG, `-vvv` TRACE), `-h, --help`.

- `zev completion bash`: generates Bash completion; requires the `bash-completion` package.
- `zev completion fish`: generates Fish completion.
- `zev completion powershell`: generates PowerShell completion.
- `zev completion zsh`: generates Zsh completion.

Each generates script text; it does not install shell configuration automatically. For installation instructions run `zev completion SHELL --help`.

## `zev doctor`

Check local configuration and provider readiness. Takes no positional arguments. **Local flags:** `--json` (boolean, false; stable JSON report), `--print` (boolean, false; human-readable report without prompts), `--confirm` (boolean, false; send one real provider request without prompting), help. **Inherited:** `--proxy URL` (empty), `-v, --verbose` (count, 0). `--confirm` can make a network request.

## History: `zev history`

Root opens the history browser; no positional arguments. Local flags: help. Inherited: `--proxy URL` (default empty), `-v, --verbose` (count, default `0`; `-v` INFO, `-vv` DEBUG, `-vvv` TRACE), `-h, --help`.

### `zev history search [query]`

Search zero or one query argument; omitted query uses interactive search behavior. Search modes: lexical (default), semantic, hybrid.

**Local flags:** `--mode MODE` (`lexical|semantic|hybrid`, default `lexical`); `--limit N` (integer, default `20`); `--cursor TOKEN` (string, empty; continue a prior page); `--from TIME` (string, inclusive UTC date or RFC3339 time); `--to TIME` (string, exclusive UTC date or RFC3339 time); `--provider NAME`, `--model NAME`, `--status STATUS`, `--type TYPE` (strings, empty; filters); `--dangerous` (boolean, false); `--fuzzy MODE` (`auto|always|off`, default `auto`); `--exact-command TEXT` (string, empty); `--literal` (boolean, false); `--json` (boolean, false); help. **Inherited:** `--proxy URL` (default empty), `-v, --verbose` (count, default `0`; `-v` INFO, `-vv` DEBUG, `-vvv` TRACE), `-h, --help`. Cursor continuation must retain the same query, limit, and filters; result coverage indicates truncation.

### `zev history show [conversation-id]`

Show one conversation, optionally scoped to a message. Accepts at most one conversation ID. **Local flags:** `--message ID` (string, empty; show message by full ID), `--json` (boolean, false), help. **Inherited:** `--proxy URL` (default empty), `-v, --verbose` (count, default `0`; `-v` INFO, `-vv` DEBUG, `-vvv` TRACE), `-h, --help`.

### `zev history export`

Export complete history; no positional arguments. **Local flags:** `--format FORMAT` (`json|jsonl`, default `json`), help. **Inherited:** `--proxy URL` (default empty), `-v, --verbose` (count, default `0`; `-v` INFO, `-vv` DEBUG, `-vvv` TRACE), `-h, --help`.

### `zev history delete <id>`

Delete exactly one conversation by ID. **Local flags:** help only. **Inherited:** `--proxy URL` (default empty), `-v, --verbose` (count, default `0`; `-v` INFO, `-vv` DEBUG, `-vvv` TRACE), `-h, --help`. Deletion is logical and does not promise secure erasure.

### `zev history clear`

Delete all conversations; no positional arguments. **Local flags:** `--yes` (boolean, false; confirm deletion), help. **Inherited:** `--proxy URL` (default empty), `-v, --verbose` (count, default `0`; `-v` INFO, `-vv` DEBUG, `-vvv` TRACE), `-h, --help`.

### `zev history index`

Index maintenance group; no positional arguments. Local flags: help. Inherited: `--proxy URL` (default empty), `-v, --verbose` (count, default `0`; `-v` INFO, `-vv` DEBUG, `-vvv` TRACE), `-h, --help`.

#### `zev history index status`

Show index status; no positional arguments. **Local flags:** `--json` (boolean, false), help. **Inherited:** `--proxy URL` (default empty), `-v, --verbose` (count, default `0`; `-v` INFO, `-vv` DEBUG, `-vvv` TRACE), `-h, --help`.

#### `zev history index rebuild`

Rebuild history indexes; no positional arguments. **Local flags:** `--fts` (boolean, false; rebuild local lexical indexes), `--vectors` (boolean, false; rebuild semantic vectors), `--retry-failed` (boolean, false; retry failed vector messages), `--yes` (boolean, false; consent to vector transmission), help. **Inherited:** `--proxy URL` (default empty), `-v, --verbose` (count, default `0`; `-v` INFO, `-vv` DEBUG, `-vvv` TRACE), `-h, --help`. Exactly one of `--fts` or `--vectors` is required; vector rebuild requires explicit `--yes`. `--retry-failed` is applicable to vector rebuild.

## Provider profiles: `zev provider`

Manage named LLM profiles. The parent accepts no positional arguments. **Local flags:** `-h, --help`. **Inherited:** `--proxy URL` (default empty), `-v, --verbose` (count, default `0`; `-v` INFO, `-vv` DEBUG, `-vvv` TRACE), `-h, --help`.

- `zev provider list`: no positional arguments. **Local flags:** `-h, --help`. **Inherited:** `--proxy URL` (default empty), `-v, --verbose` (count, default `0`; `-v` INFO, `-vv` DEBUG, `-vvv` TRACE), `-h, --help`. Lists names/types and marks the active profile.
- `zev provider use NAME`: exactly one profile name. **Local flags:** `-h, --help`. **Inherited:** `--proxy URL` (default empty), `-v, --verbose` (count, default `0`; `-v` INFO, `-vv` DEBUG, `-vvv` TRACE), `-h, --help`.
- `zev provider add NAME`: exactly one profile name; starts interactive named-provider setup. **Local flags:** `-h, --help`. **Inherited:** `--proxy URL` (default empty; used for provider requests), `-v, --verbose` (count, default `0`; `-v` INFO, `-vv` DEBUG, `-vvv` TRACE), `-h, --help`.
- `zev provider remove NAME`: exactly one profile name. **Local flags:** `-h, --help`. **Inherited:** `--proxy URL` (default empty), `-v, --verbose` (count, default `0`; `-v` INFO, `-vv` DEBUG, `-vvv` TRACE), `-h, --help`. The active profile cannot be removed.

## `zev run -- command [args...]`

Run a literal argv and proxy standard streams. Everything after `--` is passed directly to the child process, without shell parsing; the separator is required. Zev makes **one attempt per invocation**; retry count/delay/exit-code retry controls are not part of this command. Failure recovery may ask, analyze, or suggest a fix; suggestions are never automatically executed. Captured output sent for provider analysis can contain secrets.

The run-specific options must appear before `--`. **Local options:**

| Option | Accepted values/default | Meaning |
|---|---|---|
| `--on-failure=POLICY` | `ask|none|analyze|fix`; default `ask` | Choose post-failure recovery behavior. `analyze`/`fix` send captured output to the configured provider. |
| `--timeout=DURATION` | Go duration `0s`–`24h`; default `0s` (no per-attempt timeout) | Bound the child run. |
| `--log-limit=BYTES` | integer `1024`–`16777216`; default `1048576` (1 MiB) | Maximum combined captured stdout/stderr. |
| `--session=ID` | 10-character key; default empty | Link run/recovery to a conversation. |
| `--proxy=URL` | non-empty URL; default empty | Proxy recovery/provider requests through HTTP(S) or SOCKS5 where supported. |
| `--no-interactive` | boolean; false | Disable interactive recovery prompts. |
| `-h, --help` | — | Print run-specific help. |

No short aliases are defined for these run options. The command's `--help` is handled by its own parser. The run option contract is literal-argv-specific and distinct from Cobra flags. (Older builds may show retry-related options; they are removed in the documented one-attempt behavior.)

## Scripts: `zev script`

Manage saved Bash scripts. Parent accepts no positional arguments; help plus inherited proxy and verbosity.

### `zev script create [name [description [arguments-json]]]`

Create a script; accepts 0–3 positional values. Without body input, it uses the configured editor. For noninteractive creation, provide `--body-file PATH` or `--body-stdin`.

**Local flags:** `--argument NAME[=DESCRIPTION]` (repeatable string array); `--arguments-json JSON` (string, empty; replace ordered args); `--body-file PATH` (string, empty); `--body-stdin` (boolean, false; read only from stdin); `--clear-arguments`, `--clear-description` (booleans, false); `--description TEXT` (string, empty); `--edit-body` (boolean, false; explicitly open editor, requires TTY); `--generate-arguments`, `--generate-description` (booleans, false; authorize AI generation); `--json` (boolean, false; schema-versioned output, no prompts); `--non-interactive` (boolean, false); help. Inherited: `--proxy URL` (default empty), `-v, --verbose` (count, default `0`; `-v` INFO, `-vv` DEBUG, `-vvv` TRACE), `-h, --help`. Body-file and body-stdin conflict; edit-body conflicts with noninteractive/body input.

### `zev script edit [name]`

Edit a script by optional name; accepts 0–1 positional arguments. **Local flags:** `--argument NAME[=DESCRIPTION]` (repeatable string array, default empty), `--arguments-json JSON` (string, empty), `--body-file PATH` (string, empty), `--body-stdin` (boolean, false), `--clear-arguments` (boolean, false), `--clear-description` (boolean, false), `--description TEXT` (string, empty), `--edit-body` (boolean, false), `--generate-arguments` (boolean, false), `--generate-description` (boolean, false), `--json` (boolean, false), `--non-interactive` (boolean, false), `-h, --help`. Inherited: `--proxy URL` (default empty), `-v, --verbose` (count, default `0`; `-v` INFO, `-vv` DEBUG, `-vvv` TRACE), `-h, --help`. Body and metadata constraints are the same as `script create`.

### `zev script list`

List scripts; no positional arguments. **Local flags:** `--json` (boolean, false), `--non-interactive` (boolean, false), help. **Inherited:** `--proxy URL` (default empty), `-v, --verbose` (count, default `0`; `-v` INFO, `-vv` DEBUG, `-vvv` TRACE), `-h, --help`.

### `zev script remove [name]` (`rm`)

Remove by optional script name; interactive choice if omitted. **Aliases:** `rm`, `delete`. **Local flags:** `--json` (boolean, false), `--non-interactive` (boolean, false), `--yes` (boolean, false; accept named removal), help. **Inherited:** `--proxy URL` (default empty), `-v, --verbose` (count, default `0`; `-v` INFO, `-vv` DEBUG, `-vvv` TRACE), `-h, --help`.

### `zev script show <name>`

Print one script body; exactly one name. Local flags: help only. Inherited: `--proxy URL` (default empty), `-v, --verbose` (count, default `0`; `-v` INFO, `-vv` DEBUG, `-vvv` TRACE), `-h, --help`.

## Setup: `zev setup`

Run interactive initial provider setup. Parent has no local flags beyond help and inherited proxy/verbosity.

- `zev setup headers`: interactively replace custom HTTP headers; no arguments or local flags other than help. Input is one `Header-Name:Header-Value` per line; blank line ends entry. Inherited: `--proxy URL` (default empty), `-v, --verbose` (count, default `0`; `-v` INFO, `-vv` DEBUG, `-vvv` TRACE), `-h, --help`.
- `zev setup mode [inject|execute]`: zero or one value; no value displays current mode, otherwise sets delivery mode. Accepted values `inject` or `execute`; default is `inject`. Local help only. Inherited: `--proxy URL` (default empty), `-v, --verbose` (count, default `0`; `-v` INFO, `-vv` DEBUG, `-vvv` TRACE), `-h, --help`.

## `zev update [stable|main]`

Update from the stable release (default) or latest `main` commit; runs required data migrations. Accepts zero or one positional branch, only `stable` or `main`.

Before CLI dispatch, Zev synchronously performs a best-effort startup update check with a 5-second deadline. Errors and timeouts do not block the requested command. Failed or timed-out checks are cached for 1 hour before retry; successful results are cached for 24 hours in `~/.zev/update-check.json`. The explicit update operation has a 1-minute timeout for network and installation work.

**Visible flags:** help. **Inherited:** `--proxy URL` (empty), `-v, --verbose` (count, 0).

**Hidden internal flag:** `--migrate-only` (boolean, false; hidden from ordinary help). Used only by update/migration orchestration to apply local migrations without downloading/installing another build; it is not a general update mode. It retains a separate 10-minute timeout for local migrations.

## `zev version`

Print the installed version; no positional arguments. Visible local flag: help. Inherited: `--proxy URL` (default empty), `-v, --verbose` (count, default `0`; `-v` INFO, `-vv` DEBUG, `-vvv` TRACE), `-h, --help`.

**Hidden internal probes:** `--update-protocol` (boolean, false; prints the update protocol marker) and `--update-build` (boolean, false; prints the build version only). These flags are consumed by the updater's compatibility/build-detection checks; they are not user-facing version options.

## Help and completion

Use `zev --help` for root help and `zev COMMAND --help` for any command path. Help on arbitrary aliases/scripts is handled by Zev's dispatcher; it shows the entity's usage instead of issuing an LLM query. `zev help` is removed; `--help`/`-h` are the supported help interface. See [Security](SECURITY.md), [Aliases and scripts](ALIASES-SCRIPTS.md), and [the `zev run` guide](RUN-WRAPPER.md) for the corresponding operational boundaries.

For shell completion, run `zev completion bash|fish|powershell|zsh`; use the generated command's help to find platform-specific installation instructions. Completion generation does not install shell configuration automatically.
