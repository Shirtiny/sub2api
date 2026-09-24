# Verification

## Implementation
- Catalog and creation-time validation both use for_sale + presale_enabled, retaining active/non-deleted/ordinary subscription group checks. Retired visibility values cannot silently hide a published plan.
- Admin form/API no longer expose or accept the duplicate visibility switch or fixed reset-card bonus. Badge and normal plan configuration remain. New order snapshots default to zero reset bonus even when a legacy plan stores a nonzero count.
- Existing paid/order reset snapshots and their activation/refund handling are preserved. No migration, deletion, backfill or official reset-grant automation.
- Shared homepage/presale benefits use official reset-card issuance and concise daily-refund wording. Detailed refund policies are unchanged. Chinese and English are aligned.
- Removed timezone captions from landing timeline/policy/footer, subscription cards and order term. Removed caption-specific spacing and redundant card wrapper; balanced timeline padding and centered the simple footer. UTC+8 date formatting and calendar boundaries are unchanged.

## Checks
- 136 focused frontend tests passed. Full frontend: 167 files / 1315 tests passed.
- Frontend typecheck/build passed; lint has 0 errors and 12 existing unrelated warnings.
- Backend presale/create/update-plan unit tests passed in service and handler packages, including legacy visibility, ignored retired fields, new zero-bonus orders/activation and historical bonus preservation.
- Real disposable PostgreSQL concurrent presale purchase/activation integration test passed, including a legacy-hidden, nonzero-bonus plan. No shared/production DB used.
- Backend service/handler lint: 0 issues. Local backend compilation passed.
- Browser: Chinese dark at 1440/390px and English light at 1440/320px; no horizontal overflow, no timezone captions, accurate copy and detailed refund policy retained. Admin edit has one presale checkbox, no reset input/explanation/link. Existing pending subscription renders cleanly with unchanged dates. No JS runtime errors. Screenshots reviewed.
- Browser disallowed payment/admin writes; auth-history telemetry fulfilled locally. No plan/order mutations in the persistent preview database. Catalog GET confirms no fixed reset-card bonus advertised.

## Isolated preview only
- Refreshed frontend assets and recreated only sub2api-test-app, using the existing immutable image and rebuilt bind-mounted binary. App and port 4178 health OK.
- Image: ghcr.io/shirtiny/sub2api@sha256:6560b4e07a1fb72384eaab4dbc841fcd8f5a776d7456402573048571c923948b
- Binary version: cafecode-v0.0.88-local-presale-simplify
- Binary SHA256: cb0c5ae9a6e3688f7bf52724980ac8bd4022037932496cb4785d66e0c3bac429
- Test migrations: 230, unchanged. Existing test order #9 whole-row snapshot unchanged.
- Production container IDs/start times unchanged. No production lifecycle/config/database operations. No commit/tag/push.

Logs: /var/tmp/presale-simplify-*.log
State comparisons: /var/tmp/presale-simplify-{production,test-db}-{before,after}.txt
Screenshots/report: /opt/stacks/sub2api-test/artifacts/presale-simplify-*
