# Calendar-month subscription presales

## Product and administrator configuration

The public `/presale` route is available without a session; checkout requires an
active signed-in account. Home navigation links to it. In the compact home header,
ordinary menu links fade out and the open-presale message fades in without changing
the navigation slot's dimensions. The message is only shown if the catalog is open.

`/admin/orders/plans` remains the source of products. A plan must be for sale,
presale-enabled, publicly visible, and belong to an active, non-deleted, ordinary
subscription source group. Existing plans **default to presale disabled**. Configure:

- `presale_enabled`: sell through the calendar-month presale flow (immediate purchase
  of that plan is rejected, including legacy clients).
- `presale_visible`: explicitly publish it on the landing page; hidden plans are
  also not purchasable by posting their ID.
- `presale_badge`: plain-text merchandising label, max 40 characters.
- `presale_reset_cards`: quota reset count granted upon activation, 0–1000. The
  subscription's accumulated reset count remains capped at 1000.
- Existing name, description, features, sort order, price, concurrency and group
  quota configuration supply the rest of the product card.

Presale payment provider instances must enable **administrator refunds** before
accepting an order. This does not require enabling user refunds for balance orders.
Providers that cannot refund cannot accept new presales. If multiple instances
serve a method, enable refunds on all instances used for presales. The existing
payment registry, currency rules, provider credential snapshots, verified webhooks,
recovery tokens, amount validation, idempotency keys, coupons and affiliate
attribution continue to apply. No new payment SDK or credential is introduced.

The ordinary `/purchase` page is balance-first and no longer has a subscription
catalog tab. Old subscription purchase links lead to `/presale`; in-flight signed
WeChat payment resumes are retained for compatibility.

## Calendar and purchase contract

The business timezone is **Asia/Shanghai / UTC+8**, independent of host and browser
timezone. An order created in September 2026 reserves:

- Start (inclusive): 2026-10-01 00:00 +08:00.
- End (exclusive): 2026-11-01 00:00 +08:00.
- Full-refund cutoff (exclusive): 2026-09-28 00:00 +08:00.

Each purchase covers the **next calendar month**, not 30 days from payment. A
multiplier changes quota/price under the existing custom-plan rules, not the number
of months. Legacy bonus-day activities are not combined with fixed monthly terms.
The date window, price/payment amount, name, reset grant, concurrency, multiplier,
early-reset configuration and renewal relationship are snapshotted in the order.
Group quota/rate configuration continues to follow the existing live group model.
Later plan edits do not rewrite these order snapshots.

There is at most one live reservation per user/source group/calendar month. The
payment user row lock protects creation and fulfillment; pending orders also occupy
the slot. A cancelled/expired order paid after another purchase is recorded as a
paid fulfillment failure, not a second subscription. Reconcile/refund it through
admin orders. Do not erase its payment facts.

Existing subscriptions are explicitly displayed as renewal reservations. If an
existing term already extends past the new month's start, purchase is rejected
rather than silently overlapping or discarding paid time. Legacy physical custom
groups require support reconciliation before presale renewal. Virtual custom
subscriptions continue through the normal source group.

## API boundary

- `GET /payment/public/presale`: curated published plan display, `enabled`, server
  time, and `{month, timezone, starts_at, expires_at, full_refund_before}`. No account
  data, provider secrets or internal configuration.
- `GET /payment/presale/:id/quote`: authenticated eligibility and current-subscription
  renewal relationship plus the exact monthly period.
- `POST /payment/orders`: existing request plus `presale_month` (`YYYY-MM`). The
  server validates the month and published plan again under transaction locks.
  The month participates in idempotency and signed WeChat resume claims. Client
  dates/amounts are not trusted. Refresh if the month or plan changed.
- `GET /payment/presale/my`: the caller's last 100 paid presale records.
- Existing user/public signed result DTOs add `presale_starts_at`,
  `presale_expires_at`, `presale_activated_at`, `presale_plan_name`,
  `presale_renewal`, `presale_reset_cards`.
