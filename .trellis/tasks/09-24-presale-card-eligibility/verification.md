# Verification

- Frontend: 123 focused tests passed across presale landing, checkout,
  subscriptions and scroll behavior. Includes loading/no premature checkout,
  login-return gating, reserved/pending/refund states, same-group sibling plans,
  error retries, month rollover, account changes, logout and stale responses.
- Recovery tests only allow the matching server-confirmed order/plan, reject
  balance/other-order/expired recovery, and preserve provider-return handling.
- Frontend typecheck, targeted ESLint and preview build passed.
- Backend presale unit tests passed, covering all blocking/released order states,
  pending expiry, paid failure, cross-user/month isolation and sibling plans.
- Backend service lint: 0 issues; test backend compilation passed.
- Actual isolated PostgreSQL-backed API: administrator's order #9 returns only
  owned conflict metadata (COMPLETED, order 9, plan 2); another test user can
  reserve the same plan/month. No payment mutation performed.
- Browser at 1440/390/320px: reserved cards link to subscriptions and do not open
  checkout even with a plan query; a fresh account opens/closes valid checkout;
  browser-only pending/error fixtures show appropriate actions. No horizontal
  overflow or JS runtime errors. Screenshots reviewed.
- Evidence: `/opt/stacks/sub2api-test/artifacts/presale-card-*`;
  test logs: `/var/tmp/presale-card-eligibility-*`.

## Preview and production boundaries

- Refreshed only isolated frontend assets and `sub2api-test-app` using its existing
  pinned image and a local binary. Test app healthy; preview health OK on port 4178.
- Test binary SHA-256:
  `e44e30f13c1afa0b21fec56c2089f4d70007bb0f6045326239ad2fb538b90fc6`.
- Migration count remains 230; no migration/backfill introduced. Test order #9's
  entire database row is identical before and after verification.
- Production container IDs/start times are unchanged. No production writes,
  configuration or lifecycle changes. No commit/tag/push for this follow-up.

## Follow-up: graphic loading in the Reserve button

- Removed the visible checking phrase (including unused locale entries). Three
  lightweight SVG steam strokes and a restrained warm sheen occupy the original
  button, with no percentage or artificial delay. The label stays invisibly in
  flow to preserve geometry and fades back after the request finishes.
- Disabled/ARIA-busy semantics and a localized accessible loading name remain.
  The decorative loader is hidden from assistive technology and respects reduced
  motion without adding settings or changing eligibility behavior.
- 101 focused landing/checkout tests passed; frontend typecheck, lint and build
  passed. Refreshed isolated frontend assets only; no backend/container restart.
- Browser with deliberately delayed read-only quote responses: dark/light at
  1440/390/320px, no visible checking text, animated strokes, disabled clicks,
  normal recovery to Reserve, and exactly 46px button height before/after.
  Document position and width remain stable. No runtime errors or payment writes.
- Screenshots reviewed; evidence at
  `/opt/stacks/sub2api-test/artifacts/presale-loading-*` and
  `/var/tmp/presale-loading-{tests,build,browser}.log`.

## Follow-up: shared loading component

- Extracted steam SVG, staggered stroke animation, overlay sheen and reduced-motion
  fallback into the existing `components/common/LoadingSpinner.vue`, exposed through
  `variant="steam"`. Existing spinner callers keep their default size/color/style.
- Supports inherited color, sizes, inline/overlay placement and decorative mode.
  Presale owns only the button's disabled/ARIA state, invisible label and transition;
  no animation paths or keyframes remain in the view. Usage documented in common README.
- Added 17 shared-component tests; 118 focused component/landing/checkout tests pass.
  Typecheck/build pass; full lint has 0 errors and 12 unrelated existing warnings.
- Full frontend suite: 1308 passed, 1 failed (PaymentView coupon stale-response test,
  expected 2 spy calls, observed 3). That unchanged test file passed in the focused run
  and its subsequent standalone rerun (37/37); no unrelated coupon changes made.
- Refreshed isolated frontend assets only. Browser verified dark/light at
  1440/390/320px: shared component rendered, 64×24 steam SVG, animated strokes and
  sheen, reduced motion, disabled clicks, stable 46px button height and ready-state
  recovery. No runtime errors or payment mutations. Preview health OK.
- Evidence: `/var/tmp/shared-loading-{tests,typecheck,lint,build,browser}.log`,
  `/var/tmp/shared-loading-full-{lint,tests}.log`,
  `/var/tmp/shared-loading-payment-recheck.log` and the updated
  `/opt/stacks/sub2api-test/artifacts/presale-loading-*` screenshots/browser report.
- No backend restart, production changes, commit, tag or push in this follow-up.
