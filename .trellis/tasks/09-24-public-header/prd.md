# Shared public header

- Move the homepage presale menu alongside the right-hand action group, not the
  center of the page. Right-align the secondary navigation on narrow screens.
- Render the same header on the presale landing page: vector branding, backend
  site name/docs, locale/theme controls, login/dashboard routing, sticky glass and
  open-sale banner. Preserve both pages' content and checkout flows.
- Keep header geometry stable while changing state; use the same 64px/8px scroll
  hysteresis on the landing page as the homepage slider. Respect mobile widths.
- Verify component/page regressions, typecheck/lint and isolated browser fixtures.
- No production actions, preview replacement, new dependencies, commit or publish.