- `GET /payment/presale/orders/:id/refund-quote`: owner-only policy quote with actual
  `gateway_amount`, accounting `refund_amount`, currency, fee and unused days.
- `POST /payment/orders/:id/refund-request`: presales additionally require
  `expected_refund_amount` equal to the reviewed **gateway** refund amount. A
  cutoff crossed before confirmation returns `PRESALE_REFUND_AMOUNT_CHANGED`
  without cancelling the entitlement.
- `GET /admin/payment/orders/:id/presale-refund-quote`: the same policy for admin
  review. The existing admin refund endpoint executes it using the original
  provider binding. No public refund execution endpoint is added.

## Pending entitlement and activation

The paid payment order is the pending subscription entitlement ledger; no parallel
booking table or prematurely active `user_subscriptions` row is needed. Payment
completion is `COMPLETED` with a null `presale_activated_at`. Membership points and
affiliate attribution remain idempotent; no subscription time, quota, concurrency
or reset count is granted early. A presale reservation does not send the legacy
“subscription activated” email.

The existing payment maintenance worker scans due completed presales every minute
(and on startup). Activation is usually within one worker interval of the advertised
start. The underlying term is always the snapshotted interval, not worker execution
time. User + order + subscription locks and the activation marker make concurrent
workers/retries safe. Activation creates or renews the source-group subscription,
starts fresh quota meters, applies the reset grant, records dated concurrency and
early-reset entitlements, records an audit event, then invalidates auth/billing
caches after commit.

Disabled/deleted accounts, unavailable groups, later manual extensions causing an
overlap, and entirely missed terms are **not** silently activated. They remain
visible as unactivated paid records with error logs; reconcile or refund through
admin orders. Failed items do not prevent later records in the same sweep from
being examined. No reverse migration, account enabling or database cleanup runs.

## Refund policy and safety

- Before start minus 72 hours: unconditional full refund.
- During the last 72 hours before start: 80% of actual payment returned (20% fee).
- After activation, before the final seven days: remaining **whole 24-hour days**
  divided by total purchased calendar-month days. No refund during the final seven
  days, including the exact seven-day boundary.
- A paid term that was never activated because fulfillment failed remains fully
  refundable rather than being charged for unavailable service.

A user request atomically freezes its timestamp/amount, changes the order to
`REFUND_REQUESTED`, and cancels only this presale term. For pending presales no
current unrelated subscription is deducted. For active terms the exact subscription
window must still match; otherwise support must reconcile instead of blindly
subtracting days from later purchases. Its concurrency/early-reset entitlements
and unused reset grant are removed alongside the term. Refund requests prevent
future activation even if provider processing crosses the start date.
The activation audit records the reset count actually credited after the 1000-count
cap. Refund cancellation uses that credited count, not the larger advertised grant,
so reaching the cap does not cause an extra deduction from earlier reset credits.

Refund execution is **admin-processed**, not instant user-initiated money movement.
The original provider instance and existing refund machinery are reused. Provider
failure keeps the frozen request retryable; it does not reactivate cancelled quota.
Provider settlement timing and existing pending-response behavior remain unchanged.
A 20% fee refund can be `PARTIALLY_REFUNDED` financially while the subscription is
fully cancelled. It cannot activate again or receive another automatic refund.

## Migration, testing and release

Migration `201_subscription_presales.sql` adds opt-in plan fields, nullable order
snapshots and a due-order index. It does not change existing terms, settings or
live sale state, and has no backfill. Apply frontend/backend together through the
normal authorized release; do not ship the UI against the old backend.

Verification includes unit refund/calendar/ownership/visibility tests, signed
resume and idempotency contracts, real disposable-Postgres concurrent purchase and
activation tests, frontend checkout consent/admin persistence/refund review tests,
and browser fixtures (no real payment requests).

This implementation does not automatically deploy, run production migrations,
publish plans, enable provider refunds, or send real payment/email traffic.
