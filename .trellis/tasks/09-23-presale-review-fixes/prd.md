# Presale review fixes

Fix the three reproduced review findings without new infrastructure, settings,
dependencies or database migrations. Preserve unrelated local files.

## Contracts
- Restore a presale checkout only after the authenticated order endpoint confirms
  the exact plan and UTC+8 calendar month. A success event carries the server order;
  another balance, plan or month must never mark this reservation purchased.
- When JSAPI fails or the user dismisses it, retain recovery until the original
  order is confirmed cancelled. Only then may a QR fallback create another order.
  Already-paid, unknown or failed cancellation results keep the original order.
- Refund quotes and cancellation share the purchased source-order entitlement.
  Supported early resets reduce the refundable remaining days; the denominator
  stays the originally purchased month. Extensions/unrelated term changes remain
  blocked. Lock the subscription before recomputing the confirmed refund amount.
- Freeze the complete accepted quote in the existing transactional audit log so
  cancellation, retry and a later renewal cannot change it. Existing audit records
  retain their original snapshot-based calculation for compatibility.

## Verification
Regression tests for mismatched/restored orders, success identity, pending-order
fallback/cancellation races, normal/custom early resets, quote changes, retry after
cancellation/renewal, manual term edits and final-week boundaries. Frontend tests,
typecheck/lint; backend unit and disposable-Postgres integration tests.

## Safety
Local code/tests only. No production payments, emails, settings, migrations,
deployments or restarts. Publication is separate from this fix request.
