# Campaign UI polish

## Scope

- Add responsive padding to the campaign toolbar and policy footer inside the otherwise unpadded table card. Preserve the constrained flex/scroll chain and fixed pagination.
- Reserve a non-shrinking, non-wrapping `CAFE-PUBLIC-` prefix; let only the suffix input flex. Use a single field border/focus ring and an explicit input label.
- Pass the campaign success message to the shared clipboard composable instead of emitting a second toast in the caller.
- No pricing, campaign validity, eligibility, or payment changes.

## Verification

- Added regression tests for padding, prefix/input layout, single success notification on both secure clipboard and HTTP fallback paths, and failure-only notification on copy failure. The old implementation reproduced the duplicate notification (two calls instead of one).
- Focused campaign/clipboard suites: 27 tests passed.
- Full frontend suite: 171 files, 1,377 tests passed.
- Typecheck and production frontend build passed. Targeted ESLint passed; full lint has zero errors and 12 existing warnings outside these changes.
- Browser checks at 1440×900, 1280×600, 390×844, and 320×740: toolbar/footer padding, desktop table scrolling, one-line prefix, non-overflowing dialog, and exactly one notification passed. Localhost used the real browser clipboard; HTTP IP access exercised the fallback path. No browser runtime errors.
- Browser list tests intercepted the campaign list GET with 20 fixture rows to test scrolling without adding test records. Auth and frontend came from the isolated local test environment. No campaign/order writes were made.

## Preview

- Refreshed only `/opt/stacks/sub2api-test/frontend`, retaining old hashed assets and backing up the previous index.
- Preview: `http://152.53.90.186:4178/admin/promo-codes` → shared café campaigns tab.
- Backend, test account credentials/data, and production services unchanged. No commit/tag/push performed.
- Build/test/browser logs and screenshots: `/opt/stacks/sub2api-test/artifacts/campaign-ui-polish/`.
- Trellis session helper scripts are absent in this checkout; workflow/specs were read directly.
