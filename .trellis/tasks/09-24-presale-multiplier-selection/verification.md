# Presale card multiplier selection

Move the subscription multiplier control from the order confirmation input to
an existing shared Select on each public presale card. Options follow the
administrator's integer range. Price, original price and quotas update together;
concurrency and calendar-month duration stay unchanged, matching backend rules.

Persist the selection through the login redirect/query, and pass it explicitly
to checkout. Fixed plans remain 1x. Existing custom renewals retain their current
multiplier (including when the configured new-purchase range has changed).
The confirmation page no longer has an editable multiplier input.

Verification:
- PresaleView and PaymentView: 64 tests passed.
- Typecheck passed; ESLint has no errors (two existing unused-value warnings in
  PaymentView are unchanged).
- Test frontend rebuilt/refreshed; no production services or configuration changed.
- Real browser against the isolated test backend: select 2x, observe doubled
  price/quota, log in without losing the selection, confirm without an input,
  create a simulated order and verify persisted multiplier=2 / amount=2.
- Refunded only the created QA order (#7) through the local simulated refund API.
- Desktop and 390px mobile dropdown interactions passed, no horizontal overflow
  or JavaScript runtime errors.
- Browser evidence under /opt/stacks/sub2api-test/artifacts/presale-multiplier-*.

No commit, tag or push requested/performed. Existing unrelated work preserved.

## Follow-up: simplify checkout confirmation

At the user's request, removed the duplicated pending-subscription/date/refund
explanation panel and its consent checkbox from checkout. Removed the corresponding
client-only consent gate rather than leaving a hidden requirement. Server quote,
business-month validation, payment-success guard and backend purchase rules stay
in place; the public presale policy and subscription/order terms are unchanged.

66 focused tests pass, including delayed eligibility, unavailable presale and
month-change rejection. Typecheck/build passed; the two existing lint warnings
remain unchanged. Real browser confirmed no explanation/checkbox, successful 2x
local purchase (#8), correct persisted amount/multiplier, then a simulated refund.
Test preview refreshed only; no production changes or commit/tag/push.

## Follow-up: reserve label and inline checkout scroll

Changed the authenticated button to `预定` / `Reserve`. Reproduced the jump:
opening checkout wrote the plan/multiplier query, then the global router explicitly
called window.scrollTo({top: 0}), moving from y=1126 to y=0. Preserve position for
same-path, same-hash presale state updates and retain the anchor when opening or
closing checkout. Back/forward saved positions and cross-page behavior are unchanged.

80 focused route/presale tests passed; lint, typecheck and build passed. Refreshed
only the local test frontend. Desktop browser: before/open/close all y=1126, with
no programmatic scroll calls. Mobile anchor preservation is covered by a separate
browser log at /opt/stacks/sub2api-test/artifacts/presale-scroll-mobile-browser.log.
No production changes, commit, tag or push.
