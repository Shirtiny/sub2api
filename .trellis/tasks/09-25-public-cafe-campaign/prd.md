# Public presale café campaigns

The user requests an administrator-managed shared discount code: 40% off, usable
only on subscription presales, September 25–30 2026 inclusive in Asia/Shanghai.
They explicitly confirmed one successful use per account. Unpaid cancelled/expired
orders can be replaced; successful use is not restored on refund/cancellation.

- Create disabled campaign with immutable code, name, discount percentage, business
  date window; admin enable/disable and paginated usage/order records with audits.
- Separate namespace/storage from existing personal membership coupons. Reuse the
  existing checkout code field and amount/fee calculation. No stacking.
- Server checks at preview, order transaction, and paid fulfillment. DB unique
  account/campaign reservation + user lock prevent duplicate purchases across groups.
- Released old attempts cannot reclaim a newer reservation. Preserve payment facts
  and reject fulfillment when a late callback's reservation was superseded.
- Stop new reservations at expiry/disable. Cap order payment deadline at campaign
  expiry; honor a valid already-bound order snapshot under existing payment callback
  rules (including provider delivery delay), not live campaign enable state.
- Additive migration, generated ORM, tests including disposable PostgreSQL races.
- No production changes, actual issuance, real gateway calls, deploy or push.
