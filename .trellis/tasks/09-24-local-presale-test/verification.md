# Verification — local-only environment (2026-09-24)

## Result

Full homepage, presale, user console and admin console available at
`http://152.53.90.186:4178`. Isolated deployment artifacts and maintenance guide:
`/opt/stacks/sub2api-test/README.md`. Credentials are in that directory's
0600 `credentials.json`; none are stored in this repository.

Copied only a read-only allowlist of Netherlands configuration (7 non-user-owned
groups, 3 plans, site branding and purchase rules). No production users, orders,
API keys, upstream accounts, merchant credentials or SMTP configuration copied.
Two plans are public presale products; the third remains unlisted.

Internal Docker bridge contains only the three test containers. Outbound probe
fails; database and Redis have no published ports. Test API returns health OK;
all test containers healthy. UI and API are same-origin through the isolated
preview proxy. Registration and external email disabled; persistent TEST banner.

## Source changes

- Reuse existing guarded local auto-payment without modifying payment completion.
- Add similarly guarded refund simulation only for audited, already-paid,
  unbound development orders; normal refund lifecycle and audit still run.
- Fix the affiliate refund CTE projection: outer query used `source_user_id`
  without selecting it in the CTE, causing PostgreSQL failures even without
  affiliate accrual. No schema/data migration needed.
- Preserve pre-existing home billing/footer/button changes.

## Tests

- Payment/presale/refund focused service unit tests: PASS.
- New local refund guard and lifecycle tests: PASS.
- PostgreSQL regression `TestAffiliateRepository_ClawbackQuotaForOrder_NoAccrual`:
  PASS using dedicated testcontainers, not any production database.
- `golangci-lint` service + repository: 0 issues.
- Current backend compiled and running via test-only binary bind mount.
- Real API: no-pay presale purchase, correct next-month pending dates, duplicate
  purchase rejection, user refund request, admin refund, balance recharge and
  existing-subscription renewal quote: PASS.
- Real browser: login, presale checkout, immediate payment result, pending
  subscription, refund confirmation and submitted request: PASS.
- Browser admin plans (all 3), existing-subscriber renewal quote, fresh-user balance
  checkout and 390px mobile presale: PASS, no JS runtime errors or external requests.
- Fresh-user and renewal accounts have no purchased orders and remain ready for
  manual testing. Disposable QA account retains audited test orders, including a
  pending refund request for administrator UI testing.
- `git diff --check`: PASS.

Artifacts: `/opt/stacks/sub2api-test/artifacts/` (API checks, browser logs,
screenshots, builds, unit/integration tests and health evidence).

## Production safety and limitations

No production lifecycle, proxy, firewall, database or configuration changes.
Local production container IDs and start times match pre-test snapshots and
services are healthy. Netherlands access was read-only configuration export.
A historical non-production preview build was moved intact from full `/tmp` to
`/var/tmp` with a compatibility symlink, freeing space for health checks without
restarting production services. No commit/tag/push performed.

No real payments, model calls or email delivery are tested. Dates follow real
server time; no time travel/accelerated subscription activation. The test preview
is HTTP: use only test credentials/data. The proxy is detached and survives this
session; after host reboot run the documented restart-preview.sh. Containers
have restart=unless-stopped. Frontend edits can be applied using the refresh script.
