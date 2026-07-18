# Codex WebSocket v1: Implementation, Configuration, Scheduling, and Performance Contract

Status: authoritative v1 handoff specification

Repositories:

- sub2api: `/opt/stacks/sub2api`
- Aether: `/opt/stacks/aether`
- reviewed Codex source: `/opt/stacks/openai-codex`

This document replaces all earlier WebSocket design drafts. An implementation or review AI MUST use this file as the v1 source of truth. In particular, v1 does not contain an HTTP/SSE bridge and does not contain provider fallback on the Aether WebSocket route.

## 1. Goal

Provide this path:

```text
Codex client
  -- Responses WebSocket --> sub2api
  -- local Responses WebSocket + route-v1 --> Aether
  -- pinned native Responses WebSocket --> official Codex
```

The design must preserve:

- account-level enablement in both sub2api and the Aether Codex pool;
- safe account switching at an explicit reconnect boundary;
- exactly-once terminal ownership and concurrency release, plus idempotent usage settlement;
- the reviewed Codex TLS, proxy, compression, and handshake profile;
- the existing HTTP/SSE path when the new route is disabled;
- bounded memory and bounded asynchronous work under slow Redis/database/client conditions.

Whole-chain performance is a release gate, not a later optimization.

## 2. Frozen Scope

### 2.1 Included

1. Codex client WebSocket ingress at sub2api.
2. A local sub2api-to-Aether WebSocket hop.
3. Aether native WebSocket egress to the official Codex endpoint.
4. Aether official Codex OAuth pool keys only.
5. sub2api account-level Aether route control.
6. Aether Codex pool key-level native WebSocket control.
7. Initial pre-dispatch account failover.
8. Later-step controlled reconnect, failed-route exclusion, migration limits, and generation fencing.
9. Multi-response-step connections with one response step in flight.
10. Per-step billing/RPM/concurrency admission and idempotent usage reporting.
11. The pinned Codex TLS/WebSocket profile, including direct, HTTP CONNECT, HTTPS CONNECT, native roots, and Codex custom CA semantics.

### 2.2 Explicit non-goals

The following MUST NOT be implemented or advertised in v1:

- native WebSocket support for any non-Codex provider;
- an Aether HTTP/SSE-to-WebSocket bridge;
- `provider-fallback` route-v1 capability;
- `X-Aether-WS-Allow-Provider-Fallback`;
- `allow_provider_fallback` as an active account option;
- transparent provider/account replacement after a step may have executed;
- response ID portability between accounts, providers, or connection epochs;
- sharing one official WebSocket between users;
- accepting a mutable `latest` TLS profile or a single JA3 hash as proof of compatibility.

Historical unknown JSON fields may be preserved for forward compatibility, but they have no v1 behavior. If a route-control frame claims `provider_fallback_used=true`, sub2api rejects it.

## 3. Reviewed Baseline

The pinned source contract is:

```text
Codex version:                 0.144.1
Codex commit:                  1f0566d3f59298d1bb88820a0d35294f1eeb07ea
tokio-tungstenite revision:    0e5b2d73aa18dd9f0a50ee9ff199d5aef7594186
tungstenite revision:          4fffad30fe373adbdcffab9545e9e9bf4f2fc19f
Rustls:                        0.23.36
rustls-webpki:                 0.103.13
rustls-native-certs:           0.8.3
rustls-pki-types:              1.14.0
tokio-rustls:                  0.26.4
aws-lc-rs / aws-lc-sys:        1.16.2 / 0.39.0
crypto provider:               aws-lc-rs
profile schema:                3
profile ID:                    codex-ws-0.144.1-linux-x64-rustls023-aws-lc-caenv1-wbufret256k1
official URL:                  wss://chatgpt.com/backend-api/codex/responses
OpenAI-Beta:                   responses_websockets=2026-02-06
upstream tungstenite patch:   aether-tungstenite-0.27-out-buffer-retention-v1
downstream tungstenite patch: aether-tungstenite-0.28-out-buffer-retention-v1
downstream Axum patch:         aether-axum-0.8.8-ws-retention-config-v1
write buffer:                  131072 bytes
maximum write buffer:          17825792 bytes
maximum retained capacity:     262144 bytes after a complete drain
```

Any dependency or Codex revision change requires a new immutable profile manifest and repeatable protocol/TLS capture review. Do not edit the meaning of the existing profile ID.

Schema 3 adds the upstream Tungstenite patch identity and the three write-buffer
constants to the account manifest. Aether also pins the downstream Axum 0.8.8
and Tungstenite 0.28 patches in the workspace dependency graph. The upstream
manifest and downstream dependency-lock tests are both required: one cannot be
used as evidence for the other. An account carrying the former schema-2 profile
is intentionally ineligible; after the schema-3 code is deployed, disable and
re-enable that account's Codex WS switch so the immutable schema-3 manifest is
installed.

## 4. Ownership Model

sub2api owns:

- downstream API authentication and user/API-key/group scope;
- the sub2api Aether account selection;
- user and sub2api account concurrency;
- sub2api billing and usage records;
- the canonical cross-connection session hash;
- migration count, exclusion, generation, and sticky deletion in Redis;
- the decision to emit the tested Codex reconnect signal.

Aether owns:

- its authenticated principal, access policy, model policy, wallet, RPM, and global admission;
- official Codex provider/key/endpoint eligibility;
- selection of an official Codex OAuth pool key;
- OAuth materialization and proxy selection;
- official TLS/WebSocket connection construction;
- Aether usage settlement;
- the proof that an attempted step was not executed before requesting reconnect.

sub2api never receives the real Aether provider credential, provider key ID, proxy secret, or official account identity.

## 5. Non-negotiable Invariants

1. One response step is in flight per physical connection.
2. A step binding is immutable after its provider-bound write starts.
3. No replay is allowed after an indeterminate or possible provider write.
4. Initial failover is allowed only before the first provider-bound business frame.
5. A later account change requires a route-v1 proof, an atomic Redis admission, and a physical Codex reconnect.
6. `response.completed` is valid only after a matching `response.created` on the same generation.
7. `response.failed`, `response.incomplete`, and top-level in-flight `error` are failure terminals and close the binding.
8. Duplicate or stale events are consumed and never re-billed, re-forwarded, or used to settle a newer step.
9. A client disconnect, downstream write failure, or idle timeout is not evidence that an account is unhealthy.
10. Idle sockets hold no user/account inference permit.
11. Provider terminal processing never waits for database usage writes. It may
    synchronously wait only for a bounded Redis lease/concurrency release that
    is required to make the next step safe (target <= 100 ms).
12. Usage capacity is reserved before provider execution and is bounded.
13. The normal delta path performs no database query, Redis operation, scheduling, task spawn, or full JSON decode.
14. Base router v2 and route-v1 breakers default on; reconnect migration defaults off.
15. Existing HTTP/SSE behavior is unchanged when a base breaker or either account-level switch is off.
16. Every `response.create`, including step 1, revalidates the sub2api
    authentication generation and Aether's shared global/catalog/key generations
    immediately before provider write.
17. Aether acquires provider and key concurrency for the final selected account on every step; a permit from one account or provider is never reused after switching.

## 6. Protocol Contract

### 6.1 Client to sub2api

The downstream request is a WebSocket upgrade to:

```text
GET /openai/v1/responses
```

The first frame must be a JSON `response.create` object and must include a model. Before policy, billing, concurrency, usage reservation, or scheduling, sub2api runs one bounded scanner over the entire object. It rejects duplicate top-level keys including escaped aliases, non-string routing fields, more than 128 top-level fields, keys over 256 decoded bytes, nesting deeper than 64, event types over 128 decoded bytes, and model/response identifiers over 256 ASCII bytes. The same validation runs for every later `response.create`; no parser may use first-key-wins or last-key-wins semantics.

The physical connection freezes the requested model. A later `response.create` may omit the model and inherit it, but it may not change it. A model change requires a new physical connection.

The one-in-flight gate executes before later-step billing, RPM, policy, usage metadata mutation, concurrency acquisition, or provider write.

### 6.2 sub2api to Aether handshake

An effective Aether-managed account adds exactly:

```http
X-Aether-WS-Control-Accept: route-v1
```

Aether returns exactly one value for each header:

```http
X-Aether-WS-Control: route-v1
X-Aether-WS-Capabilities: close-after-terminal,client-reconnect
```

Missing, duplicate, comma-list, unknown, or additional capability values fail the handshake before the first provider-bound frame. There is no provider-fallback negotiation header.

The local Aether hop deliberately does not perform address allowlisting, DNS or
public-host checks, TLS requirements, origin checks, redirect policy checks, or
system-proxy lookup. These accounts are administrator-managed and local
`http://`/`ws://` targets are supported. URL parsing still rejects unusable
syntax and schemes. For an effective Aether account, sub2api must dial through
the option-aware WebSocket dialer with `DirectNoProxy=true` and compression
disabled. It must not fall back to the generic dialer, environment proxy, or
per-account proxy. If the installed dialer cannot guarantee those options, the
route fails closed before the upgrade instead of silently changing transport
semantics.

All `X-Aether-WS-*` headers terminate at Aether and MUST NOT reach the official endpoint.

### 6.3 Step fence

sub2api overwrites the reserved `client_metadata.aether.sub2api_step_control` value for every step with:

```json
{
  "version": 1,
  "sub2api_step_correlation_id": "opaque-id",
  "sub2api_binding_epoch_id": "opaque-id",
  "sub2api_binding_generation": 1
}
```

Aether must echo the exact fence in an internal typed route-control frame. Client-provided reserved values never become trusted proof.

### 6.4 Route-control actions

`close_after_terminal` is accepted only for a terminal current attempt and takes effect after the current terminal.

`client_reconnect` is accepted only before provider output and only for one of these proof classes:

- prepared and not dispatched; or
- rejected before execution with `codex_official_ws.not_executed`, proof version 1.

Every `client_reconnect` frame also includes exactly one `middle_route_disposition`:

