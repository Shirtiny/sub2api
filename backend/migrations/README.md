# Database Migrations

## Overview

This directory contains SQL migration files for database schema changes. The migration system uses SHA256 checksums to ensure migration immutability and consistency across environments.

For detailed checksum mismatch incident handling, see [`../../docs/MIGRATION_CHECKSUM_PLAYBOOK.md`](../../docs/MIGRATION_CHECKSUM_PLAYBOOK.md).

## Migration File Naming

Format: `NNN_description.sql`
- `NNN`: Sequential number (e.g., 001, 002, 003)
- `description`: Brief description in snake_case

Example: `017_add_gemini_tier_id.sql`

### `_notx.sql` 命名与执行语义（并发索引专用）

当迁移包含 `CREATE INDEX CONCURRENTLY` 或 `DROP INDEX CONCURRENTLY` 时，必须使用 `_notx.sql` 后缀，例如：

- `062_add_accounts_priority_indexes_notx.sql`
- `063_drop_legacy_indexes_notx.sql`

运行规则：

1. `*.sql`（不带 `_notx`）按事务执行。
2. `*_notx.sql` 按非事务执行，不会包裹在 `BEGIN/COMMIT` 中。
3. `*_notx.sql` 仅允许并发索引语句，不允许混入事务控制语句或其他 DDL/DML。

幂等要求（必须）：

- 创建索引：`CREATE INDEX CONCURRENTLY IF NOT EXISTS ...`
- 删除索引：`DROP INDEX CONCURRENTLY IF EXISTS ...`

这样可以保证灾备重放、重复执行时不会因对象已存在/不存在而失败。

## Migration File Structure

This project uses a custom migration runner (`internal/repository/migrations_runner.go`) that executes the full SQL file content as-is.

- Regular migrations (`*.sql`): executed in a transaction.
- Non-transactional migrations (`*_notx.sql`): split by statement and executed without transaction (for `CONCURRENTLY`).

```sql
-- Forward-only migration (recommended)
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS example_column VARCHAR(100);
```

> ⚠️ Do **not** place executable "Down" SQL in the same file. The runner does not parse goose Up/Down sections and will execute all SQL statements in the file.

## Important Rules

### ⚠️ Immutability Principle

**Once a migration is applied to ANY environment (dev, staging, production), it MUST NOT be modified.**

Treat a migration as applied once it has been merged into a branch that can be deployed, included in a published image, or run in any shared environment. Do not amend that migration in a follow-up commit. If the desired behavior changes, create a new migration with the next lexical filename.

Why?
- Each migration has a SHA256 checksum stored in the `schema_migrations` table
- Modifying an applied migration causes checksum mismatch errors
- Different environments would have inconsistent database states
- Breaks audit trail and reproducibility
- Can prevent the application from starting because migrations run during service startup

### Checksum Compatibility Is Emergency-Only

For the full incident playbook and command examples, see [`../../docs/MIGRATION_CHECKSUM_PLAYBOOK.md`](../../docs/MIGRATION_CHECKSUM_PLAYBOOK.md).

The migration runner keeps checksum validation enabled. `migrationChecksumCompatibilityRules` is not a global bypass.

Use a compatibility rule only after an incident where an already-applied migration was accidentally changed and real environments now have known, conflicting `schema_migrations.checksum` values. A compatibility rule must:

- Match one exact migration filename.
- Match the exact current file checksum.
- Accept only exact, known historical database checksums.
- Include unit tests for accepted and rejected checksum combinations.
- Be paired with a new migration for any actual schema/data behavior change.

Do not add a compatibility rule for normal feature work, review feedback, or convenience. The normal fix for changing database behavior is always a new migration.

### ✅ Correct Workflow

1. **Create new migration**
   ```bash
   # Create new file with next sequential number
   touch migrations/018_your_change.sql
   ```

2. **Write forward-only migration SQL**
   - Put only the intended schema change in the file
   - If rollback is needed, create a new migration file to revert

