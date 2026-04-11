# Directory Structure

> How backend code is organized in this project.

---

## Overview

The backend is a Go application with a clear layered layout:

- `backend/cmd/server/` is the executable entrypoint and Wire bootstrap.
- `backend/internal/server/` owns Gin router setup and HTTP middleware.
- `backend/internal/handler/` translates HTTP requests into service calls and response envelopes.
- `backend/internal/service/` contains business logic and service interfaces.
- `backend/internal/repository/` owns database, Redis, and upstream persistence/integration code.
- `backend/internal/pkg/` contains reusable infrastructure packages such as logging, responses, and API compatibility helpers.
- `backend/ent/schema/`, generated `backend/ent/`, and `backend/migrations/` define the database contract.

The codebase generally follows `route -> handler -> service -> repository`.

---

## Directory Layout

```text
backend/
├── cmd/
│   ├── server/                # main binary, startup, Wire generation
│   └── jwtgen/                # small utility binary
├── ent/
│   ├── schema/                # Ent schema source
│   └── ...generated files     # generated ORM code
├── internal/
│   ├── config/                # config loading and wiring
│   ├── domain/                # shared domain constants
│   ├── handler/               # HTTP handlers and DTO mapping
│   ├── integration/           # end-to-end/integration test flows
│   ├── middleware/            # app middleware outside HTTP server package
│   ├── model/                 # request/response or domain-adjacent models
│   ├── pkg/                   # reusable infra packages
│   ├── repository/            # DB/Redis/upstream integrations
│   ├── server/                # router, route registration, HTTP middleware
│   ├── service/               # business services and interfaces
│   ├── setup/                 # first-run setup flow
│   ├── testutil/              # test helpers
│   ├── util/                  # focused utilities
│   └── web/                   # embedded frontend serving
└── migrations/                # ordered SQL migrations
```

---

## Module Organization

- Put HTTP-only concerns in handlers and route registration. Examples: `backend/internal/server/router.go`, `backend/internal/server/routes/admin.go`, `backend/internal/handler/auth_handler.go`.
- Put business rules and orchestration in services. Services usually depend on interfaces, not concrete repositories. Examples: `backend/internal/service/account_service.go`, `backend/internal/service/auth_service.go`, `backend/internal/service/wire.go`.
- Put persistence and low-level integration code in repositories. Repositories use Ent for normal CRUD and raw SQL when needed. Examples: `backend/internal/repository/account_repo.go`, `backend/internal/repository/user_repo.go`, `backend/internal/repository/usage_log_repo.go`.
- Keep transport shaping near handlers via DTOs when backend models differ from API payloads. Example: `backend/internal/handler/dto/`.
- Keep shared infrastructure under `internal/pkg/` rather than sprinkling helper code across handlers and services. Examples: `backend/internal/pkg/logger/logger.go`, `backend/internal/pkg/response/response.go`.

---

## Naming Conventions

- Use snake_case file names by concern: `*_handler.go`, `*_service.go`, `*_repo.go`, `*_test.go`.
- Group route registration by area under `backend/internal/server/routes/`, for example `admin.go`, `auth.go`, `gateway.go`, `user.go`.
- Keep service and repository constructors as `NewXxxService` / `NewXxxRepository`.
- Keep interfaces close to the consumer layer when they define a boundary. Example: `AccountRepository` is defined in `backend/internal/service/account_service.go`.
- Use build-tagged test names to signal scope, for example `*_integration_test.go` and `*_e2e` style tests under `backend/internal/integration/`.

---

## Examples

- Route setup and middleware chain: `backend/internal/server/router.go`
- Handler to service boundary: `backend/internal/handler/auth_handler.go`
- Service boundary and repository interface: `backend/internal/service/account_service.go`
- Repository implementation with Ent plus SQL: `backend/internal/repository/account_repo.go`
- Generated schema source location: `backend/ent/schema/user.go`

---

## Anti-Patterns

- Do not put database access directly in handlers; use services and repositories instead.
- Do not add new business logic to `cmd/server/`; startup code belongs there, not feature code.
- Do not edit generated files in `backend/ent/` by hand; change `backend/ent/schema/` or migrations instead.
- Do not bypass the route grouping pattern by registering large unrelated endpoints in one file.
