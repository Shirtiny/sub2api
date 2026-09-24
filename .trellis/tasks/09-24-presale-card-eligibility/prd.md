# Presale eligibility before checkout

The user must see whether a plan can be reserved on the landing card, not only
after pressing Reserve and opening checkout.

- Reuse authenticated, read-only presale quotes before enabling each card.
- Show already-reserved, unpaid and refund states with relevant existing links.
- Keep controls disabled while checking; failures offer a retry, never permission.
- Login-return query parameters must not bypass the check. Preserve legitimate
  provider returns and recovery for the exact server-confirmed unpaid order/plan.
- Recheck after closing checkout or returning from another tab; ignore stale
  responses on reload, account change and unmount.
- Keep creation-time server validation and locking unchanged. No new API route,
  migrations, production deployment or automatic order cancellation.
