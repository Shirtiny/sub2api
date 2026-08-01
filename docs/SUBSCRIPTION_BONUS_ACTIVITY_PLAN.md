# Subscription Bonus Activity Plan

## Goal

Add a reusable promotion-activity capability to the subscription-plan administration page. The first activity type grants extra subscription days to eligible users.

The initial business rules are:

- Administrators configure activities from `/admin/orders/plans`.
- An activity has an inclusive start time and exclusive end time: `[starts_at, ends_at)`.
- An activity explicitly targets one or more subscription plans, with a configurable bonus-day value per plan.
- `max_uses_per_user` is shared across all plans in the same activity. For example, a limit of `1` means a user can receive the activity benefit once regardless of which eligible plan they buy.
- Activities of the same type cannot overlap for the same plan.
- Eligibility is decided when an order is created. A successfully created order reserves the benefit.
- Cancelled, expired, or provider-creation-failed orders release the reservation.
- Successful subscription fulfillment grants the benefit and permanently consumes one use. Refunds do not restore the use.
- A paid order that never reached subscription assignment releases its reservation when the refund succeeds; a granted order remains consumed after refund to prevent repeat-purchase/refund abuse.
- Existing purchases made before the activity do not count against the activity.
- Bonus days are fixed per purchase and are not multiplied by a custom subscription multiplier.

## Data Model

### `promotion_activities`

Stores the common activity definition.

| Column | Type | Notes |
| --- | --- | --- |
| `id` | bigint | Primary key |
| `name` | varchar(100) | Administrator-facing name |
| `activity_type` | varchar(50) | Initially `subscription_bonus_days` |
| `enabled` | boolean | Disabled activities never match |
| `starts_at` | timestamptz | Inclusive |
| `ends_at` | timestamptz | Exclusive |
| `max_uses_per_user` | integer | Positive |
| `created_at` | timestamptz | Audit timestamp |
| `updated_at` | timestamptz | Audit timestamp |

The display status is derived rather than stored: disabled, scheduled, active, or ended.

### `promotion_activity_plans`

Stores the subscription-plan configuration for the activity.

| Column | Type | Notes |
| --- | --- | --- |
| `id` | bigint | Primary key |
| `activity_id` | bigint | Activity reference |
| `plan_id` | bigint | Subscription plan reference |
| `bonus_days` | integer | Positive, maximum 36500 |
| `created_at` | timestamptz | Audit timestamp |
| `updated_at` | timestamptz | Audit timestamp |

`(activity_id, plan_id)` is unique. V1 requires explicit plan selection so newly created plans do not unexpectedly join a running activity.

### `promotion_activity_participations`

Stores benefit reservations and grants.

| Column | Type | Notes |
| --- | --- | --- |
| `id` | bigint | Primary key |
| `activity_id` | bigint | Activity reference |
| `user_id` | bigint | Participating user |
| `order_id` | bigint | Payment order, unique |
| `plan_id` | bigint | Purchased plan snapshot |
| `bonus_days` | integer | Benefit snapshot |
| `status` | varchar(20) | `reserved`, `granted`, or `released` |
| `reserved_at` | timestamptz | Reservation time |
| `granted_at` | timestamptz nullable | Fulfillment time |
| `released_at` | timestamptz nullable | Release time |
| `release_reason` | varchar(100) nullable | Lifecycle reason |
| `created_at` | timestamptz | Audit timestamp |
| `updated_at` | timestamptz | Audit timestamp |

Indexes cover `order_id` uniqueness and `(activity_id, user_id, status)` eligibility lookups. Reservation and grant counts are checked while holding the existing payment user row lock.

### Payment-order snapshot

Add these fields to `payment_orders`:

- `subscription_bonus_activity_id` nullable bigint
- `subscription_bonus_days` integer, default `0`

The snapshot guarantees that editing or ending an activity does not change an already-created order.

## Admin API

Add these authenticated routes below `/api/v1/admin/payment`:

- `GET /activities`
- `POST /activities`
- `GET /activities/:id`
- `PUT /activities/:id`
- `DELETE /activities/:id`

Example create/update payload:

```json
{
  "name": "August subscription bonus",
  "type": "subscription_bonus_days",
  "enabled": true,
  "starts_at": "2026-08-01T00:00:00+08:00",
  "ends_at": "2026-09-01T00:00:00+08:00",
  "max_uses_per_user": 1,
  "plan_bonuses": [
    { "plan_id": 1, "bonus_days": 10 },
    { "plan_id": 2, "bonus_days": 15 }
  ]
}
```

Validation rules:

- `ends_at` must be later than `starts_at`.
- `max_uses_per_user` must be positive.
- Every plan must exist, be a subscription plan, and have bonus days between 1 and 36500.
- The base plan validity plus bonus days must not exceed the subscription validity limit.
- Plan validity units accept both historical singular values (`day`, `week`, `month`) and the current UI values (`days`, `weeks`, `months`); the server normalizes them before applying the limit.
- Enabled activities of the same type cannot have overlapping time ranges for the same plan.
- While an activity has a `reserved` or `granted` participation, its type, limits, time range, plan set, and benefit values are immutable. It may still be disabled to stop new orders.
- An activity whose participations are all `released` may still be edited, but any activity with participation history cannot be hard-deleted. Administrators must disable it instead so activity, participant, and order-level history remain connected.
- A subscription plan cannot be deleted while it is referenced by an enabled, active, or scheduled activity. After an activity is disabled or ended, its association is detached transactionally before the plan is deleted; participation rows keep their plan and bonus snapshots for history.
- If a detached activity still has a reserved or granted participation, its immutable fields remain locked, but its name and enabled state can still be maintained without reattaching a deleted plan.
- A plan validity edit is rejected when it would make any enabled, active, or scheduled activity exceed the maximum after its configured bonus days are added.