- `retain`: keep the selected sub2api Aether account healthy and preserve its sticky binding. A reconnect may select another internal Codex key, but the current client step never redials the same middle-route account;
- `exclude`: exclude the selected sub2api Aether account for the migration dwell window and delete its sticky binding.

`close_after_terminal` omits `middle_route_disposition` entirely. The disposition is part of the `control_id` identity and Redis idempotency identity; missing, unknown, changed-on-replay, or action-incompatible values fail closed.

The frame includes a unique `control_id`, exact step/binding fence, Aether step/attempt IDs, write state, execution disposition, reason, retry delay, and recommended action. Unknown fields, duplicate fields, oversized values, stale generations, or a reused control ID with different identity fail closed.

Official provider frames with the reserved top-level type `aether.route_control`, including escaped spellings, are never interpreted as trusted Aether control.

### 6.5 Terminal provenance

The relay binds the active response ID only from `response.created`. ID-bearing incremental/boundary events must match that active ID. Completed must match both the active ID and generation.

The settled-ID set is bounded. Stale/duplicate settled events are consumed before usage parsing. High-frequency ID checks use atomic snapshots; the gate mutex is limited to response boundaries and terminals and never covers a socket write or callback.

## 7. Account-level Configuration

Both layers must be enabled. Enabling only one layer is intentionally ineffective.

### 7.1 sub2api global switches

In the sub2api configuration:

```yaml
gateway:
  openai_ws:
    enabled: true
    force_http: false
    apikey_enabled: true
    responses_websockets_v2: true
    mode_router_v2_enabled: true

    aether_route_control_enabled: true
    reconnect_migration_enabled: false
    reconnect_signal_mode: unset
    max_migrations_per_session: 3
    migration_window_seconds: 600
    route_min_dwell_seconds: 30

    # Independent retained-connection caps; these include first-frame wait and idle bindings.
    max_ingress_connections: 10000
    max_ingress_connections_per_user: 64
    max_ingress_connections_per_api_key: 32
```

`enabled`, `apikey_enabled`, `responses_websockets_v2`, `mode_router_v2_enabled`, and
`aether_route_control_enabled` default true. They remain hidden global breakers: an
operator may explicitly set any of them false, and runtime eligibility then fails
closed. `force_http` and `reconnect_migration_enabled` default false;
`reconnect_signal_mode` defaults `unset`.

Configuration validation requires:

- Reconnect migration requires the base WebSocket route, router v2, Responses WebSocket v2, API Key WebSocket, and Aether route control.
- Reconnect migration requires the pinned `websocket_connection_limit_reached` signal mode.
- Migration count/window must be positive.
- Connection limits are non-negative; zero disables only that dimension.

There is no `provider_fallback_enabled` switch in v1.

### 7.2 sub2api Aether account

The account must be:

```text
platform: openai
type: apikey
status: active
schedulable: true
concurrency: > 0
credentials.base_url: local Aether HTTP/WS base, for example http://aether:8080/v1
credentials.api_key: Aether API key
```

Enable the account with:

```json
{
  "openai_apikey_responses_websockets_v2_mode": "passthrough",
  "openai_apikey_responses_websockets_v2_enabled": true,
  "aether_ws": {
    "schema_version": 1,
    "enabled": true,
    "required_control_protocol": "route-v1"
  }
}
```

Disable only this account route with:

```json
{
  "openai_apikey_responses_websockets_v2_mode": "off",
  "openai_apikey_responses_websockets_v2_enabled": false,
  "aether_ws": {
    "schema_version": 1,
    "enabled": false,
    "required_control_protocol": "route-v1"
  }
}
```

The admin UI exposes one Aether WS account switch and manages the mode mirror. It does not expose the hidden global breakers or a provider-fallback switch.

Operator path: in sub2api Admin -> Accounts, create or edit an `OpenAI / API Key` account and enable `作为 Aether WS 账号`. The same switch is available in bulk edit. The account's base URL remains the local Aether HTTP base such as `http://aether:8080/v1`; sub2api derives `ws://aether:8080/v1/responses` for the WebSocket upgrade. Do not enter the `/responses` suffix in the account base URL.

Use a dedicated Aether API key for this middle-hop account, and bind that key
to an Aether group containing only the intended official Codex OAuth pool.
Native WS capability filtering already excludes other provider types, but a
dedicated key keeps cold policy/billing context and operator observability
independent from unrelated Aether traffic.

**Mixed-group behavior is intentional.** Selecting an Aether-managed account is
not a promise that every connection or failover remains on Aether: a group may
also contain ordinary official WS/HTTP accounts, and the scheduler may switch
to them when transport, health, quota, or capacity requires it. The route-v1
handshake and the 16 MiB Aether payload fence apply only while the selected
account is actually Aether-managed. To guarantee the product chain
`Codex client -> sub2api -> Aether -> official Codex`, place only Aether-managed
accounts in the API-key/group used by that client (an **Aether-only group**).

Single-account editing sends `extra_patch`, not a stale full `extra` object:

```json
{
  "extra_patch": {
    "set": {
      "aether_ws": {
        "schema_version": 1,
        "enabled": true,
        "required_control_protocol": "route-v1"
      },
      "openai_apikey_responses_websockets_v2_mode": "passthrough",
      "openai_apikey_responses_websockets_v2_enabled": true
    },
    "delete": []
  }
}
```

The repository applies only explicitly supplied base columns and this patch in one transaction. A route-only edit must never rewrite credentials, status, schedulable state, rate-limit state, or last-used state from a stale form snapshot. Top-level deletion is explicit; `aether_ws` is deep-merged so unknown nested fields survive. Concurrent runtime-owned `extra` keys are not overwritten. Bulk edit also deep-merges `aether_ws` in one SQL statement and must not perform N+1 reads.

The equivalent single-account API call is:

```http
PUT /api/v1/admin/accounts/{account_id}
Content-Type: application/json

{
  "extra_patch": {
    "set": {
      "aether_ws": {
        "schema_version": 1,
        "enabled": true,
        "required_control_protocol": "route-v1"
      },
      "openai_apikey_responses_websockets_v2_mode": "passthrough",
      "openai_apikey_responses_websockets_v2_enabled": true
    },
    "delete": []
  }
}
```

This endpoint is administrator-only. For this local hop, performance is the
governing requirement: the runtime intentionally performs no Aether-address
validation or allowlist, DNS/public-host security probe, TLS requirement,
account/system proxy lookup, or separate capability probe. The Aether-account
path always requests direct, uncompressed dialing; inability to enforce those
options is a configuration/runtime error, not permission to use a slower
fallback path.

### 7.3 Aether global switch

The Aether system configuration key is `codex_ws`. Through the admin API:

```http
PUT /api/admin/system/configs/codex_ws
Content-Type: application/json

{
  "value": {
    "enabled": true,
    "native_codex_ws_enabled": true
  },
  "description": "Official Codex native WebSocket"
}
```

Both booleans default true when the `codex_ws` record or an individual field is absent. An explicit false is the emergency breaker; a malformed top-level value or present-but-malformed field fails closed. Deleting the record restores the on-by-default state. The admin UI does not expose this breaker. The process reads an atomic snapshot after initialization; a successful admin write or delete refreshes it immediately. A frame/step loop must not read this configuration from the database.

Aether uses process-wide bounded reporters, not one queue or worker per
connection. All queue/worker/timeout tuning variables and their implemented
defaults are:

```text
AETHER_CODEX_WS_USAGE_REPORT_QUEUE_CAPACITY=16384  # clamped to 10000..65536
AETHER_CODEX_WS_USAGE_REPORT_WORKERS=32            # clamped to 1..128

AETHER_CODEX_WS_SETTLEMENT_QUEUE_CAPACITY=16384    # clamped to 10000..65536
AETHER_CODEX_WS_SETTLEMENT_WORKERS=64              # clamped to 1..128
AETHER_CODEX_WS_SETTLEMENT_TIMEOUT_MS=2000         # clamped to 100..10000

AETHER_CODEX_WS_SLOW_SETTLEMENT_QUEUE_CAPACITY=4096 # clamped to 128..16384
AETHER_CODEX_WS_SLOW_SETTLEMENT_WORKERS=8           # clamped to 1..32
AETHER_CODEX_WS_SLOW_SETTLEMENT_TIMEOUT_MS=10000    # clamped to 500..30000
```

The usage item is a compact terminal outcome only. Candidate lease release,
sticky-renewer stop, candidate settlement, and health feedback occur before
usage persistence. Primary settlement receives one bounded attempt; an item
that exceeds its hard timeout is offered once to the independent bounded slow
settlement lane, whose retry also has one hard timeout. A full slow lane is
logged and rejected; it never creates an unbounded task or blocks the relay.

All three lanes retain compact IDs/status/usage and compact body-free plans
only. They never retain a request body, OAuth material, full terminal JSON, or
a live socket context. Required usage and primary settlement reservations
happen before provider dispatch; a full/closed required lane backpressures
admission instead of spawning a synchronous or asynchronous fallback. Capacity
must be reviewed with retained/in-flight connection ceilings and measured
settlement lag; increasing it is not a substitute for fixing slow storage.

Large-frame CPU work uses a fourth, process-wide bounded lane. Its complete
environment surface is:

```text
AETHER_CODEX_WS_LARGE_FRAME_CPU_WORKERS=<available_parallelism/4, min 1> # clamped to 1..64
AETHER_CODEX_WS_LARGE_FRAME_CPU_ADMISSION_CAPACITY=<workers*4>           # clamped to workers..256
```

Invalid, zero, or empty values use the implemented default. The admission
capacity is a strict running-plus-waiting bound acquired with `try_acquire`.
Ordinary parse/classify/materialize/delta work rejects when admission is full
and an admitted operation waits at most 250 ms for a worker. Provider send
acquires both permits synchronously after its final fence. Completed terminal
delivery is the exception: it polls outside the semaphore and may wait for
admission/worker only until its 5-30 second delivery deadline, so it neither
adds an unbounded waiter nor discards a terminal while bounded delivery budget
remains. Section 11 defines the exact ordering.

### 7.4 Aether Codex pool key

