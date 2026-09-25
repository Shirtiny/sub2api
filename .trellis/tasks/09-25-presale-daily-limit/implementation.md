# Result

- Test-only admin API update changed exactly `daily_limit`: 1500 to 0 (unlimited).
  Read-back confirmed; no orders, balances, coupons or subscription records reset.
- Checkout now uses the presale error namespace only for defined presale errors,
  falling back to shared payment translations for purchase limits and other
  shared errors. Remaining allowance is interpolated. Updated Chinese and English
  wording to refer to purchases rather than just balance top-ups.
- Added regressions for a generic daily-limit rejection and a presale-specific
  duplicate-reservation rejection. Pending order guards remain in place.
- Full frontend suite: 175 files / 1,433 tests passed. Typecheck and Vite build
  passed; changed-file lint has zero errors and two existing unused-value warnings.
- Refreshed the isolated port-4178 frontend. Verified test admin API read-back
  and public health, presale page and catalog HTTP 200. No production changes,
  backend restarts or real/test purchases were performed.
