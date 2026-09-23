# Verification — presale review fixes

## Scope

Three reproduced review findings addressed without new infrastructure, runtime
configuration, dependencies, schema changes or edits to migration 201:

1. Shared checkout recovery now verifies the authenticated server order's plan,
   order type and UTC+8 month. Success carries and checks that order identity;
   mismatched recovery is preserved for its own flow. Restoring a matching pending
   order does not recheck new-purchase eligibility against its own reservation.
2. A failed/dismissed WeChat JSAPI payment keeps its original recovery until
   cancellation/expiry is confirmed. QR fallback cannot create a replacement when
   cancellation fails, remains pending, or races with payment success. The regular
   status-panel cancel button also checks the actual order rather than assuming
   every successful cancellation HTTP response means the order was cancelled.
3. User/admin refund quotes and cancellation use the same source-order entitlement.
   Legitimate early resets reduce remaining refundable days and the final-week
   cutoff, while the per-day denominator remains the purchased month. Subscription
   locking protects confirmation; unrelated/manual term changes remain blocked.
   The accepted full quote is saved in the existing transactional audit record,
   surviving cancellation, provider retries and later renewal of the same row.
   Older accepted audit records retain their original request-time calculation.

## Passed

- Full frontend Vitest: **1190 tests in 161 files passed**.
- `pnpm typecheck`: passed.
- `pnpm lint:check`: zero errors, 12 existing warnings; no unrelated cleanup.
- Backend focused unit regressions:
  `go test -tags=unit ./internal/service ./internal/handler ./internal/handler/admin
  -run 'TestPresale|Test.*Refund|Test.*EarlyReset' -count=1` passed (admin has no
  matching tests). Existing presale calendar, ownership/DTO, cancellation,
  fulfillment and refund tests remain green.
- New backend tests cover regular/custom early resets, stale reviewed amounts,
  actual final-week boundaries, frozen retries after later renewal, manual term
  changes and old audit compatibility.
- `golangci-lint run --timeout=5m ./internal/service ./internal/handler
  ./internal/handler/admin`: **0 issues**.
- Go formatting and `git diff --check`: passed.

## Initial environment-limited checks (superseded by publication checks below)

- The broader service/handler/admin unit command was attempted. Service and
  handler suites abort in unrelated HTTP-server fixtures because this sandbox
  prohibits listening on sockets (`httptest: ... socket: operation not permitted`).
  The full admin package passed. No test or production setting was changed to
  bypass the restriction.
- Added disposable-Postgres coverage exercising the real repository's early-reset
  CAS and refund row-lock path for regular/custom terms. The integration test
  package **compiled**, but its harness explicitly skipped execution because the
  Docker socket is inaccessible. Run the presale integration tests in normal CI
  before release; a zero harness exit here is not evidence of a database test pass.
- No live gateway, payment, email or browser end-to-end transaction was executed.

## Safety and artifacts

Logs and the sandbox-writable Go/lint caches are under
`/var/tmp/sub2api-presale-fixes/`. The original read-only Go cache could not accept
writes, so checks used a task-local cache instead. Trellis scripts are absent;
task and verification records were maintained directly.

During implementation there was no commit, tag, push, production/preview restart,
deployment, database migration, account mutation or production configuration change.
Existing concept assets, operational exports and unrelated untracked files remain
untouched.

## Authorized publication — cafecode-v0.0.85

- The user requested commit, tag and push. The initial restricted-session attempt
  could not write `.git` or reach GitHub; no refs changed. The user subsequently
  restored the execution environment and repeated the publication request.
- Remote `custom-prod` was confirmed at
  `142508b2708d0512bfde32c527816f108338ba5f`; the next tag `cafecode-v0.0.85` is unused.
  Publish only this task's source, tests and documentation using an annotated tag
  and an atomic branch/tag push, without rewriting any existing refs.
- With socket access restored, the full service/handler/admin unit packages passed
  (105.421s / 26.081s / 0.359s). Frontend source is unchanged since its 1190-test,
  typecheck and lint passes above.
- The real PostgreSQL run exposed a duplicate name in the new table-driven fixture
  and, after correcting the fixture, an actual custom-multiplier activation error:
  clearing and setting `custom_expires_at` in one Ent mutation generated duplicate
  SQL assignments. Activation now chooses either the regular clear branch or the
  custom set branch. No generated ORM or migration changes are necessary.
- The final disposable PostgreSQL/Redis run passed in **13.015s**, including
  concurrent reservation/activation and both regular/custom early-reset refunds.
  Final focused presale/refund/early-reset regressions passed after the activation
  correction (service 1.130s, handler 0.221s); final backend lint reported **0 issues**.
- Publication logs and ref verification are kept under
  `/var/tmp/sub2api-presale-publish-085/`; additional verification logs are
  `/var/tmp/sub2api-presale-fixes/backend-{unit,integration,focused,lint}-publish*.log`.
- GitHub SSH on port 22 times out here; use its authenticated `ssh.github.com:443`
  transport for these commands, with normal host-key validation. Do not alter the
  repository remote or global SSH configuration.
- The inspected tag workflow builds/publishes the container artifact and build
  record only. No production deployment, restart, migration, plan/provider setting
  change or real payment/email action is authorized or performed.