Only a key whose provider type is `codex` and auth type is `oauth` may be enabled:

```http
PUT /api/admin/endpoints/keys/{key_id}/codex-ws
Content-Type: application/json

{
  "enabled": true,
  "profile_id": "codex-ws-0.144.1-linux-x64-rustls023-aws-lc-caenv1-wbufret256k1"
}
```

Disable it with:

```json
{
  "enabled": false
}
```

The operation atomically merges existing key JSON and preserves unrelated capabilities/fingerprint fields. Enabling writes:

- `capabilities.codex_official_ws=true`;
- the immutable schema-3 `fingerprint.websocket_transport_profile` manifest,
  including the exact Codex/tokio-tungstenite/Tungstenite revisions, upstream
  retention patch ID, crypto provider, and 128 KiB/17 MiB/256 KiB write-buffer
  constants.

Disabling writes `capabilities.codex_official_ws=false` and preserves the pinned
profile record plus all unrelated JSON. It does not disable HTTP scheduling or
mutate `is_active`. Existing committed steps finish; later steps see the
shared global/catalog/key fence and reconnect before provider write.

Operator path: in Aether Admin -> Pool Management, locate an official Codex
OAuth key and use `启用账号级 Codex WS`; batch operation `启用 Codex WS` is
equivalent. The account action installs the immutable profile manifest
automatically; an operator must not hand-edit or substitute the profile ID.

The response distinguishes:

- `configured`;
- `profile_effective`;
- `runtime_eligible` (`null` when no concrete request was evaluated);
- `profile_id`;
- `runtime_state` (`request_scoped`, `profile_blocked`, `soft_draining`, or
  `hard_revoked`);
- separate machine-readable `profile_reasons` and `runtime_reasons`.

`profile_effective=true` proves only the static profile prerequisites. It must
never be presented as a successful request-time scheduling decision. A concrete
request is runtime-eligible only after all of:

- both global flags true;
- provider type `codex` and provider active;
- key auth type `oauth` and key active;
- account capability true;
- every immutable profile field matches;
- endpoint active;
- endpoint provider API format exactly `openai:responses`;
- HTTPS, host `chatgpt.com`, port 443;
- base path exactly `/backend-api/codex` (optional trailing slash only);
- no endpoint query/fragment/custom-path override;
- normal model, quota, circuit, proxy, and concurrency eligibility.

The Aether global switch, Aether key switch, and sub2api account switch are
dynamic admin changes and do not require a process restart. Aether mutations
advance the shared runtime fence before changing local/database state so other
instances fail closed rather than dispatching a later step with stale account
eligibility. The sub2api `gateway.openai_ws` YAML is static process
configuration; applying it is a separate operator-controlled
deployment/restart action and is not performed by this implementation task.

### 7.5 Activation order

Use this order in staging, then production only after explicit operator authorization:

1. Deploy code with both account switches and reconnect migration off; verify no stale explicit-off or malformed hidden global breaker remains.
2. For every account previously enabled with schema 2, disable then re-enable
   `启用账号级 Codex WS`; merely leaving the old switch on does not upgrade its
   immutable manifest.
3. Enable one Aether official Codex OAuth key and verify
   `configured=true`, `profile_effective=true`, `runtime_eligible=null`, and
   `runtime_state=request_scoped`.
4. Enable one sub2api Aether account.
5. Keep the default-on base route-v1 breakers on and migration off; run one-step and multi-step tests.
6. Enable reconnect migration only after the real Codex reconnect fixture passes.
7. Expand accounts gradually while watching CPU admission, connection latency,
   write-buffer retention, queue depth, and settlement lag.

For a guaranteed Aether chain, verify the client API key resolves to an
Aether-only group before enabling migration. A mixed group is a deliberate
best-effort pool and can legitimately leave the Aether hop during failover.

Emergency disable order:

1. Set sub2api `gateway.openai_ws.force_http=true` for the broad HTTP fallback, or disable only `aether_route_control_enabled` for this route.
2. Disable affected sub2api accounts.
3. Disable affected Aether keys.
4. Disable Aether global flags.

Changing configuration is not part of source-code implementation and requires explicit production authorization.

## 8. Scheduling and Migration

### 8.1 Binding epochs

A sub2api binding epoch fixes one Aether account and one middle connection. An Aether binding epoch fixes one official provider/key/endpoint/proxy/profile and one official physical connection.

Affinity is a ranking preference only. Health, explicit capability, auth, account state, model support, quota, circuit, and concurrency are hard filters.

### 8.2 Initial failover

sub2api may select another Aether account only when the first attempt is proven pre-dispatch:

- DNS/connect/upgrade failure;
- route-v1 negotiation failure;
- Aether typed `prepared/not_dispatched` route control for step 1;
- a selected account whose inference slot is busy (skip without health penalty);
- local credential materialization failure (exclude and report account failure);
- another explicitly fenced pre-provider-write failure.

Only `OpenAIWSInitialStepFailoverError` can restart the outer account-selection loop. A plain provider error cannot replay the connection's initial payload.

For a middle-route failure, or an Aether control with `middle_route_disposition=exclude`, the failed account is excluded for the current selection loop. For an Aether-internal key/catalog/concurrency change with `middle_route_disposition=retain`, the same Aether account remains eligible and the next physical middle connection returns to that local address. User concurrency and the pre-reserved usage slot are retained; the old account slot is released before selecting again. `retain` never deletes sticky state and never records a sub2api account health failure.

### 8.3 Later-step migration

After step 1, no outer-loop transparent replay is allowed. Aether may send `client_reconnect` only with valid not-executed proof. sub2api then performs the Redis admission before constructing or writing the synthetic Codex reconnect event.

If Redis is unavailable, the limit is exhausted, the generation is stale, or the control identity is invalid, no reconnect payload is emitted. The connection closes deterministically.

The tested signal is the official Codex `websocket_connection_limit_reached` error. No other error text/code may be configured without a pinned real-client fixture.

### 8.4 Canonical session identity

Migration uses only a hash of:

```text
group scope + user ID + API-key ID + exact session-id + exact thread-id
```

Header/body/`x-client-request-id` conflicts fail closed. Body projection is allowed only for the pinned Codex CLI fingerprint when dash-form headers are missing. Content hashes, prompt text, legacy underscore headers, and user-only fallback identities never authorize migration.

### 8.5 Atomic Redis transaction

The Lua admission atomically owns:

- migration window start;
- migration count;
- binding generation;
- failed account exclusion deadline;
- `control_id` idempotency identity;
- sticky account deletion.

For every admitted control it checks the expected generation and count, increments count/generation, stores the disposition in the control identity, and sets one relative window TTL. For `exclude` only, it also writes the exact exclusion deadline and deletes the sticky key. For `retain`, it does neither. Window and exclusion time come from Redis `TIME`, not an application-node clock; the key uses `PEXPIRE`, not application-calculated `PEXPIREAT`. A duplicate identical control returns the original count, generation, disposition, and exclusion deadline without incrementing or extending the deadline. A reused control ID with a different account/generation/disposition errors. A denied admission never deletes sticky state.

There is no process-local correctness fallback.

### 8.6 Aether pool changes

The global Codex WS switch, catalog switch, and per-key eligibility switch are
shared Redis/runtime-state records, not process-local cache authority. Each
record contains a stable/transition state, eligibility meaning, and an opaque
generation. Initial selection freezes the global, catalog, and key generations;
the final pre-write check reads all three in one shared-state MGET. Missing,
unstable, ineligible, unreadable, or generation-mismatched state fails closed
before the official write. This is the same authority on every Aether instance.

The shared catalog generation also binds the local planner snapshot. On first
observation of each new generation, an Aether instance invalidates its
minimal-candidate, provider-catalog, provider-transport, and scheduler-affinity
caches before planning. Cache load epochs prevent an in-flight load from the
previous generation from repopulating candidate or transport cache after
invalidation. The selected local scheduler epoch and shared Redis generation
are both rechecked before provider write; an old snapshot can never be
relabeled with a new generation.

Every relevant mutation uses strict distributed serialization:

1. acquire and renew the resource-specific distributed mutation lock;
2. publish a short-lived, ineligible transition with a new generation;
3. perform the database/local mutation and read the authoritative result back;
4. while still owning the lock, compare-and-set only the exact transition into
   a stable value with another new generation;
5. release the lock, or restore a restrictive state if lock ownership/CAS is
   lost.

Restrictive dynamic writes discovered by OAuth/runtime or health processing,
including a key becoming hard-ineligible, use this full locked transaction.
They must not use a process-local flag or a one-shot best-effort Redis
projection. Every meaning or stability change advances generation, preventing
ABA-compatible retained bindings. Redis HA and operational repair remain
release requirements; this protocol does not turn an unavailable shared state
into a permissive local fallback.

Hard hot-state blockers are the static dispatch authorities needed at the final
write boundary: key active/auth/capability/profile state and hard OAuth
invalidation. Access expiry, quota, adaptive circuit, and other request-time
conditions remain cold-scheduler eligibility inputs; they do not continuously
rewrite the hot fence.

An already provider-committed step is never interrupted or moved. Disabling a key is soft drain for current committed work and hard exclusion for later steps. Hard credential revocation closes the binding after the current safe boundary.

Account switching inside Aether remains restricted to eligible official Codex OAuth keys. Cross-provider switching is outside this WebSocket route.

### 8.7 Cold candidate planning

Only the first `response.create` on an Aether physical connection executes the
expensive access/model policy lookup path. It consumes the already parsed JSON
value directly; it must not serialize and parse the request again. For up to 16
eligible candidates, billing model context is fetched through one mandatory
batch repository call that preserves input order and missing entries. Memory,
SQLite, MySQL, and PostgreSQL repositories implement the batch API; a silent
fallback to 16 single-item calls is forbidden.

Later response steps still run authentication-fence and RPM checks, but the
connection's `cold_policy_validated` fence skips policy repository/database work.
Thus later steps perform zero policy DB queries. Reconnect migration establishes
a new physical Aether connection and therefore runs cold policy once for that
new connection.

