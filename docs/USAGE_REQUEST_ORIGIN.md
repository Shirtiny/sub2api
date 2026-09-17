# Usage request origin and client address

## Data contract

`usage_logs.request_host` / JSON `request_host` records the normalized public hostname (`www.cafeshop.ai`, `nl.cafeshop.ai`, etc.), independently of `inbound_endpoint` (API path). Ports are removed; invalid/ambiguous forwarded hosts fall back to HTTP Host. Existing rows remain NULL: do not infer historical hostnames from model, account, IP or current routing.

`ip_address` is now visible in a user's **own** usage records as well as the administrator's. Ownership filtering is unchanged; upstream accounts, pricing multipliers and other administrator metadata stay private. Existing historical IPs are retained as recorded, even if they were proxy addresses. Corrected capture only affects new requests after deployment.

HTTP, WS, Gemini, Anthropic-compatible and media usage capture plain hostname/address strings before asynchronous billing work. Usage IP uses Gin's configured trusted-proxy chain; arbitrary `CF-Connecting-IP` and `X-Real-IP` headers are not sufficient to override it.

## Deployment prerequisites (not applied by this code change)

1. Apply additive migration `197_usage_log_request_host.sql` through the normal application migration runner. No backfill or reverse migration is required; no live migration was executed during development.
2. Configure `server.trusted_proxies` using the actual immediate application peer and required intermediate proxy IPs. Never use `0.0.0.0/0`, `::/0` or entire private ranges by default.
3. At the public edge, overwrite/strip client-supplied forwarding identity headers. Caddy does this for X-Forwarded-For and X-Forwarded-Host by default when the client is untrusted. Preserve the original external host even when TLS SNI / upstream Host is changed to `www`.
4. At the origin Caddy, trust only the expected proxy addresses, with strict right-to-left client IP parsing. Propagate the verified host and IP through the last hop to the application.

NL's setup on 2026-09-17 already satisfies the direct-edge path: edge `162.211.230.96`, origin `185.114.49.57`; origin Caddy trusts the edge and preserves X-Forwarded-Host `nl.cafeshop.ai`; app trusts the measured local Docker peer `172.20.0.1` and the edge.

**Release gate for www/Cloudflare and other installations:** the existing origin must also resolve real IPs securely for CDN requests. Merely forwarding raw `CF-Connecting-IP` is not enough for the new usage capture. On the origin, trust verified/current Cloudflare CIDRs only for traffic actually received from Cloudflare; use its sanitized connecting-IP header to resolve the client, overwrite untrusted host/IP headers, and send a canonical client address to the application (or preserve a fully trusted XFF chain). Do not blindly preserve client-supplied X-Forwarded-Host from a CDN, and do not trust the CDN header on direct public requests. If preserving XFF rather than collapsing it, configure the corresponding CDN hops in the application's trusted list too. Recheck Docker peer addresses after network changes. This is an explicit pre-release configuration check, not an automatic production change.

## Acceptance checks after an authorized deployment

- Issue authenticated, billable requests through `www` and `nl` and verify each new usage record shows the corresponding full hostname and actual external client IP.
- Test IPv4/IPv6 and HTTP/WS paths; an unauthenticated `/v1/models` request alone does not create a billable usage row.
- Send forged XFF, X-Real-IP, CF-Connecting-IP and X-Forwarded-Host headers from an untrusted external client; they must not change the recorded identity.
- Verify a user cannot list another user's rows or fetch another user's usage ID.
- Verify historical missing hostname displays `-` and existing API-path statistics remain unchanged.

Production deployment is separate from code completion; no images, production applications, proxy configuration or database rows are changed by these source edits/tests.

## NL maintenance release

`cafecode-v0.0.75-nl.1` is based on the deployed v0.0.75 revision plus only this feature. It is deployed directly by immutable digest, not a phased/canary release. The NL suffix keeps the build off the shared `latest`/major-minor tags so other installations do not receive a maintenance branch accidentally. v0.0.76 WebSocket fallback, v0.0.77 recharge settings and unrelated dirty stream retry work are not included.

Review scope: billable usage history and its export. The separate redacted failed-request tab is unchanged.
