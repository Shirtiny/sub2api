# Quality Guidelines

> Code quality standards for backend development.

---

## Overview

Backend quality is enforced through Go tests, `golangci-lint`, layer boundaries, and a strong preference for real integration coverage around persistence-heavy code.

Core quality signals in this repo:

- Layering matters: handlers and most services do not import repositories directly.
- Use typed errors, `context.Context`, and constructors/interfaces consistently.
- Test unit, integration, and route behavior close to the layer being changed.
- Avoid changing migration history or generated ORM output by hand.

---

## Forbidden Patterns

- Do not import `internal/repository`, `gorm`, or Redis directly from handlers or most services; this is enforced in `backend/.golangci.yml`.
- Do not edit generated Ent files in `backend/ent/` manually.
- Do not modify previously applied migration files; add a new migration instead.
- Do not put significant business logic in route registration or `cmd/server` startup code.
- Do not skip error checks or hide failures behind blank assignments unless already explicitly allowed by lint config.

---

## Required Patterns

- Accept `context.Context` on service and repository methods. Example: `backend/internal/service/account_service.go` and `backend/internal/repository/account_repo.go`.
- Keep business logic behind service types and repository interfaces. Example: `AccountRepository` interface in `backend/internal/service/account_service.go`.
- Wrap lower-level errors with operation context using `%w`.
- Prefer focused constructors and provider functions for wiring. Example: `backend/internal/service/wire.go`.
- Keep tests next to the code they cover using `*_test.go`, `*_integration_test.go`, and build tags where needed.

---

## Testing Requirements

- Run `make test` for the normal backend check path; it executes `go test ./...` and `golangci-lint run ./...`.
- Use `make test-unit`, `make test-integration`, and `make test-e2e` when the change is scoped to a specific test tier.
- Repository code is expected to have real integration coverage with Postgres/Redis containers when behavior depends on DB semantics. Example: `backend/internal/repository/integration_harness_test.go` and `backend/internal/repository/account_repo_integration_test.go`.
- Handler and route behavior is also tested directly. Examples: `backend/internal/handler/openai_gateway_handler_test.go`, `backend/internal/server/routes/gateway_test.go`.

---

## Code Review Checklist

- Does the change keep the existing handler -> service -> repository boundary intact?
- Are errors wrapped and returned through shared response/error helpers?
- If persistence changed, was the right combination of Ent schema, migration, and integration tests updated?
- If logging changed, does it avoid sensitive data and use structured fields?
- Were the appropriate backend test commands run for the touched layer?

---

## Examples

- Lint and dependency boundary rules: `backend/.golangci.yml`
- Standard backend commands: `backend/Makefile`
- Service interface and context-first methods: `backend/internal/service/account_service.go`
- Repository integration harness with real containers: `backend/internal/repository/integration_harness_test.go`
- Route and handler tests: `backend/internal/server/routes/gateway_test.go`, `backend/internal/handler/openai_gateway_handler_test.go`