Candidate transport materialization uses one provider batch read, one endpoint
batch read, and one key batch read for the candidate set. Results remain in
candidate order and missing/invalid entries are isolated; there is no
per-candidate transport query. Official-only hard filters, endpoint/profile
eligibility, and proxy compatibility all run before the 16-candidate connection
attempt cap.

Proxy selection is resolved exactly once for each actual pool key during
preflight. The same resolution supplies both the connector route and the
settlement/reporting topology. Direct, reviewed HTTP CONNECT, reviewed HTTPS
CONNECT, and a manual proxy node with an explicit `http://` or `https://` URL
are supported. SOCKS, tunnel mode, invalid URLs, and unsupported schemes are
removed before the 16-candidate cap. The execution/lifecycle plan retains only
`enabled`, `mode`, and `node_id`; URL, credentials, label, and extra proxy data
are not retained or logged. After the connector consumes an authenticated proxy
route, the idle candidate no longer owns that secret route.

Sequential initial connects share one aggregate deadline. Its duration is the
saturating sum of the eligible candidates' individual connect budgets, clamped
to at least 1 ms and at most 60 seconds. A later candidate never receives a new
60-second window after the aggregate deadline has expired.

### 8.8 Owned request-body materialization

Candidate planning and idle bindings are body-free. Only the selected candidate
materializes the current `response.create`. Aether moves the owned JSON value
through model directives, normalization, service-tier/body filtering, and
provider-body patching in place, then serializes the resulting provider body
once. It must not retain or clone the complete original tree for reporting or
settlement.

Because the owned path deliberately does not keep a second original tree, an
enabled WS body rule whose condition uses `source=original`, including nested
conditions, is ineligible and fails closed before dispatch. Disabled rules do
not block the account. Supported rules must evaluate against the current owned
body/header inputs. This restriction applies to native Codex WS only and must
not silently change existing HTTP semantics.

### 8.9 Per-step dispatch gates

The final provider-bound write order is fixed:

1. validate the retained principal and request/RPM gates and the frozen
   candidate's cold eligibility;
2. reserve required bounded usage and settlement capacity plus global
   admission;
3. for a payload larger than 64 KiB, acquire bounded CPU admission, materialize
   and serialize it on the blocking pool, record its encoded size, then release
   that materialization permit before any Redis wait;
4. acquire the final candidate's distributed provider/key concurrency and key
   RPM admission;
5. wait for official socket readiness under one frozen provider-write deadline;
   a readiness timeout occurs before `start_send` and is proven not executed;
6. MGET and validate the frozen global/catalog/key generations once, then check
   the distributed permit leases;
7. for a serialized payload larger than 64 KiB, synchronously `try_acquire` both
   send-side admission and a CPU worker without yielding; mark the write
   attempted immediately before synchronous `start_send`, keep
   compression/framing/copy work under the CPU permit, release that permit, and
   await socket flush under the same frozen deadline. A start/flush failure is
   execution-unknown and is never replayed.

There is no duplicate shared Redis eligibility validation before preparation;
the final MGET is the authoritative check immediately adjacent to the provider
`start_send`. All inference permits belong to one `StepExecutionGuard` and
release on success, rejection, cancellation, timeout, disconnect, lease loss,
or panic.
The connection holds no provider/key inference permit while idle. Candidate
enumeration applies the exact `codex` + `oauth` + boolean
`codex_official_ws=true` hard filter before ranking, then full
endpoint/profile/runtime/proxy eligibility before the 16-candidate cap. An
ineligible candidate can never crowd an eligible candidate out of that cap.

## 9. Official TLS and WebSocket Profile

Aether uses the isolated `aether-codex-ws-connector` crate. It must retain the reviewed fork revisions instead of replacing them with crates.io or a generic browser-emulation client.

### 9.1 Official connector identity

Required behavior:

- Rustls 0.23.36 with rustls-webpki 0.103.13 and the exact pinned supporting
  dependency graph listed in section 3;
- explicit AWS-LC provider with KX order frozen to the pinned Codex no-default-
  feature order: X25519, secp256r1, secp384r1, X25519MLKEM768;
- safe default protocol versions and official certificate/SNI verification;
- native certificate roots cached as immutable input;
- `CODEX_CA_CERTIFICATE`, then `SSL_CERT_FILE`, with the same additive custom-CA semantics as the pinned Codex source;
- invalid configured CA fails connector construction, never disables verification;
- a fresh `ClientConfig` per physical connection;
- empty ALPN list;
- Rustls default resumption state, initially empty per built config and never shared across users/connections;
- permessage-deflate enabled on the official hop;
- 16 MiB maximum frame and 64 MiB maximum official message;
- 128 KiB write threshold, 17 MiB maximum write buffer, and 256 KiB maximum
  retained write-buffer capacity after a complete drain;
- Codex/Tungstenite Nagle default retained;
- transport-default, direct, HTTP CONNECT, and HTTPS CONNECT routes;
- target TLS remains inside CONNECT;
- proxy credentials never reach the official request;
- SOCKS and unsupported schemes fail explicitly.

The immutable key manifest installed by the Aether account switch is exactly
schema 3 in meaning and includes at least these runtime identity fields:

```json
{
  "schema_version": 3,
  "profile_id": "codex-ws-0.144.1-linux-x64-rustls023-aws-lc-caenv1-wbufret256k1",
  "codex_commit": "1f0566d3f59298d1bb88820a0d35294f1eeb07ea",
  "tokio_tungstenite_rev": "0e5b2d73aa18dd9f0a50ee9ff199d5aef7594186",
  "tungstenite_rev": "4fffad30fe373adbdcffab9545e9e9bf4f2fc19f",
  "tungstenite_patch_id": "aether-tungstenite-0.27-out-buffer-retention-v1",
  "write_buffer_size_bytes": 131072,
  "max_write_buffer_size_bytes": 17825792,
  "max_retained_write_buffer_capacity_bytes": 262144,
  "crypto_provider": "aws-lc-rs"
}
```

Every field is compared exactly during static profile eligibility. Missing,
schema-2, or drifted fields make the key ineligible; Aether never repairs an
arbitrary hand-edited manifest on the request path.

The official handshake copies only the selected account's materialized authorization, ChatGPT account ID, stable concrete User-Agent/originator profile, canonical Codex identity, required beta features, and reviewed optional Codex headers. Aether trust/control headers are stripped.

TLS acceptance is a normalized fingerprint family across randomized extension order, not one JA3 string. Tests must capture direct, HTTP CONNECT, and HTTPS CONNECT handshakes and compare protocol/cipher/extension/ALPN/SNI behavior to the pinned Codex binary.

### 9.2 Bidirectional write-buffer retention

Both Aether WebSocket directions configure the same 128 KiB/17 MiB/256 KiB
limits. The official upstream uses the vendored Tungstenite 0.27 fork at the
pinned revision plus
`aether-tungstenite-0.27-out-buffer-retention-v1`. The client-facing downstream
uses vendored Tungstenite 0.28 plus
`aether-tungstenite-0.28-out-buffer-retention-v1`, exposed through the vendored
Axum 0.8.8 patch `aether-axum-0.8.8-ws-retention-config-v1`.

The retention patch shrinks capacity only after all queued bytes have been
successfully drained. Partial or failed writes keep Tungstenite's original
buffering semantics; the patch does not discard unsent bytes or alter wire
output. A single connection may grow toward the 17 MiB hard maximum under
backpressure, but after a complete drain it may retain no more than 256 KiB.
The 17 MiB value is intentionally above the 16 MiB routed frame plus framing
overhead. Downstream permessage-deflate remains disabled; official-upstream
permessage-deflate remains the pinned Codex behavior.

## 10. Usage, Billing, and Resource Ownership

### 10.1 sub2api

Before each provider write, sub2api reserves one slot spanning a bounded required-finalizer queue and a separate bounded required-persistence queue. The terminal callback only transfers user/account release handles and the usage task with an O(1) channel send.

The worker order is:

1. release account concurrency;
2. release user concurrency;
3. close the next-step release barrier;
4. hand persistence to the independent required-persistence workers;
5. record usage/billing with the same idempotency key and bounded retry.

The next step waits for concurrency release but not for database usage settlement. Optional usage saturation and required database latency cannot delay the release workers. Queue exhaustion backpressures before provider execution, never after a terminal while a relay lock is held. Scheduler feedback is committed immediately after terminal provenance is claimed, before asynchronous persistence.

The current worker is process-local. Billing apply and usage-log writes are idempotent and transient failures receive bounded retry, but process loss after terminal commit can still lose an unpersisted task. A durable outbox/replay worker is therefore a production release gate if crash-surviving at-least-once settlement is required; this document does not call the process-local queue durable or exactly-once.

All retries for one persistence task share one total worker deadline; a retry does not receive a fresh timeout budget. The default required task budget is 5 seconds and the detached billing context is additionally capped at the earlier of its parent deadline or 15 seconds. Shutdown rejects new reservations immediately and waits at most 10 seconds by default. If that wait expires, reserved queues remain open and drain safely in the background rather than being closed under an outstanding commit.

Completed terminals report scheduler success. Failed/incomplete/error terminals report failure. Idle timeout, client read/write disconnect, local admission rejection, and controlled reconnect do not synthesize another turn and do not penalize account health.

### 10.2 Aether

Aether reserves bounded usage-queue capacity and per-step global/provider/key admission permits before the official write. Terminal/abort drops all inference permits before committing usage. One bounded consumer drains terminal usage and the existing report path; no per-terminal unbounded task is spawned.

Exactly one terminal report is produced for completed, failed, incomplete, official write failure, client close during an active step, and poisoned protocol exit. Duplicate/stale official terminals cannot settle a newer step.

The Aether usage reporter, primary settlement reporter, and slow-settlement
retry reporter are separate bounded worker pools. Candidate lease release and
health/pool feedback do not wait behind a slow usage database write. Primary
settlement timeout gets at most one bounded slow-lane retry; neither required
queue can be bypassed with an unbounded spawn.

