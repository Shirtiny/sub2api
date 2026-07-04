# Migration Checksum Incident Playbook

This playbook documents how to handle `migration ... checksum mismatch` safely.

The short version: **do not edit an already-applied migration**. Restore the original file when possible, and put follow-up schema/data changes in a new migration. Use checksum compatibility rules only as a narrowly scoped incident-recovery tool.

## Why this matters

`backend/internal/repository/migrations_runner.go` records every applied SQL file in `schema_migrations`:

- `filename`: migration file name
- `checksum`: SHA256 of the exact migration file content used by the runner
- `applied_at`: application time

On startup, the runner recalculates the current file checksum and compares it with `schema_migrations.checksum`. A mismatch stops startup because the database may no longer match the code's migration history.

Important details:

- Line-ending changes (`CRLF` vs `LF`) change the checksum.
- Adding/removing a UTF-8 BOM changes the checksum and may also break the first SQL statement.
- Formatting-only edits, comment edits, and reordered statements still change the checksum.
- Migrations run automatically during service startup, so a checksum incident can become an availability issue.

## Prevention checklist

Before editing any existing migration file, ask:

1. Has this migration run in my local database?
2. Has this branch been pushed, merged, deployed, or used by another developer/agent?
3. Has the migration been included in a Docker image or release artifact?

If the answer to any question is "yes" or "not sure", **do not edit the existing migration**. Create a new migration with a later lexical filename instead.

For review feedback on a migration that may already be applied:

- Keep the original migration file unchanged.
- Add a follow-up migration, for example `158_fix_previous_change.sql`.
- Make the follow-up idempotent when possible (`IF EXISTS`, `IF NOT EXISTS`, guarded `UPDATE`, etc.).

## Triage: what kind of incident is this?

When startup fails with:

```text
migration NNN_name.sql checksum mismatch (db=<dbChecksum> file=<fileChecksum>)
```

classify the situation first.

### Case A: local-only development database

Use this only when the migration has not been shared, pushed, or applied outside your own disposable DB.

Preferred options:

1. Restore the migration file to the version recorded in your DB; or
2. Recreate/reset the local development database; or
3. If you intentionally want the new file content, delete/recreate only the local DB state.

Do **not** add a compatibility rule just for a disposable local DB.

### Case B: original migration can be restored

This is the normal production-safe fix.

1. Find the historical version:

   ```powershell
   git log --oneline -- backend/migrations/NNN_name.sql
   ```

2. Restore the file version that matches applied environments:

   ```powershell
   git checkout <commit> -- backend/migrations/NNN_name.sql
   ```

3. Put any new schema/data changes into a new migration:

   ```powershell
   New-Item backend/migrations/NNN_followup_change.sql -ItemType File
   ```

4. Run tests and startup validation.

### Case C: real environments already have conflicting checksums

Use a compatibility rule only when all of the following are true:

- At least one real environment has already recorded a known old checksum.
- The repository now needs to boot with a known current file checksum.
- Restoring the old file alone would not safely recover all affected environments.
- Any behavior/schema delta is moved into a new follow-up migration.

Required remediation:

1. Capture the exact error message and checksums.
2. Compute the current file checksum from the embedded file content.
3. Add a filename-specific rule in `migrationChecksumCompatibilityRules`.
4. Add unit tests covering accepted and rejected combinations.
5. Add a new migration for any schema/data/index changes that were introduced after the migration was first applied.
6. Verify startup against an affected database.

Do not use wildcard matching, prefix matching, or a global bypass.

### Case D: production is down and no code deploy is immediately possible

Manual edits to `schema_migrations` are a last resort. They require explicit incident approval, a precise statement, and a follow-up code fix. Prefer restoring/deploying code first.

If manual intervention is approved:

- Use a transaction.
- Scope by exact filename and exact current checksum.
- Record the before/after values in the incident notes.
- Deploy a code-level compatibility rule or restored migration immediately after.

Example shape only; do not copy without confirming values:

```sql
BEGIN;
UPDATE schema_migrations
SET checksum = '<known-safe-checksum>'
WHERE filename = 'NNN_name.sql'
  AND checksum = '<exact-old-checksum>';
COMMIT;
```

