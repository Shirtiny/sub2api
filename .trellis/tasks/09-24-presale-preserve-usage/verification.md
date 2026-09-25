# Verification

- Removed forced zeroing of all three usage counters and clearing of all three
  window anchors from presale activation. Normal window/reset services unchanged.
- Expanded renewal regression: counters and anchors survive payment, activation
  and retries; the previous weekly quota remains enforced with its original reset
  timestamp. First purchases retain the usual zero/null defaults.
- Presale plus normal window-maintenance/limit unit regressions passed in service
  and handler packages. Command selector:
  `Test.*(Presale|CheckAndResetWindows|EnsureWindowMaintenance|ValidateAndCheckLimits|UserSubscriptionNeeds|CheckUsageLimits)`.
- Backend service/handler golangci-lint: zero issues. `git diff --check` passed.
- Logs: `/var/tmp/presale-preserve-usage-{tests,lint}.log`.

No new UI text, frontend behavior, configuration or schema changes in this task.
Earlier uncommitted renewal-window work is preserved. No production actions,
historical usage reconstruction, shared test-app restart, real payments, or
commit/tag/push. PostgreSQL integration was not run in this task.
