# Cafe-style public homepage

## Goal
Translate the cafe concept artwork into a visitor-first landing page, not a dashboard.

## Requirements
- Reuse the supplied cafe banner; keep the reference images unchanged.
- Use warm ivory/espresso colors, editorial typography, restrained borders, and responsive artwork placement.
- Include brand introduction, service capabilities, getting-started instructions, FAQ, and working login/documentation links.
- Exclude sidebar navigation, account balances, subscriptions, personal statistics, fabricated announcements, availability metrics, and unsupported free-trial promises.
- Respect configured branding, documentation URL, existing custom homepage overrides, locale, theme, and role-aware dashboard routing.
- Only read public settings for landing content; no new backend API, dependency, or environment variable.
- Do not modify production or unrelated in-progress changes.

## Acceptance
- Guest/user/admin navigation works, optional links are conditional, and HTML/iframe overrides still work.
- English/Chinese and light/dark layouts work on desktop and mobile without overflow.
- Unit tests, lint, typecheck, isolated build, and local browser checks are recorded.

## Workflow
The Trellis start instructions and frontend specs were read. The repository's Trellis scripts/identity helpers are absent; context was inspected manually using the existing codex-agent workspace. No production actions are authorized.
