# Verification

## Changes
- Replaced the spinning orbital decoration with a warm paper stack, fine coffee
  cup outline, soft local lighting and slow steam motion (SVG/CSS only).
- The ticket pairs the large numeric month with jan–dec, derived from the catalog
  period. Localized month/year, next-year marker and service dates remain intact.
- Light and dark ticket colors are separate, including readable small-print colors.
- Preserved the pending homepage copy fix and all existing checkout behavior.

## Checks
- Frontend suite: 163 files / 1211 tests passed.
- Focused final presale tests: 18 passed, covering all 12 abbreviations, both
  locales, a UTC/Shanghai year boundary and the unavailable-data fallback.
- Typecheck passed. ESLint: zero errors, 12 existing warnings; final changed-file
  lint also passed. `git diff --check` passed.
- Final isolated Vite build (`dist-verified`): exit 0; existing Browserslist and
  bundle-size warnings only. An earlier rebuild emitted successfully but its
  command wrapper exited 143; the clean final rerun confirmed successful exit.
- Chromium: 20 fixture-only cases (Chinese/English, dark/light, widths 1440, 1024,
  768, 390 and 320). Verified month labels, paper colors, steam animation, ticket
  containment and no horizontal overflow; no page errors.
- Visually checked full desktop/mobile dark layouts and the light ticket detail.
- Artifacts: `/var/tmp/sub2api-presale-month-card/`, including final build log,
  browser harness/results and screenshots. All API calls mocked, external network
  blocked, temporary loopback server closed after checks.
- No commit, tag, push, public preview restart or production deployment.

## Typography follow-up
- Month abbreviations are lowercase (`10 oct`) with restrained serif italics.
- Reduced the month numeral from 112 to 100px (mobile 92 to 84px), balanced the
  lowercase label at 26px/24px, and kept the pair centered on a shared baseline
  with 12px spacing and more even vertical breathing room.
- All 18 presale tests, changed-file ESLint, isolated Vite build and diff checks
  passed. Chromium verified 12 light/dark, Chinese/English, 1440/390/320px cases,
  including exact sizes, baseline alignment, lowercase text and no overflow.
- Visually inspected the final card. Follow-up artifacts are in
  `/var/tmp/sub2api-presale-month-type/`. No production or publication actions.
