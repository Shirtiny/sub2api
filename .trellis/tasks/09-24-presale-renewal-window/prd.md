# Presale renewal window

## Requirement
The first two weeks of a current subscription must not allow next-month renewal.
For calendar-month terms starting on the first, renewals open on the fifteenth
at 00:00 Asia/Shanghai. Continue selling only the next calendar month.

## Implementation boundaries
- Share the eligibility rule between the landing quote and user-locked creation.
- Scope to the same user/source group, including sibling plans and paid current
  terms awaiting fulfillment/activation. Use term start, not payment/worker time.
- Preserve existing accepted orders and their payment recovery/fulfillment.
- New users, unrelated groups and expired/cancelled terms are not active renewals.
- Show the opening timestamp on the disabled card in Chinese/English.
- No new configuration, schema, deployment or historical order changes.

## Validation
Test fourteen-day boundaries, timezones, mid-month legacy starts, group/user
isolation, expired/refunded terms, pending activation, transactional validation,
existing payment compatibility, localized card state and refreshed eligibility.
