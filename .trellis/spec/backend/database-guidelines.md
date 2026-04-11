# Database Guidelines

> Database patterns and conventions for this project.

---

## Overview

This backend uses Ent as the primary ORM, with generated schema under `backend/ent/schema/` and generated code under `backend/ent/`.

Key conventions observed in the codebase:

- Use Ent builders for routine CRUD and entity loading.
- Use raw SQL only when Ent becomes awkward for batch updates, aggregation, or migration execution.
- Keep persistence logic inside repositories.
- Keep SQL migrations immutable and ordered in `backend/migrations/`.
- Wrap DB errors into service-level or application-level errors before they leave the repository/service boundary.

---

## Query Patterns

- Repositories own queries. Services should call repository interfaces, not Ent directly, unless the service is explicitly responsible for a transaction with multiple repositories/entities.
- Use Ent query/builders for common reads and writes. Example: `backend/internal/repository/account_repo.go` uses `r.client.Account.Create()` and `Query().Where(...).Only(ctx)`.
- Use raw SQL for complex bulk operations or reporting paths. Examples: `backend/internal/repository/account_repo.go`, `backend/internal/repository/channel_repo.go`, `backend/internal/repository/dashboard_aggregation_repo.go`.
- Pass `context.Context` through every DB call.
- Translate persistence failures instead of leaking raw driver errors. Example: `translatePersistenceError(...)` in `backend/internal/repository/account_repo.go`.

---

## Transactions

- Use `client.Tx(ctx)` for Ent-backed transactional flows. Examples: `backend/internal/repository/account_repo.go`, `backend/internal/repository/user_repo.go`, `backend/internal/service/redeem_service.go`.
- Use `db.BeginTx(ctx, nil)` for raw SQL transactions. Examples: `backend/internal/repository/channel_repo.go`, `backend/internal/repository/usage_billing_repo.go`, `backend/internal/repository/migrations_runner.go`.
- Roll back on every failure path and wrap errors with operation context such as `begin transaction`, `commit transaction`, or the business step name.
- Reserve transactions for multi-step writes that must remain atomic; the codebase does not start transactions for ordinary single-entity reads.

---

## Migrations

- Migration files live in `backend/migrations/` and are executed in lexical order by `backend/internal/repository/migrations_runner.go`.
- Treat applied migrations as immutable. The runner checksum-checks files and fails startup when an applied migration changes.
- Use numbered snake_case SQL files such as `001_init.sql` and suffix `_notx.sql` only for migrations that must run outside a transaction, such as concurrent index operations.
- Keep schema source in Ent definitions, but operational schema changes still land as SQL migrations. Ent generation is configured in `backend/ent/generate.go`.
- Repeated numeric prefixes already exist in this repo; keep new file names unique and lexically ordered rather than assuming a strict one-file-per-number rule.

---

## Naming Conventions

- Database tables and columns are snake_case. Example: `users`, `password_hash`, `totp_enabled` in `backend/ent/schema/user.go`.
- Ent schema types are PascalCase, while table names are annotated explicitly where needed. Example: `User` with `entsql.Annotation{Table: "users"}` in `backend/ent/schema/user.go`.
- Use plural table names in SQL migrations and schema annotations.
- Keep migration file names descriptive: `061_add_usage_log_request_type.sql`, `076_add_usage_log_upstream_model_index_notx.sql`.

---

## Common Mistakes

- Do not modify an already-applied migration file; add a new migration instead.
- Do not put direct repository imports into handlers or most services; the layering rules in `backend/.golangci.yml` enforce this.
- Do not write ad-hoc SQL in handlers.
- Do not skip error translation or contextual wrapping around DB failures.

---

## Examples

- Ent schema source: `backend/ent/schema/user.go`
- Ent generation config: `backend/ent/generate.go`
- Repository with Ent plus raw SQL: `backend/internal/repository/account_repo.go`
- SQL transaction helper pattern: `backend/internal/repository/channel_repo.go`
- Migration runner and checksum enforcement: `backend/internal/repository/migrations_runner.go`
- Integration harness applying real migrations: `backend/internal/repository/integration_harness_test.go`
