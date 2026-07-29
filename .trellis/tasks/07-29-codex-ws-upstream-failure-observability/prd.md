# Aether Codex WebSocket Upstream Failure Observability

## Incident

On 2026-07-29, Codex WebSocket request `ws-606bfba9-0574-54d1-ae6b-26d8c98e25fc` failed with an externally visible HTTP 502 and the generic message `official Codex WebSocket protocol failed`. The stored result did not contain the underlying WebSocket transport or protocol reason.

Read-only diagnosis showed that the same bound session succeeded before and after the failure, while one official Codex key produced repeated protocol failures. The failure was therefore consistent with a transient upstream WebSocket path rather than a client request validation error.

## Ownership Boundary

Aether remains a transport and scheduling intermediary for this path. It does not replay an official Codex request after provider execution may have started. Retry ownership stays with the official Codex client.

The client-visible failure contract is:

| Failure source | WebSocket error status | Error type | Error code | Retry owner |
|---|---:|---|---|---|
| Client protocol violation | 400 | `invalid_request_error` | `websocket_protocol_error` | None |
| Official binding, transport, or protocol failure | 502 | `server_error` | `upstream_websocket_error` | Official Codex client |
| Official first-byte, read, or total timeout | 504 | `server_error` | `upstream_timeout` | Official Codex client |

An upstream failure detected while the connection is idle does not enqueue a synthetic error for the next turn. Aether logs the failure and closes the downstream WebSocket so the official client observes a closed connection and reconnects on its next request.

## Root Cause

Several upstream failure paths used the same synthetic 400 event as client protocol failures. The official Codex client maps an ordinary 400 WebSocket error to a non-retryable invalid request, so transient upstream failures could be incorrectly attributed to the client.

The relay also collapsed low-level receive errors and official Close frames into generic messages. Close code/reason and Tungstenite receive/send/flush/close errors were therefore unavailable in operational logs.

## Implementation

Aether commit `2786588a14c546e36e2494266168f15cd0fb2f9b` (`fix(gateway): classify Codex WebSocket upstream failures`) changed only:

- `apps/aether-gateway/src/codex_ws/runtime.rs`
- `apps/aether-gateway/src/codex_ws/session.rs`

The implementation:

- preserves official Close code and reason;
- preserves low-level receive, send-readiness, send, flush, and close errors;
- distinguishes client and upstream failures during binding, idle, and active response phases;
- returns neutral public error codes that do not expose the intermediary name;
- emits a retryable 502 or 504 only when a request is active;
- closes an idle failed binding without leaving a stale 400 event for the next turn;
- does not add an Aether request retry loop.

The existing route-control event contract was not renamed because it is a separate compatibility-sensitive control protocol, not a `type:error` response.

## Structured Logs

Protocol and transport failures emit one warning event per failed attempt:

```text
event_name=codex_ws_official_protocol_failed
status_code=502
request_id=<ws request id>
key_id=<provider key id>
model=<model>
protocol_phase=binding|idle|response
protocol_reason=<bounded internal reason>
transport_detail=<underlying WebSocket detail or none>
```

Timeouts emit:

```text
event_name=codex_ws_official_timeout
status_code=504
request_id=<ws request id>
key_id=<provider key id>
model=<model>
timeout_type=<first-byte|read|total error type>
timeout_reason=<timeout reason>
```

Successful requests, normal streaming frames, and normal Ping/Pong traffic do not emit these events. An official client retry creates a new attempt, so a persistent failure can produce one log event for each retry attempt.

The events do not include request bodies, response bodies, API keys, tokens, email addresses, or user content. `transport_detail` is not explicitly truncated by Aether in this commit; established WebSocket and Close details are normally short, but a future hard cap may be added as defense in depth.

## Verification

The following checks passed in `/opt/stacks/aether`:

```text
cargo check -p aether-gateway --lib
cargo test -p aether-gateway codex_ws::session::tests:: --lib  # 35 passed
cargo test -p aether-gateway codex_ws::runtime::tests:: --lib  # 16 passed
cargo test -p aether-gateway codex_ws:: --lib                   # 100 passed
git diff --check
```

Coverage includes active receive errors, invalid official binding state, idle upstream failure, timeout logging, neutral public errors, provenance mismatch, and preservation of Close code/reason.

## Delivery Status

- Aether branch: `custom`
- Aether commit: `2786588a14c546e36e2494266168f15cd0fb2f9b`
- Pushed remote: `origin/custom`
- Production deployment: not performed
- Service restart: not performed
