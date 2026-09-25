# Presale balance gift activity

- Extend existing promotion management, not a second activity system. Keep legacy bonus-day activities unchanged.
- Configurable fixed CNY face-value gift per participating presale plan, converted to USD ledger credit at the locked recharge rate; preserve legacy USD definitions; never multiply by subscription multiplier. Allow café coupons to stack. Dates and per-account participation limits remain configurable.
- September 25–28 means Beijing September 25 00:00 through September 29 00:00 exclusive. No production activity is created/enabled automatically.
- Show an elegant brand-matched activity section and integrated plan benefits on the presale landing page, with precise personal eligibility at checkout.
- Snapshot the reward and recharge conversion rate in the order. Grant once after verified timely payment, with balance history and transactional audit. No membership points/rebates for the gift itself.
- Refunds reclaim available balance; any shortfall reduces the cash refund using the order's locked recharge conversion rate. Show this breakdown before confirmation. Reject insufficient refundable money for manual reconciliation, never silently forgive a shortfall. Admin offline cancellation/refund must not bypass gift recovery.
- Keep unpaid cancellation/retry safe, paid refunded activity uses consumed, time boundaries server-authoritative, and preserve all existing presale/coupon safeguards.
- Test locally and on isolated port 4178 only. No production changes or commit/tag/push unless requested.

## Review follow-up
- Fully isolate committed PostgreSQL gift fixtures (participation FKs, balance history, orders and activity data) from subsequent repository suites.
- Verify a stable economic version at checkout, including signed OAuth and QR fallback; refresh stale quotes without auto-charging.
- Allow safe name/disable edits after a plan leaves presale, while validating reactivation.
- Public activity collection includes active/upcoming supported gifts, with visible-plan filtering. Refresh independently without replaying checkout routes.
- Reusable card design anticipates bonus days; approved future policy extends the current subscription expiry. Fulfillment of calendar-presale day rewards remains a separate change, not the legacy day path.
