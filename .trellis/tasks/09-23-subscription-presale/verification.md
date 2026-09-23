# Verification — subscription presales

## Scope and safety

Read AGENTS.md, Trellis workflow, frontend/backend quality, component/type, database,
error handling and cross-layer guides. Trellis scripts are absent; tracking files
are maintained directly. No production lifecycle/database/settings/account/email
mutations or real payment traffic. The implementation phase made no commits, tags
or pushes; the subsequently authorized publication is documented below. Existing email/auth/domain
changes remain intact. No new environment variables or dependencies.

## Implementation

- Public /presale, restrained calendar-ticket SVG animation, dynamic catalog,
  explicit dates, refund policy, balance CTA, reusable in-page existing checkout.
- Home navigation crossfade to open-presale banner, expanded billing explanation;
  console navigation, balance-first recharge, admin opt-in/visibility/badge/reset.
- Migration 201 + generated Ent fields; server-side calendar, quote, locked duplicate
  protection, snapshots, pending entitlement ledger, scheduled activation.
- Matching active term / renewal handling, strict overlap rejection, source-group
  custom multipliers, dated concurrency/early-reset entitlement, reset-card grants.
- User/admin refund quotes, reviewed amount precondition, timestamp freeze, exact
  term cancellation, original provider execution and safe retry state.
- Explicit pending/renewal dates in subscriptions/results/admin order detail, localised
  Chinese/English text. Provider must enable administrator refunds for new presales.

## Verification artifacts

All outputs are under /var/tmp/sub2api-presale-*; offline browser assets/screenshots
and scripts under /var/tmp/sub2api-presale-preview/. Browser APIs are intercepted;
external SDK requests blocked. Build output never overwrites a served production or
preview directory. PostgreSQL/Redis tests use harness-owned disposable containers.

Final results are recorded below.

## Confirmed results so far

- Frontend: **1173 tests in 161 files passed** (full suite); vue-tsc passed;
  ESLint has zero errors and the same 12 baseline warnings in existing files.
- Backend: full service/handler/admin/DTO/routes unit suites passed on the first
  complete run (service 105.418s); focused presale/refund/WeChat tests passed after
  the reviewed-refund precondition, refundable-channel and retry-state refinements.
- Disposable PostgreSQL/Redis integration: passed in 7.157s; eight concurrent
  create attempts produce one paid pending entitlement; four concurrent activation
  sweeps grant one snapshotted monthly term and one reset allowance. Real migrations
  applied only to that disposable DB. Test containers exited afterwards.
- Isolated Vite build passed (existing stale Browserslist / chunk-size warnings).
- First browser pass: **13 fixture-only scenarios passed**, including zh/en,
  light/dark, 390/1280/1440px and authenticated consent → payment → pending term.
- Follow-up checks include home header geometry/compact banner and expanded billing
  reachability; final logs are recorded below when complete.

## Final verification

- Full frontend suite: **1173 tests / 161 files passed**. Production build written
  only to `/var/tmp/sub2api-presale-preview/dist`; no served directory overwritten.
- Final Vue typecheck and targeted admin refund-dialog ESLint passed after the
  final UI adjustment. Full ESLint had zero errors and 12 pre-existing warnings.
- Final backend golangci-lint (new issues relative to HEAD, service/handler/routes/
  repository scopes) reported **0 issues**, including the last reset-grant fix.
- Full backend unit suites: service (109.103s), handler (25.610s), admin, DTO,
  quota-view and routes all passed. The final focused presale/refund regression
  also passed after the reset-grant cap regression was added (service 0.457s,
  handler 0.023s).
- PostgreSQL/Redis concurrent integration passed (7.157s), using disposable local
  harness containers and migrations, not production databases.
- Browser fixtures: **14 scenarios passed**, covering zh/en, light/dark,
  390/1280/1440px, consent and checkout through a paid pending reservation, home
  compact-header geometry and billing explanation reachability. Public APIs and
  payments were mocked; no real payment or external SDK traffic.
- Refund review protects against crossing a policy cutoff after confirmation;
  provider failures never reactivate cancelled entitlement; cap-limited reset
  grants are audited and refunds do not deduct the uncredited portion.
- `git diff --check` passed. Deployment remains **not performed**; migration 201,
  plan presale flags and payment-provider refund settings remain unapplied to any
  production environment. Existing unrelated email/auth edits remain untouched.

Final logs: `/var/tmp/sub2api-presale-final-{tests,typecheck,frontend-lint,backend-tests,backend-lint,build}.log`,
`/var/tmp/sub2api-presale-last-regression.log`,
`/var/tmp/sub2api-presale-integration.log`, and
`/var/tmp/sub2api-presale-preview/browser-check.log`.

## Authorized publication — cafecode-v0.0.84

- User requested commit, push and tag. Publish the completed presale feature plus
  the previously completed shared email theme, existing-account waitlist recovery,
  and stale frontend-URL settings protection; all were verified together above.
- Remote `custom-prod` still points to `5747c474ab84105dbb8521e33d442f33d157a8d1`
  (`cafecode-v0.0.83`); `cafecode-v0.0.84` is unused. Create an annotated tag and push
  branch/tag atomically without rewriting existing history or tags.
- Additional final check: the server command compiles with unit tags (`go test
  -tags=unit ./cmd/server -run '^$'` passed). Prior full tests/build/browser checks
  remain applicable; no application source changed during publication preparation.
- Only source, tests and project documentation are included. Concept assets, root
  temporary files and local operational history remain untracked and untouched.
- The inspected tag workflow builds/publishes an image and build-record artifact;
  it does not deploy. No service restart, production migration, account/payment/
  email action, preview change or production configuration change is authorized
  or performed. Migration 201 and presale settings require a later release action.
- Publication checks and ref verification are recorded locally under
  `/var/tmp/sub2api-presale-publish-084/`.