After an official candidate connects successfully, Aether drops the planning
attempt and redundant plan/report copies immediately. The retained connection
keeps one compact lifecycle/settlement template, frozen IDs/topology, and the
body-free candidate data needed for later steps. It does not keep the selected
candidate's original request body, proxy URL/credential, or duplicate
`ExecutionPlan` for the lifetime of an idle connection.

## 11. Performance Contract

### 11.1 Hot-path design

The local sub2api-to-Aether hop uses the configured `ws://` address directly:

- no address validation or allowlist lookup;
- no security DNS/public-host policy request;
- no TLS handshake;
- no reverse proxy required;
- no account/system proxy;
- no system proxy lookup;
- no permessage-deflate;
- no extra capability-probe HTTP request;
- route-v1 negotiated in the same 101 response, zero extra RTT.

These are enforced transport options, not best-effort hints. sub2api sets
`DirectNoProxy=true` and compression off on the option-aware dialer. If a custom
dialer does not implement that contract, the Aether account fails closed rather
than falling back to generic dialing. The runtime does not validate whether the
administrator-entered Aether host is local; avoiding that address/DNS policy is
intentional for this trusted local hop.

The steady-state delta path at each gateway is direct `read -> validate bounded metadata -> await write`. Socket backpressure is the queue. Per-frame goroutines, unbounded channels, Redis, database, billing, and full scheduling are forbidden. A bounded socket write/read deadline may reuse one step-scoped timer; it must not spawn a timer task per frame.

Exactly-64-KiB and smaller frames bypass the Aether CPU lane. Ordinary work on
a frame larger than 64 KiB must first obtain strict, non-waiting admission into
the bounded running-plus-waiting lane, then waits at most 250 ms for a worker.
Provider-send and completed-terminal exceptions follow the bounded rules in
section 8.9. Large client
`response.create` parsing, owned body materialization/serialization, and large
provider-event classification run through `spawn_blocking` while budgeted.
Provider bytes are cheap-cloned into the blocking classifier rather than copied
on the async executor. Idle official frames use the same classifier lane.
Large downstream deltas/terminals and serialized upstream requests wait for
sink readiness before acquiring the CPU worker, keep synchronous
compression/framing/frame-copy work under the same budget in `start_send`, then
drop the permit before awaiting `flush`. A slow client or official socket can
therefore apply backpressure without occupying the global CPU lane.

Materialization CPU is released before Redis or network waits. Provider
send-side CPU is acquired synchronously only after bounded socket readiness and
the final shared hot-state MGET, so neither slow readiness nor Redis I/O owns a
CPU worker and no async yield separates acquisition from framing the provider
write. Small frames remain inline and do not acquire a semaphore or spawn a
task. The two CPU environment variables and strict bounds are listed in
section 7.3.

A completed terminal remains withheld until scheduler/concurrency release has
finished, but terminal delivery does not use a fixed 250 ms grace. It uses the
configured step write budget and remaining total budget, with a 5-second floor
and 30-second cap. Unlike ordinary deltas, a terminal polls for strict admission
and then waits for a worker within that same delivery deadline, without joining
an unbounded semaphore waiter list. Provider/key permits are already released
while a large or backpressured terminal drains to sub2api.

Ordinary delta frames perform no Redis or database work. The WS cold path first acquires the process/user/key retained-connection limiter, then reads one repository-fresh API-key/user/group/RPM snapshot fenced by Redis generation reads and synchronously primes the billing cache. Repository fallback is permitted only on this cold admission.

Immediately before each provider-bound `response.create`, sub2api performs exactly one Redis authentication-generation read. An Aether-managed account also performs one Redis scheduler-snapshot read and validates account status, capability, credential/base route, concurrency, current group membership, frozen model support, and the frozen account model mapping. Later turns then read/increment only the required Redis billing/RPM entries. A billing cache miss fails closed and requires reconnect; it never falls back to a database on the retained path. Thus a turn can have several bounded local Redis operations, not a falsely advertised single cache lookup, but it performs zero database queries. Ordinary HTTP API-key authentication is unchanged and does not gain this WS lease RTT.

The final-fence rule is a transport-adapter invariant, not a passthrough-only
feature. Passthrough, `ctx_pool`/shared/dedicated, the ordinary and oversized
HTTP bridges, and the Grok bridge all invoke the same hook after their final
payload/replay/model mutation and asynchronous connection admission, directly
before the provider WS write or HTTP `Do`. A fence error stops dispatch. One
logical client turn invokes it once; a proven-safe internal write/read recovery
of that same turn reuses the result rather than repeating authorization,
billing, or future consume side effects.

The scheduler's retained full-account value and slim bucket metadata share a
monotonic source-revision key, `sched:v2:acc:version:<account_id>`, derived from the
authoritative account `updated_at`. The guarded Redis script rejects a pending
mutation, a deletion tombstone, an older source revision, and an equal revision
after a fenced epoch. A strictly newer ordinary rebuild atomically replaces the
full value, metadata, and revision in the existing pipeline, so a prior fenced
mutation does not freeze the lease value forever. Fenced publishes establish
the same watermark; if their safety fence has expired and a strictly newer
ordinary revision already won, a delayed publish acknowledges its epoch but
does not overwrite or relabel the newer value. Deletion removes the revision,
and a rolling-upgrade legacy value without a revision is adopted only through
a bounded optimistic `WATCH` comparison. The normal retained lease read remains
one fail-closed Redis script and gains no additional round trip.

The full/meta/epoch/fence/tombstone/version keys intentionally use the isolated
`sched:v2:*` namespace. A pre-version binary can continue writing the legacy
keys without corrupting the new payload/version invariant, but this is not a
claim that mixed-version scheduling is release-ready: the durable outbox
watermark remains shared and does not replay every event separately into v2.
All pre-v2 processes must be drained before a final v2 rebuild. Operators then
verify every target `sched:v2:ready:*` bucket plus each enabled Aether account's
full and version keys before enabling WS. Until that hydration is complete, a
v2 lease read intentionally misses and fails closed. Legacy scheduler-key
cleanup is a separate, observed, explicitly authorized operation after the
rollback window; deployment must not delete those keys automatically.

This revision rule includes the quota-threshold path: the epoch-less refresh
emitted by `IncrementQuotaUsed` must update the full value, not only bucket
metadata. Consequently, an already-bound Aether WS route observes total-quota
exhaustion in `ValidateAetherWSBindingLease` and closes before another provider
write. If its database timestamp is equal or older, a rare bounded `WATCH`
path merges only total-quota fields into the newer route snapshot. Daily or
weekly exhaustion instead adds a synthetic temporary-unschedulable deadline at
the persisted window reset while retaining the newer identity, credentials,
status, groups, and route. It never copies an expiring stale account payload
that could become schedulable after reset. Repeated transaction contention is
returned as an error so the outbox does not advance and can retry;
reset/configuration changes remain fenced authoritative mutations.

Content moderation and OpenAI fast-policy settings are also connection/runtime
snapshots. Cold admission loads them once; retained turns use atomic snapshot
loads and perform zero setting-repository reads. Content moderation refreshes
the snapshot immediately on local admin updates and by a low-frequency
30-second background refresh for other instances. A refresh failure leaves the
last good snapshot in place; a missing cold snapshot fails admission and asks
the client to reconnect. The moderation async queue strips the original body
after extraction, so a large WS frame is not retained a second time. For a
flagged result, the bounded non-blocking record/side-effect task must be
accepted before its dedup hash is published. Queue saturation therefore drops
neither the enforcement opportunity nor every retry for the hash-retention
window: a rejected task publishes no hash, and a later identical request may
reserve capacity and retain ownership of violation counting, notification, and
auto-ban side effects.

Queue acceptance is still process-local, not durable. A crash after hash
publication but before the worker persists the accepted task can leave the
same suppression window without a corresponding side effect. If crash-proof
moderation enforcement is a product requirement, couple reservation and hash
publication through a durable outbox/state machine before release; an in-memory
channel alone cannot prove that guarantee.

Fast-policy snapshot failure is fail-closed for the WS session: sub2api closes
before account/provider dispatch with a retryable WebSocket error. Initial
account failover reuses the same immutable snapshot; no retry or later frame
re-enters the settings repository.

The per-key Redis generation has a bounded 30-day lifetime renewed when initialized or invalidated, and is allocated from a permanent global sequence seeded by Redis `TIME`, preventing expiry/restart ABA. A normal validation `GET` does not renew the TTL. Invalidation advances the distributed fence before local eviction/pubsub and always advances the local process generation. A Redis generation read failure fails the WS step closed. Strict cross-instance revocation still requires Redis HA plus alerting, or a durable invalidation outbox/replay path, because simultaneous generation-write and pubsub failure followed by process loss cannot be repaired by process-local state alone.

Aether's global/catalog/key hot authority is shared Redis state with strict
generations, CAS, and mutation locks as defined in section 8.6. The final
pre-write MGET is shared across instances; there is no process-local permissive
fallback. Provider/key concurrency is acquired from distributed keyed
semaphores on each step using limits captured in the candidate snapshot, with
zero steady-state DB reads. Lease-loss notification aborts at the safe boundary
instead of waiting for a polling interval. A multi-instance deployment MUST
configure the shared runtime-state/concurrency backend; a process-local permit
backend is valid only for an explicitly single-instance deployment.

Normal retained bindings avoid full scheduling. The first step performs one
cold parsed-JSON policy evaluation, one billing-context batch for at most 16
candidates, and batch transport reads; later steps perform authentication/RPM
and shared hot-state checks but zero policy DB work. Scheduling runs on initial
binding, hard invalidation, or reconnect migration. Full official/profile/proxy
eligibility precedes truncation, and candidate count must not cause N+1
provider/endpoint/key/billing queries.

### 11.2 Bounded work

- one in-flight response step per connection;
- one preparing step that is not attributable until hooks finish immediately before provider write;
- independent process-wide, per-user, and per-API-key retained-connection caps;
- sub2api required usage capacity = configured worker count + queue size;
- one Aether process-wide usage reporter, default capacity 16,384 and 32 workers, with the bounded environment controls in section 7.3;
- one bounded primary settlement lane and one bounded single-retry slow
  settlement lane, with all defaults in section 7.3;
