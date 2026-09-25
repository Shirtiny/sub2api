# Checkout dismissal, refund wording and console branding

## Changes

- Compare scalar authentication state/user ID in the presale watcher. Replacing the current user's profile no longer remounts checkout or reloads the catalog.
- Treat checkout return parameters as a one-shot intent during a page visit. Manual opening or dismissal consumes that intent synchronously, before asynchronous URL cleanup or eligibility checks finish. Initial login/provider returns still restore checkout; users can still reopen it explicitly.
- Coalesce focus/visibility eligibility refreshes while a check is pending. Availability updates do not reopen checkout.
- Shorten the conditional refund coupon notice to `退款后，下单时使用的优惠券无法退还。`, with matching English. No changes to coupon redemption or refund rules.
- Restore the configured sidebar image (default `/logo.png`) next to the shared homepage wordmark. Halve the wordmark on desktop/mobile; retain only the original image when collapsed. Both image and wordmark navigate home. This supersedes the preceding task's compact letter mark; the public header is unchanged.

## Verification

- Three regressions reproduced before the fix: same-account profile replacement, delayed initial eligibility response after dismissal, and stale multiplier-query cleanup after dismissal.
- Full frontend suite: **172 files / 1,394 tests passed**.
- Full frontend lint: **0 errors**, 12 existing warnings.
- Typecheck and production frontend build passed; `git diff --check` passed.
- Playwright against the isolated preview verified same-account checkout stability, dismissal surviving a late check, coalesced focus/visibility requests, explicit reopening, and first-entry return-link restoration. Eligibility fixtures only; no orders or payments created.
- Desktop (1440px) and mobile (390px): configured/default image remains 36px, wordmark is exactly half the homepage width, both links navigate home, collapsed sidebar retains only the image. Screenshots inspected.
- Test preview `/health` returned `{"status":"ok"}`.

Artifacts: `/opt/stacks/sub2api-test/artifacts/checkout-dismiss-brand/` (regression-before.log, vitest.log, lint.log, frontend-build.log, browser.json, browser.cjs, screenshots).

## Scope

Only the isolated test frontend on port 4178 was refreshed. No backend restart, database/migration change, real refund/payment, or production operation. Existing uncommitted work is preserved; no commit, tag or push requested this turn.
