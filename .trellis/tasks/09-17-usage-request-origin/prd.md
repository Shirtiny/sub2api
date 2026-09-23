# Usage request origin and client IP

## Goal
Display the client-facing hostname (www/nl/etc.) and real client IP in a user's own request history, separately from the API path.

## Contract
- New nullable usage_logs.request_host VARCHAR(253), service RequestHost *string, JSON request_host.
- Expose existing ip_address to the owner of usage records; preserve all unrelated admin-only fields and existing ownership checks.
- Trust X-Forwarded-Host only from configured server.trusted_proxies. Normalize to lowercase hostname without port. Reject malformed/multi-valued forwarded hosts; fall back to Host.
- Use Gin's configured trusted-proxy IP chain for usage snapshots, not arbitrary CF-Connecting-IP/X-Real-IP headers.
- Capture before async usage jobs and cover OpenAI HTTP/WS, compatible/Anthropic, Gemini and media handlers.
- Historical host/IP data is not inferred/backfilled. UI renders missing values as a dash.

## Validation
- Direct www, trusted NL double-proxy, IPv6, spoofed and malformed forwarding headers.
- Service snapshots, repository insert/read and batch paths, DTO privacy, user/admin UI and CSV.
- Deployment prerequisite: proxies must sanitize IP/host headers and preserve trusted client metadata (including Cloudflare for www); no production actions in this code task.

## Scope
Code and non-production tests only; no production migration/update. Existing unrelated working changes are preserved. Trellis helper scripts and unit-test spec index are absent, so task tracking is maintained manually.