- one strict large-frame CPU admission lane, at most 64 workers and 256 admitted
  running-plus-waiting operations;
- no usage sync fallback on a relay task;
- bounded settled response IDs and route-control IDs;
- Aether settled response history <= 64 IDs and <= 8 KiB; protocol IDs <= 256 bytes and turn state <= 4 KiB;
- bounded 16 KiB route-control frames;
- shared ingress endpoint retains the legacy configurable 64 MiB client read
  ceiling until account selection, for compatibility with non-Aether accounts;
- after an Aether account is selected, the client read limit is 16 MiB and the
  routed frame limit is 16 MiB plus a bounded 4 KiB route-control overhead;
- Aether official messages remain capped at 64 MiB;
- idle Aether candidates retain a body-free execution-plan template, not the first request body;
- a successfully connected candidate releases its planning attempt, duplicate
  plan/report copies, and connector-owned proxy secret;
- queued Aether usage outcomes and candidate settlement never retain request bodies or full terminal JSON values;
- both Aether Tungstenite directions retain at most 256 KiB write-buffer
  capacity after a complete drain, with 17 MiB hard queued-buffer maxima;
- connect, provider-write, first-byte, upstream-read, downstream-write, and total-step waits are bounded;
- slow consumers close or backpressure instead of increasing memory without limit.

### 11.3 Initial budgets

Measure on the deployment-class CPU and network namespace:

| Operation | Release budget |
|---|---|
| Local sub2api -> Aether 101 | p50 <= 1 ms, p95 <= 3 ms, p99 <= 10 ms |
| Process-local retained-connection/global/catalog lease check | p99 <= 20 us, target 0 allocations |
| Per-step sub2api auth generation Redis GET | p95 <= 1 ms, p99 <= 3 ms, zero DB |
| sub2api Aether-account scheduler snapshot read | p95 <= 1 ms, p99 <= 3 ms, zero DB |
| Later-turn billing/RPM cache-only gate | p95 <= 2 ms, p99 <= 5 ms, zero DB and no repository fallback |
| Aether per-step provider+key permit acquisition without contention | p95 <= 1 ms, p99 <= 3 ms, zero DB |
| Hot-snapshot initial/migration scheduling | p95 <= 2 ms, p99 <= 10 ms, no N+1 |
| First-step 16-candidate policy/billing work | one parsed-JSON policy evaluation and one billing batch; zero single-candidate fallback calls |
| Candidate transport materialization | one provider + one endpoint + one key batch read; zero per-candidate transport reads |
| Aggregate sequential initial connect | sum of candidate connect budgets, hard cap <= 60 s |
| Large-frame CPU admission | ordinary O(1) `try_acquire` + worker wait <= 250 ms; provider send synchronous; terminal bounded by delivery deadline; no unbounded waiter list |
| <4 KiB ordinary delta classification per gateway | <= 1 us, 0 allocations target |
| Local middle-hop frame relay | p95 <= 150 us, p99 <= 500 us |
| 64 KiB `response.create` processing (normal policy) | p95 <= 350 us, at most one payload-sized output copy |
| 1 MiB `response.create` processing (normal policy) | p95 <= 5 ms, at most one payload-sized output copy |
| 64 KiB `response.create` with `service_tier` filter/alias | p95 <= 400 us, at most one payload-sized output copy |
| 1 MiB `response.create` with `service_tier` filter/alias | p95 <= 6 ms, at most one payload-sized output copy |
| Usage queue commit | p99 <= 50 us |
| Required persistence queue handoff | p99 <= 100 ms |
| Settlement lag | p99 <= 2 s |
| 10k idle connections | combined application RSS <= 384 KiB/connection target |

A budget miss blocks rollout until either implementation is fixed or the budget is explicitly revised with measured evidence. Do not silently remove the benchmark.

The local EPYC-Genoa benchmark snapshot (Go `-benchtime=500ms -count=5`)
measured normal frames at 186–283 us (64 KiB) and 2.76–3.32 ms (1 MiB), with
19/20 allocations and one payload-sized output. The integrated tier-filter
path measured 219–319 us and 3.01–4.85 ms, with 23–25 allocations and one
payload-sized output. These are process microbenchmarks, not network or
10k-connection load results. Canonical upstream delta events, including
`response.reasoning_text.delta` and `response.custom_tool_call_input.delta`,
measure under 0.6 us with zero allocations at 128 B and 4 KiB.

### 11.4 Required load cases

Run:

- 128 B, 4 KiB, 64 KiB, 1 MiB, and maximum-size frames;
- 1, 100, 1,000, and 10,000 concurrent connections;
- 100 sequential steps per retained connection;
- slow downstream and slow official upstream;
- Redis latency/failure and database latency/failure;
- full usage queues;
- full primary and slow-settlement queues;
- saturated large-frame CPU admission with small-frame traffic continuing;
- repeated 16 MiB writes followed by complete drain, proving both Aether
  directions return to <= 256 KiB retained write-buffer capacity;
- sequential candidate connect timeouts proving the aggregate 60-second cap;
- catalog changes between validation, admission, and provider write;
- race detector/loom-style terminal versus disconnect interleavings.

Report throughput, p50/p95/p99 handshake/TTFT/frame/step latency, allocations, RSS per idle connection, queue depth, backpressure duration, reconnect rate, and settlement lag.

## 12. Observability

Every binding/step log should include non-secret identifiers:

```text
trace_id
sub2api_account_id
sub2api_binding_epoch_id
binding_generation
step_correlation_id
migration_count
control_id hash
Aether step/attempt hash
transport=native_codex_ws
profile_id
proxy topology (no credentials)
global/catalog/key hot-state generation hashes
terminal event type
provider write state
execution disposition
usage/settlement/slow-settlement queue depth and backpressure
large-frame CPU admission rejection/wait
TTFT and total duration
```

Never log prompts, OAuth tokens, Aether API keys, proxy credentials, full provider response bodies, or raw official account IDs.

Metrics must separate:

- client disconnect/idle/local rejection from account failure;
- initial failover from later reconnect migration;
- direct, HTTP proxy, and HTTPS proxy connection latency;
- cold snapshot misses from steady-state checks;
- route-control validation failures by reason;
- usage reserve wait, queue depth, required-persistence handoff lag, and settlement lag;
- primary settlement timeout, slow-lane enqueue failure, and slow retry timeout;
- large-frame CPU admission rejection, worker wait, blocking duration, and
  frame-size class;
- auth-generation Redis latency/failure and provider/key permit wait by dimension;
- scheduler guarded-write rejection by reason, delayed-publish preservation,
  legacy-version adoption/contention, and version-present/full-missing repair
  alarms; these counters must not add another retained-lease round trip;
- global/catalog/key MGET latency, generation mismatch, mutation lock loss, and
  restrictive recovery failure;
- stale/duplicate terminal frames;
- migration generation mismatch, idempotent replay, limit exhaustion, and Redis failure.

## 13. Implementation Map

### 13.1 sub2api

Primary files:

```text
backend/internal/config/config.go
backend/internal/handler/openai_gateway_handler.go
backend/internal/handler/openai_ws_turn_finalizer.go
backend/internal/repository/gateway_cache.go
backend/internal/repository/account_repo.go
backend/internal/repository/scheduler_cache.go
backend/internal/service/openai_aether_ws.go
backend/internal/service/openai_aether_ws_control.go
backend/internal/service/openai_ws_failover.go
backend/internal/service/openai_ws_migration_state.go
backend/internal/service/openai_ws_route_session.go
backend/internal/service/openai_ws_v2/passthrough_relay.go
backend/internal/service/openai_ws_v2_passthrough_adapter.go
backend/internal/service/usage_record_worker_pool.go
frontend/src/utils/aetherWsAccount.ts
frontend/src/utils/accountExtraPatch.ts
frontend/src/components/account/CreateAccountModal.vue
frontend/src/components/account/EditAccountModal.vue
frontend/src/components/account/BulkEditAccountModal.vue
deploy/config.example.yaml
```

### 13.2 Aether

Primary files:

```text
apps/aether-gateway/src/codex_ws/
apps/aether-gateway/src/codex_ws_config.rs
apps/aether-gateway/src/api/ai/registry.rs
apps/aether-gateway/src/scheduler/candidate/selection.rs
apps/aether-gateway/src/handlers/admin/provider/endpoint_keys/mutations/codex_ws.rs
crates/aether-codex-ws-connector/
crates/aether-codex-ws-connector/profiles/codex-ws-0.144.1-linux-x64-rustls023-aws-lc-caenv1-wbufret256k1.json
crates/aether-ai-formats/src/provider_compat/proxy/rules.rs
crates/aether-provider-transport/src/codex_ws.rs
crates/aether-provider-transport/src/snapshot.rs
crates/aether-scheduler-core/src/candidate/
crates/aether-data/src/repository/provider_catalog/
frontend/src/features/pool/utils/codexWsAccount.ts
frontend/src/views/admin/PoolManagement.vue
vendor/axum-0.8.8/
vendor/tungstenite/
vendor/tungstenite-0.28.0/
```

No schema migration is required: both implementations use existing JSON capability/fingerprint/extra fields.

## 14. Required Tests

### 14.1 sub2api correctness

- base router v2/route-v1 switches default on, explicit false overrides fail closed, and reconnect migration defaults off with strict dependency validation;
- no pre-101 whole-group candidate scan or duplicate scheduling read;
- explicit account capability and local-address fast path with no address/DNS
  validation;
- effective Aether accounts set direct/no-proxy and compression-disabled dial
  options, and fail closed when a dialer cannot guarantee them;
