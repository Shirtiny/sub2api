# Directory Structure

> How frontend code is organized in this project.

---

## Overview

The frontend is a Vue 3 + TypeScript + Vite application organized by responsibility:

- `src/views/` contains route-level pages.
- `src/components/` contains reusable UI, split into `common/` and domain folders.
- `src/api/` contains all HTTP modules and central client setup.
- `src/composables/` contains reusable stateful logic (`useXxx`).
- `src/stores/` contains shared Pinia stores.
- `src/router/`, `src/i18n/`, `src/types/`, and `src/utils/` hold app-wide infrastructure.
- Tests live alongside code in `__tests__/` and at the integration level in `src/__tests__/integration/`.

---

## Directory Layout

```text
frontend/src/
├── api/                 # typed HTTP modules and axios client
├── components/          # reusable UI components by domain
├── composables/         # reusable Vue composition logic
├── i18n/                # locale setup and message catalogs
├── router/              # route definitions, guards, title helpers
├── stores/              # Pinia stores
├── styles/              # shared styling assets
├── types/               # shared TypeScript interfaces and aliases
├── utils/               # focused stateless helpers
├── views/               # route-level screens
├── __tests__/integration/ # integration-style frontend tests
├── App.vue
├── main.ts
└── style.css
```

---

## Module Organization

- Route pages go in `src/views/` and compose domain components. Examples: `frontend/src/views/admin/DashboardView.vue`, `frontend/src/views/admin/UsageView.vue`.
- Shared or base UI goes in `src/components/common/`; feature-specific UI goes in domain folders such as `components/account/`, `components/admin/`, and `components/charts/`.
- API calls go through `src/api/`, not directly from random files. Examples: `frontend/src/api/client.ts`, `frontend/src/api/admin/users.ts`, `frontend/src/api/index.ts`.
- Cross-component reusable logic belongs in `src/composables/`. Examples: `frontend/src/composables/useForm.ts`, `frontend/src/composables/useTableLoader.ts`, `frontend/src/composables/useOpenAIOAuth.ts`.
- Shared global state goes in Pinia stores under `src/stores/`. Example: `frontend/src/stores/auth.ts`.

---

## Naming Conventions

- Vue SFC files use PascalCase, for example `BaseDialog.vue`, `DashboardView.vue`, `UsageStatsCards.vue`.
- Composables use `useXxx.ts`, for example `useForm.ts`, `useTableLoader.ts`, `useOpenAIOAuth.ts`.
- Store files are short lowercase names by domain, for example `auth.ts`, `app.ts`, `subscriptions.ts`.
- Test files use `*.spec.ts` and are usually colocated under `__tests__/`.
- Use `index.ts` barrel exports for top-level API/store folders or tightly related component groups. Examples: `frontend/src/api/index.ts`, `frontend/src/stores/index.ts`, `frontend/src/components/account/index.ts`.

---

## Examples

- App bootstrap and plugin wiring: `frontend/src/main.ts`
- Route definitions and lazy loading: `frontend/src/router/index.ts`
- Central API exports: `frontend/src/api/index.ts`
- Shared store exports: `frontend/src/stores/index.ts`
- Domain barrel exports: `frontend/src/components/account/index.ts`

---

## Anti-Patterns

- Do not scatter HTTP calls across components when an API module already exists.
- Do not place reusable business logic directly inside many views when it can live in a composable.
- Do not add global state to Pinia for one-screen transient UI state.
- Do not put unrelated domains into a single giant `index.ts` barrel; keep barrels local and purposeful.
