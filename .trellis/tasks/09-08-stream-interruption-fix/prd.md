# Pre-content interruption rescue and missing media-type follow-up

The observed Aether incident opened HTTP 200 then recorded 503, with missing
Content-Type and skipped prefetch. Correlated Sub2API logs reported a missing
terminal event. Body capture was disabled, so fixtures reproduce the failure
shape, not the original response contents.

- Reuse the existing one-rescue budget for known pre-content EOF/read/missing
  terminal failures, including failures after heartbeat/header writes.
- Recognize native structured terminal errors from the Aether fix before protocol
  conversion, retaining one genuine final error on exhaustion.
- Do not replay after text/reasoning/tool/unknown activity, malformed partial
  output, cancellation, auth/policy/input/quota errors. Do not widen WS replay.
- Distinguish overload and interruption retry events in logs. No new config,
  environment variables, migrations, production actions or moved release tags.
- Tests: plain missing-terminal after heartbeat, mixed-error shared budget,
  Aether-shaped native errors, cancellation, post-content/tool refusals, protocol
  processors and full local regressions/race/lint.

Trellis scripts are absent; setup/guidelines were read and tracking is manual.
Developer: codex-agent. Base: fe970c045 (cafecode-v0.0.73).

## Local validation

- 28 top-level `TestStreamRetry*` tests across service and handler: passed.
- `go test -p 1 ./...`: all 40 default test-bearing packages passed.
- Focused `-race` service/handler tests: passed.
- `golangci-lint run ./...`: 0 issues.
- Six response processors each exercised both EOF and exact native Aether terminal
  errors after a heartbeat, with recovery and no failed preamble/error leakage.
- Added guards for EOF after committed preamble+text (no stale-buffer indexing),
  unforwarded policy/auth errors, numeric Chat timeout codes and hidden tool events.

## Publication

The user explicitly authorized commit/tag/push on 2026-09-08. Release target:
`cafecode-v0.0.74`, paired with Aether `backend-v0.7.115`. CI builds the release
image; production deployment is separate and has not been authorized by this
publication request. Existing published release tags remain unchanged.
