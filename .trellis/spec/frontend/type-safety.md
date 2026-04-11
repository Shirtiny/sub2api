# Type Safety

> Type safety patterns in this project.

---

## Overview

The frontend uses strict TypeScript with Vue SFC support.

Observed conventions:

- Shared domain/API types are centralized in `src/types/index.ts`.
- API modules return typed promises and use generic axios helpers where helpful.
- Components and composables type props, emits, params, and return values explicitly.
- `any` is allowed by lint, but the codebase still mostly uses concrete interfaces, unions, or `Record<string, unknown>` at dynamic boundaries.

---

## Type Organization

- Put shared application types in `frontend/src/types/index.ts`.
- Re-export convenient store-related types from `frontend/src/stores/index.ts` when it improves ergonomics.
- Keep API-specific request/response types near the API layer when they are not broadly reused. Example: `BalanceHistoryItem` and `BalanceHistoryResponse` in `frontend/src/api/admin/users.ts`.
- Keep component-local prop/emits types in the SFC when they are only used there. Example: `frontend/src/components/common/BaseDialog.vue`.

---

## Validation

- There is no project-wide runtime schema library like Zod in use right now.
- Runtime validation is mostly done at API/error boundaries with narrow checks, request-shape assumptions, and safe defaults. Example: response envelope and error-shape handling in `frontend/src/api/client.ts`.
- When consuming backend error objects or OAuth payloads, prefer tolerant typed objects with optional properties instead of large unsafe casts. Example: `frontend/src/composables/useOpenAIOAuth.ts`.

---

## Common Patterns

- Use shared interfaces for API payloads and entity models: `frontend/src/types/index.ts`.
- Use literal unions for constrained state such as roles, statuses, widths, or metrics. Examples: `frontend/src/types/index.ts`, `frontend/src/components/common/BaseDialog.vue`, `frontend/src/views/admin/UsageView.vue`.
- Use typed axios calls such as `apiClient.get<AdminUser>(...)` and `apiClient.get<PaginatedResponse<ApiKey>>(...)`. Example: `frontend/src/api/admin/users.ts`.
- Use typed `defineProps`, `withDefaults`, and `defineEmits` in SFCs. Examples: `frontend/src/components/common/BaseDialog.vue`, `frontend/src/components/common/ConfirmDialog.vue`.

---

## Forbidden Patterns

- Do not add new untyped props, emits, or API return values when a local interface is easy to define.
- Do not bypass the shared `@` path alias with long fragile relative imports unless required.
- Do not spray `any` through new code for convenience when a union, interface, or `unknown` boundary is more accurate.
- Do not duplicate shared entity/API types across multiple files.

---

## Examples

- Strict TS config and path alias: `frontend/tsconfig.json`
- Central shared types: `frontend/src/types/index.ts`
- Typed axios API methods: `frontend/src/api/admin/users.ts`
- Shared API client envelope typing: `frontend/src/api/client.ts`
- Typed props/emits in SFCs: `frontend/src/components/common/BaseDialog.vue`, `frontend/src/components/common/ConfirmDialog.vue`
