# Quality Guidelines

> Code quality standards for frontend development.

---

## Overview

Frontend quality is driven by ESLint, `vue-tsc`, Vitest, and shared infrastructure around API access, routing, accessibility, and async state.

Core expectations observed in this repo:

- Use the shared axios client and typed API modules.
- Use strict TypeScript and typed Vue SFC patterns.
- Add or update tests close to the changed behavior.
- Prefer reusable composables/components over copying async or dialog logic.

---

## Forbidden Patterns

- Do not call raw `fetch` or ad-hoc axios instances from random components when `src/api/client.ts` and module APIs already exist.
- Do not add untyped props/emits or broadly untyped API responses in new code.
- Do not re-implement auth, locale, timezone, or response-envelope logic outside the shared API client.
- Do not ignore cancellation/cleanup for request-heavy table or search UIs.
- Do not duplicate modal shells or confirmation flows when base dialog components already fit.

---

## Required Patterns

- Use `apiClient` and API modules under `src/api/`. Examples: `frontend/src/api/client.ts`, `frontend/src/api/admin/users.ts`.
- Use `<script setup lang="ts">` with typed props/emits for SFCs.
- Use composables for reusable async logic such as forms, tables, navigation loading, or OAuth.
- Keep route pages lazy-loaded through `frontend/src/router/index.ts`.
- Preserve accessibility details in shared UI primitives such as dialogs.

---

## Testing Requirements

- Run `pnpm lint:check`, `pnpm typecheck`, and `pnpm test:run` from `frontend/` for meaningful frontend changes.
- Vitest is configured with `jsdom`, colocated `*.spec.ts` / `*.test.ts` discovery, and coverage thresholds of 80% globally. See `frontend/vitest.config.ts`.
- Unit tests are colocated near components, stores, composables, router utilities, and utils. Examples: `frontend/src/components/__tests__/LoginForm.spec.ts`, `frontend/src/composables/__tests__/useTableLoader.spec.ts`, `frontend/src/stores/__tests__/auth.spec.ts`.
- Integration-style flows live in `frontend/src/__tests__/integration/`.

---

## Code Review Checklist

- Does the change use shared API/store/composable infrastructure instead of introducing a one-off path?
- Are props, emits, route meta, and API responses typed?
- If UI primitives changed, were accessibility and keyboard interactions preserved?
- If async fetching changed, are stale requests cancelled or otherwise handled safely?
- Were lint, typecheck, and the relevant tests updated or run?

---

## Examples

- Frontend scripts and dependencies: `frontend/package.json`
- ESLint baseline: `frontend/.eslintrc.cjs`
- Vite build/type checker setup: `frontend/vite.config.ts`
- Vitest config and thresholds: `frontend/vitest.config.ts`
- Shared HTTP client with interceptors: `frontend/src/api/client.ts`
- Example component test: `frontend/src/components/__tests__/LoginForm.spec.ts`
