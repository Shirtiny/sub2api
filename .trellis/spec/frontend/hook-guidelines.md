# Hook Guidelines

> How hooks are used in this project.

---

## Overview

This repo is Vue-based, so "hooks" here means composables under `src/composables/`.

Observed conventions:

- Name composables `useXxx`.
- Return refs, reactive state, and explicit methods.
- Put reusable async control flow, request cancellation, debouncing, or OAuth flows in composables.
- Let composables call API modules and shared stores instead of duplicating request plumbing in views.

---

## Custom Hook Patterns

- Keep composables focused on one concern, such as form submission, table loading, clipboard, or OAuth. Examples: `frontend/src/composables/useForm.ts`, `frontend/src/composables/useTableLoader.ts`, `frontend/src/composables/useOpenAIOAuth.ts`.
- Expose a small surface area: state refs plus clearly named actions.
- Use app-level stores for shared notifications instead of embedding toast logic in every caller. Example: `useForm` and `useOpenAIOAuth` use `useAppStore()`.
- Clean up side effects on unmount when the composable starts async work. Example: abort handling in `frontend/src/composables/useTableLoader.ts`.

---

## Data Fetching

- Fetch through `src/api/` modules rather than calling `fetch` or raw axios inline.
- Use `AbortController` for cancellable requests in rapidly changing views. Example: `frontend/src/composables/useTableLoader.ts`.
- Use debounced reloads for filter/search UIs. Example: `useDebounceFn` in `frontend/src/composables/useTableLoader.ts`.
- OAuth or multi-step remote flows belong in composables so views stay declarative. Example: `frontend/src/composables/useOpenAIOAuth.ts`.

---

## Naming Conventions

- Prefix all composables with `use`.
- Use names that describe the user-facing behavior, not the implementation detail: `useForm`, `useTableLoader`, `useRoutePrefetch`, `useOpenAIOAuth`.
- Keep test files aligned with the composable name under `src/composables/__tests__/`.

---

## Common Mistakes

- Do not duplicate the same async loading or form-submission pattern in multiple views before checking `src/composables/`.
- Do not bypass API modules inside composables when a typed client already exists.
- Do not forget request cancellation or cleanup for long-lived async composables.
- Do not move one-off component-local state into a composable unless it is reused or meaningfully shared.

---

## Examples

- Shared form submission flow: `frontend/src/composables/useForm.ts`
- Paginated server-table loader with debounce and abort: `frontend/src/composables/useTableLoader.ts`
- OAuth flow state and actions: `frontend/src/composables/useOpenAIOAuth.ts`
- Tests for composables: `frontend/src/composables/__tests__/useForm.spec.ts`, `frontend/src/composables/__tests__/useTableLoader.spec.ts`
