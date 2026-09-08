# Sub2API pre-content overload recovery

## Requirements

- Protect streaming Responses, Chat Completions and Messages served by the
  OpenAI gateway (including passthrough and protocol-conversion paths).
- Hold only known empty opening/lifecycle events, with a 256 KiB cap. Heartbeats
  must bypass that opening buffer without publishing failed response IDs.
- Commit immediately on content, reasoning, tool starts, unknown/malformed data
  or overflow. Do not wait for the end of the response or add a lookahead timer.
- Recognize structured overload errors in HTTP failures, `response.failed`,
  `event: error`, and untyped JSON error payloads. Do not retry policy/auth/input
  errors or replay after substantive output/tool activity.
- At most one extra same-account forward per inbound request, inside the already
  acquired account slot; retain the request body/session identity. Exhaustion
  must not enter the existing outer same-account/failover loops. With Aether's
  two retries this bounds an all-overload chain at six upstream dispatches.
- Cancel before retry on disconnect. Failed hidden attempts do not reach normal
  usage settlement. Preserve genuine terminal errors after exhaustion/commit;
  never synthesize successful completion for an overload.
- No environment variables, migrations, production changes, or release tag.

## Implementation boundaries

Use a small Gin response-writer adapter around the existing forward call. It
tracks actual substantive output, separately from HTTP header/heartbeat writes,
and never overrides `Written` to pretend headers were not sent. The adapter
keeps the existing service conversions; converted Responses errors are detected
before conversion so they cannot be turned into a successful finish. Handler
failover guards use explicit safe-opening/exhaustion flags only from this layer.

## Verification

- Protocol classifier tests: whitespace/empty parts, heartbeat, queued/reasoning
  openings, tool starts, policy errors, split UTF-8/CRLF/multiline SSE, bounds.
- Writer tests: 4 KiB auto-flush, neutral heartbeat isolation, immediate content
  line release, no replay or suppression after content, writer errors.
- Real forward tests: delayed opening overload then success, HTTP overload,
  raw and converted routes, same account/request, one retry, cancellation,
  terminal-error preservation and successful-attempt-only usage.
- Existing focused service and handler regressions; race tests where practical.
- Commit only task files, preserve unrelated work. No production deployment.

## Session setup

Read AGENTS.md, Trellis workflow and backend/error/logging/quality/cross-layer
guides. The referenced `.trellis/scripts` are absent in this checkout; task
tracking is maintained manually. Developer identity: codex-agent.

## Verified implementation (2026-09-08)

- The adapter is installed around the existing forward closure for all three
  OpenAI gateway HTTP streaming entry points, inside the same acquired account
  slot. It covers raw and converted Responses/Chat/Messages processors.
- The explicit opening flag bypasses the legacy size guard only while output
  is still safe. An exhausted rescue cannot reenter same-account/candidate loops.
- Upstream error events are captured before conversion so a failure cannot become
  a successful message stop. Converted upstream tool/unknown activity prohibits
  replay even if the converter does not expose that event to the client.
- Auth, permission, input/policy and quota failures are not rescued, even when
  an inconsistent payload also contains the overload phrase. Upstream Retry-After
  is retained internally even when response-header filtering removes it.
- WebSocket transport keeps its existing replay/session logic; the new adapter
  is bypassed for that branch. No whole-response buffering or mid-output replay.
- The response status remains truthful after a heartbeat. A later HTTP error is
  finalized by the handler as one native SSE error, not bare JSON in an SSE body.
- Successful result timing includes both attempts/backoff, but only the final
  attempt's result reaches normal usage settlement. Failed response IDs remain
  buffered and are discarded before a rescue.

### Validation

Using the existing Go 1.26.6 toolchain, offline dependencies, `GOMAXPROCS=2`,
`-p 1`, and a 2 GiB soft Go memory limit:

- 20 focused top-level `TestStreamRetry*` tests passed, including six real
  protocol processors and loopback HTTP forward tests.
- `go test -p 1 ./...`: all 40 test-bearing backend packages passed (generated
  packages without tests are not included in this count).
- `go test -race -p 1 ./internal/service ./internal/handler -run TestStreamRetry
  -count=1`: both packages passed without races.
- `golangci-lint run --concurrency 2 --timeout 10m ./...`: 0 issues.
- Virtual-time real keepalive/4 KiB-buffer test: 47 seconds to the error plus
  500 ms retry backoff produces first content at exactly 47,500 ms, with no
  additional lookahead delay. The mock HTTP test also verifies that recovery
  does not wait for the failed upstream to close the connection.

Default backend tests were run locally, not production/integration database
containers. Logs are in `/var/tmp/sub2api-stream-retry/`. No production service,
configuration, database, migration or image was changed. No release tag was made.
