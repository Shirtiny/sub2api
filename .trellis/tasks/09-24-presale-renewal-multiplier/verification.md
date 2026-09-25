# Verification

## Implementation
- Presale multiplier selection uses the current plan range independently of the
  active term. The existing immediate-renewal resolver remains unchanged apart
  from extracting its shared plan-range calculation.
- Validation, creation (including its transaction), and coupon previews use the
  same presale-aware resolver. Final price comes from the server plan/selection.
- Landing selects no longer lock to an active multiplier; checkout permits a
  different future multiplier and applies current bounds/disabled customization.
- Existing payment snapshots apply only at activation. No usage/window reset,
  current-term mutation, duplicate slot or early-renewal bypass is introduced.

## Checks
- Backend service/handler presale, multiplier, coupon and simulated-payment
  regression suite passed. New tests cover 1x->3x, 2x->4x, 4x->2x, 3x->1x,
  current/future separation, preserved usage/windows, snapshot survival after
  plan edits, range and customization-disabled guards, duplicate/early renewal
  rejection, coupon preview/payment agreement and legacy resolver compatibility.
- All payment tests use isolated local databases and guarded test-only simulated
  payments, not real gateways. PostgreSQL integration was not run this turn.
- Frontend PresaleView, PaymentView and SubscriptionPlanCard: 123 tests passed.
- Frontend typecheck passed. Changed-file ESLint: no errors, two existing unused
  binding warnings in PaymentView (`totalAmount`, `cafeCouponDisplayPayableAmount`).
- Backend service/handler golangci-lint: zero issues. `git diff --check` passed.
- Logs: `/var/tmp/presale-multiplier-{regressions,frontend-tests,lint}.log`.

No production actions, shared test-app restart, migrations, historical data
changes, commit/tag/push or new runtime configuration. Earlier uncommitted work
and unrelated untracked files are preserved.
