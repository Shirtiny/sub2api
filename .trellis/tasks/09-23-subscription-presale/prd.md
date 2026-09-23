# Calendar-month subscription presales

## Scope
- Public `/presale` landing page, home navigation/compact promotion, home billing explanation.
- Admin plan controls for presale availability, public visibility, badge, and reset-card grant.
- Balance-first recharge page, no subscription sales tab; use the existing payment flow for presale checkout.
- Immutable order snapshots of next calendar month's start/end, plan name, renewal relationship and reset cards.
- Paid orders represent pending subscription entitlements. Activate only when due; no early extension/access/quota changes. Reuse existing billing/subscription/concurrency machinery.
- Display pending entitlements in subscriptions and order/payment results.
- Presale refunds: full before T-72h, 80% during last 72h before activation; after activation unused whole days except final seven days. User requests go through existing admin refund execution, not an unverified new payment-provider implementation.

## Contract / assumptions
- Business calendar: Asia/Shanghai (explicitly labelled; confirmation requested asynchronously).
- Next month only, one purchase per user/source group/month. Renewal buys that fixed next month, not arbitrary extra months. Refuse overlapping already-paid coverage rather than silently losing days.
- Presale mode defaults off for existing plans; listing requires for_sale + presale_enabled + presale_visible + active subscription group. No automatic live sale on upgrade.
- POST existing payment order endpoint gains `presale_month` (YYYY-MM); server recomputes and validates month under transaction. Legacy immediate orders cannot buy presale plans.
- Public catalog exposes curated plan display and dates only; authenticated quote returns renewal/eligibility; no user data in public endpoint.
- Paid presales stay COMPLETED with null activation timestamp until due; payment failure/refund states never activate. A worker uses transaction locks and idempotent assignment.
- Refund request time freezes policy; failed refunds remain visible/retryable. No deductions from unrelated active subscriptions for pending refunds.

## Verification
- Calendar/month rollover/leap year and refund boundaries, stale month, hidden/disabled plan, duplicate purchases, ownership.
- Paid pending has no access, due activation exact dates, renewal, retry/idempotency, refund blocks activation.
- Frontend localization, balance-first routing, public auth return, admin persistence, responsive landing and home compact navigation.

## Safety
Local source/test work only. No live deployment, migrations, real orders, email, account, or settings mutations. Preserve previous uncommitted email/auth fixes.
