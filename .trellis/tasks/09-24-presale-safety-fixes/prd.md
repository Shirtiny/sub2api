# Presale safety fixes (review findings 1, 2, 4)

## Scope
- Preserve the newest user/source-group/month reservation attempt when an older
  cancelled/expired payment arrives late. Retired attempts remain auditable and
  refundable but cannot poison or reclaim the replacement's slot.
- Reverse membership points actually refunded, only if that presale credited
  points, atomically with successful refund finalization. Preserve unrefunded
  points, failed-refund retry safety, legacy grant records, and nonnegative totals.
- Distinguish normal pending activation from a recorded activation failure. Keep
  the 20% cancellation fee during normal worker delay, retain failure refunds and
  immutable accepted refund quotes (including legacy requests).
- Explicitly do NOT change affiliate subscription redemption (review finding 3).

## Constraints
No production actions, new runtime configuration, migrations, or historical data
backfills. Use existing order/audit state, user-row locks, and refund machinery.

## Validation
Unit regressions for callback orderings, expired/cancelled replacements, retries,
membership full/partial/unfulfilled refunds and rollback, activation delay versus
failure, and accepted legacy quotes. Add PostgreSQL concurrency coverage; report
clearly if the sandbox cannot run disposable PostgreSQL. Existing presale/payment
regressions, lint, build/type checks as appropriate.