- bounded whole-object client envelope validation, including escaped duplicate keys and type/model/ID limits, before any admission side effect;
- exact route-v1 handshake and rejection of provider fallback capability;
- escaped/duplicate/nested reserved type handling;
- large ordinary delta containing the reserved marker remains allocation-free and is never materialized as a control candidate;
- one-in-flight gate before billing/RPM/policy side effects;
- created-ID binding, incremental ID match, completed match;
- failed/incomplete/top-level error semantics;
- stale/duplicate terminal consumption and exactly-once callback;
- downstream terminal write failure still finalizes exactly once;
- no gate mutex across socket write/callback;
- initial-only failover fence;
- canonical session identity and auth-scope isolation;
- Redis Lua idempotency, generation race, exhaustion, corrupt state, and sticky deletion atomicity;
- `retain` versus `exclude` Redis semantics, exact idempotent exclusion deadline, and disposition replay mismatch;
- no reconnect payload before successful Redis admission;
- idle/client disconnect does not synthesize a turn or penalize account;
- TTFT-before-first-frame client close/cancel releases the active turn promptly;
- bounded two-stage required usage reservation, O(1) commit, and release workers isolated from persistence;
- auth invalidation epoch and cache-only Aether account lease close before later provider write;
- final-fence success and failure before dispatch in passthrough,
  `ctx_pool`/shared/dedicated, ordinary/oversized HTTP bridge, and Grok bridge,
  including exactly-once behavior for an internal retry of one logical turn;
- Aether account lease rejects group removal, frozen-model loss, and account model-route changes without a DB fallback;
- scheduler source-revision races reject an old/equal ordinary snapshot after a
  fenced epoch, accept a strictly newer rebuild, preserve a newer ordinary
  value against a delayed epoch publish, keep deletion restrictive, and safely
  adopt a legacy unversioned value;
- an epoch-less total-quota threshold refresh replaces the full retained lease
  value and makes the Aether binding fail closed before provider dispatch;
- WS cold authentication is repository-fresh and fenced before/after by the Redis generation;
- each later `response.create` performs exactly one auth-generation Redis read and zero auth DB reads;
- auth generation expiry/reinitialization cannot validate an old lease (no ABA);
- Redis generation read failure closes before provider write;
- independent retained-connection caps cover first-frame wait and idle bindings;
- single-account `extra_patch` preserves concurrent runtime fields;
- bulk Aether JSON deep merge without N+1;
- auth snapshot resolved-nil RPM override causes zero repository calls;
- cold billing admission synchronously primes required entries and every later billing/RPM gate is cache-only and fail-closed on miss;
- content-moderation and fast-policy snapshots perform zero retained-turn setting-repository reads, refresh safely, and fail closed when no cold snapshot exists;
- moderation async tasks do not retain the original request body;
- a full moderation record queue publishes no dedup hash, while a retry after a
  slot is available still owns violation/notification/auto-ban side effects;
- model, route fence, and service-tier filter/alias edits share one payload-copy pass on the normal Aether path;
- race detector for relay, finalizer, and migration.

Representative commands:

```bash
cd /opt/stacks/sub2api/backend
GOCACHE=/tmp/sub2api-gocache go test ./internal/config -count=1
GOCACHE=/tmp/sub2api-gocache go test ./internal/service/openai_ws_v2 -count=1
GOCACHE=/tmp/sub2api-gocache go test -race ./internal/service/openai_ws_v2 -count=1
GOCACHE=/tmp/sub2api-gocache go test ./internal/service -run 'AetherWS|OpenAIWS|UsageRecord' -count=1
GOCACHE=/tmp/sub2api-gocache go test ./internal/handler -run 'OpenAIWS|UsageRecord' -count=1
GOCACHE=/tmp/sub2api-gocache go test ./internal/repository -run 'OpenAIWSMigration|ExtraPatch' -count=1
GOCACHE=/tmp/sub2api-gocache go test -tags unit ./internal/repository -run '^TestScheduler' -count=1
GOCACHE=/tmp/sub2api-gocache go test -tags unit ./internal/service -run 'CheckRPM' -count=1

cd /opt/stacks/sub2api/frontend
npm run typecheck
npm run test -- --run
```

### 14.2 sub2api benchmarks

```bash
cd /opt/stacks/sub2api/backend
GOCACHE=/tmp/sub2api-gocache go test ./internal/service \
  -run '^$' -bench 'AetherWSRouteControlNonMatch|OpenAIWSForwarderHotPath' \
  -benchmem -count=5
GOCACHE=/tmp/sub2api-gocache go test ./internal/service \
  -run '^$' -bench 'BenchmarkOpenAIWSPassthroughBusinessFramePipeline(PolicyFiltered)?$' \
  -benchmem -benchtime=500ms -count=5
GOCACHE=/tmp/sub2api-gocache go test ./internal/service/openai_ws_v2 \
  -run '^$' -bench 'BenchmarkObserveUpstreamMessage(Delta|ReasoningTextDelta|CustomToolCallInputDelta)$' \
  -benchmem -benchtime=500ms -count=5
GOCACHE=/tmp/sub2api-gocache go test ./internal/handler \
  -run '^$' -bench OpenAIWSConnectionLimiterAcquireRelease \
  -benchmem -count=5
```

The ordinary route-control negative path must remain allocation-free. Benchmark all required frame sizes, not only the smallest fixture.

Local microbenchmark snapshot on 2026-07-15, AMD EPYC-Genoa, Go benchmark `-benchtime=500ms -count=3`:

| Operation | Observed range | Allocations |
|---|---:|---:|
| Route-control negative, 128 B | 26.8-34.0 ns | 0 B / 0 alloc |
| Route-control negative, 4 KiB | 149-165 ns | 0 B / 0 alloc |
| Route-control negative, 64 KiB | 2.40-3.21 us | 0 B / 0 alloc |
| Route-control negative, 1 MiB | 45.1-64.6 us | 0 B / 0 alloc |
| Retained-connection limiter acquire+release | 106-123 ns | 0 B / 0 alloc |
| Business frame pipeline, 64 KiB | 186-283 us | ~81 KiB / 19 alloc |
| Business frame pipeline, 1 MiB | 2.76-3.32 ms | ~1.064 MiB / 20 alloc |
| Tier-filter pipeline, 64 KiB | 219-319 us | ~81 KiB / 23 alloc |
| Tier-filter pipeline, 1 MiB | 3.01-4.85 ms | ~1.064 MiB / 24-25 alloc |
| Canonical deltas, 128 B/4 KiB | 0.32-0.51 us | 0 B / 0 alloc |

These are isolated process microbenchmarks, not whole-chain latency or a 10k-connection load result. They satisfy their local negative-path/limiter budgets but do not close release gate 11.

### 14.3 Aether correctness

- route registration and 412/426 pre-upgrade behavior;
- exact route-v1 headers without fallback header;
- `client_reconnect` emits typed `retain|exclude`, while `close_after_terminal` omits the disposition field;
- official-only hard candidate filtering before scoring;
- every immutable schema-3 profile eligibility reason, including endpoint API
  format, retention patch identity, and all write-buffer constants;
- account/global admin switches and atomic JSON merge on memory/Postgres/MySQL/SQLite repositories;
- shared Redis global/catalog/key authority, batched MGET validation, strict
  generation changes, lock renewal/loss, CAS failure, and restrictive recovery;
- shared catalog-generation binding invalidates local candidate/transport cache
  epochs and prevents stale in-flight loads from repopulating a new epoch;
- admin, OAuth/runtime, and health-driven restrictive changes all use the full
  locked hot-state mutation transaction;
- first-step parsed-JSON cold policy runs once, billing context uses one batch
  for 16 candidates, and later steps perform zero policy DB reads;
- transport snapshot fanout performs one provider, one endpoint, and one key
  batch read without a single-candidate fallback;
- per-step snapshot admission and zero steady-state DB reads;
- per-step distributed provider/key concurrency acquisition and release on every terminal/cancel/error path;
- catalog epoch race immediately before provider write;
- global disable/re-enable and auth/catalog generation ABA races invalidate retained connections;
- initial candidate retry only before official write;
- all sequential initial candidate connects share the aggregate <= 60-second
  deadline;
- connect/write/first-byte/read/total timeout behavior, with no replay after a possibly successful write;
- response ID provenance and duplicate/stale terminal handling;
- model inheritance on later steps, explicit model-change rejection, bounded protocol identifiers, body-free idle candidate templates, and release of the
  successful planning attempt/duplicate plans;
- owned request-body normalization preserves large moved allocations, serializes
  once, and fails closed for enabled WS rules using `source=original`;
- bounded usage reservation and permit release ordering;
- candidate lease/renewer/health settlement is independent of the usage FIFO;
  primary timeout enters at most one bounded slow-settlement retry, and queued
  outcomes retain neither request bodies nor full terminal values;
- >64 KiB strict CPU admission, ordinary-work 250 ms worker bound, synchronous
  provider-send acquisition, deadline-bound terminal acquisition, blocking
  parse/classify/materialization, idle-frame classification, small-frame
  bypass, readiness before CPU acquisition, and CPU release before socket
  flush;
- 16 MiB terminal delivery after settlement under bounded write/remaining-total
  budget, including slow downstream readiness;
- direct/HTTP CONNECT/HTTPS CONNECT connector tests;
- one proxy resolution per actual pool key, manual HTTP/HTTPS URL support,
  SOCKS/tunnel exclusion before the candidate cap, and redacted frozen topology;
- custom CA precedence, additive roots, invalid PEM failure;
- proxy credential isolation;
- upstream Tungstenite 0.27 and downstream Tungstenite 0.28/Axum retention patch
  tests, including wire-equivalence and shrink-only-after-complete-drain;
- profile manifest and Cargo dependency/patch drift tests;
- frontend key and batch controls.

Representative commands should use targeted crates first to avoid monolithic linker OOM:

```bash
cd /opt/stacks/aether
cargo check -p aether-codex-ws-connector --locked
cargo test -p aether-codex-ws-connector --locked
cargo check -p aether-provider-transport --locked
cargo test -p aether-provider-transport codex_ws --locked
cargo check -p aether-gateway --tests --locked
cargo test -p aether-gateway --lib 'codex_ws::' --locked -- --test-threads=1
cargo test --manifest-path vendor/tungstenite/Cargo.toml
cargo test --manifest-path vendor/tungstenite-0.28.0/Cargo.toml

cd /opt/stacks/aether/frontend
npm run type-check
npm run test -- --run
```

