# Journal - shirtiny (Part 1)

> AI development session journal
> Started: 2026-04-11

---



## Session 1: Bootstrap Guidelines Initialization

**Date**: 2026-04-11
**Task**: Bootstrap Guidelines Initialization

### Summary

Filled Trellis backend/frontend guideline documents from actual repository patterns, then finalized and archived the bootstrap task.

### Main Changes



### Git Commits

| Hash | Message |
|------|---------|
| `0efe9009` | (see git log) |
| `a3a52b3f` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 2: Fix pool mode account top-level statuses

**Date**: 2026-04-12
**Task**: Fix pool mode account top-level statuses

### Summary

(Add summary)

### Main Changes

| Area | Description |
|------|-------------|
| Backend status semantics | Excluded pool mode accounts from top-level `rate_limited`, `overloaded`, and `error` runtime semantics |
| Repository filters | Updated active/schedulable/error/rate-limited queries to treat pool mode historical state as non-blocking |
| API output | Returned effective account status for pool mode accounts so admin UI reflects corrected backend semantics |
| Tests | Added targeted service and repository coverage for pool mode status handling |

**Updated Files**:
- `backend/internal/service/account.go`
- `backend/internal/service/ratelimit_service.go`
- `backend/internal/service/gateway_service.go`
- `backend/internal/service/openai_account_scheduler.go`
- `backend/internal/service/token_refresh_service.go`
- `backend/internal/service/antigravity_internal500_penalty.go`
- `backend/internal/service/account_usage_service.go`
- `backend/internal/service/openai_gateway_service.go`
- `backend/internal/repository/account_repo.go`
- `backend/internal/handler/dto/mappers.go`
- `backend/internal/repository/account_repo_integration_test.go`
- `backend/internal/service/account_usage_service_test.go`
- `backend/internal/service/openai_gateway_service_test.go`
- `backend/internal/service/openai_ws_ratelimit_signal_test.go`
- `backend/internal/service/account_test_service_openai_test.go`

**Validation**:
- Ran `gofmt` on touched Go files
- Ran `go test ./internal/service ./internal/repository ./internal/handler/dto`
- Ran targeted service/repository tests for pool mode status behavior


### Git Commits

| Hash | Message |
|------|---------|
| `88f4376f` | (see git log) |
| `0c91adff` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 3: Refactor pre-submit workflow and stabilize rate-limit tests

**Date**: 2026-04-12
**Task**: Refactor pre-submit workflow and stabilize rate-limit tests

### Summary

(Add summary)

### Main Changes

| Area | Description |
|------|-------------|
| Trellis workflow | Refactored `/trellis:finish-work` into an executable pre-submit validation workflow with deterministic change classification and required command selection. |
| Backend tests | Fixed OpenAI account test snapshot/rate-limit handling so pool-mode accounts still update in-memory reset state and usage snapshots without persisting local rate-limit state. |
| Backend policy | Fixed pool-mode + custom error code behavior so explicitly configured custom error handling still applies instead of being skipped. |
| UI adjustments | Included committed frontend payment/order view tweaks that were part of the same commit. |

**Updated Files**:
- `.claude/commands/trellis/finish-work.md`
- `.claude/commands/trellis/onboard.md`
- `backend/internal/service/account_test_service.go`
- `backend/internal/service/account_test_service_openai_test.go`
- `backend/internal/service/ratelimit_service.go`
- `frontend/src/components/payment/SubscriptionPlanCard.vue`
- `frontend/src/views/user/PaymentView.vue`
- `frontend/src/views/user/UserOrdersView.vue`

**Summary**:
- Added deterministic pre-submit rules for backend/frontend/build-affecting changes.
- Restored passing backend unit coverage around OpenAI pool-mode 429 handling and custom error-code policy behavior.
- Recorded the combined workflow, backend, and frontend updates from commit `496bf76279069e809a926db661f182ec6f23391f`.


### Git Commits

| Hash | Message |
|------|---------|
| `496bf76279069e809a926db661f182ec6f23391f` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 4: Merge main into custom-prod

**Date**: 2026-05-01
**Task**: Merge main into custom-prod

### Summary

(Add summary)

### Main Changes

