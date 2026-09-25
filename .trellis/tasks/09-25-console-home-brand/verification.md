# Shared console/home brand

- Replaced the console sidebar bitmap logo and separate bold title with the same `HomeBrand` component used by the public header. The configured site name and SVG steam animation are preserved.
- The shared router link navigates to `/home`. Clicking it also closes the mobile sidebar, so returning to the console does not reopen the drawer.
- Added an optional compact mode for collapsed sidebars: a 36px mark using the configured name's initial and the same vector lettering, retaining the full accessible name and home link. Non-Latin initials use the existing local-font fallback with a bounded viewBox. Default public-header rendering is unchanged.
- Sidebar colors use existing theme tokens; no custom brand asset, API, or backend changes.

## Verification

- Focused sidebar/brand/header suites: 28 tests passed.
- Full frontend suite: 171 files, 1,380 tests passed.
- Typecheck, targeted ESLint, frontend production build, and diff whitespace check passed.
- Browser checks with real isolated-test admin/user sessions: dark desktop 1440×900, light desktop 1280×800, dark mobile 390×844.
- Verified identical expanded SVG geometry between sidebar and home header; no old image; home navigation; collapsed 36px mark and keyboard navigation; expansion restoration; mobile drawer closure; correct theme colors; no browser runtime errors.
- Only refreshed the isolated test frontend at `http://152.53.90.186:4178`. No services restarted, no database mutations beyond test login bookkeeping, no production changes, and no commit/tag/push.
- Logs, previous index, and browser screenshots: `/opt/stacks/sub2api-test/artifacts/console-home-brand/`.