## Admin Activity Records

The plan page exposes an **Activity Records** dialog beside **Activity Configuration**. History is retained and can be inspected in three levels:

1. Activity summaries with distinct participant count, total participation count, lifecycle-status counts, and total granted bonus days.
2. Participants grouped by user, including first/last participation and per-status totals.
3. Order-level participation records with order snapshots, plan, bonus days, reservation/grant/release times, and release reason.

Paginated admin endpoints:

- `GET /api/v1/admin/payment/activity-records`
- `GET /api/v1/admin/payment/activities/:id/participants`
- `GET /api/v1/admin/payment/activities/:id/participations`

The activity and participation endpoints accept `keyword`; activity summaries accept activity `status`; order-level records accept `user_id` and participation `status`.

## Checkout Contract

`GET /api/v1/payment/checkout-info` is authenticated and returns a user-specific optional benefit on each eligible plan:

```json
{
  "subscription_bonus": {
    "activity_id": 123,
    "days": 10,
    "ends_at": "2026-09-01T00:00:00Z"
  }
}
```

The field is omitted when the activity is disabled, outside its active interval, does not target the plan, or the user has reached the reservation/grant limit.

Order creation accepts an optional `expected_subscription_bonus_activity_id`. It is an optimistic-consistency token only; the server always resolves and validates the benefit. If a displayed benefit is no longer available, order creation returns `409 ACTIVITY_BENEFIT_CHANGED` instead of silently removing the benefit.

The WeChat payment resume flow preserves this expectation field.

## Order Lifecycle

```text
eligible checkout
      |
      v
order created -> reserved
      |             |
      |             +-- cancel / expire / provider create failure -> released
      v
payment accepted
      |
      v
subscription fulfilled -> granted
```

Order creation resolves the activity inside the existing payment transaction after locking the user. It snapshots the activity and days on the order and creates a reserved participation before commit.

Cancellation, timeout expiry, and provider creation failure release the participation alongside the existing Cafe-coupon release behavior.

If a released order is later reported paid, the service attempts to reacquire its original activity. If the user's limit is already occupied, the payment is not silently fulfilled without the advertised benefit; the order is audited as `SUBSCRIPTION_BONUS_REACQUIRE_FAILED` for refund or administrative handling.

Payment confirmation records the provider's paid amount, trade number, and timestamp before fulfillment can fail. A failed reacquisition therefore leaves a paid order in a retryable `FAILED` fulfillment state instead of losing the payment fact. Paid failed orders remain eligible for administrator refund. Orders whose expiry grace period has elapsed are not reopened and cannot reacquire a released benefit; the payment is recorded as `FAILED` with reason `PAYMENT_AFTER_EXPIRY` and is refund-only, including on duplicate webhooks.

Refund deduction uses the same total validity (`base + bonus`) for fulfilled subscription orders. A paid order that failed before subscription assignment has no subscription days to deduct, so refunding it cannot reduce an unrelated active subscription. `PAYMENT_AFTER_EXPIRY` is known to fail before any fulfillment, so neither balance nor subscription assets are deducted when it is refunded.

Terminal transitions use one transaction for the conditional pending-order update and reservation release. The payment user row is locked for order creation, reacquisition, and terminal cleanup; activity and plan rows are locked while activity definitions are checked or changed. This prevents a cancel/webhook race from releasing a benefit after payment wins and serializes overlap validation for the same plan.

Subscription fulfillment uses:

```text
total_validity_days = subscription_days + subscription_bonus_days
```

Subscription assignment, participation transition to `granted`, and the fulfillment audit record occur in one transaction. The unique order reference makes fulfillment retries idempotent.

Recommended payment audit actions:

- `SUBSCRIPTION_BONUS_RESERVED`
- `SUBSCRIPTION_BONUS_GRANTED`
- `SUBSCRIPTION_BONUS_RELEASED`
- `SUBSCRIPTION_BONUS_REACQUIRE_FAILED`

## Frontend

The plan administration view adds an `Activity configuration` button that opens a list dialog. The list supports create, edit, enable/disable, and delete operations. The activity editor contains common activity fields plus a subscription-plan table with one bonus-day input per selected plan.

The purchase plan card renders `+N days` beside the existing validity suffix only when `subscription_bonus.days > 0`. It uses the same color as the Cafe-coupon discount:

```html
<span class="text-[#3D2E2A] dark:text-[#F5C66B]">+10 days</span>
```

Both Chinese and English locale strings are required.

## Verification

Backend coverage includes:

- Activity validation and overlap detection.
- Start/end boundary behavior.
- Cross-plan per-user limits.
- Concurrent order creation for the same user.
- Reservation release on cancel, expiry, and provider failure.
- Order-snapshot stability after activity changes.
- Fulfillment and retry idempotency.
- Fixed bonus days for custom-multiplier plans.
- Late-payment reacquisition success and conflict paths.
- Late payments beyond grace, paid-but-failed refund eligibility, and terminal-transition races.
- Late-payment fact retention and refund-only retry protection.
- Release of an unfulfilled reservation after a successful refund.
- Singular/plural validity-unit normalization and plan-edit validity guards.
- Stale plan snapshots, detached activities with participation history, and bonus-aware refund deductions.

Frontend coverage includes:

- Admin list/editor validation and CRUD behavior.
- Eligible and ineligible plan-card rendering.
- Expected activity ID propagation into normal and WeChat-resumed order creation.

Deployment order is migration and backend first, then frontend. With no enabled activities, the feature is behaviorally inert. Rollback disables the activity while preserving existing reservations and audit history.
