# Verification — 2026-09-25

## Implemented

- Admin-only `POST /admin/payment/orders/:id/presale-offline` with fresh-version
  review, explicit confirmation, bounded reason/reference/actual currency amount.
- Separate paid cancellation state (not unpaid cancellation or fictitious refund).
- Correct entitlement cancellation; no usage/window reset, later term untouched.
- Idempotent actual-offline-refund recording with atomic earned-point reversal,
  strict actor audit and safe secondary-bookkeeping retry. No provider call.
- First online-refund attempt marker persisted before provider dispatch and reused
  on retries under the existing order/action unique audit index. Uncertain online
  attempts cannot be settled again offline.
- Presale slot release; webhook cannot revive cancellation. Paid-but-not-refunded
  cancellation still counts toward revenue, customer daily and channel daily limits.
- Localized administrator dialog, order audit/refund detail and user cancellation
  status. Stale details, double submission and mismatched response guards.

## Checks

- Payment package tests passed.
- Unit/service + admin handler + routes regression passed for presale, refunds,
  multiplier, coupon and order-detail scenarios. Final offline/marker rerun passed.
- Frontend regression: 205 tests / 12 files passed. Final dialog/parent rerun:
  19 tests / 2 files passed.
- `pnpm typecheck`: passed. ESLint on changed frontend: zero errors, two existing
  unused-variable warnings in PaymentView (`totalAmount`,
  `cafeCouponDisplayPayableAmount`). New dialog/parent/tests lint clean.
- Disposable PostgreSQL/Redis harness: all 7 `TestPresalePostgres*` tests passed,
  including concurrent offline replays and cancellation racing activation. Real
  PostgreSQL locks/unique audit constraints were exercised; tests were NOT skipped.
- Online marker test uses localhost httptest only, checks audit exists before
  dispatch, blocks dispatch on audit failure, and allows retries with one marker.
- Unit-tag lint across the existing service/handler/routes suites reports 30
  pre-existing findings in unchanged test files; none in this task's files.
- Lint with `--tests=false` additionally surfaces 50 existing unused test-helper
  functions in unchanged source files. No unrelated cleanup was made.
- Final changed-code lint (`--build-tags unit --new-from-rev=HEAD`): **0 issues**.
  Checked type assertions in the new audit tests and the earlier multiplier test;
  final offline/marker/multiplier unit rerun passed after those test-only fixes.
- `git diff --check`: clean.

## Safety / scope

No production containers/services, real orders, provider credentials, real refunds,
or emails changed. Only isolated test containers were created by the existing
integration harness. No migration required. No commit, tag, push or deployment.
Previous uncommitted renewal-window, preserve-usage and multiplier changes remain.
Unrelated untracked assets were left untouched. A SQLite scratch artifact produced
by a duplicate subtest name was removed; subtest names now use distinct prefixes.

Logs: `/var/tmp/presale-offline-*.log`.

## Release authorization

The user subsequently authorized commit, tag and push of the accumulated presale
renewal-window, usage-preservation, selectable-multiplier and offline-handling
changes as `cafecode-v0.0.92` on `custom-prod`. This authorizes source/artifact
publication only, not a production or preview update.
