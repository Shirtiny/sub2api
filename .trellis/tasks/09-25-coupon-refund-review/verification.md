# Coupon limit message and presale refund review

## Diagnosis and scope

- Read-only test-database inspection confirmed the test administrator's shared code was consumed by order #11, later partially refunded. The consumed timestamp remains set, as intended; no quota was reset.
- Added an account-specific shared-code limit error with `limit: "1"` metadata and Chinese/English checkout messages. Personal coupon errors and stale payment-reservation protections remain unchanged.
- Added a required customer-written reason to the subscriptions-page presale refund form. Frontend and user-refund service reject whitespace-only and over-500-code-point reasons; the trimmed text is stored in the existing refund-request field. Duplicate submissions are blocked; a failed request preserves the draft; a new review clears it.
- Added an informational `coupon_applied` refund-quote flag from the order snapshot. Show coupon non-return/usage non-restoration notice only for applicable orders, including legacy accepted quote snapshots. Refund amounts, fees, entitlement logic, coupon eligibility and use counts are unchanged.

## Automated checks

- Focused frontend suites: 45 tests passed.
- Full frontend suite: 172 files, 1,391 tests passed.
- Frontend typecheck, targeted ESLint and production build passed.
- Service/handler unit suites matching presale, café campaign/coupon, refund and simulated-payment flows passed.
- Added backend tests for blank/Unicode-whitespace/oversized reasons with no order or entitlement mutation; 500-code-point reasons, trimmed persistence, idempotent retry, conditional coupon flags, and unchanged legacy quote amounts.
- Changed-code Go lint: zero issues. No schema or migration changes.

## Isolated preview

- Updated only the local test app and frontend at port 4178 after backing up the old binary, frontend index and test database. Test app version: `0.0.93-local-coupon-refund` (base `4c76d602861e355f7bb7a606b2c9dc1ec1c1bab4` plus local changes).
- Production container fingerprints unchanged. Test PostgreSQL/Redis containers and migration state unchanged. Simulated payments and isolated networking retained.
- Build/test logs, backups and browser verification artifacts: `/opt/stacks/sub2api-test/artifacts/coupon-refund-review/`.
- Browser refunds use intercepted fixture orders and POSTs, not actual user orders. No real refund, new purchase, or coupon issuance is performed during verification.
- Actual test APIs returned the new account-limit reason and limit metadata for the administrator, while an unused account could still inspect the same code. An intentionally invalid refund request (blank reason and negative expected amount) was rejected without changing its order. Actual presale checkout rendered the new Chinese limit message.
- Desktop 1440×900 and mobile 390×844 browser fixtures verified reason validation, conditional coupon notice, trimmed submission payload, cleared drafts, and no runtime errors. User/order/subscription/campaign/use counts were unchanged after verification.
