# Verification

## Changes
- The actual /admin/orders View action now opens the shared AdminOrderDetail modal, not the former duplicated inline fields.
- Purchase summary prioritizes product/type, multiplier, customer, payment state and recorded currency. Separate presale entitlement status/start/end distinguish a paid reservation from active access.
- Read-only admin summary adds historical/current plan-name provenance, current group/provider names and stored payment mode/currency. No current prices/quotas substituted for purchase snapshots. Missing metadata is explicit.
- Request host/referrer path/IP are separate from payment provider/method/instance. Referrer credentials/query/fragment are not displayed; origin-only referrers do not imply an entry page. Payment initiation can use existing ORDER_CREATED audit evidence.
- Lifecycle, failures, coupons, refund request/ledger information and expandable audit records retained. Payment deadline is distinct from service expiry.
- Typed API response, loading/retry UI and stale request guards; refund target separated from detail selection. No payment/refund execution rule changes.

## Checks
- Full frontend suite: 169 files / 1333 tests passed. After the final origin-only-referrer and timeline refinements, all 20 focused detail/view tests passed (13 component + 7 view).
- Frontend typecheck and preview build passed. Full lint: 0 errors, 12 unrelated existing warnings; final touched-file lint clean.
- Backend service/admin-handler order summary, response sanitization and presale unit tests passed. Backend service/admin-handler lint: 0 issues; local compilation passed.
- Real isolated PostgreSQL-backed admin order #9: snapshot name/currency/summary correct, provider snapshot excluded. Non-admin detail request returns 403; user order DTO remains restricted.
- Browser: Chinese dark at 1440/390px and English light at 1440/320px. Real presale #9 and browser-only EUR balance fixture both show appropriate product/currency/source/term information. No modal horizontal overflow or runtime errors. Audit expansion works; source tokens absent from visible text. Screenshots reviewed. No payment/refund/admin mutations performed.

## Release / preview boundaries
- Previous changes committed and pushed first as f37a5327b / cafecode-v0.0.89 at user request. This task is the subsequent cafecode-v0.0.90 commit/tag/push.
- Refreshed only isolated frontend assets and sub2api-test-app, with the existing pinned image and updated bind-mounted test binary. Port 4178 and test app healthy.
- Test image: ghcr.io/shirtiny/sub2api@sha256:6560b4e07a1fb72384eaab4dbc841fcd8f5a776d7456402573048571c923948b
- Test binary version: cafecode-v0.0.90-local-admin-order-detail
- Test binary SHA256: 21f72601e36f897daa687f27fba954f7d089fe1325ef99aee67ab20aa4c325bf
- Test migration count remains 230; entire test payment_orders rows match before/after. No schema changes, migrations or backfills. Production container IDs/start times unchanged; no production lifecycle/config/database writes.

Evidence: /var/tmp/admin-order-detail-*.log; /var/tmp/admin-order-detail-{production,db}-{before,after}.txt; /opt/stacks/sub2api-test/artifacts/admin-order-{detail,source,balance}-*.png and admin-order-detail-browser.json.
