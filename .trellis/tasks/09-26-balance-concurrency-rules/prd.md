# Configurable balance concurrency

## Goal
Admin users page can edit site-wide balance intervals and concurrent request limits.

## Contract
GET/PUT /api/v1/admin/users/concurrency-rules (admin middleware):
`{ "balance_tiers": [{"min_balance":0,"concurrency":1},{"min_balance":20,"concurrency":2},{"min_balance":100,"concurrency":3}] }`.
Store as one JSON settings value, no migration/env variable needed. 1–20 tiers,
first lower bound 0, subsequent finite strictly increasing lower bounds <=1e12;
integer concurrency 1–1000. Invalid requests => 400 and no change. DB failures
must not overwrite runtime settings. User DTO exposes `balance_concurrency_rules`
(array) so profile tooltip follows effective policy. No subscription rule changes.

## Acceptance
- Admin modal uses shared dialog, zh/en, dark/light, mobile; add/remove tiers,
  derived interval upper bounds; loading/error/save feedback and list refresh.
- Current defaults retained until saved. Active subscription priority preserved.
- HTTP/profile/admin sorting and retained WS apply config, not stale auth snapshot.
- WS admission never synchronously falls back to DB; bounded cache freshness.
- Policy saves immediate locally, other instances bounded TTL; in-flight not cancelled.
- Tests: defaults, boundaries, invalid inputs, failed saves, reload, cache updates,
  subscription precedence, admin authorization, API/frontend integration.
- Refresh isolated 4178 environment without changing real production or isolation/data.

## Context
Trellis scripts are absent; manually following workflow as codex-agent.
See docs/USER_CONCURRENCY.md and .trellis/spec/backend/database-guidelines.md,
frontend/component-guidelines.md, type-safety.md, quality-guidelines.md.

## Review fixes (2026-09-26)
- Maintain rule freshness with a lifecycle-managed periodic worker, independent
  of traffic. Retained WS turns remain DB-free and still reject rules older than
  30 seconds during real outages; errors cannot extend the safety window.
- Share refreshes with singleflight, independent bounded read contexts, promptly
  cancelable callers, and a short error cooldown. A cancelable read/write gate
  prevents old refreshes overwriting saves; write timeouts include gate waiting.
- Resolve active subscription priority before querying balance rules on HTTP
  admission. Cover both cold/cache-hit auth and return to balance rules on expiry.
- Full UTC unit tests, focused race regressions x3, PostgreSQL/Redis integration,
  backend lint and isolated-test browser/API verification passed. No schema or
  frontend changes for these fixes; production untouched.
