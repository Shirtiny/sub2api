# Cancelled-stream billing preservation

## Regression and contract

The incident identified by request prefix `ba04a112` completed at Aether while
Sub2API logged client cancellation and never reached usage/billing persistence.
A local mock-SSE reproduction confirmed that the existing processor drains a
detached upstream and returns valid usage with nil error, but the newly added
retry wrapper changes this to context.Canceled and the handler exits early.

- Preserve an existing successful, non-nil forwarding result on downstream
  cancellation/deadline/write failure; do not attempt another write or retry.
- Do not promote an upstream failure, captured retryable error, rejected HTTP
  status, or absent forwarding result to billing success.
- Keep the existing pricing/subscription and transactional billing-dedup paths.
  Do not estimate new usage or introduce a second billing path in the wrapper.
- Preserve duration/first-content timing across rescue attempts and mark retained
  results as client-disconnected. WebSocket bypass behavior remains unchanged.
- Cover cancellation and first-content/heartbeat/final-flush write failure,
  six processors, post-rescue timing, upstream error boundaries, detached handler
  usage submission and wallet/subscription billing command construction.

No new configuration, environment variables, migrations, provider-pool changes,
production update or historical charge repair. Local code and tests only unless
the user separately authorizes publication/deployment. Trellis scripts are absent;
workflow and backend/cross-layer guidelines were read, with manual tracking.

## Validation results

- 8 new top-level tests passed, including six processors under cancellation,
  first-content/heartbeat/post-content write failure, plus final-flush failure.
- Wallet and subscription tests reach the real `RecordUsage` implementation with
  mock repositories, verifying one billing command and one usage write, original
  client idempotency key, payload fingerprint and uncancelled billing contexts.
- Handler tests verify detached usage submission through both worker-pool and
  synchronous fallback paths, preserving original client/server request IDs.
- `go test -p 1 ./...`: all 40 default backend test-bearing packages passed.
- Focused `-race` service/handler checks: all 36 `TestStreamRetry*` tests passed.
- Full `golangci-lint run ./...`: 0 issues; gofmt and diff whitespace checks passed.

The implementation restores the processors' existing successful billing outcome;
it does not reinterpret failed/partial upstream results, invent usage, increase
retry limits or add a separate settlement route. Historical charges are untouched.
The original implementation remained local and uncommitted/unpublished.
2026-09-23 follow-up: the owner requested consolidation into `custom-prod`;
publication of the unified source is in scope, production deployment is not.
