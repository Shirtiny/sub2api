# Shared café codes for presales

## Administrator workflow

`/admin/promo-codes` → **Shared café codes** (通用咖啡券) → Create:

- Suffix: e.g. `SEP40`, producing `CAFE-PUBLIC-SEP40`. This is an example, not
  an issued production code. The prefix distinguishes campaigns from personal
  membership coupons, whose claim, transfer and cooldown logic is unchanged.
- Name: an internal campaign label.
- Percentage off: **40** means subtract 40%, pay **60%** (not pay 40%). Compute
  against the server-priced plan and multiplier; existing currency rounding and
  channel fees apply after the discount. Never trust a client-supplied price.
- Start: `2026-09-25`; inclusive end date: `2026-09-30`. Both use Asia/Shanghai.
  Precisely: `[2026-09-25T00:00:00+08:00, 2026-10-01T00:00:00+08:00)`.
- UI creates the campaign paused. Review, enable, then copy/share the complete code.

Scope is fixed to subscription presales, all eligible presale plans/multipliers.
Each authenticated active account has at most **one successful use per campaign**,
across plans and source groups. There is no global issuance cap. This is an account
limit, not proof of a unique physical person. Normal registration/access controls
continue to apply. It cannot stack with another café coupon (one checkout code).

Codes, percentage and dates are immutable after creation. To correct a mistake,
pause it and create a different code; codes are not deleted or recycled. Enable /
pause requires a fresh version and explicit administrator confirmation. These
rules protect prices already accepted and prevent resetting old usage history.

Usage records show the current order for each account, its financial status and
whether the code is reserved or consumed. Click an order to inspect its full
administrator detail. An unpaid replacement changes the current binding; every
older order and its reservation audit remains in ordinary order management.

## Checkout and concurrency

The existing authenticated coupon input, information and preview endpoints accept
the shared code. Existing lookup rate limiting remains. Information/preview never
claim a code or create a usage row. Balance top-ups and immediate subscriptions
cannot use it. Public information contains terms only, not other users' records.

Creation holds the payment user lock, then the campaign lock, revalidates enabled
state, the date window, scope and calculated price, and writes the order,
reservation and strict audit in one transaction. The campaign lock serializes
administrator pausing against new reservations. `(campaign_id, user_id)` and
`order_id` have database unique constraints. No process-local lock is the only
protection. An existing unpaid, unexpired order blocks another use, even on a
different plan/group. Different accounts may use the same code independently.

An unpaid cancelled/expired/failed order can be replaced. Expired reservations are
also recognized by the stored payment deadline even before the timeout worker.
The order deadline is capped at the campaign's expiry. The old attempt remains
retired after replacement: a late payment records payment facts but cannot reclaim
the coupon, grant a second subscription or move the newer binding. Reconcile its
paid fulfillment failure using the existing administrator refund workflow.

A verified paid order checks the durable binding, immutable terms, scope, original
creation window and discount snapshot under user/order locks before fulfillment.
It persists a consumed timestamp and strict `CAFE_CAMPAIGN_USED` audit atomically.
Even if fulfillment has not yet written this marker, the bound order's `paid_at`
already prevents new use. Provider callbacks and fulfillment retries are idempotent.
Refunds (including offline recording) and paid reservation cancellation do not
clear the binding/consumed timestamp or grant a second use. Payment failures keep
their payment facts; no money is silently converted to balance or erased.

Callback retries never overwrite the first verified payment timestamp, amount or
trade reference: a database predicate, not a stale in-memory snapshot, protects
them. A timely payment remains fulfillable after transient coupon/audit failures
even when callback redelivery is after the deadline. A genuinely late first payment
remains late on every retry. Campaign names allow 100 Unicode characters, matching
the administrator form, service validation and PostgreSQL character limit.

Pause/expiry stops **new** reservations. A previously accepted order retains its
immutable discount when the provider callback is delivered later, subject to the
existing payment deadline/grace and presale fulfillment rules. It is not repriced
because an administrator subsequently paused the campaign. Expired coupons cannot
create fresh orders. The campaign path also enforces the stored deadline plus the
existing five-minute callback grace even if the timeout worker has not yet changed
PENDING to EXPIRED. Beyond-grace paid attempts retain payment facts but cannot
fulfill at a stale discount; support must reconcile/refund them. Previously consumed
orders keep their successful marker on retries. This explicitly separates accepted
quotes from new issuance.

## Persistence, permissions and rollout boundary

Migration `202_public_presale_cafe_campaigns.sql` adds two empty tables, their
uniqueness, foreign-key and validity constraints. It does not insert a campaign,
modify a member coupon, or update historical orders. Ent schema and generated
client code accompany the migration. Monetary snapshots reuse the existing
`cafe_coupon_code` / `cafe_coupon_discount` order fields. Existing membership-point
and presale refund calculations therefore continue to use actual discounted money.

Administrator endpoints (under existing admin authentication):

- `GET/POST /api/v1/admin/promo-codes/cafe-campaigns`
- `PATCH /api/v1/admin/promo-codes/cafe-campaigns/:id/status`
- `GET /api/v1/admin/promo-codes/cafe-campaigns/:id/usages`

Only the authenticated administrator ID is audited; request-body actor fields are
ignored. Creation/status changes have strict audits; no status-change API can edit
terms or clear a used slot. PostgreSQL stores timestamps with microsecond precision,
so version tokens returned by create/update explicitly use that precision.

Deploy frontend/backend/migration together only when authorized. No production
code is issued and no production service is changed by this development task.
