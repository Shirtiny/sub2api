# Verification

## Implementation
- The newest order is the reservation attempt for a user/source group/month.
  Existing payment user locks protect creation and fulfillment. Retired late-paid
  failures/refund requests no longer poison the newer attempt; even cancelling or
  refunding the replacement cannot resurrect the older attempt. No paid facts are
  removed. The newest of existing mutually failed paid orders can be retried.
- Presale reservation audits snapshot membership points. Successful refunds
  reverse actual returned money, capped at the recorded grant and remaining
  points, atomically with terminal status and a strict audit. Legacy grant audits
  use their original paid amount. Paid-but-unreserved failures deduct nothing.
  Failed/refund-request/repeated-finalization paths do not double-deduct. Auth
  caches are invalidated after commit; balances and third-party refund mechanics
  are unchanged.
- Normal activation delay keeps the 20% fee without inventing used days. Actual
  worker errors are audited for failure-refund eligibility; later successful
  activation ignores that old failure. Entirely missed terms and accepted legacy
  refund quotes remain supported.
- Chinese/English admin audit names added. Corrected the existing PostgreSQL
  early-reset test's outdated no-fee expected amounts to the already-established
  20% daily-refund policy (224 accounting / 179.2 gateway).
- Finding 3's affiliate redemption handler/service/repository/frontend unchanged.

## Checks
- Backend expanded regression selection: 61 top-level tests / 157 including
  subtests passed before the final two added cases and final lint refinement.
- Frontend focused suite: 8 files / 162 tests passed.
- Frontend typecheck and changed-locale ESLint passed.
- Backend server binary built locally, not executed or installed.
- PostgreSQL integration package and new concurrency tests compiled, but the
  harness skipped execution because Docker is unavailable in this sandbox.
  This is NOT a passing PostgreSQL runtime/concurrency claim. CI should execute
  `go test -tags integration ./internal/repository -run '^TestPresalePostgres'`.
- Final presale suite: 38 top-level tests / 129 including subtests passed,
  including repeated full-refund membership cycles and activated daily refunds.
- Backend lint on service, handler, admin-handler and repository packages: zero
  issues. Fixed its unused initial-quote warning by reusing the base quote when
  no activation-failure lookup is necessary. `git diff --check` passed.

Logs: `/var/tmp/presale-safety-{regressions.jsonl,final-tests.jsonl,lint.log,
postgres.log,frontend-tests.log,typecheck.log,frontend-lint.log,build.log}`.
Build tooling used an existing writable `/var/tmp` Go cache because the default
cache is read-only; no repository/runtime configuration was introduced.

## Safety
No production actions, image pulls, real payment/email traffic, shared test-app
changes, new configuration, migrations, or historical backfills. The user later
authorized committing, tagging and pushing these fixes; this does not authorize
production deployment. Unrelated existing untracked files left untouched.
