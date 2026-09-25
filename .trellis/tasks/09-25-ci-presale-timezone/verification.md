# CI presale test regressions

## Diagnosis

- GitHub CI run `36092033549` failed in `test / Unit tests`. Frontend and Go lint passed. The separate `cafecode-v0.0.94` image-release run `36092033489` succeeded.
- Full remote logs require repository authentication, unavailable here. GitHub's public job API confirms the failing step; local reproduction supplies the precise assertion failure.
- All local unit packages passed in the host timezone. Repeating uncached with `TZ=UTC CI=true` failed only `TestPresaleRenewalMultiplierChangesOnlyAtActivation/1x_to_3x`, at the daily-window timestamp assertion.
- The fixture retains a `time.Local` location while the database round trip returns `time.UTC`. They denote the same instant. Deep equality incorrectly treats their internal location representation as a usage-window change.
- Running the subsequent integration tier exposed additional test-only failures: the campaign concurrency/refund test still expected the old personal-coupon error; user-suite setup silently ignored a failed user cleanup due to the new restrictive campaign-use foreign key; group list assertions expected more than the requested page size once fixtures filled the first page.

## Fix

- For all three usage windows, require a non-nil persisted timestamp and exact `time.Time.Equal` instant equality. No tolerance is introduced; actual window resets still fail.
- Preserve all usage amounts, multiplier, activation and idempotency assertions.
- Update shared-campaign integration expectations to `CAFE_CAMPAIGN_USAGE_LIMIT`, retaining the single-winner and permanently-consumed-after-refund checks.
- In the existing disposable-database user-suite setup, delete campaign-use fixtures before deleting users and require every cleanup statement to succeed. Production foreign keys and financial records are unchanged.
- Check group-page lengths against the page cap while still asserting exact total-count increments, including the platform-filtered list.
- No production implementation, timezone configuration, payment behavior, migration or CI skip changes.

## Verification

Local logs: `/var/tmp/sub2api-ci-9736645/`.

- Renewal/multiplier targeted tests: five uncached repetitions each under UTC, Europe/Amsterdam and Asia/Shanghai passed.
- Full uncached UTC unit suite passed (`unit-fixed.log`), including the service package in 104.878s. Full Go lint passed with zero issues (`lint-fixed.log`).
- Original failing integration run saved as `integration-fixed.log` (before the follow-up integration corrections).
- Repository integration recheck passed in 15.321s (`repository-fixed.log`). Full uncached UTC integration suite then passed (`integration-final.log`), including the repository package in 17.307s and service package in 62.807s.
- Integration-tag changed-code Go lint passed with zero issues (`integration-lint.log`). `git diff --check` passed.
- Existing GitHub runs have not been rerun; these are local verifications of the corrected tests. The remote failure summary supplied by the user identifies the same service package; surrounding account error simulations and Gemini INFO messages are not assertion failures.

No application build, preview/production update, migration edit, commit, tag or push performed. Testcontainers uses its own disposable PostgreSQL/Redis, not any configured app database.