Format/check every touched non-vendor Rust file. Do not use a broad vendor
`cargo fmt --all` diff as a release verdict: the vendored sources retain their
reviewed upstream formatting and are verified through patch metadata,
wire-equivalence, retention, dependency-lock, and functional tests. A real
format error in workspace-owned code still fails the gate.

If the full gateway test binary is killed by the linker, record the OOM separately and still run all affected module/crate tests. OOM is not a passing full-suite result.

## 15. Release Gates

All of these are required before production enablement:

1. Both repositories format, compile, and pass focused correctness/race tests.
2. No unresolved P0/P1 review finding.
3. No provider-fallback or HTTP bridge behavior/header/UI remains in the v1 route.
4. Real Codex handshake and reconnect fixtures pass for the pinned revision.
5. Direct/HTTP CONNECT/HTTPS CONNECT TLS fingerprint-family checks pass.
6. Custom CA behavior matches the pinned Codex source.
7. Exactly-once terminal callback/release and idempotent usage tests pass under terminal/disconnect/write-failure races.
8. Atomic migration tests pass with Redis failure and concurrent controls.
9. Account enable/disable and catalog-race tests prove no later step writes a disabled binding.
10. Steady-state step admission demonstrates zero database reads.
11. Whole-chain load report meets the performance budgets or contains an approved measured revision.
12. Existing HTTP/SSE regression suite passes with every new switch off.
13. Staging reverse-proxy upgrade, buffering, idle timeout, and maximum connection age are validated.
14. Operators can disable the route without a schema rollback.
15. If crash-surviving usage delivery is required, a durable usage outbox/replay test passes; the in-memory required queue alone does not satisfy this gate.
16. API-key revocation has a durable cross-instance invalidation path or Redis HA/alerting with an explicitly accepted fail-closed operational contract.
17. All official connect/write/first-byte/read/total waits and slow downstream writes are bounded, and timeout tests prove that a possibly dispatched step is never replayed.
18. Every enabled Aether Codex WS key carries the exact schema-3 profile; no
    schema-2 account remains schedulable after rollout.
19. Large-frame CPU admission and usage/primary-settlement/slow-settlement lanes
    remain bounded under saturation, and both WebSocket directions demonstrate
    post-drain write-buffer retention at or below 256 KiB.

### 15.1 Unverified gates at handoff

Source implementation and focused local tests do not by themselves authorize production enablement. The execution AI must keep these gates open until evidence is attached:

- normalized real ClientHello captures from the pinned Codex binary and Aether,
  using real official credentials, for direct, HTTP CONNECT, and HTTPS CONNECT;
- a real official Codex credential-backed one-step, retained multi-step, and
  reconnect fixture;
- the 1/100/1,000/10,000-connection load report, including RSS per idle connection and 100 sequential steps;
- multi-instance verification that provider/key permits and global/catalog/key
  hot state use the shared Redis runtime-state backend, including mutation-lock
  loss, strict-CAS recovery, generation changes, and node-loss lease recovery;
- real MySQL and PostgreSQL integration runs for the new billing batch,
  provider/endpoint/key batch, account manifest merge, and restrictive
  hot-state mutation paths; in-memory/SQLite/unit coverage is not a substitute;
- full Aether usage, primary-settlement, and slow-settlement queue saturation
  evidence showing immediate candidate lease release and bounded compact memory;
- large-frame CPU saturation and bidirectional 16 MiB write/drain evidence;
- staging reverse-proxy upgrade, buffering, idle-timeout, and maximum-connection-age validation;
- staging verification that schema-2 keys are rejected and re-toggling installs
  the exact schema-3 manifest before traffic is enabled;
- crash-surviving usage delivery if it is a product requirement;
- durable moderation side-effect delivery, or explicit acceptance of the
  process-crash window between in-memory task acceptance and worker persistence;
- durable repair for the simultaneous Redis generation-write plus pubsub failure window, unless operations explicitly accepts the documented Redis HA contract.

Absence of this evidence is a release block, not an implementation test pass.

Local source evidence recorded on 2026-07-16:

- sub2api `go test -p 1 ./internal/... -count=1`: passed;
- sub2api `go vet ./internal/...`: passed;
- sub2api `go test -race ./internal/service/openai_ws_v2 -count=1`: passed;
- sub2api `go test -tags=unit ./internal/repository -run '^TestScheduler' -count=1`: passed;
- sub2api complete `-tags=unit ./internal/repository` suite: passed;
- the same complete scheduler suite under `-race`: passed;
- focused `-race` coverage for ctx-pool final fencing/retry, Grok and ordinary
  HTTP bridge fencing, moderation queue saturation, and quota-exhausted Aether
  lease validation: passed;
- an explicit HTTP-bridge rejection test proved a final-fence error reaches no
  `httpUpstream.Do` call or downstream event;
- independent scheduler re-review found no remaining P0/P1 in the v2
  generation protocol; its mixed-version hydration/metrics/key-cleanup P2
  items remain operational gates documented above;
- sub2api account/Aether frontend focused suite: 44/44 passed;
- sub2api frontend typecheck: passed;
- sub2api full frontend suite: 793/796 passed. The three failures are in
  untouched, unrelated tests:
  `EmailOAuthButtons.spec.ts`, `HelpTooltip.spec.ts`, and
  `usePersistedPageSize.spec.ts`. They remain an existing release-suite gap and
  were not masked or modified by this work;
- the Go microbenchmarks in section 14.2 passed their revised local budgets;
- Aether vendored Tungstenite 0.27 retention suite: 50/50 passed;
- Aether vendored Tungstenite 0.28 retention suite: 44/44 passed;
- Aether Codex connector focused suite: 17/17 passed;
- Aether gateway `codex_ws::` focused suite: 83/83 passed, including aggregate
  connect budget, catalog snapshot generation, provider readiness versus flush
  semantics, idle large-frame classification, and 16 MiB terminal delivery;
- Aether gateway test-target compile check: passed;
- Aether provider-transport Codex WS suite: 13/13 passed; its ignored-by-default
  performance probe was also run explicitly and passed. Planning artifacts
  stayed body-size-independent at 458 bytes for one candidate and 7,325 bytes
  for 16 candidates for both 64 KiB and 1 MiB inputs;
- Aether runtime-state suite: 33/33 passed;
- Aether owned-body move and `source=original` fail-closed focused tests: 2/2
  passed; billing-context batch memory/SQLite focused tests: 2/2 passed;
- Aether pool-management Codex WS focused frontend suite: 5/5 passed;
- Aether frontend typecheck: passed;
- these Aether results are focused local dependency/UI evidence only; they do
  not close the real-credential, normalized TLS capture, shared-Redis
  multi-instance, MySQL/PostgreSQL, staging, or load gates above;
- no production service, container, proxy, database, or runtime configuration
  was changed while collecting this evidence.

## 16. Forbidden Shortcuts

Do not:

- call a full HTTP execution shell from a WS adapter;
- parse SSE back into WebSocket events for v1;
- retry after provider write outcome is unknown;
- infer execution safety from an HTTP status alone;
- use prompt content as migration identity;
- use a process-local migration counter;
- send the reconnect event before Redis admission commits;
- hold inference permits while the socket is idle;
- run database usage settlement inside the terminal callback;
- spawn one task per terminal/frame without a bound;
- put a mutex around frame writes or callbacks;
- allow a sticky/cached route to bypass hard eligibility;
- use process-local hot eligibility or an unlocked one-shot Redis projection as
  authority for a restrictive Aether key mutation;
- treat client disconnect/idle timeout as account failure;
- overwrite full account JSON from a stale admin form;
- bypass certificate/SNI verification;
- substitute a browser TLS client for the pinned Codex Rustls connector;
- use a mutable dependency revision or profile ID;
- accept a schema-2 account after schema-3 rollout, or omit either upstream or
  downstream write-buffer retention patch;
- let an effective local Aether account fall back to a generic/system-proxy or
  compression-enabled WebSocket dialer;
- retain a second original WS request tree merely to support an enabled
  `source=original` body rule;
- deploy, restart, migrate, or update production as part of source implementation without explicit authorization.

## 17. Handoff Checklist

Before declaring implementation complete, the execution AI must:

1. Re-read the explicit non-goals.
2. Inspect both worktrees and preserve unrelated user changes.
3. Confirm the exact schema-3 profile, upstream/downstream patch identities, and
   write-buffer constants against the pinned Codex commit and vendored manifests.
4. Trace one completed, one failed, one initial-failover, and one later-reconnect step end to end.
5. Prove resource ownership on every return path.
6. Prove account/provider selection cannot change after dispatch commit.
7. Prove sub2api migration is admitted atomically before signaling.
8. Prove Aether official-only filtering occurs before candidate scoring.
9. Prove the first-step policy/billing/transport batch counts and that retained
   steps perform zero policy DB work.
10. Prove steady-state frame/step paths have no database query, unbounded task,
    unbounded CPU waiter, or unbounded retained write buffer.
11. Exercise shared Redis mutation locks/CAS/generations and restrictive
    recovery across instances.
12. Run focused race, repository, frontend, connector, vendor-retention, and performance tests.
13. Run `git diff --check` in both repositories.
14. Report any test that could not run, including linker OOM or unavailable
    real credentials, TLS capture, Redis, MySQL/PostgreSQL, load, or staging fixtures.
15. Do not change production state.

## 18. Final Decision

The v1 design is:

```text
Codex client WS
  -> sub2api account-aware WS ingress
  -> local uncompressed direct Aether WS with route-v1
  -> Aether official-Codex-only account selection
  -> pinned native Codex TLS/WebSocket connector
  -> official Codex

+ initial pre-dispatch failover
+ later atomic reconnect migration
+ two independent account-level switches
+ immutable response provenance, exactly-once terminal ownership, and idempotent settlement
+ bounded backpressure and whole-chain performance gates

- no non-Codex native WS
- no HTTP/SSE bridge
- no Aether provider fallback
- no transparent post-dispatch account switch
```