3. **Test locally**
   ```bash
   # Apply migration
   make migrate-up

   # Test rollback
   make migrate-down
   ```

4. **Commit and deploy**
   ```bash
   git add migrations/018_your_change.sql
   git commit -m "feat(db): add your change"
   ```

### ❌ What NOT to Do

- ❌ Modify an already-applied migration file
- ❌ Delete migration files
- ❌ Change migration file names
- ❌ Reorder migration numbers
- ❌ "Fix" a deployed migration by editing `schema_migrations` during normal development
- ❌ Add broad checksum bypasses or wildcard compatibility rules

### 🔧 If You Accidentally Modified an Applied Migration

**Error message:**
```
migration 017_add_gemini_tier_id.sql checksum mismatch (db=abc123... file=def456...)
```

**Solution:**
```bash
# 1. Find the original version
git log --oneline -- migrations/017_add_gemini_tier_id.sql

# 2. Revert to the commit when it was first applied
git checkout <commit-hash> -- migrations/017_add_gemini_tier_id.sql

# 3. Create a NEW migration for your changes
touch migrations/018_your_new_change.sql
```

If production is already down, restore service first with the narrowest possible incident-response action, then add a filename/checksum-specific compatibility rule with tests. Do not leave the system relying on manual `schema_migrations` edits as the long-term fix. Follow the detailed decision tree in [`../../docs/MIGRATION_CHECKSUM_PLAYBOOK.md`](../../docs/MIGRATION_CHECKSUM_PLAYBOOK.md).

## Migration System Details

- **Checksum Algorithm**: SHA256 of the migration content after surrounding whitespace is trimmed by the runner; line endings and BOM changes inside that content still affect the checksum
- **Tracking Table**: `schema_migrations` (filename, checksum, applied_at)
- **Runner**: `internal/repository/migrations_runner.go`
- **Auto-run**: Migrations run automatically on service startup

## Best Practices

1. **Keep migrations small and focused**
   - One logical change per migration
   - Easier to review and rollback

2. **Write forward-only migrations**
   - Do not put executable Down SQL in the same file
   - If rollback is needed, create a later corrective migration

3. **Use transactions**
   - Wrap DDL statements in transactions when possible
   - Ensures atomicity

4. **Add comments**
   - Explain WHY the change is needed
   - Document any special considerations

5. **Test in development first**
   - Apply migration locally
   - Verify data integrity
   - Test rollback

## Example Migration

```sql
-- Add tier_id field to Gemini OAuth accounts for quota tracking
UPDATE accounts
SET credentials = jsonb_set(
    credentials,
    '{tier_id}',
    '"LEGACY"',
    true
)
WHERE platform = 'gemini'
  AND type = 'oauth'
  AND credentials->>'tier_id' IS NULL;
```

## Troubleshooting

### Checksum Mismatch
See "If You Accidentally Modified an Applied Migration" above and the full playbook in [`../../docs/MIGRATION_CHECKSUM_PLAYBOOK.md`](../../docs/MIGRATION_CHECKSUM_PLAYBOOK.md).

### Migration Failed
```bash
# Check migration status
psql -d sub2api -c "SELECT * FROM schema_migrations ORDER BY applied_at DESC;"

# Manually rollback if needed (use with caution)
# Better to fix the migration and create a new one
```

### Need to Skip a Migration (Emergency Only)
```sql
-- DANGEROUS: Only use in development or with extreme caution
INSERT INTO schema_migrations (filename, checksum, applied_at)
VALUES ('NNN_migration.sql', 'calculated_checksum', NOW());
```

Manual changes to `schema_migrations` in production require explicit incident approval, a precise `WHERE` clause when updating, and a follow-up code fix. They are not an acceptable substitute for immutable migrations.

## References

- Migration runner: `internal/repository/migrations_runner.go`
- PostgreSQL docs: https://www.postgresql.org/docs/
