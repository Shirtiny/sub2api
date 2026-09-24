# Administrator order details

`/admin/orders` uses `components/admin/payment/AdminOrderDetail.vue` for its View
modal. The primary summary identifies the purchase type, product, multiplier,
customer, payment status and paid/due amount. Presales separately show entitlement
status and immutable start/end dates so completed payment is not mistaken for
immediate access.

## Data contract

`GET /admin/payment/orders/:id` returns the existing sanitized `order` and
`auditLogs`, plus a read-only `summary`:

- `currency` and `payment_mode` come from the stored provider snapshot. Missing
  legacy currency follows the existing CNY fallback. No provider credentials or
  full provider snapshot are returned.
- `plan_name` prefers the presale purchase snapshot (`plan_name_source=snapshot`).
  An ordinary historical order may use the linked plan's current name, explicitly
  marked `current`; deleted/missing records have no invented names.
- `group_name` and `provider_name` are current identification labels only. Their
  queries select ID/name, not live prices, quota limits or provider configuration.
- The existing admin list/detail order DTOs also include safe `currency`, allowing
  accurate cached-row display while the detail request is in flight.

Amounts, coupon discount, multiplier, concurrency, purchased days, promotional
days and presale terms come from order records, never a current plan configuration.
Balance credit is USD-denominated; payment amounts use the recorded payment
currency. The refund amount is explicitly labeled as a ledger amount, not inferred
as the actual gateway payout. This view performs no financial recomputation.

## Origin and audit

Purchase origin is separate from the payment channel. The view shows recorded
request host, client IP and referrer origin/path. Credentials, query strings and
fragments are omitted from displayed referrers. An absent, invalid or origin-only
referrer does not establish an exact entry page. The request metadata is not
presented as verified marketing attribution. Recorded ORDER_CREATED payment-source
metadata identifies standard checkout / in-WeChat resumption when present.

Payment provider/type/instance/mode and transaction numbers are shown independently.
Refund requests, failures and chronological lifecycle events remain visible;
payment deadline is not confused with subscription expiry. Audit details are
expandable, localized for common actions, and escaped as text (including unknown
legacy action/detail formats).

## Safety and failure behavior

No routes, schema, migrations, backfills or payment/refund execution rules change.
Metadata is added only to existing administrator-authenticated endpoints; user and
public order DTOs remain restricted. Query/audit failures return errors instead of
silently omitting detail. The UI retains the list record with an explicit error and
retry control; late results after switching/closing/unmounting cannot replace the
selected order. A refund dialog owns a separate order reference and cannot be
retargeted by a late detail response.
