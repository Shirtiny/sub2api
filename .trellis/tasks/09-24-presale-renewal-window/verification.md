# Verification

- Backend presale service/handler suite passed:
  `go test -p 2 -tags unit ./internal/service ./internal/handler -run 'Test.*Presale' -count=1`.
- Added five renewal regression tests covering boundary instants/timezones,
  mid-month starts, source-group/user isolation, expired service, paid current
  terms awaiting workers, transactional order rejection, and grandfathered orders.
- Frontend PresaleView and PaymentView: 106 tests passed in two files, including
  Chinese/English opening timestamps, disabled checkout, missing metadata fallback
  and eligibility refresh after returning to the page.
- Frontend typecheck and ESLint on changed Vue/TypeScript files passed.
- Backend golangci-lint on service and handler packages: zero issues.
- `git diff --check` passed.

Only local unit/component/static checks were run; no real payment gateway,
production database or disposable PostgreSQL integration was used this turn.
No deployment, preview restart, migration, configuration, commit/tag/push, or
historical order changes. Unrelated untracked files left untouched.
