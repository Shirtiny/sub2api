# Logging Guidelines

> How logging is done in this project.

---

## Overview

The project uses a custom logging package in `backend/internal/pkg/logger/` built on Zap, and bridges both `slog` and the standard library logger into the same pipeline.

Observed usage patterns:

- Use structured logging with fields, not string concatenation, when context matters.
- Use the shared logger package rather than creating ad-hoc loggers.
- Include component names and important IDs in log fields.
- Keep sensitive values redacted or truncated.

---

## Log Levels

- `debug`: noisy diagnostics for request/flow inspection. Example: 2FA request/session debugging in `backend/internal/handler/auth_handler.go`.
- `info`: normal operational milestones and startup status. Example: startup messages in `backend/cmd/server/main.go`.
- `warn`: degraded but non-fatal behavior. Example pattern: startup fallbacks and service init warnings in `backend/internal/service/wire.go`.
- `error`: failed operations, internal faults, and request-processing failures. Example: token pair generation failure in `backend/internal/handler/auth_handler.go`.
- `fatal`: startup or shutdown failures that should stop the process. Example: config and server startup failures in `backend/cmd/server/main.go`.

---

## Structured Logging

- Prefer field-based logs with `slog` or Zap-backed helpers.
- Use request-aware loggers when available. Example: `requestLogger(...)` in `backend/internal/handler/logging.go` pulls logger context from the request.
- Use the shared logger package for global config, sinks, and slog/stdlog bridging. Example: `backend/internal/pkg/logger/logger.go`.
- Legacy format logging still exists via `logger.LegacyPrintf(...)`; use it only when the surrounding code already follows that pattern.

---

## What to Log

- Startup and shutdown milestones: `backend/cmd/server/main.go`.
- Internal failures with enough context to debug, especially component and entity IDs.
- Background-worker or outbox failures that otherwise disappear. Example: scheduler outbox enqueue warnings in `backend/internal/repository/account_repo.go`.
- Request flow diagnostics only when needed, typically at debug level. Example: 2FA debug logging in `backend/internal/handler/auth_handler.go`.

---

## What NOT to Log

- Do not log full tokens, secrets, refresh tokens, passwords, or raw credential blobs.
- Prefer lengths, prefixes, or IDs over full sensitive values. Example: `temp_token_len` and short prefix logging in `backend/internal/handler/auth_handler.go`.
- Do not invent separate logger stacks inside features; go through `internal/pkg/logger`.
- Do not rely on bare `fmt.Println` style debugging in committed code.

---

## Examples

- Logger setup and slog/stdlog bridge: `backend/internal/pkg/logger/logger.go`
- Request-scoped logger helper: `backend/internal/handler/logging.go`
- Startup and process logs: `backend/cmd/server/main.go`
- Structured debug/error usage in a handler: `backend/internal/handler/auth_handler.go`
- Legacy component logging in repository code: `backend/internal/repository/account_repo.go`
