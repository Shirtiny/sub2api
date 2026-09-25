# Review fixes — verification

## Changes

- The common payment transition now requires `paid_at IS NULL` in the atomic SQL
  update, including the expired-payment receipt path. Already-paid FAILED orders
  retry fulfillment without rewriting receipt timestamp, amount or trade number.
  No deadline/grace relaxation and no reset of consumed campaign slots.
- Campaign administration preserves the constrained desktop flex/scroll chain,
  with fixed pagination and policy/footer. Mobile retains natural page flow.
  Fixed-size pagers no longer display an unhandled page-size selector.
- Shared order details render loading, errors and retry independently of the
  presence of an order. Product fields still require loaded data; retry is
  disabled during loading. Associated-order errors use the real component in tests.
- Ent uses `MaxRuneLen(100)` for campaign names. Frontend counts Unicode code
  points, displays the count and blocks 101 characters (without HTML's incompatible
  UTF-16 maxlength). Existing PostgreSQL VARCHAR(100) already has the correct
  semantics; no new DB migration is needed. Ent was regenerated, not hand-edited.

## Passing checks

- Backend unit suite: payment/coupon/presale/fulfillment/refund/notification and
  related service + admin/user handler tests passed. New regressions cover delayed
  provider callbacks after coupon-audit rollback, manual recovery, truly late
  payments staying rejected, receipt preservation and Unicode name boundaries.
- Disposable PostgreSQL/Redis harness: all 11 selected top-level tests passed,
  not skipped. Includes four concurrent delayed callbacks to an originally timely
  paid FAILED campaign order, exactly one coupon consumption/points credit,
  existing concurrent checkout/refund/activation checks, and 100-character Chinese
  and supplementary-plane names stored/reloaded through real PostgreSQL.
- Frontend: 156 tests passed across six files (campaign management, shared order
  details, order/promo management, checkout and presale). Typecheck passed;
  changed-component/test ESLint passed; Go changed-code lint reports 0 issues.
- Vite build passed to `/var/tmp/cafe-review-fixes-ui-build`, not a served directory.
  Existing chunk-size, Browserslist and runtime deprecation warnings remain.
- Headless Chromium with all requests intercepted: 1440×900, 1280×600, 390×844 and
  320×740 passed. Verified last-row reachability after virtualized measurements,
  unclipped pagination, working page 2, no horizontal overflow, initial detail
  loading, a simulated failed detail request and successful retry. Desktop table
  viewports are approximately 500px and 200px high and actually scroll.
- `git diff --check` clean; no dependency changes.
- The unchanged standalone Go review reproductions also pass after the fixes:
  both delayed callback/manual recovery branches and the 34-Chinese-character
  name that previously failed. See `cafe-review-fixes-original-repros.log`.

## Artifacts and boundaries

Logs: `/var/tmp/cafe-review-fixes-*.log`. Fully mocked browser script/screenshots:
`/var/tmp/cafe-review-fixes/`. The original review's standalone Go overlay remains
outside the repository at `/var/tmp/sub2api-campaign-review/`.

No production or preview service changes, live payment/email/coupon issuance,
production database changes, commits/tags/pushes. Unrelated untracked files were
left untouched. Test database writes were confined to isolated SQLite and the
existing disposable PostgreSQL test harness.
