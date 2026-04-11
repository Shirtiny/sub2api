# Error Handling

> How errors are handled in this project.

---

## Overview

The backend standardizes HTTP errors through `internal/pkg/errors` and `internal/pkg/response`.

Observed pattern:

- Build or return typed application errors for expected failures.
- Wrap lower-level errors with operation context using `%w`.
- In handlers, convert service errors with `response.ErrorFrom(c, err)`.
- Use direct `response.BadRequest`, `response.Forbidden`, or `response.InternalError` only for simple handler-local failures.
- Log internal failures with details, but avoid exposing raw internals to clients.

---

## Error Types

- `internal/pkg/errors.ApplicationError` is the main typed error for controlling HTTP responses.
- The status payload includes `code`, `reason`, `message`, and optional `metadata`.
- Services define reusable domain errors with helpers such as `infraerrors.NotFound(...)` and `infraerrors.BadRequest(...)`. Example: `ErrAccountNotFound` and `ErrAccountNilInput` in `backend/internal/service/account_service.go`.
- Unknown errors fall back to a generic internal error via `FromError(...)` / `ToHTTP(...)` behavior in `internal/pkg/errors`.

---

## Error Handling Patterns

- Validate and bind input at the handler boundary, then return early. Example: `backend/internal/handler/auth_handler.go` uses `c.ShouldBindJSON(&req)` followed by `response.BadRequest(...)`.
- After calling a service, handlers typically do:
  - `if err != nil { response.ErrorFrom(c, err); return }`
- Wrap errors with context at service/repository boundaries. Examples: `fmt.Errorf("create account: %w", err)` and `fmt.Errorf("list accounts: %w", err)` in `backend/internal/service/account_service.go`.
- Transaction flows use explicit `begin`, `apply`, `record`, and `commit` error wrapping. Example: `backend/internal/repository/migrations_runner.go`.
- For partial fallback behavior, log and degrade intentionally rather than panic. Example: token-pair generation fallback in `backend/internal/handler/auth_handler.go`.

---

## API Error Responses

- Successful API responses use the envelope in `backend/internal/pkg/response/response.go` with `code: 0`, `message: "success"`, and `data`.
- Error responses keep the same envelope shape and may include `reason` and `metadata`.
- Pagination also uses the same success envelope with `items`, `total`, `page`, `page_size`, and `pages` inside `data`.
- Handlers should not invent one-off error response formats.

---

## Common Mistakes

- Do not leak raw DB or upstream error strings directly to clients when a typed application error exists.
- Do not skip `response.ErrorFrom`; that loses the shared envelope and reason/metadata fields.
- Do not ignore the original cause when wrapping errors; use `%w`.
- Do not panic for request-level failures that can be returned as structured HTTP errors.

---

## Examples

- Typed error definition and conversion: `backend/internal/pkg/errors/errors.go`
- Shared HTTP response helpers: `backend/internal/pkg/response/response.go`
- Handler-side early returns and `ErrorFrom`: `backend/internal/handler/auth_handler.go`
- Service-side contextual wrapping: `backend/internal/service/account_service.go`
- Transaction error context and rollback paths: `backend/internal/repository/migrations_runner.go`