| Area | Summary |
|------|---------|
| Merge | Merged `origin/main` (`48912014`) into `custom-prod` and created merge commit `a2bcb5ab`. |
| Conflict resolution | Resolved conflicts in backend service files, `.gitignore`, and `frontend/src/views/user/UserOrdersView.vue`. |
| Custom behavior preserved | Kept `custom-prod` Codex exhausted snapshot/extra runtime rate-limit behavior for non-pool OpenAI accounts while skipping pool mode. |
| Upstream behavior preserved | Kept upstream 429 reconciliation, `IsSchedulable()` sticky-session clearing, and upstream `canRequestRefund(row)` condition inside the commented restore block. |
| Validation | Conflict marker checks and focused backend tests passed; full `/trellis:finish-work` later stopped at `golangci-lint` because another parallel lint process was running. |

**Key Files**:
- `backend/internal/service/account_test_service.go`
- `backend/internal/service/account_usage_service.go`
- `backend/internal/service/admin_service.go`
- `backend/internal/service/gateway_service.go`
- `backend/internal/service/openai_gateway_service.go`
- `backend/internal/service/openai_ws_ratelimit_signal_test.go`
- `frontend/src/views/user/UserOrdersView.vue`
- `.gitignore`


### Git Commits

| Hash | Message |
|------|---------|
| `a2bcb5ab` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 5: Account pass-through toggle

**Date**: 2026-05-11
**Task**: Account pass-through toggle

### Summary

OpenAI API Key account passthrough now preserves Chat Completions format; frontend edit modal save path is covered.

### Main Changes

- Updated OpenAI Chat Completions forwarding so `extra.openai_passthrough=true` on API Key accounts sends the original Chat Completions body to the upstream `/v1/chat/completions` endpoint without converting to Responses.
- Added a backend regression test that verifies the passthrough request keeps `messages`, omits generated `input`, and targets the raw chat endpoint.
- Updated OpenAI Messages forwarding so the same account passthrough switch sends Anthropic Messages bodies directly to the upstream `/v1/messages` endpoint without converting to Responses.
- Added a backend regression test for Messages passthrough URL, authorization, body shape, response passthrough, and usage extraction.
- Added a stable test id to the existing OpenAI passthrough toggle in the edit account modal.
- Added a frontend component test that toggles passthrough and verifies `extra.openai_passthrough=true` is submitted.


### Git Commits

(No commits)

### Testing

- [OK] `env GOCACHE=/private/tmp/sub2api-go-build-cache go test -tags unit ./internal/service -run 'TestForwardAsChatCompletions_OpenAIAPIKeyPassthroughUsesRawChatEndpoint|TestForwardAsRawChatCompletions|TestAccount_IsOpenAIPassthroughEnabled'`
- [OK] `env GOCACHE=/private/tmp/sub2api-go-build-cache go test -tags unit ./internal/service -run 'TestForwardAsAnthropic_OpenAIAPIKeyPassthroughUsesRawMessagesEndpoint|TestForwardAsChatCompletions_OpenAIAPIKeyPassthroughUsesRawChatEndpoint|TestAccount_IsOpenAIPassthroughEnabled'`
- [OK] `npm test -- --run src/components/account/__tests__/EditAccountModal.spec.ts`

### Status

[OK] **Completed**

### Next Steps

- None - task complete

---

## Session 6: End-to-end Codex WebSocket transport

**Date**: 2026-07-17
**Task**: Add end-to-end Codex WebSocket transport

### Summary

Implemented account-controlled client WebSocket ingress in sub2api, direct local route-v1 Aether WebSocket transport, and an official-Codex-only native WebSocket connector in Aether with TLS fingerprint/profile parity. Added scheduling and reconnect migration, bounded backpressure, authorization/billing/quota/moderation fences, administrator controls, tests, and rollout documentation.

### Main Changes

- Added sub2api WebSocket passthrough, context-pool, shared, dedicated, HTTP bridge, and Grok bridge paths with bounded framing and backpressure.
- Added the performance-oriented direct local route-v1 WebSocket link from sub2api to Aether with account-level controls on both sides.
- Added Aether native official Codex WebSocket support with pinned profile and TLS behavior; other providers remain outside this scope.
- Added initial failover, later-turn reconnect/migration, versioned scheduler cache generations, quota lease closure, and final pre-dispatch fences for all transports.
- Added bounded usage finalization and moderation queue ordering safeguards.
- Added frontend account controls and the implementation/release plan document.

