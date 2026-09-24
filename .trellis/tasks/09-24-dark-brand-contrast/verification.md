# Verification

- `pnpm test:run`: 163 files, 1197 tests passed (including 3 new palette tests).
- `pnpm typecheck`: passed.
- `pnpm lint:check`: no errors; 12 existing unused-variable warnings.
- Isolated Vite build: passed; existing Browserslist/chunk-size warnings remain.
- `git diff --check`: passed.
- Fixture-only Chromium checks: before/after text palette, hover, exact light-mode
  color parity, exact button fill/label parity, plus presale/login/purchase in both
  themes. No page errors or unexpected API requests in the final run.
- Visually inspected the dark palette and real console purchase screen. Primary-400
  text on the card surface improves from 2.46:1 to 9.34:1. All primary text shades
  tested on page/card/secondary/hover/dark-700 surfaces exceed 4.5:1; representative
  selected backgrounds and existing text opacity modifiers also pass.
- Artifacts: `/var/tmp/sub2api-dark-brand-contrast/` (logs, isolated dist,
  browser harness, JSON results, before/after palette and real-page screenshots).
- Browser served only an ephemeral loopback port; all API responses were mocked,
  external requests blocked, and the server closed. No live services, public
  preview artifacts, databases, payments or deployments changed.
- Existing pending shared-header edits preserved. No commit, tag or push.
