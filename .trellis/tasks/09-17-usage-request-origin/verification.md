# Verification — 2026-09-17

## Passed
- Middleware/host normalization: www vs trusted NL, spoofed forwarding headers, malformed/duplicate hosts, strict IP chain, IPv6, mapped IPv4, invalid trust config, host snapshot survives rewrite.
- Service host/IP snapshot tests for gateway/OpenAI; unit-tag GatewayServiceRecordUsage tests.
- Full repository and migration unit tests.
- Real isolated PostgreSQL/Redis repository integration: null/www/nl and IPv6 through sync/fallback/batch/best-effort creation and Ent/repository reads.
- Full handler, handler DTO, middleware and httputil packages.
- Frontend focused usage view/table tests: 32 pass. Typecheck passes, lint has 0 errors and 13 existing warnings.
- Full frontend run: 900 pass, 5 failures. All five failures reproduce against a pristine HEAD frontend extracted into /tmp/sub2api-origin-ui-baseline, confirming they predate this feature (page-size default, OAuth affiliate code, tooltip hover, two subscription-date expectations).

## Broader regression limitations
Full service package contains 8 failing top-level StreamRetry tests. The checkout already had unrelated edits in openai_stream_retry.go and untracked stream retry billing tests when this task started. They were preserved rather than mixed into this feature; source/usage-focused tests pass. Do not claim a clean full backend regression run.

Logs:
- /tmp/sub2api-origin-final-focused.log
- /tmp/sub2api-origin-unit-tests.log
- /tmp/sub2api-origin-full-tests2.log
- /tmp/sub2api-origin-lint2.log
- /tmp/sub2api-source-ui-full-tests.log
- /tmp/sub2api-origin-ui-baseline.log

## Release gate
See docs/USAGE_REQUEST_ORIGIN.md: additive migration, trusted proxy/header chain, and Cloudflare-origin trust must be checked before release. Production configuration was not edited and production migrations were not applied by this task. Historical rows are not backfilled. No commit/push was made (project workflow reserves commits for the user).

## Consolidation follow-up — 2026-09-23

- Unified the existing request-origin feature commit and latest cafecode-v0.0.79
  history in `custom-prod`; no NL-only release branch is needed going forward.
  Restored the standard alpha/beta/rc release-tag validation, without NL tags.
- Included the previously uncommitted cancelled-stream billing fix, with a
  separate upstream-failure guard so non-retryable overload/business errors
  cannot become successful billing results after cancellation. Overload retries
  remain disabled; pre-content interruption rescue remains bounded to one retry.
- Corrected stale test expectations for the public usage DTO, interruption-only
  retries, OAuth affiliate gating, tooltip delays, persisted page size and local
  date formatting. SQLite lease fixtures use UTC and quota-reset assertions use
  a deterministic test clock; production lease/quota behavior is unchanged.
- Final frontend suite: 150 files / 1073 tests passed, including the latest local
  GPT-6 display labels and wordmark refinements. Typecheck and isolated build
  passed; lint has zero errors and 12 existing warnings.
- Focused service/handler stream-billing race tests passed. Request-host/client-IP
  repository round-trip integration passed using isolated PostgreSQL/Redis.
  Full backend lint: 0 issues.
- The SQL tree is unchanged from the already deployed request-origin feature.
  No production deployment, restart, proxy edit, database change or backfill was
  performed. Build output is isolated from the live application/preview.
- Original working files/stash and test logs are retained under
  `/root/backups/custom-prod-consolidation-20260922T235441Z`. Unrelated SQL
  exports, operational history, artwork and stray files are not release source.
- Final `go test -p 2 -tags=unit ./...`: all packages passed. The earlier
  broader-regression limitations above describe the historical source task, not
  this unified revision. Full integration beyond the focused round-trip is left
  to remote CI; no full integration pass is claimed locally.
- After the full frontend pass, two brand scale constants and their matching
  assertions were refined (`.7` desktop / `.62` mobile). Re-ran the 46 relevant
  brand/home-view tests and scoped lint successfully; rebuilt isolated assets.
  No functional source changed after that final verification.
