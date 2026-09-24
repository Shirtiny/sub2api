# Calendar-month subscription presales

## Product and administrator configuration

The public `/presale` route is available without a session; checkout requires an
active signed-in account. Home navigation links to it. In the compact home header,
ordinary menu links fade out and the open-presale message fades in without changing
the navigation slot's dimensions. The message is only shown if the catalog is open.

`/admin/orders/plans` remains the source of products. A plan must be for sale,
presale-enabled, and belong to an active, non-deleted, ordinary
subscription source group. Existing plans **default to presale disabled**. Configure:

- `presale_enabled`: sell through the calendar-month presale flow (immediate purchase
  of that plan is rejected, including legacy clients).
- `presale_badge`: plain-text merchandising label, max 40 characters.
- Existing name, description, features, sort order, price, concurrency and group
  quota configuration supply the rest of the product card.

Publication and purchase use the same `for_sale && presale_enabled` rule; no extra
visibility switch is required. Legacy `presale_visible` values are ignored.
Reset cards follow official issuance, not a fixed plan-configured gift. The admin
API no longer accepts either retired setting and new orders do not copy the old
plan reset count. The catalog no longer advertises a fixed bonus. This does not add
an official-grant synchronization mechanism. Legacy columns and paid-order reset
snapshots remain for compatibility; activation and refunds honor those historical
snapshots without retroactively removing entitlements. No schema migration or
backfill is needed.

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

The shared payment recovery slot is not proof of a presale purchase. A presale
checkout restores it only after the authenticated order API confirms the selected
plan and UTC+8 month; success uses the same server-order identity. Unrelated
balance/plan/month recovery is left intact for its own flow. A resumed order does
not request new-purchase eligibility against the reservation it already holds.
After a native WeChat failure or dismissal, keep the original order and recovery
until the server confirms cancellation/expiry. Only then may QR fallback create a
replacement. Cancellation HTTP success alone is insufficient: it may mean the
original payment already succeeded. The status panel also verifies this state.

## Calendar and purchase contract

### Eligibility on landing cards

Signed-in visitors' cards query the existing read-only quote endpoint before
enabling Reserve. The server checks the source group and month (not merely a
matching plan ID), including unpaid orders that have not expired. Conflict errors
return only the matching user's `order_id`, `order_plan_id` and `order_status`.
Cards show reserved, payment-pending and refund states with subscription/order
links; coverage/legacy restrictions are explained inline. Network failures and
month changes require a retry rather than being treated as eligibility.

Login-return plan selections wait for these checks. The exact server-confirmed
pending order can still offer Continue payment when this browser has matching
recovery; checkout revalidates ownership, plan and month as before. Signed provider
return flows remain supported. Closing checkout or returning to the tab refreshes
card state. Creation-time validation and the payment user lock remain authoritative
against concurrent purchases after the page's read-only check.

### Service period

The business timezone is **Asia/Shanghai / UTC+8**, independent of host and browser
timezone. Repeated timezone captions are omitted from the UI, but date formatting
and service/refund boundaries still use this timezone. An order created in
September 2026 reserves:

- Start (inclusive): 2026-10-01 00:00 +08:00.
- End (exclusive): 2026-11-01 00:00 +08:00.
- Full-refund cutoff (exclusive): 2026-09-28 00:00 +08:00.

Each purchase covers the **next calendar month**, not 30 days from payment. A
multiplier changes quota/price under the existing custom-plan rules, not the number
of months. Legacy bonus-day activities are not combined with fixed monthly terms.
The date window, price/payment amount, name, concurrency, multiplier,
early-reset configuration and renewal relationship are snapshotted in the order.
Group quota/rate configuration continues to follow the existing live group model.
Later plan edits do not rewrite these order snapshots. Historical fixed reset
grants remain snapshotted; new presales carry no fixed reset-card bonus.

There is at most one live reservation per user/source group/calendar month. The
payment user row lock protects creation and fulfillment; pending orders also occupy
the slot. The newest order is the reservation attempt for that slot: creation can
only replace an attempt after it releases its reservation. Older attempts remain
retired even if their payment callbacks subsequently change their payment status.
A cancelled/expired order paid after a replacement is recorded as a paid
fulfillment failure, not a second subscription; it must not block the replacement
or reclaim the slot after that replacement is cancelled/refunded. Reconcile/refund
it through admin orders. Do not erase its payment facts. Existing mutually failed
paid attempts can be recovered by retrying the newest order and refunding the old
one; no historical data rewrite is needed.

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
being examined. An actual worker failure is recorded as
`PRESALE_ACTIVATION_FAILED` in the existing audit log, once per order rather than
on every retry. A successful later activation makes that old failure irrelevant
to new refund quotes. No reverse migration, account enabling or database cleanup
runs.

