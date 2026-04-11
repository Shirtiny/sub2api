# State Management

> How state is managed in this project.

---

## Overview

State is split across Pinia stores, local component/view state, and route/query parameters.

Observed pattern:

- Pinia is used for cross-route or app-wide state.
- Page-local async state usually stays inside the view or composable with `ref` / `reactive`.
- Server state is commonly fetched on demand through API modules rather than a dedicated query library.
- Route query params are used when state needs to survive navigation or deep links.

---

## State Categories

- Global app state: `frontend/src/stores/app.ts` for toasts, public settings, and app-wide UI values.
- Auth/session state: `frontend/src/stores/auth.ts` for tokens, current user, refresh timing, and login/logout flows.
- Shared feature state: `frontend/src/stores/adminSettings.ts`, `frontend/src/stores/subscriptions.ts`, `frontend/src/stores/announcements.ts`, `frontend/src/stores/onboarding.ts`.
- Local screen state: views often use `ref` / `reactive` directly for filters, loading flags, dialogs, and chart selections. Example: `frontend/src/views/admin/UsageView.vue`.
- Reusable page-level server state mechanics: composables such as `frontend/src/composables/useTableLoader.ts`.

---

## When to Use Global State

Use Pinia when the state is any of the following:

- Needed across routes or many unrelated components.
- Session-related and must persist or auto-refresh.
- A shared cache/config source for the whole app.
- A shared notification or app-shell concern.

Keep state local when it is only used by one view, one dialog, or one short-lived interaction.

---

## Server State

- Server data is usually requested via `src/api/` modules and stored in local refs or a store depending on reuse.
- The project does not use TanStack Query or SWR; request lifecycle is handled manually with refs, abort controllers, and stores/composables.
- The shared axios client centralizes auth, locale, timezone, token refresh, and response-envelope unwrapping. Example: `frontend/src/api/client.ts`.
- Route query state is merged into page filters when deep-linking matters. Example: `frontend/src/views/admin/UsageView.vue`.

---

## Common Mistakes

- Do not move one-view modal/filter state into Pinia without a real cross-view need.
- Do not bypass `apiClient` and re-implement auth or envelope handling in components.
- Do not leave overlapping in-flight table requests uncancelled on fast filter changes.
- Do not duplicate shared auth/app settings logic outside the existing stores.

---

## Examples

- Global auth/session store: `frontend/src/stores/auth.ts`
- Global app settings/toasts store: `frontend/src/stores/app.ts`
- Store barrel export: `frontend/src/stores/index.ts`
- Local view-owned dashboard/usage state: `frontend/src/views/admin/UsageView.vue`
- Reusable server-table state management: `frontend/src/composables/useTableLoader.ts`
