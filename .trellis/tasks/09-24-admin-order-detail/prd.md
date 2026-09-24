# Clear administrator order details

- Replace the actual admin orders View dialog with the shared AdminOrderDetail component.
- Prioritize purchased product/type, customer, immutable multiplier/concurrency/duration/presale terms, and payment state.
- Distinguish recorded request origin/referrer from payment method/provider; never invent missing acquisition data.
- Read-only safe admin-only metadata for current plan/group/provider names and stored payment currency/mode; preserve historical names when snapshotted. No database schema/backfill or current quotas presented as history.
- Retain lifecycle/refund/audit information, with readable grouping and expandable raw audit details.
- Explicit loading/error/retry and stale-response protection. Do not change payment/refund behavior.
- Chinese/English, mobile/desktop, tests and isolated preview. Commit/tag/push on completion; no production deployment.