### Git Commits

- sub2api: `f4a25381b9ad6f7ede39ea60f8b990bba1da240f` — `feat(openai): add end-to-end Codex websocket transport`
- Aether: `dbf4bd82e7d91dfdfa55564da25931dfe700efe3` — `feat(codex): add native websocket transport`

### Testing

- [OK] `go test -p 1 ./internal/... -count=1`
- [OK] `go vet ./internal/...`
- [OK] repository unit-tag suites and scheduler race tests
- [OK] `openai_ws_v2` and focused service race tests
- [OK] frontend typecheck and 44/44 tests
- [OK] focused Aether connector, gateway, provider, runtime, and frontend suites
- [OK] `git diff --check` in both repositories
- [OK] benchmark: 64 KiB approximately 223 us / 19 allocations; 1 MiB approximately 3.0 ms / 20 allocations

### Status

[OK] **Completed and committed; no production deployment performed**

### Next Steps

- Before production enablement: drain pre-v2 sessions and complete the final v2 scheduler rebuild/readiness gate.
- Validate real official credentials/TLS capture, shared Redis across multiple instances, MySQL/PostgreSQL integration, staging proxies, and load evidence.
- Consider a durable moderation outbox as a later reliability enhancement.

---

## Session 7: Admin subscription stats, usage series, and window shifting

**Date**: 2026-07-21
**Task**: Add bulk reset-window shifting and a subscription statistics panel to `/admin/subscriptions`

### Summary

Added three admin capabilities: filter-scoped bulk shifting of subscription reset windows, a cross-plan quota/usage statistics panel, and per-subscription daily/weekly/whole-cycle usage rates backed by a new durable rollup table. Settled the daily-versus-weekly quota accounting question by replacing an ad-hoc coefficient with an explicit time horizon.

### Main Changes

- Added `POST /admin/subscriptions/bulk-shift-window` with dry-run preview. Only rows whose window start is non-null and whose group actually sets that limit are touched; rows whose shifted start would land in the future are skipped whole and counted separately; usage counters are never modified.
- Added `GET /admin/subscriptions/stats` returning remaining-today, remaining-this-week, an N-day consumable ceiling, per-plan breakdown, and daily/weekly usage-ratio rankings. Daily and weekly limits are not implicitly converted into each other; the third metric exposes the conversion as a selectable 1/3/7/14/30-day horizon, truncated by `expires_at`.
- Added `GET /admin/subscriptions/:id/usage-series` returning per-day, per-week and whole-cycle usage ratios, with derived denominators explicitly flagged.
- Added the `subscription_usage_daily` rollup table (migration 174), written incrementally by `DashboardAggregationService`, retained for 400 days and decoupled from `usage_logs`. No foreign keys, so history survives subscription deletion. Limit columns are per-day snapshots including `custom_multiplier`, so repricing does not shift historical denominators.
- Added the frontend shift-window dialog, the statistics dialog with drill-down, a shared `ratioToneClass` helper, a shared `createIdempotencyKey` util, and full zh/en locale entries.
- Added `docs/SUBSCRIPTION_ADMIN_STATS_AND_WINDOW_SHIFT.md` covering accounting decisions, the recompute hazard, rollout steps and the backfill SQL, plus a `.gitignore` allowlist entry for it.

### Findings

- Production `usage_logs` retains only about one day. This is not the application's 90-day `usage_logs_days` setting but the host script `/root/clean` (`SUB2API_KEEP_DAYS=1`, weekly cron). Its purge list is an explicit allowlist, so `subscription_usage_daily` is unaffected.
- No pre-existing data source could serve per-subscription history: `usage_dashboard_daily` is site-wide only, `*_daily_users` carries no cost, and `billing_usage_entries` is empty because that write path is not enabled.
- Rollup totals exceed the subscription counters for a few rows. This is correct: the rollup records actual spend while the counter records spend since the last reset, and administrators had reset quotas mid-window. Usage ratios can legitimately exceed 100%.
- The `3.5 x daily limit` figure previously used for manual estimates was really "about four days remaining until the weekly reset". It understates exposure by roughly 40%, since a $150/day limit is $1,050/week rather than $525.

