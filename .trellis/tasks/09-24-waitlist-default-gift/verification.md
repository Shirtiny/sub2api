# Waiting-list default balance gift

## Contract

- First approval credits an existing account with the configured default balance.
- Credit, approval and the used `admin_balance` recharge history commit together.
- The history note is `候补名单通过赠送`; no paid recharge totals or affiliate rewards.
- New applicants receive their normal signup balance once at verified signup,
  recorded in the same transaction as consuming admission.
- Concurrent approvals, email retries and changed defaults do not repeat a gift.
- Zero defaults create no monetary record; invalid amounts fail without mutation.
- Existing approved accounts are not retroactively credited. No migrations needed.

## Checks

- Focused service/handler waiting-list tests passed before the session interruption.
- Expanded service/handler regression checks passed after resuming, including
  waiting-list auth, registration, signup defaults and OAuth email registration.
  Log: `/var/tmp/sub2api-waitlist-gift-unit.log`.
- Disposable PostgreSQL integration tests passed: `go test -p 2 -tags=integration
  ./internal/repository -run '^TestWaitlist' -count=1 -timeout=180s`.
  Includes concurrent approval, duplicate retries after spending/suspension,
  recharge-history failure rollback, closed signup, signup rollback/no double gift,
  and invalid/zero defaults. Log: `/tmp/sub2api-waitlist-gift-integration.log`.
- Administrator waiting-list component tests: 6 passed.
- Frontend typecheck and lint of both updated language files passed.
- Backend lint of service, repository and handler packages: 0 issues.
- Server compilation passed; verification binary only at
  `/var/tmp/sub2api-waitlist-server`, not installed into any running service.
- `git diff --check` passed. Initial verification made no commit, tag or push.
- Production and the separate preview deployment remain unchanged by this task.

## Authorized release follow-up

- User subsequently requested commit, tag and push, targeting `cafecode-v0.0.88`
  on `custom-prod`, together with the preceding pending presale refinements.
- Release gate: 170 frontend tests across 8 files passed; tagged backend unit
  checks for presale/refund/development-payment, waiting-list and signup passed.
- Unrelated concept assets, historical database backups and local test credentials
  are excluded. This release only pushes source/tag for remote artifact builds;
  it does not update any running production service.
