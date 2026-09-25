# Implementation and verification

## Configuration
Existing /admin/orders/plans → activity configuration → "Presale balance gift".
Set a fixed CNY face-value gift per participating presale plan, start/end time (Beijing),
and a shared per-account use limit. Legacy USD definitions keep their currency. September 25–28 inclusive is
2026-09-25 00:00 through 2026-09-29 00:00 exclusive. Legacy bonus-day activities
remain separate. No production activity is automatically enabled.

## Settlement
- One gift per eligible order, independent of multiplier; café coupons may stack.
- Server selects/reserves eligibility under the user lock, snapshots gift + recharge
  conversion rate, caps checkout expiry, and confirms the expected activity ID and economic version.
- Verified timely payment credits balance immediately, in the same transaction as
  the pending presale entitlement and strict audit. Balance adjustment history is
  visible to the user. Gift itself earns neither recharge points nor commission.
- Payment retries cannot double-credit. Paid-but-unfulfilled attempts are refundable.
- Refund requests lock/revalidate the user/order and reclaim available balance.
  The shortfall divided by the locked balance-per-payment-unit rate is deducted
  from cash refund (rounded up to payment currency precision). Freeze this quote
  in the audit for retries; do not reprice against later balance or configuration.
- Admin offline cancellation/refund uses the same recovery and cannot bypass the
  shortfall deduction. Insufficient cash refund fails closed for reconciliation.
- Successful gift uses remain consumed on refund. Unpaid cancellation releases
  reservations; cancellation of an unfulfilled order releases an ungranted gift.

## UI
Warm coffee receipt activity block with SVG steam; integrated plan gift amounts;
personal checkout eligibility; refund balance/cash breakdown; administrator
configuration, balance-aware activity records, order snapshot and audit labels.
Chinese/English. Dark/light and mobile layouts. No new external assets or env vars.

## Validation
- Full backend unit suite passed (UTC).
- Full frontend suite: 173 files / 1399 tests passed; focused tests re-run afterward.
- PostgreSQL tests: concurrent purchase across plans, callback idempotency,
  concurrent refund, locked rate, money constraints; existing presale/café campaign
  PostgreSQL regressions passed.
- Go lint: 0 issues. Frontend lint: no errors (existing warnings).
- Typecheck and production frontend/backend builds passed.
- Isolated 4178 preview: migration 203 applied, health verified. Test-only activity
  gifts $3/$8, shared per-person limit 1, September 25–28. No production changes.
- Dedicated disposable test buyer: 2× plan + 40% café coupon still receives $3.
  Desktop/mobile landing, activity admin and refund review checked with Playwright.
  Evidence and pre-update DB/binary backups:
  /opt/stacks/sub2api-test/artifacts/presale-balance-gift-20260925/

Final isolated API validation: order #13, dedicated test user #5, 2× plan,
40% café coupon, fixed $3 gift. User cancellation + simulated administrator
refund completed; balance returned to zero, exactly one credit and one recovery
history row, activity use remained consumed. Existing user/admin/renewal purchases
were not modified. Final focused frontend run: 145 tests passed.

## Review repairs and CNY configuration (2026-09-25)
- Cleaned the complete committed gift fixture, including redeem-code history.
  Full repository integration suite passes with UserRepo and RedeemCodeRepo suites,
  not only the presale subset.
- Economic quote hash propagates through create, idempotency identity, OAuth
  context/cookie/resume and QR fallback. Stale amount/rate/missing expected version
  rolls back before reservation. UI updates the quote but does not auto-submit.
- Safe disable/name edits no longer require currently presale-enabled plans;
  activation and economic changes retain eligibility/immutability checks.
- Administrator gift input is now CNY; legacy USD gifts are retained as USD.
  Migration 204 adds immutable face-currency/amount order snapshots. Actual credit
  and refund recovery remain USD ledger amounts, with the purchase rate locked.
- New public activity collection + reusable cards, current/upcoming states,
  per-plan gift values, first-screen anchor, multi-card layout and periodic/focus
  refresh. No speculative day fulfillment or advertised legacy-day presale rewards.
- Backend full unit suite passed; real-Postgres full repository suite passed.
  Frontend 173 files / 1407 tests passed; typecheck/build passed. Go lint 0 issues;
  frontend lint 0 errors, 12 pre-existing warnings.
- 4178 updated to 0.0.95-local-presale-cny, migration 204 applied; backup and
  before/after health/migration records retained. Production not changed.
- New isolated test activity gives CNY20/50; previous USD demo disabled without
  changing its history. Dedicated user6/order14 verified stale-quote rejection,
  2x + 40% café coupon, one fixed gift, user cancellation/admin mock refund,
  credit reclaimed once and use not restored. Existing test buyer accounts untouched.
- Browser checks: desktop/mobile, dark/light, English, multi-type rendering-only
  fixture, administrator CNY configuration, real refund review.
- Evidence: /opt/stacks/sub2api-test/artifacts/presale-activity-review-20260925/.

## Customer-facing copy refinement
- Renamed only the isolated demo activity to “秋日预售礼遇”; economic terms,
  currency and participation history unchanged. The preview-environment banner stays.
- Public CNY gifts use the ￥ symbol without redundant “人民币余额” / “CNY credit”
  suffixes in activity and plan cards. Keep conversion/refund disclosure and
  explicit administrator configuration units.

## Activity emphasis refinement
- Section title now “活动进行中” / “Current offers”; upcoming-only collections
  use “活动预告” / “Upcoming offers” instead of claiming they have started.
- Strengthened warm-gold card edges, a restrained background wash and reward
  contrast, with a compact status badge; upcoming cards remain subdued.
- Card titles continue to render the administrator-configured activity name,
  including refresh after administrator renames. No hardcoded campaign name.

## Paper-card contour and release checks
- Removed clipping at the card padding box so the fold cutouts can interrupt
  the actual outer stroke; panels retain their own corner radii. Cutout arcs use
  the same stroke color as the card, including mobile/multi-card layouts.
- Browser checks cover desktop/mobile, dark/light and multiple cards; sampled
  notch-edge pixels match the page background instead of a crossing border.
- Before cafecode-v0.0.96: full `go test -tags=integration ./...` passed;
  `make test-frontend` passed (lint: existing warnings only; typecheck;
  6 critical suites / 135 tests). Latest contour preview build passed.