### Bug Caught During Implementation

`recomputeRangeInTx` initially deleted the rollup range before rebuilding, mirroring the four sibling statements for `usage_dashboard_*`. Those tables are derived views of `usage_logs` and must roll back with their source; `subscription_usage_daily` is the opposite and exists precisely to outlive it. `UsageCleanupService` triggers `TriggerRecomputeRange` over the same range immediately after purging `usage_logs`, so one administrator cleanup would have permanently destroyed that period's subscription usage history. Both the incremental and recompute paths now use pure upsert; only the retention job may delete.

### Git Commits

- sub2api: `62285b4b` — `feat(subscriptions): 管理端订阅统计、使用率明细与窗口平移`

### Testing

- [OK] `go build ./...`, `go vet ./...`
- [OK] `go test -tags=unit` across service, handler, repository, server and config packages
- [OK] `go test -tags=integration` for the new `SubscriptionUsageDailySuite` (testcontainers, real migrations)
- [OK] Mutation-verified the regression guard: restoring the delete makes `TestRecomputeKeepsRollupAfterUsageLogsPurged` fail on the intended assertion
- [OK] `golangci-lint` exit 0; the six reported issues are all in untouched files
- [OK] Both hand-written SQL statements executed against the production database inside rolled-back transactions: rollup produced 276 rows across 116 subscriptions; the shift matched 54 rows with usage untouched and daily-limited groups unaffected; the future-guard skipped all 54 rows at +720h
- [OK] Migration verified idempotent by running it twice on a scratch database
- [OK] Frontend `pnpm typecheck` and `pnpm lint:check` (0 errors)
- [!] Frontend `pnpm test:run`: 840/843 passing. The three failures (`usePersistedPageSize`, `EmailOAuthButtons`, `HelpTooltip`) are pre-existing and touch no file changed in this session.

### Status

[OK] **Completed and committed; not yet deployed**

### Next Steps

- After deployment, run the backfill SQL in the documentation. Only the days still present in `usage_logs` can be recovered; earlier history is permanently gone, so the table needs a full subscription cycle before the series view is complete.
- Investigate raising `SUB2API_KEEP_DAYS` from 1 to about 3, so a container outage longer than a day cannot lose usage before the rollup captures it.
- Decide whether the shift-window double-click guard should move from the frontend in-flight check into a handler-held idempotency key.
- Consider consolidating the three remaining duplicate `createIdempotencyKey` copies in `api/keys.ts`, `api/user.ts` and `api/payment.ts` onto the new shared util.

---

## Session 8: Aether Codex WS upstream failure observability

**Date**: 2026-07-29
**Task**: Correct Aether Codex WebSocket upstream failure signaling and diagnostics

### Summary

Diagnosed an Aether Codex WebSocket request that exposed only a generic 502 protocol failure. Corrected failure ownership so client protocol violations remain 400, active upstream failures become retryable 502 responses, timeouts become retryable 504 responses, and idle upstream failures close the binding without leaving a stale error for the next turn. Aether does not replay the provider request.

### Main Changes

- Added structured warning logs with request, key, model, phase, protocol reason, and underlying transport detail.
- Preserved official Close code/reason and Tungstenite receive/send/flush/close errors.
- Removed the intermediary name from public `type:error` codes.
- Added binding, active-response, idle, timeout, and Close-detail regression coverage.
- Added the detailed task record at `.trellis/tasks/07-29-codex-ws-upstream-failure-observability/`.

### Git Commit

- Aether: `2786588a14c546e36e2494266168f15cd0fb2f9b` - `fix(gateway): classify Codex WebSocket upstream failures`

### Testing

- [OK] `cargo check -p aether-gateway --lib`
- [OK] Aether Codex WS session tests: 35/35
- [OK] Aether Codex WS runtime tests: 16/16
- [OK] Complete Aether Codex WS module tests: 100/100
- [OK] `git diff --check`

### Status

[OK] **Completed, committed, and pushed to Aether `origin/custom`; no production deployment or service restart performed**

### Next Steps

- Monitor `codex_ws_official_protocol_failed` and `codex_ws_official_timeout` after a user-approved deployment.
- Consider imposing a small hard cap on `transport_detail` as additional log-size defense.