## Refund policy and safety

For new refund requests, the refundable payment base excludes the payment-channel
fee. Reconstruct that fee using the order's immutable `amount`, coupon discount
and `fee_rate`, rounding upward exactly as checkout does for the payment currency;
subtract it from `pay_amount` before applying the policies below. Do not use live
plan prices or channel settings. Accounting `refund_amount` still prorates the
original order amount; only `gateway_amount` determines the actual money returned.

- Before start minus 72 hours: unconditional refund of the refundable payment base.
- During the last 72 hours before start: 80% of that base returned (20% cancellation fee).
- After activation, before the final seven days: remaining **whole 24-hour days**
  divided by total purchased calendar-month days, then a **20% fee** is deducted
  from that prorated refundable amount (multiply by 0.8). No refund during the final seven
  days, including the exact seven-day boundary. Supported early resets shorten the
  remaining term and its final-week cutoff, not the original purchased-day divisor.
- A paid term that was never activated because fulfillment failed returns the
  refundable payment base without a cancellation fee. This requires a paid
  fulfillment failure, a recorded activation failure, or an entirely missed term.
  Merely reaching the start while the normal activation worker is pending does
  not prove failure: the 20% cancellation fee still applies, without deducting
  used days for service not yet activated.

A user request atomically freezes its timestamp and full quote in the existing
`PRESALE_REFUND_REQUESTED` audit event, changes the order to
`REFUND_REQUESTED`, and cancels only this presale term. For pending presales no
current unrelated subscription is deducted. For active terms the subscription and
this order's early-reset entitlement must still describe the same purchased term;
valid early-reset deductions are supported, but manual edits/extensions or a later
term still require support reconciliation. Lock the subscription before recomputing
the reviewed amount and cancelling it, so a concurrent reset cannot silently change
the refund. Its concurrency/early-reset entitlements
and unused reset grant are removed alongside the term. Refund requests prevent
future activation even if provider processing crosses the start date.
The activation audit records the reset count actually credited after the 1000-count
cap. Refund cancellation uses that credited count, not the larger advertised grant,
so reaching the cap does not cause an extra deduction from earlier reset credits.

Refund execution is **admin-processed**, not instant user-initiated money movement.
The original provider instance and existing refund machinery are reused. Provider
failure keeps the frozen request retryable; it does not reactivate cancelled quota.
Retries use the audited quote even if the subscription row now holds a later term.
Older accepted requests, with or without a full audited quote, retain their audited
amounts; do not retroactively deduct daily-refund/payment fees or reinterpret the
cancelled row's dates.
Provider settlement timing and existing pending-response behavior remain unchanged.
A 20% fee refund can be `PARTIALLY_REFUNDED` financially while the subscription is
fully cancelled. It cannot activate again or receive another automatic refund.

Successful presale refunds also reverse earned membership points in the same
user-locked transaction as the terminal refund status, with a
`PRESALE_MEMBERSHIP_REFUNDED` audit record and post-commit auth-cache invalidation.
The deduction is the actual gateway refund, capped at the order's credited points
and the user's remaining nonnegative points. Retained fees/used service retain
their points; nominal plan or coupon values do not inflate the deduction.
`PRESALE_RESERVED` snapshots the grant; older records fall back to the paid amount
that was credited atomically with that audit. Paid-but-unreserved failures deduct
nothing. A request or failed gateway attempt does not remove points, and retries
cannot deduct twice. No historical completed refunds or already-spent benefits
are automatically rewritten. Affiliate subscription redemption is unchanged.

## Migration, testing and release

Migration `201_subscription_presales.sql` adds opt-in plan fields, nullable order
snapshots and a due-order index. It does not change existing terms, settings or
live sale state, and has no backfill. Apply frontend/backend together through the
normal authorized release; do not ship the UI against the old backend.

Verification includes unit refund/calendar/ownership/visibility tests, signed
resume and idempotency contracts, disposable-Postgres concurrent purchase,
activation and early-reset/refund tests, frontend checkout recovery/cancellation,
consent/admin persistence/refund review tests,
and browser fixtures (no real payment requests).

This implementation does not automatically deploy, run production migrations,
publish plans, enable provider refunds, or send real payment/email traffic.
