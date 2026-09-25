# Implementation

- Entry: admin subscription plan management → Presale notice.
- Explicit audience checkbox includes restricted users and pending waitlist mailboxes; active existing accounts / approved unregistered waitlist users are default. Existing account status overrides historical approval; soft-deleted accounts excluded.
- Server-derived next Shanghai month, published plan prices and current gift campaigns; draft/test allowed before publication, formal sends fail closed until payment and presale plans are live.
- Reuses shared CafeShop letter and optional notification preferences. Frontend-domain links; no request Host/API-proxy origin. No external image/font/tracking assets.
- New migration 205: compact delivery receipts, atomic mailbox/month claim, actor + reviewed content version, no raw recipient address persisted. Distinct test cooldown, no formal budget consumption. Ambiguous/accepted deliveries never auto-retried.
- Browser-controlled sequential send, explicit confirm, stop on error, pause/close after current recipient, fresh content/month/audience version check per request. Individual To only, no exposed CC/BCC lists.
- Tests: service race/guards/monthly dedup/unsubscribe/URL+HTML, Postgres concurrent claim/audience/cooldown, admin route 401/403, frontend review/test/explicit-confirm/stale-response/stop-on-error, dark/light/mobile browser.
- Non-production app/frontend refreshed; no production deployment or plan publication. Test DB/Redis unchanged except additive 205 schema.
- User-authorized one-off sample accepted by smtp.resend.com on 2026-09-25 09:12:46 UTC, from the configured support sender to the user-designated administrator mailbox; the provider receipt is retained only in the protected operational artifacts. No broadcast. Delivery acceptance is not proof of mailbox arrival.
- Sample/audit artifacts: `/var/tmp/presale-notice-sample/`, remote `/root/backups/presale-notice-sample-20260925/`, isolated UI and database backup `/opt/stacks/sub2api-test/artifacts/presale-notice-20260925/`.

## Verification results

- Full backend unit suite: passed (`/var/tmp/presale-notice-full-unit.log`).
- Race-enabled presale notice service checks: passed.
- PostgreSQL recipient audience and concurrent/cooldown receipt tests: passed.
- Full frontend suite: 174 files / 1413 tests passed.
- Frontend typecheck/build and changed-file ESLint: passed.
- Browser preview: 1440px zh dark, 390px zh dark, 1440px en light, no overflow / runtime errors / email writes.
- Outbound sample: one authorized SMTP submission accepted; no automatic duplicate send.
- Final Go lint: 0 issues. Final isolated app healthy, restart count 0, receipt table empty (no UI mail send); PostgreSQL/Redis IDs and start times unchanged.
