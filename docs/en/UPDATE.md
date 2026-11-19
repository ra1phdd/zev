<div align="center">

**English** | [Русский](../ru/UPDATE.md)

</div>

# Updating Zev

```bash
zev update [stable|main]
```

The default `stable` channel checks the latest **stable GitHub Release** in
`ra1phdd/zev`, downloads the archive for its operating system and architecture,
verifies its SHA-256 against that release's `checksums.txt`, and replaces the
running executable. `zev update main` downloads the current `main` commit's source
and builds it locally with the installed Go toolchain before replacement. Neither
channel runs an installation shell script or uses LLM credentials, provider
endpoints, or custom provider HTTP headers. Versioned stable builds never
automatically downgrade.

For ordinary interactive/query commands, Zev performs a synchronous best-effort
startup update check with a 5-second deadline. It is skipped for help, `version`,
machine-output (`--print`/`--json`) and provider/alias/script list commands. Errors
and timeouts do not block the requested command. Unversioned development builds
such as `go run ./cmd/zev` skip the startup check because the release API cannot
meaningfully order them against a stable release. A failed or timed-out check is
cached for 1 hour; successful results are cached for 24 hours in
`~/.zev/update-check.json`. The check does not create `~/.zev` or its cache lock
when that directory is absent. A cached or successful result can print an update
notice; run `zev update` to install the latest stable release, or `zev update main`
to build the latest development commit. `--proxy URL` applies to the check.

An explicit `zev update [stable|main]` operation has a 1-minute timeout for its
network and installation work. The hidden `zev update --migrate-only` operation
retains a separate 10-minute timeout for local migrations.

The `main` branch is under active development and may contain regressions or
errors. Compatibility between `main` and a stable release is not guaranteed;
switching back to `stable` may require manual file repairs or reinstalling Zev.

The verified candidate must support Zev's self-update protocol before it can
replace the executable. Releases predating this feature are rejected, rather
than accidentally invoking an ordinary query in place of the migration command.
An older Zev without `update` must first be upgraded using the installation
instructions in the [README](../../README.md).

## Data migration boundary

After installation, the **new executable** immediately runs the registered local
migration orchestration before opening a store:

- configuration: the supported v1.0.0 dotenv file `~/.zev/config` to current versioned
  YAML directly;
- aliases and scripts: v1.0.0 unversioned metadata to current v2 metadata;
- history: initialize current SQLite from one embedded SQL baseline (`.up.sql`),
  then import the v1.0.0 JSONL file separately.

The current readers in `internal/core` and `internal/entities` accept current formats
only. They do not detect legacy data, interpret development schemas, or run migrations
while reading or writing. Orchestration must complete the required migration before the
store is used. The JSONL importer is not a SQLite schema step, and the SQL baseline does
not replay a history of development schemas.

If a future released format changes a store, its release-to-release transition is
retained and chained for users who skip releases. Development snapshots are not
backwards-compatible migration inputs. Do not infer a supported source or transition
from an unreleased schema.

No migration generates an archive, archive sidecar, or backup copy. Existing user-owned
backups, exports, and other historical copies are outside the migration protocol: Zev
does not sweep or promise to delete them, and migration is not secure erasure.

Migrations do not generate AI metadata, call an LLM, rebuild embeddings, execute saved
scripts, or change provider configuration. Explicit update migrations run even when
query history is disabled. A missing `~/.zev` directory is left absent. Independent
migration steps are attempted even when another step fails; cancellation stops further
work. These migrations are individually recoverable, not one atomic transaction across
every store.

Recovery is limited to a current-protocol journal or importer ledger whose source and
destination state still match. Zev does not interpret or recover markerless or
unpublished archives/journals from development snapshots, and it does not sweep unknown
paths. A changed, malformed, or conflicting record fails closed without guessing.
The shared filesystem primitives in `internal/migrations/filetxn` cover staging,
publication, durability classification, and recovery for filesystem migrations only.
They are not SQLite transaction/rollback machinery and do not provide rollback for
script CRUD operations.

If the installed version is already current or newer, Zev still runs its migration
steps. This allows a second `zev update` to retry a previous migration failure.
For a network-free retry using the installed executable:

```bash
zev update --migrate-only
```

This maintenance flag is intentionally hidden from the ordinary help listing.

## Failure and installation boundaries

Back up `~/.zev` before upgrading. Download, checksum, archive, compatibility and
pre-installation failures leave the previous executable in place. The updater
serializes cooperating update commands and stages the replacement beside the
installed executable, rather than truncating the running file. Symlinked launch
paths resolve to the actual installed target.

Automatic executable replacement is supported on Linux and macOS. On Windows,
`zev update` refuses replacement; install the verified release manually and run
`zev update --migrate-only` afterward.

The process needs write access to the executable's directory. Zev does not invoke
`sudo` or request elevated privileges automatically. For a protected or
package-manager-owned installation, use its installation/upgrade mechanism instead.
An alias or script named `update` does not override this built-in command; rename
it if you previously used `zev update` to invoke your own entity.

If replacement succeeded but a migration failed, the command exits unsuccessfully
and **keeps the new executable**. It never rolls back the binary automatically:
some stores may already have migrated, and an older binary may no longer read
them. Fix the reported local problem and retry migration. A crash between
replacement and migration likewise requires a retry; the synchronous startup check
has a five-second deadline, caches failures for one hour and never replaces the
executable or runs migrations.

Checksums from the same GitHub Release verify integrity, not an independent
publisher signature. GitHub/network failures and API rate limits are reported as
errors, not as successful "already current" results. Native runtime verification
on each target OS remains separate from cross-compilation.
