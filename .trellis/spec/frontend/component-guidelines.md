# Component Guidelines

> How components are built in this project.

---

## Overview

This frontend primarily uses Vue single-file components with `<script setup lang="ts">`, typed props/emits, and Tailwind utility classes.

Observed component style:

- Prefer composition through reusable base components and slots.
- Keep route-level orchestration in views, while reusable UI lives in `components/`.
- Type props and emits explicitly.
- Build accessibility into shared primitives such as dialogs.

---

## Component Structure

- Use `<template>` + `<script setup lang="ts">` as the default SFC shape. Examples: `frontend/src/components/common/BaseDialog.vue`, `frontend/src/components/common/ConfirmDialog.vue`.
- Define props with interfaces and `withDefaults(defineProps<Props>(), ...)` when defaults are needed.
- Define events with typed `defineEmits` signatures.
- Prefer computed values and focused helpers over deeply nested inline template logic.
- For route screens, compose multiple child components rather than duplicating shared sections. Example: `frontend/src/views/admin/DashboardView.vue`.

---

## Props Conventions

- Type props explicitly with interfaces or inline generics.
- Use `withDefaults` for optional props with stable defaults. Example: `frontend/src/components/common/BaseDialog.vue`.
- Keep emitted events explicit and named around user intent (`close`, `confirm`, `cancel`). Examples: `frontend/src/components/common/BaseDialog.vue`, `frontend/src/components/common/ConfirmDialog.vue`.
- Prefer narrow unions for prop options when the UI only supports a fixed set of values. Example: dialog width union in `frontend/src/components/common/BaseDialog.vue`.

---

## Styling Patterns

- Tailwind utility classes are the dominant styling approach in components and views.
- Shared primitives expose variants through props or computed class maps instead of duplicating markup. Example: width presets in `frontend/src/components/common/BaseDialog.vue`.
- Dark-mode classes are applied inline alongside light styles rather than in a separate styling system.
- Reuse base components like dialogs instead of re-implementing modal shells.

---

## Accessibility

- Shared dialogs should set `role="dialog"`, `aria-modal="true"`, and connect titles with `aria-labelledby`. Example: `frontend/src/components/common/BaseDialog.vue`.
- Manage keyboard escape, focus restore, and body scroll lock in modal primitives. Example: `frontend/src/components/common/BaseDialog.vue`.
- Buttons should have labels or accessible names; close buttons use `aria-label`. Example: `frontend/src/components/common/BaseDialog.vue`.

---

## Common Mistakes

- Do not create untyped props or events in new shared components.
- Do not duplicate generic modal markup when `BaseDialog` or `ConfirmDialog` already fits.
- Do not bury API calls inside low-level reusable UI primitives.
- Do not skip accessibility wiring on overlays, dialogs, and interactive controls.

---

## Examples

- Reusable dialog primitive: `frontend/src/components/common/BaseDialog.vue`
- Dialog built on top of a primitive: `frontend/src/components/common/ConfirmDialog.vue`
- Domain component barrel export pattern: `frontend/src/components/account/index.ts`
- Route-level composition of many child components: `frontend/src/views/admin/DashboardView.vue`
