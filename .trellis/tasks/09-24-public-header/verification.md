# Public header verification

## Implementation

- Extracted the existing homepage header into `HomeHeader.vue`; both public pages
  render it rather than maintaining different markup/styles. The pages supply
  settings, authentication, theme and scroll state; checkout/content is unchanged.
- Desktop uses a flexible brand column and two content-sized right columns.
  Navigation sits 24px before the action group, not at the viewport center.
  Mobile navigation is also right-aligned, with reserved space when a sale closes.
- Presale now has the same vector logo, backend-configured name/docs, locale,
  dark/light toggle, login/user/admin dashboard entry, sticky glass, shadows and
  open-sale announcement. Document scrolling uses the slider's 64px/8px hysteresis.
- Clear inactive navigation's `inert` attribute entirely instead of rendering
  `inert="false"`; hidden menu/banner links do not remain interactive.
- Header dimensions remain fixed during compact transitions. Narrow layouts fit
  the reserved header area; the new sticky header has a plans-anchor offset.

## Checks

- Full frontend suite: **1194 tests / 162 files passed**.
- Vue typecheck passed; ESLint: zero errors and the same 12 existing warnings.
- Isolated Vite build passed (existing Browserslist/chunk-size warnings).
- Real Chromium: **20 scenarios passed**, comparing home/presale geometry at
  1440, 1024, 768, 390 and 320px in Chinese/dark and English/light. Checks cover
  right alignment, cross-page brand/menu/button dimensions, no overflow/overlap,
  sticky glass, stable transition geometry, enter/exit thresholds and theme toggles.
  Screenshots were visually inspected at desktop and mobile sizes.
- The browser harness disables homepage CSS scroll snapping only for deterministic
  intermediate scroll-offset assertions; otherwise snapping returns 32px to zero.
- `git diff --check` passed. All browser API data is intercepted fixture data;
  no real payment, email or production database requests.

## Artifacts and safety

Logs, fixture script, measurements and screenshots are under
`/var/tmp/sub2api-public-header/`. Build output is confined to its `dist/` directory;
the fixture-only HTTP server binds an ephemeral loopback port and closes after use.
No existing production/preview assets or services were changed. No dependencies,
migrations, commit, tag, push or deployment. Unrelated local assets remain intact.
