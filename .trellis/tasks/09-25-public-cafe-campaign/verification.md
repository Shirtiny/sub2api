# Verification — shared café campaigns

## Scope

Implemented shared presale-only percentage codes, immutable date/price terms,
administrator create/enable/pause/use records, and existing checkout integration.
The user explicitly confirmed one successful use per account. Defaults in the
creation UI are 40% off and the current Beijing date through the month's inclusive
last day. No real campaign was created; `CAFE-PUBLIC-SEP40` is an example only.

## Security checks

- Server pricing (including configured multipliers) determines the 40% reduction;
  cash remains 60% of base, before existing channel fee rules. A forged client
  price/discount cannot change the committed amount; no coupon stacking.
- Scope checked at preview, transactional reservation and paid consumption.
  User/source-group/month presale eligibility still applies normally.
- User lock + database unique account/campaign slot across groups; public codes do
  not mutate membership coupons or membership claim cooldowns.
- Unpaid cancellations/expiry/failures may be replaced. Superseded callbacks keep
  paid facts but cannot steal the replacement or grant another subscription.
- Paid facts block reuse before the consumed marker; refund and cancellation never
  restore it. Strict reservation/consumption audits roll back atomically on failure.
- Immutable accepted payment terms survive a pause and short callback delay, but
  payment deadline plus existing 5-minute grace is enforced independently of the
  timeout worker. Overdue payments are retained for support reconciliation.
- Admin identity comes from middleware, not body actor fields. Campaign updates
  require fresh versions. PostgreSQL microsecond precision is used explicitly in
  returned version tokens (caught and fixed by real PostgreSQL tests).
- Additive migration 202 creates empty tables, unique/check/FK constraints only.
  No changes to previously published migrations or automatic campaign issuance.

## Checks

- Ent schema regenerated with the repository's go:generate command; no dependency
  or environment configuration changes.
- Backend service/admin/user-handler presale + coupon + campaign suite passed.
  Tests cover exact UTC+8 September 25–30 boundaries, invalid/duplicate definitions,
  percentage semantics and currency fees, immutable scope, cross-group/account
  isolation, expired/cancelled replacements, delayed callbacks, replay, refunds,
  pause between preview and create, forged price and strict-audit failures.
- PostgreSQL/Redis disposable harness: all 9 campaign/presale top-level tests
  passed. Final campaign-only rerun: both tests passed. Includes real unique/FK /
  window checks, repeated administrator toggles, cross-group concurrent checkout,
  callback idempotency and consumed state surviving offline refunds. NOT skipped.
- Frontend focused suite passed all 126 tests in 4 files, covering admin defaults and
  confirmation, invalid discounts, associated orders, localization, presale and
  checkout compatibility. A wall-clock debounce/lifecycle flake in the pre-existing
  checkout staleness test was stabilized using auto-unmount and controlled timers.
- Frontend typecheck passed. Changed-file ESLint: no errors; two existing unused
  variable warnings in PaymentView. Build passed to `/var/tmp/cafe-campaign-ui-build`
  (not the served frontend directory); existing large-chunk/Browserslist warnings.
- Changed-code Go lint passed with 0 issues (`--build-tags unit --new-from-rev=HEAD`).
- `git diff --check` clean.

## Safety

No production/preview deployment or lifecycle changes, real refunds/payments/emails,
production DB writes, actual code issuance, commits/tags/pushes. Test payments used
existing guarded simulation; database writes used isolated test databases only.
Unrelated untracked assets/history files left untouched. See docs/PUBLIC_CAFE_CAMPAIGNS.md.
Logs: `/var/tmp/cafe-campaign-*.log`.

Follow-up review findings and fixes are recorded in
`../09-25-campaign-review-fixes/verification.md` (receipt replay, layout, detail
error states, Unicode names, and expanded regression/browser coverage).
