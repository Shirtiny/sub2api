# Journal 1

## 2026-09-08: Pre-content stream overload recovery

Implemented a bounded same-account rescue for OpenAI gateway streaming routes,
with heartbeat-safe opening buffering and error handling before conversion.
Text/reasoning/tool activity commits immediately; no mid-output replay. Only one
extra forward per incoming request and no outer retry amplification on exhaustion.

Verification: 20 focused tests, 40 default backend test-bearing packages, focused
race checks, and full lint (0 issues). Includes a virtual 47-second delayed error
and loopback HTTP recovery without waiting for upstream EOF. Task and detailed
results: `../../tasks/09-08-stream-overload-retry/`.

No production deployment, new environment variables, migrations or release tag.
Unrelated untracked files were preserved. Recorded manually because this checkout
has no `.trellis/scripts` directory.

## 2026-09-08: Stream interruption follow-up

Reused the same one-rescue budget for known pre-content EOF/read/missing-terminal
failures; mapped native Aether error events before conversion. Healthy output
remains immediate. No replay after content, tool activity, unknown/malformed
output or explicit business denials; WebSocket forwarding bypasses the SSE gate.

Validation: focused service/handler tests, all 40 default backend test-bearing
packages, focused race tests and full lint (0 issues). Six processor tests cover
both heartbeat-then-EOF and heartbeat-then-native-error. Task/results:
`../../tasks/09-08-stream-interruption-fix/`.

The user explicitly authorized source commit/tag/push for `cafecode-v0.0.74`,
paired with Aether `backend-v0.7.115`. CI builds artifacts; no production deployment,
config or DB changes are part of this publication. Existing unrelated files were
preserved. Session recorded manually because Trellis scripts are absent.