## Computing checksums

Use the same principle as the runner: SHA256 over the exact file bytes/content.

PowerShell:

```powershell
Get-FileHash backend/migrations/NNN_name.sql -Algorithm SHA256
```

Python, useful when checking line-ending variants:

```powershell
python -c "from pathlib import Path; import hashlib; b=Path('backend/migrations/NNN_name.sql').read_bytes(); s=b.decode('utf-8-sig'); print('as-is', hashlib.sha256(b).hexdigest()); print('LF', hashlib.sha256(s.replace('\r\n','\n').encode()).hexdigest()); print('CRLF', hashlib.sha256(s.replace('\r\n','\n').replace('\n','\r\n').encode()).hexdigest())"
```

Check the DB value:

```sql
SELECT filename, checksum, applied_at
FROM schema_migrations
WHERE filename = 'NNN_name.sql';
```

## Adding a compatibility rule

Rules live in `backend/internal/repository/migrations_runner.go`:

```go
"NNN_name.sql": newMigrationChecksumCompatibilityRule(
    "<current-file-checksum>",
    "<known-db-checksum-1>",
    "<known-db-checksum-2>",
),
```

Guidelines:

- The key must be one exact filename.
- The first checksum should be the current repository file checksum.
- Variadic checksums should be exact DB checksums observed in real environments.
- Include line-ending variants only when they are actually needed and documented by tests.
- Keep the rule narrow; never accept arbitrary checksums.
- If the migration file was modified to add schema/index changes, move those changes to a new migration as well.

## Required tests

Add tests in `backend/internal/repository/migrations_runner_checksum_test.go`.

Minimum coverage:

- Known DB checksum + current file checksum returns `true`.
- Known DB checksum + LF/CRLF variant returns `true` only if that variant is intentionally accepted.
- Unknown DB/file checksum returns `false`.

When possible, read the actual embedded migration file in the test and compute its checksum. This prevents tests from going stale when line endings change.

Run:

```powershell
cd backend
$env:GOCACHE = Join-Path $env:TEMP 'sub2api-codex-gocache'
Remove-Item Env:\GOTOOLCHAIN -ErrorAction SilentlyContinue
go test ./internal/repository -run TestIsMigrationChecksumCompatible
go test ./migrations
go test ./internal/repository
```

Then start the service against the affected DB from the repository root:

```powershell
go -C ./backend run ./cmd/server
```

## Follow-up migration pattern

If an applied migration was accidentally edited to add indexes, constraints, columns, or data backfills, do not rely on the edited old migration. Add a follow-up migration:

```sql
-- Keep this separate because NNN_name.sql was already applied in some environments.
ALTER TABLE example_table
    ADD COLUMN IF NOT EXISTS new_column TEXT;

CREATE INDEX IF NOT EXISTS idx_example_table_new_column
    ON example_table(new_column);
```

For `CONCURRENTLY`, use a separate `*_notx.sql` migration and follow the `_notx.sql` rules in `backend/migrations/README.md`.

## Review checklist for PRs touching migrations

- [ ] No already-applied migration was edited unless this is a documented incident fix.
- [ ] New behavior is in a new migration file.
- [ ] Migration is idempotent where practical.
- [ ] Checksum compatibility rule, if any, is filename-specific and checksum-specific.
- [ ] Compatibility tests include accepted and rejected cases.
- [ ] No normal-development manual edit to `schema_migrations` is required.
- [ ] Startup was tested against a DB with the affected historical checksum when applicable.

## Example: pre-deployment consolidation

If a migration series has not been applied in the target environment yet, prefer consolidating review fixes into the first unapplied migration instead of shipping repair-only follow-ups.

For example, when production has applied migrations only through `156_*.sql`, review fixes for the custom subscription rollout should be folded into `157_custom_subscription_multiplier.sql`, `161_user_subscription_virtual_custom_entitlement.sql`, or `163_retire_orphan_legacy_custom_subscription_groups.sql` as appropriate. Do not add checksum compatibility rules for migrations that have not been recorded in any shared database.
