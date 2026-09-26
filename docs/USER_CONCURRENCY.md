# Effective user concurrency

The limit is shared by all API keys of a user, not granted separately per key.
Account/provider capacity, billing eligibility and RPM limits still apply
independently; a concurrency limit does not guarantee upstream capacity.

## Policy

1. Only active, already-started, unexpired subscriptions count. Pending presales,
   cancelled and expired subscriptions grant no concurrency.
2. Use the maximum subscription concurrency, never the sum. For overlapping terms
   of the same subscription, the latest started term wins. Custom subscription
   multipliers do not multiply concurrency. Legacy/admin subscriptions without
   a configured grant use 2.
3. Without an effective subscription, use the current account balance:
   - balance >= 100: 3
   - 20 <= balance < 100: 2
   - balance < 20: 1
   These are the balance units shown in the profile (not RMB recharge amounts,
   lifetime top-ups, or exchange-rate-converted values). Zero/negative balance
   still fails the separate billing eligibility check.
4. The persisted `users.concurrency` field is no longer an effective override or
   floor. Migration 207 normalizes existing non-deleted users and the registration
   default to 2. It changes no balances, plans, orders, usage counters or dates.
   Old per-user overrides/concurrency-code adjustments cannot bypass this policy.

## Enforcement and display

- `User.EffectiveConcurrencyAt` is used for profile/admin/ops and gateway limits.
- Repository auth/detail/list hydration includes effective grant windows plus
  subscription periods for legacy fallback; auth snapshot version 18 rejects old
  snapshots lacking those periods. Admin sorting follows the same policy.
- HTTP authentication resolves balance from the billing cache, with the existing
  DB fallback on cache miss. It never writes into an immutable auth-cache entry.
- Retained WebSocket turns re-evaluate time windows and balance before acquiring
  a user slot. Balance reads are cache-only; a missing entry requires reconnect
  for cold hydration instead of silently retaining an outdated limit.
- A reduced limit affects new slot acquisition, not cancellation of work already
  in flight. Billing-cache writes follow the existing asynchronous update path.
- The profile shows `effective_concurrency` with a bilingual question-mark tooltip.

## Release

Migration 207 is a one-time user-configuration normalization. Before an authorized
production update, save user concurrency/default-setting values with the normal
deployment backup. Do not reset production values separately ahead of the code
release. Updating the isolated test environment is not production authorization.
