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

## 2026-09-08: Cancelled-stream billing regression

Fixed the retry wrapper incorrectly replacing processor-approved drained usage
with a client cancellation/write error, causing handlers to skip RecordUsage.
Only successful non-nil results without a captured upstream/HTTP error are retained;
client disconnect is marked and replay/final wrapper writes remain prohibited.
Duration/first-content timing still includes prior rescue attempts.

Validation: 8 new top-level tests; all 36 stream-retry tests under race; all 40
default backend test-bearing packages; full lint 0 issues. Covers six processors,
wallet/subscription command construction, original billing key/fingerprint,
detached handler worker/synchronous usage contexts, and upstream failure guards.
Task: `../../tasks/09-08-cancelled-stream-billing/`.

No commit, tag, push, deployment, configuration change or retroactive charge in
this task. Only the local Sub2API source/docs changed. Recorded manually because
Trellis scripts are absent.

## 2026-09-19: Homepage wordmark and subtle tool rotation

Replaced the narrow italic wordmark with wider upright Lora SVG outlines while
retaining the backend site name. Added a separate pausable Codex/pi icon loop,
with reduced-motion and mobile layout support. No runtime font download.
25 targeted tests, typecheck, scoped lint and isolated build pass. Full suite:
925 pass, five existing failures; full lint retains 12 unrelated warnings.
Only the user-authorized temporary preview on port 4178 was updated; no production
changes, commit or push. Browser checks/details are in
`../../tasks/09-18-public-home-redesign/verification.md`.

### Homepage follow-up: caption-free brand and native SVG choreography

Removed the wordmark's rule/caption and the icon frame. Replaced whole-image
fades with native SVG contour, terminal-face, ink-fill and three-part pi drawing
animations. Retained the exact product geometry and accessibility controls.
29 targeted tests/typecheck/build pass; full suite has 929 passes and the same
five unrelated failures. Seven browser layouts checked. Only the temporary
preview on 4178 was updated; detailed verification remains in the homepage task.

### Homepage refinement: upper-right icon

Shrank the existing SVG animation and anchored it to the site's upper-right
lettering edge using the computed wordmark width. Preserved the drawing timeline.
29 targeted tests, typecheck, scoped lint and eight browser layouts pass.
Updated only the temporary preview; details remain in the homepage task log.

### Homepage narrative: coffee steam, letter impacts and liquid pi

Replaced the corner icon for eligible configured names with a 22-second native
SVG story: the é accent condenses into Codex, parabolic impacts compress S/o,
the cloud sinks into p, and monochrome pi drains back to the exact original p.
Added per-glyph geometry, matching contour morphs and native-clock accessibility
controls; other configured names remain intact with the corner fallback.
36 targeted tests/typecheck/build/scoped lint pass; eight real-browser layouts
checked after initial sandbox restrictions were lifted. Full suite retains five
unrelated failures (935 passes before final refinements). Only the authorized
4178 static preview was updated; details are in the homepage verification log.

## 2026-09-20: Permanent steam and separate cloud birth

Made steam the initial/resting accent and added a separate cloud-condensation
phase before Codex drops out with rotation. Kept the accepted collision/pi/liquid
sequence. 37 targeted tests pass; typecheck/full lint pass with existing warnings;
full suite has 937 passes and the same five baseline failures. Offline timeline
review only: the current sandbox blocks sockets and Chromium. Port 4178 has NOT
been refreshed. Isolated build and details are in the homepage verification log.

### Cloud preview continuation

After environment access was restored, all eight real-browser layout/animation/
axe checks passed. Refreshed only the existing temporary 4178 preview to the
prepared cloud build; local and external HTTP 200. No production changes.

## 2026-09-20: More fluid vapor and a lighter cloud

Replaced fixed-looking steam with two offset, upward-traveling tapered ribbons
on a 4.4-second native SVG cycle. Rising stroke segments now gather into a
hand-drawn cloud outline instead of a flattened filled blob; accepted Codex
collisions and pi/liquid phases are unchanged. 38 targeted tests, typecheck,
isolated build and lint pass (12 existing warnings). Eight real-browser layouts
and axe checks pass. Refreshed only the authorized static preview on 4178;
local/external HTTP 200. Details/artifacts are in the homepage verification log.

## 2026-09-20: Restore the accepted first steam

Recovered the exact original single-wisp contours and unfurling interval from
its retained preview build; added an independently recovered regression fixture.
Removed the later double strands/branches, retained initial steam and the current
cloud/Codex/pi sequence. 38 targeted tests, typecheck, build and lint pass;
full suite has 938 passes and the same five unrelated failures. Eight browser
layouts also verify original contour interpolation and accessibility. Updated
only the authorized 4178 preview; local/external HTTP 200. See verification log.

## 2026-09-21 — Public homepage policy presentation
- Made Usage & Pricing permanently visible with an enforcement banner, Codex/Pi client panel and subscription/balance split. Important terms highlighted; responsive light/dark styles and translated copy retained.
- Passed 19 homepage tests, scoped lint, typecheck and isolated build. Updated temporary static preview using complete compiler output and an entry bridge for cached preview HTML; production untouched. Browser checks restricted by sandbox.

## 2026-09-21 — Animated model gallery
- Added inline SVG ChatGPT/Claude/Grok/Gemini gallery above everyday features with distinct restrained motion; only ChatGPT available. Pause, reduced-motion, viewport/background handling included.
- 46 related tests, scoped lint, typecheck and isolated build passed. Temporary preview refreshed from full build; no production changes. Browser verification remains sandbox-restricted.

## 2026-09-21 — Celestial model animations based on verified official visuals
- Network research confirmed GPT-5.6 Luna/Terra/Sol and GPT-6 Astra visuals via official API documentation; corrected Astra spelling. Built independent locally rendered moon, Earth, Sun and galaxy scenes, not claimed as official animation code.
- CC BY 4.0 texture attribution included; 20fps canvas work pauses offscreen/background and honors reduced-motion. No new runtime dependencies or model requests.
- Scoped lint/typecheck/build and 7 new/model tests passed; one unrelated header assertion was stale after concurrent umbrella animation changes. Chromium verified actual motion, four viewport sizes, dark/reduced modes and offscreen pause. Temporary preview only; production untouched.

## 2026-09-21 — Stylized SVG celestial models
- Swapped realistic textures/canvas for pure inline SVG and CSS; explicit Luna/Terra/Sol radii 26/48/80 on one viewBox. Distinct phase/globe/sun/galaxy motion and reduced-motion/offscreen handling retained.
- Removed obsolete renderer and texture assets. 31 focused tests passed (one unrelated known header assertion skipped); scoped lint/typecheck/build and real Chromium checks passed. Temporary preview updated; production unchanged.

## 2026-09-21 — Official-reference vector redraw
- Reworked the four scenes toward official model artwork cues rather than generic icons, retaining SVG-only rendering and 26/48/80 size hierarchy. Gray lunar surface, blue/clouded globe, warm solar filaments and SVG-star galaxy.
- 12 focused tests plus lint/typecheck/build passed; real-browser responsive, animation, dark, reduced-motion and offscreen checks passed. Temporary preview synchronized; production untouched.

### 2026-09-21 — Model scene refinement
- Refined Luna matte surface, Terra vector coastlines/clouds and Sol flowing ribbons; rounded Astra by removing vertical compression and using 480 stars. Kept the approved layout, reveal sequence and body size hierarchy.
- Added round-galaxy regression coverage and documented public-domain coastline provenance. 33 focused tests passed (one unrelated stale header assertion excluded), lint/typecheck/isolated build passed; browser checks and desktop/mobile inspection passed.
- Only temporary homepage preview refreshed (HTTP 200); production untouched. See task verification for build, backup and screenshot paths.

### 2026-09-21 — Sol surface correction
- Removed intersecting solar strokes that resembled basketball seams. Replaced with soft vector light cells and subtle filled limb flames; other model scenes unchanged.
- 13 tests, lint/typecheck/build and browser checks passed. Temporary preview refreshed only; screenshots and backup recorded in task verification.

### 2026-09-21 — Stellar weight
- Reworked Sol into deep ember SVG plasma with a bright irregular corona, replacing cartoon-like soft dots. Increased Astra diameter by one third and slowed rotation to 240s/revolution (from 45s).
- 13 tests and lint/typecheck/build/browser checks passed, including computed animation speed, accessibility motion settings and responsive layouts. Final visual inspected. Only temporary preview refreshed; production untouched.

### 2026-09-21 — Brightness and visible animation
- Brightened solar plasma to gold-orange and made Luna/Terra/Sol motion clearly perceptible; preserved Astra's slow 240s rotation. Prevented moving moon shadow from revealing an unshaded edge strip.
- 13 tests, lint/typecheck/build and browser checks passed. Actual before/after pixel comparisons confirmed visible change for all three planets (57.4%/27.1%/57.4%); not just nonidentical transform values. Only temporary preview refreshed, public HTTP 200, production unchanged.

### 2026-09-21 — Spherical rotation, not planar spinning
- Replaced planet surface CSS spins with orthographic geographic projection, limb compression and hemisphere clipping. Fixed lighting/axis; retained solar brightness and slow Astra.
- Pinned existing d3-geo/type versions directly; added geographic source generator and physical-projection regression tests. 18 core tests, lint/typecheck/build and browser checks passed, including frozen SVG paths while paused/reduced-motion. Preview only; production untouched. See task verification for screenshots and backup.

### 2026-09-21 — Reduce Sol flame maximum
- Reduced only corona/prominence peak scales to 1.015, preserving brightness and all rotation behavior. 14 tests and build/lint/browser peak checks passed. Temporary preview only; production untouched.

### 2026-09-21 — Earth cloud refinement
- Replaced ring-like zonal cloud strips with broken, oblique cloud systems and a thin translucent veil; retained true spherical projection/drift. Added cloud-width regression at four longitudes.
- Tests, lint/build/typecheck and browser checks passed; visually inspected Earth detail. Only temporary preview refreshed; production unchanged.

### 2026-09-21 — Lunar texture refinement
- Added fine mineral grain and 111 small geodesic crater depressions/rim details; softened and connected mare shapes. Preserved spherical rotation and other models.
- Rendering/projection regression tests, lint/build/typecheck and browser checks passed. Desktop/detail inspected. Only temporary preview refreshed; production unchanged.

### 2026-09-21 — Simplify lunar illustration
- Removed gritty texture and dense crater detail. Kept low-contrast mare layers, a few shallow craters and spherical rotation; other models unchanged.
- 20 tests, lint/build/typecheck and browser checks passed. Temporary preview refreshed and visually inspected; production untouched.

### 2026-09-21 — Recognizable lunar landmarks
- Removed mare patches; kept only a few clearly visible larger craters on a clean silver sphere. Preserved spherical motion and other models.
- 20 tests, lint/build/typecheck and browser checks passed; desktop/detail visually inspected. Temporary preview only, production unchanged.

### 2026-09-21 — Center planetary illustrations
- Horizontally centered Luna/Terra/Sol including clipping and solar corona origin; preserved artwork, sizes and motion. 20 tests and browser center measurements across four widths passed, plus lint/build/typecheck. Temporary preview only; production untouched.

### 2026-09-21 — Subtle lunar finish
- Added restrained rock-like tonal texture and softened crater edges without returning to dense realism. Large crater landmarks, centering and spherical motion preserved.
- 20 tests, lint/build/typecheck and browser checks passed; detail visually inspected. Temporary preview only; production untouched.

### 2026-09-21 — Refine lunar crater lighting
- Added per-crater bowl gradients and basin light; tapered/thinned the bright rim and softened inner-wall shadows. Preserved the eight landmark locations, radii, centered placement and spherical motion.
- 21 tests, lint/build/typecheck and browser checks passed. Normal-size and 4x crater detail inspected. Temporary preview refreshed only; production untouched.

### 2026-09-21 — Rework crater shape
- Changed smooth circular scoops into irregular-rimmed pits with inset floors, a light terrace and central relief on larger craters only. Preserved spherical projection and all other model behavior.
- Tests, lint/build/typecheck and browser checks passed; normal and 4x detail inspected. Temporary preview updated only; production unchanged.

### 2026-09-21 — Sticky header and homepage section slider
- Added six focused native vertical chapters with animated sticky-header contraction, desktop chapter rail/mobile dock, direct smooth navigation and reduced-motion handling. Kept long content scrollable and preserved native touch/wheel, keyboard/anchor access, custom homepages and public settings.
- Moved supported models immediately after Made for your everyday; preserved SVG/logo artwork and motion. All 69 homepage tests passed; lint/typecheck/isolated build and responsive/browser/celestial checks passed. Full suite retains the same five unrelated baseline failures (969 pass).
- Updated only the authorized static preview (HTTP 200); production untouched. See task verification for artifacts, build and backup.

### 2026-09-21 — Stable frosted header and staged section motion
- Kept header logo/action geometry fixed; only the background morphs into actual translucent backdrop glass, with enter/exit hysteresis. Removed native snap-back and replaced chapter fly-through with interruptible outgoing/incoming dissolves.
- Added staggered heading/copy/button/card reveals, reset only when their chapter leaves; model reveal wrappers preserve existing artwork and physical motion. 74 targeted tests, lint/typecheck/isolated build and browser geometry/stagger/responsive/celestial checks passed. Full suite: 974 pass, same five unrelated baseline failures.
- Updated only the isolated static preview (HTTP 200); production unchanged. See task verification for build, backup and screenshots.

### 2026-09-21 — Remove glass-edge settling motion; strengthen shadow
- Replaced animated blur-region geometry with two fixed-size backdrop planes that crossfade opacity only. Kept persistent compositing hints and existing foreground geometry; increased contact/ambient shadows with a dark variant.
- 34 tests, scoped lint, isolated build/type checking and four-width browser frame sampling passed (fixed geometry, monotonic settling, reverse/reduced motion). Updated static preview only, HTTP 200; production unchanged.

### 2026-09-21 — Reference Aether's continuous chapter motion
- Read the local Aether homepage/helper as a motion reference only; independently replaced whole-main dissolves and delayed jumps with native smooth chapter travel, desktop snapping and viewport-progress staggered arrivals. Retained long mobile content, reduced motion, glass header/shadow and all SVG artwork.
- 75 homepage tests, final 12-test slider retarget regression, lint/typecheck/isolated build and responsive/motion/celestial browser checks passed. Full suite: 975 pass, same five baseline failures. Preview refreshed (HTTP 200) only; production untouched. See task verification for artifacts and backup.

### 2026-09-21 — Append everyday SVG feature carousel
- Preserved the existing everyday cards and added an eight-second, accessible SVG carousel: woven intelligence signals, three-route low-latency stream, and ledger verification. Independent vector art, bilingual copy, cafe light/dark palette, direct/keyboard selection, visibility gating and reduced-motion support; no extra libraries or data calls.
- 86 homepage tests passed; lint/typecheck/isolated build and five-width browser checks passed, including actual autoplay, moving SVGs and nested navigation. Full suite retains the same five unrelated failures (986 passing). Desktop section fits 1440×900; long layouts stay scrollable.
- Updated only the isolated preview (HTTP 200), with pre-feature and pre-final backups. Production unchanged; no commit/push. See task verification for logs and artifacts.

### 2026-09-21 — Unify feature cards with SVG selection; merge closing into pricing
- Original feature cards are now the only carousel controls, with full-card click targets, active/progress synchronization and a shared surface. Removed duplicated tabs; preserved original content, scene artwork, keyboard/manual/autoplay behavior and motion preferences.
- Merged the complete closing callout beneath pricing and kept the footer in that final chapter; navigation now has five stops. Header and model scenes unchanged.
- 87 homepage tests, lint/typecheck/build and responsive browser checks passed. Full suite retains five unrelated baseline failures (987 passing). Updated only the authorized static preview, with backup; no production changes or commit/push.

### 2026-09-21 — Roomier chapters and full-width SVG tableaux
- Preserved first-slide/header geometry; expanded all subsequent chapter capacity, section gutters and internal spacing. Kept five chapters, original feature selection, merged pricing close and native long-content scrolling.
- Removed redundant carousel copy and redrew rich neural-processing, transport and audit SVG scenes at fixed landscape proportions, with a complete portrait recomposition for phones. Existing visibility/reduced-motion controls and celestial artwork preserved.
- 94 homepage tests, lint/typecheck/isolated build and five-width browser checks passed; first-screen before/after geometry matches exactly. Full suite: 994 pass, same five unrelated failures. Preview updated with backups; production untouched, no commit/push.

### 2026-09-22 — Restore single-screen desktop chapter sizing
- Removed the oversized chapter minimum introduced in the previous revision. Added desktop height-aware spacing and text/art grid sizing; preserved complete copy, SVG proportions, original first screen/header and mobile scrolling fallback.
- 94 homepage tests, lint/typecheck/build passed; full suite still has five unrelated failures (994 passing). Prepared desktop geometry browser assertions, but this session cannot launch Chromium or read the preview port under the restricted sandbox; no visual/HTTP verification claimed.
- Refreshed only preview files offline using the recorded cached/current entry aliases and backed up affected files. Production unchanged; no commit/push. See verification for the exact limitation and pending browser check.


### 2026-09-22 — Fill pricing's vertical space, not its width
- Changed only final-chapter desktop layout: available-height grid, flexible vertically centered policy cards, height-aware block padding, and closing/footer at the bottom. Kept horizontal sizing, full content, other slides and short/mobile fallback.
- 94 homepage tests, lint/typecheck/isolated build passed; full suite retains the same five unrelated failures (994 passing). Prepared/syntax-checked vertical geometry regression; browser and HTTP checks remain unavailable under the recorded sandbox restriction.
- Refreshed only static preview files offline with backup and import validation. No production changes or commit/push. See task verification for artifacts and pending browser verification.


### 2026-09-22 — Restore cards; make only the closing gap flexible
- Followed the user's clarification: original card padding/block layout restored; only the space before Make room for an idea takes spare desktop height. Kept the full-height wrapper, minimum gap and small-screen fallback.
- 94 homepage tests, scoped lint and isolated Vite/type-checker build passed. Updated preview files only, with backup. Browser/HTTP checks remain unavailable; prepared corrected intrinsic-card geometry assertions. Production untouched; no commit/push.


### 2026-09-22 — Dedicated billing and compact setup / request controls
- Expanded subscription/balance details into the former guide chapter; moved setup into the opening of Usage & Pricing. Replaced the old billing card with request format and audit-retention copy: ordinary content one day; Cyber and violating request content permanent. Presentation only, not backend retention changes.
- Preserved five chapters, existing guide links through nested-anchor resolution, intrinsic policy-card height and the flexible closing gap. Added both-locale copy/order/billing and nested-anchor regressions.
- 99 homepage tests, full lint/typecheck/build passed; full suite 999 pass with the same five unrelated failures. Preview files updated offline with backup; browser/HTTP checks remain unavailable. No production changes or commit/push. Exact request-format rule asked asynchronously; current wording neutrally follows client/setup documentation.


### 2026-09-22 — Clearer request-control presentation
- Condensed repeated prose into one format/audit summary and a responsive ordinary-versus-exception retention comparison. Preserved one-day and permanent rules explicitly in both languages; no backend change.
- 99 homepage tests, full lint/typecheck and isolated build passed. Refreshed preview files offline with backup; browser/HTTP verification remains unavailable. No production changes or commit/push.


### 2026-09-22 — Align quick-guide content and add breathing room below
- Gave title/docs their own header and the three steps equal-width columns below. Added a 24–32px gap before Usage Policy; phone stacking and existing content/motion preserved.
- 99 homepage tests, scoped lint and Vite/type-checker build passed. Preview files refreshed offline with backup. Prepared geometry checks, but browser/HTTP verification remains unavailable; production untouched, no commit/push.


### 2026-09-22 — Remove client-button external-link glyphs
- Removed only the arrow glyphs from Codex/Pi buttons; links and brand marks remain. 29 view tests, scoped lint and build passed. Refreshed preview files offline; production untouched.


### 2026-09-22 — Use shared Sub2API primary action styling
- Both homepage CTAs now use global btn/btn-primary gradient, shadows and interactive sheen instead of local flat colors. 30 view tests, scoped lint and build passed. Preview refreshed offline; production untouched.


### 2026-09-22 — Faster ChatGPT intro and manual persistent brand toggle
- Tightened initial brand-to-model sequence to ~3.6s and added a visibility/reduced-motion-aware 36s ChatGPT-mark rotation. The mark becomes an accessible control after intro: models → persistent brand, then another click → models. Manual brand never auto-advances and hidden scenes pause.
- 101 homepage tests, scoped lint, typecheck and isolated build passed. Prepared browser timing/state checks but cannot execute Chromium/HTTP in this sandbox. Preview files refreshed offline with backup; production untouched, no commit/push.


### 2026-09-22 — Pointer-only centered ChatGPT switch
- Removed tooltip/keyboard semantics from the ChatGPT mark. Fixed the second click by placing brand above and disabling pointer events on the hidden grid. Manual brand is now centered on both axes within the content area and still persists until clicked.
- 44 focused tests, scoped lint, typecheck and build passed. Prepared center/return browser assertions but cannot run them in this sandbox. Preview refreshed offline; production untouched.


### 2026-09-22 — Native vector rotation for the compact ChatGPT mark
- Replaced CSS rotation of the HTML/SVG wrapper with native SVG path-group rotation, retaining geometry, size, centered stage switching and 36s speed. Existing visibility gating pauses/resumes the SVG clock without remount/reset.
- 103 homepage tests, scoped lint and isolated build/type-checker passed. Preview files updated offline with backup; browser/visual verification remains unavailable. No production changes or commit/push.


### 2026-09-22 — Compact feature tabs and ASTRA galaxy travel
- Moved the three original descriptions into their scene panels and reduced selectors to icon/title/index. Redrew intelligence as a full-bleed SVG ASTRA galaxy with layered stars, luminous core, batched dust and forward-flight trails. Other artwork and homepage sections unchanged.
- 106 homepage tests, full lint/typecheck and final build passed; full suite 1006 pass with five known unrelated failures. Inspected actual SVG artwork via offline librsvg landscape/portrait renders; browser animation/layout checks remain pending in this sandbox.
- Updated only preview files offline, with backup/import checks. No production change or commit/push. See verification for tests, static renders, pending browser checks and publication artifacts.

### 2026-09-22 — Looping first-person ASTRA film
- Replaced the oblique galaxy with a 28s looping first-person flight: perspective stars, passing nebula, three timed copy passages and a final vector ASTRA wordmark. Motion continues through the end card and returns to the opening composition. Removed playback/countdown controls and automatic tab advancement; hover/focus do not stop the film.
- 105 homepage tests pass; full suite 1005 pass with five known unrelated failures. Full lint/typecheck and isolated build pass. Inspected offline vector shot compositions; actual browser animation/layout remains unverified in the restricted environment.
- Preview files refreshed offline to `index-B-oXqgYW.js`, with backup and import validation. No production changes or commit/push. See task verification for artifacts and limitations.

### 2026-09-22 — Sharper and faster ASTRA flight
- Removed the cloudy blur layers; added sharp tapered trails, brighter heads and a close-flyby layer. Reworked perspective depth/cross-section so stars travel across the frame immediately instead of clustering at the center. Halved the film/copy loop to 14s; uninterrupted looping and manual feature selection remain.
- 106 homepage tests and final 20 focused tests pass. Full suite: 1006 pass, five known unrelated failures. Lint/typecheck/build pass; static landscape/portrait art checked, live browser motion remains unverified.
- Updated preview files only to `index-tEX0ACKp.js`, with backup/import checks. No production change, commit or push.

### 2026-09-22 — Quiet captions, distinct shots and batched particle motion
- Reduced captions to short, muted, drifting asides. Reworked the 14s film into grazing approach, offset warp, gravitational light bending and ASTRA arrival instead of repeated star-field shots.
- Batched 300 independent particle animations into 12 depth sheets; measured SVG elements decrease from 1,804 to 101. No new JS frame clock/filter/dependency. Live browser FPS remains unverified; offline shot artwork and geometry budget checked.
- 108 homepage tests and final 22 focused tests pass; full suite 1008 pass with five existing unrelated failures. Lint/typecheck/build pass. Preview files updated to `index-DGF4-Ny2.js` with backup/import checks; production untouched, no commit/push.

### 2026-09-22 — Native intelligence as a continuous system
- Replaced the cosmic film with the approved code/constraints/dependencies -> relationships -> clear-result concept. Warm themed SVG layers and quiet margin notes remain in one continuous 14s loop; ASTRA is a small final signature, not another scene.
- Responsive ports/routes reflow vertically on mobile. 83 SVG elements, no particles/filters/frame loop. Light/dark/portrait static artwork checked; actual browser motion and FPS remain unverified.
- 107 homepage tests pass; full suite 1007 pass with five known unrelated failures. Lint/typecheck/build/diff checks pass. Preview files refreshed to `index-CqpD_725.js` with backup/import checks; production untouched, no commit/push.

### 2026-09-22 — Numbered intelligence captions
- Added sequential number prefixes and removed terminal punctuation in both locales, leaving animation/layout unchanged. 39 focused tests, scoped lint and isolated build pass.
- Preview files refreshed to `index-DpK2i_Rd.js` with backup/import checks; no production changes. Browser/HTTP checks remain unavailable.

### 2026-09-22 — Restore the full feature descriptions
- Removed the intelligence paragraph's screen-reader-only treatment; the original full copy is visible below its artwork again. Numbered short captions stay in the animation, whose overlay is now confined to the artwork rather than the entire frame.
- Reserved an intrinsic text row with responsive spacing, without moving descriptions back into the tabs. 108 homepage tests, lint/typecheck/build/diff checks pass; browser geometry remains unverified.
- Authorized preview files refreshed to `index-arb3wLo4.js` with backup/import validation; production untouched, no commit/push. See task verification for suite details and artifacts.

### 2026-09-22 — Unified right-aligned feature footers
- Added the same top divider and right-aligned complete-description footer to intelligence, speed and trust. 40 focused tests, scoped lint and isolated build pass.
- Preview refreshed to `index-BJphfetP.js` with backup/import checks; no production change or commit/push.

### 2026-09-22 — Compact footer and a connected intelligence focal path
- Removed the 820px description cap and tightened desktop footer type/spacing without changing text, right alignment or mobile wrapping. Replaced independent card/plane motion with fixed ports and one continuous input-to-core-to-result highlight; muted secondary routes and sequential outline/halo emphasis clarify focus.
- 112 homepage tests pass; full frontend suite 1012 pass with the same five unrelated failures. Lint/typecheck pass; standalone isolated build passes after an earlier concurrent build exited 143. Static light/dark/portrait SVG geometry checked (92 elements); browser geometry/playback remains unverified.
- Preview files refreshed to `index-DE1uVZpO.js` with backup/import validation. Backend embedded index unchanged; production untouched, no commit/push. See task verification for artifacts and detailed regressions.

### 2026-09-22 — Living associations before a resolved result
- Added staggered input-content acquisition, three source streams, three shared-junction association circuits, 12 small nodes, bridge paths, local pulses and two intermediate alternatives. The final result now emerges only after association; anchored geometry, 14s loop, numbered captions, original descriptions and desktop footer remain unchanged.
- 115 homepage tests pass; full suite 1015 pass with five known unrelated failures. Lint/typecheck and isolated build retry pass; initial build exited 143. Inspected representative light/dark/portrait SVG artwork (138 elements), not browser motion/FPS.
- Preview files refreshed to `index-BWzW886I.js` with backup/import/byte checks; backend embedded index unchanged. Production untouched, no commit/push. Detailed artifacts and validation limits are in task verification.

### 2026-09-22 — Global speed SVG redesign
- Replaced the speed tunnel with an orthographic globe, seven raised geographic routes, three optimized lanes and an early first-byte/rapid streaming response. Reused existing public-domain geography and `d3-geo`; fixed projected ports, CSS-only motion and full portrait recomposition. Preserved intelligence, all copy/footers and trust primitives.
- 122 homepage tests pass; full suite 1022 pass with five known unrelated failures. Full lint/typecheck pass; final 24 focused tests/scoped lint/build pass after smoothing lane joins. Inspected 130-element native SVG artwork in light/dark/portrait; browser playback and page geometry remain unverified.
- Preview files refreshed to `index-7Dr2j_Cb.js` with backup/import/byte checks. Backend embedded index unchanged; no production operation, commit or push. See task verification for artifact paths and limits.

### 2026-09-22 — Readable speed round trip, right-hand globe
- Moved the globe to the right and the client left; preserved source-first order on mobile. Replaced overlapping independent packet clocks with one 5.6s request/target/return/first-byte/stream sequence. Only one lane and target activate; quieter background routes, acceleration, direction nudges, echoes and a cursor clarify movement without disconnecting ports.
- All 122 homepage tests pass; full suite 1022 pass with five known unrelated failures. Full lint/typecheck, final scoped lint and isolated build pass. Inspected 118-element light/dark/portrait native SVG samples, not actual browser animation/FPS.
- Preview files refreshed to `index-BPga8Zw4.js` with free-space, backup, import, alias and byte checks. Backend embedded index unchanged; production untouched, no restart, commit or push. Detailed artifacts and verification limits recorded in task verification.

### 2026-09-22 — Netherlands destination and Pi-style client
- Corrected the sole server marker to the Netherlands, removed the onward geographic hop and restored all seven independently animated global client round trips. Rebuilt the foreground terminal from the installed Pi documentation's transcript/editor/footer hierarchy, with first-byte receipt followed by thinking and sequential reply typing. Preserved other scenes, page layout and original copy.
- 127 homepage tests pass; full suite 1027 pass with five known unrelated failures. Lint/typecheck and isolated build pass. Inspected 152-element bilingual/light/dark/portrait native SVG artwork; browser motion/FPS/HTTP remain unverified. Web reference access returned 403; used the available local Pi docs without a network workaround.
- Preview files refreshed to `index-wVMp-AZX.js`, with backup/import/alias/byte checks and unchanged backend/component checksums. No production operation, restart, commit or push. Detailed artifacts are recorded in task verification.

### 2026-09-22 — Unbranded streaming long task
- Renamed the selector to `快速响应 · Speed`. Removed scene branding/example copy and the server rack glyph; only `Server` text remains. A single graphical input now holds and moves into history without the old fade/duplicate swap. Added a 16s task with two 18-chunk matched network/output streams, tool progress, transcript scrolling and completion; global traffic remains active.
- 128 homepage tests pass; full suite 1028 pass with five known unrelated failures. Lint/typecheck and isolated build pass. Inspected 272-element landscape/portrait/dark native SVG phase samples, not browser playback/FPS/HTTP.
- Preview files refreshed to `index-dvlI3-MN.js` with free-space, backup, import, alias and byte checks. Backend, geography and other homepage component checksums unchanged. No production operation, restart, commit or push. See task verification for exact artifacts and limits.


### 2026-09-22 — Continuous output and no client footer meter
- Replaced the rapid output packets with a steadily lit full solid connection during both stream intervals. Removed the left client's bottom progress meter and completion tick; kept the internal tool progress, long-task scrolling and global routes.
- 128 homepage tests pass; final full suite 1028 pass with five known unrelated failures. Lint/typecheck and isolated build pass. Inspected 233-element landscape/portrait SVG compositions; browser playback/FPS/HTTP remain unverified.
- Preview files refreshed to `index-DrkkDYnR.js`, with free-space, backup, import, alias and byte checks. Backend embedded index, other components and locale checksums unchanged. Production untouched; no restart, commit or push. See task verification for exact artifacts and limits.


### 2026-09-22 — Directional, continuously flowing response
- Replaced the whole-line fade with a Server-to-client advancing front, unbroken moving gradient and a draining tail, synchronized to both output batches. Preserved client/tool/global behavior and the removed bottom meter; no added copy or controls.
- 130 homepage tests pass; full suite 1030 pass with the same five unrelated failures. Lint and sequential typecheck/build pass; initial concurrent typecheck exited 143. Inspected 243-element SVG fill/flow/drain samples, not live browser playback/FPS/HTTP.
- Preview files refreshed to `index-CeT3JB79.js` with backup/import/alias/byte checks. Backend and unrelated component/locale checksums unchanged. No production action, restart, commit or push. Exact artifacts and verification limits recorded in task verification.


### 2026-09-22 — Slower model-paced return flow
- Slowed surface motion from 1.2s to 3.2s and doubled leading-edge travel time. Lengthened draining tails and gently slowed client fragment growth/spacing, keeping both batches synchronized. Initial request/first-byte, world routes, geometry and the 16s loop are unchanged; these remain illustrative timings.
- 130 homepage tests pass; full suite 1030 pass with five known unrelated failures. Full lint/typecheck/build pass; browser playback/FPS/HTTP remain unverified.
- Preview files refreshed to `index-DlHpkNhg.js` with backup/import/alias/byte checks. Backend and unrelated component/locale checksums unchanged. No production action, restart, commit or push. Exact artifacts and limits recorded in task verification.


### 2026-09-22 — Trust tab: transparent billing animation
- Replaced the decorative receipt/shield with a 16s SVG billing sequence: usage split → model prices → itemized costs → one effective multiplier → balance OR subscription settlement. Checked local backend billing rules and used an explicitly labeled standard-tier Terra example, not live pricing/account data. Preserved the original description/divider and other scenes. Added bilingual rule notes and an accessible summary; no backend behavior or dependencies changed.
- 139 homepage tests pass, including nine trust tests. Final full suite: 1039 pass with the same five unrelated failures. Lint/typecheck/build pass; inspected 118-element bilingual desktop/mobile/dark SVG samples, not browser playback/FPS/HTTP.
- Isolated preview refreshed to `index-DlbEvyOV.js` with backups and import/byte/alias checks. Backend and unrelated homepage checksums unchanged. `/tmp` now has only ~9.2 MB free; keep subsequent artifacts in `/var/tmp` and check capacity before publishing. Production untouched; no restart, commit or push. See task verification for exact artifacts and limits.


### 2026-09-22 — Abstract, wordless trust tab
- Simplified the pricing demo to a 64-element SVG: usage tiles, a cache echo, a shared weighting lens and a confirmed graphical record. Removed all visible scene copy/figures and the demo fixture. Kept the original heading/description and other scenes; one ordered 10s loop with existing visibility pauses.
- 138 homepage tests pass; full rerun 1038 pass with the same five unrelated failures after an interrupted first run. Lint/typecheck/build pass. Inspected offline desktop/portrait/dark SVG phases, not browser playback/FPS/HTTP.
- Preview files updated to `index-skf4xmBx.js`, with asset/import/alias/backup checks. To preserve publication headroom, 35 files from seven old non-served preview backups were safely archived under `/var/tmp/sub2api-home-trust-abstract-backup-archive/`; original paths remain intact as verified symlinks. No backup loss, active-asset relocation or general cleanup. `/tmp` still near capacity. Production untouched; no restart, commit or push. Exact artifacts and limits recorded in task verification.


### 2026-09-22 — Trust: request window to itemized receipt
- Replaced the overly abstract flowchart with recognizable request/receipt objects. Cached pieces move into a separate row, matching slips populate the bill, coin pictograms appear, a rate tag adjusts the combined total once and a final check confirms. Kept the artwork wordless and the original description unchanged. One 12s loop, 127 SVG elements; no backend behavior or dependencies changed.
- 140 homepage tests pass; full suite 1040 pass with the same five unrelated failures. Lint/typecheck/build pass. Inspected offline desktop/mobile/phase/dark SVG samples, not live browser motion/FPS/HTTP.
- Preview files refreshed to `index-hvoPdePO.js` with backup/import/byte/alias checks. Forty files from eight additional non-served historical preview backups were preserved under `/var/tmp/sub2api-home-trust-receipt-backup-archive/` with verified original-path symlinks to retain preview copy headroom. No backup loss or general cleanup. Production untouched; no restart, commit or push. Exact artifacts and limits recorded in task verification.

### 2026-09-22 — Trust: attached cable and sequential receipt printing
- Replaced flying slips with source branches sharing one visible, physically attached printer cable. Signals arrive before the matching line prints. The stationary printer feeds unscaled paper upward in row-height steps, with pauses, per-row ink reveal and hidden future rows. Total/rate/check come last; the reset is hidden. Existing copy, cache separation, one settlement and other tabs are untouched. 132 elements / 12s native loop, no new dependencies or backend changes.
- Final homepage suite 142 pass; full suite 1042 pass with the same five unrelated failures. Lint/build pass; typecheck passed on a standalone retry after exit 143. Inspected offline partial/complete/mobile/dark SVG compositions, not browser playback/FPS/HTTP. Exact artifacts and verification limits are in the task log.
- Preview refreshed to `index-6Q9HH-1v.js` with backup/import/byte/alias checks. Preserved 28 files from six documented historical non-served backups under `/var/tmp/sub2api-home-trust-print-backup-archive/`, retaining verified original-path symlinks; no backup loss or general cleanup. `/tmp` remains near capacity. Production unchanged; no restart, commit or push.

### 2026-09-22 — Trust: request history → individual billing → usage records
- Corrected the left/right semantics to three independent requests and three matching usage rows. Added a shared center that selects model prices, partitions cached input, prices each usage category, sums and applies that request's multiplier once. Identical request/model fingerprints and computed graphical charge widths survive through the printed record. Removed the misleading global receipt adjustment. One 16s native loop / 4s request offsets; grounded incoming/outgoing routes and sequential paper feed remain. No actual requests, private usage logs or backend rules are changed.
- Read local usage DTO/UI and token-billing logic; graphic quantities/prices/rates remain explicitly illustrative. 145 homepage tests pass, full suite 1045 pass with the same five unrelated failures. Lint/typecheck/build pass; inspected offline desktop/mobile/dark and calculation/print phases, not browser playback/FPS/HTTP. Exact artifacts and limits are recorded in task verification.
- Preview updated to `index-BsoG7QvM.js` with import/byte/alias/backup checks. Preserved 51 payloads from 17 additional documented non-served backups under `/var/tmp/sub2api-home-trust-per-request-backup-archive/`, retaining hash-verified original paths as symlinks. No backup loss or broad cleanup; `/tmp` remains near capacity. Production untouched; no restart, commit or push.

### 2026-09-22 — Equal feature footer spacing
- Removed the generic footer's extra margin and matched divider-to-copy and bottom spacing with the intelligence full-bleed footer at base, desktop and mobile breakpoints. Added a regression for the shared vertical rhythm; descriptions, alignment and scenes are unchanged.
- Final focused tests 18/18, scoped lint, typecheck and isolated build pass. The preview CSS was refreshed atomically without a restart; entry remains `index-BYZz8xLM.js`. Browser/HTTP verification remains unavailable. Production untouched; no commit or push.

### 2026-09-22 — Preserve the first-tab footer
- The owner clarified that intelligence was the intended reference. Moved its original responsive padding into the common footer rule and made speed/trust extend that same footer across their otherwise inset animation frames. There is no intelligence-specific footer CSS now.
- Focused tests 18/18, scoped lint, typecheck and isolated build pass. Preview stylesheet updated atomically with a backup; production and preview processes untouched.

### 2026-09-22 — Top-anchored second-stage ChatGPT logo
- Fixed the center-origin scale offset that pushed the compact brand into model cards on shorter screens. Only the brand transform origin changes to top center; full-size centering, rotation, transitions, models and approved feature footers remain intact.
- Model regressions 9/9 and homepage tests 148/148 pass. Full frontend suite 1048 pass with five documented unrelated failures; lint/typecheck/isolated build pass. Preview received only the corresponding compiled CSS declaration with an atomic replacement and backup; no process or production changes. Browser/HTTP not checked. Artifact paths and validation limits are in task verification.

### 2026-09-22 — All everyday tabs use the first tab's frame
- Promoted the original intelligence frame and canvas geometry to shared CSS. Removed speed/trust frame padding, differing canvas minimums and compensating footer margins. All three now share the full-width background/canvas/divider/footer structure at every breakpoint; only animation artwork differs.
- Focused tests 19/19 and homepage tests 149/149 pass. Full suite 1049 pass with the same five unrelated failures; lint/typecheck/build pass. Atomically refreshed only the compiled carousel CSS block in the existing preview, preserving the model-logo fix and other CSS/JS. No production/process changes; browser/HTTP not checked. Full artifacts and limits recorded in task verification.

### 2026-09-22 — Automatic feature tour with longer endings
- Moved numbered intelligence captions to the right. Added CSS-clock-driven automatic tab rotation, with an extra 1.5s completed-result hold per scene. Any manual tab selection cancels rotation; the chosen animation continues looping with its longer ending. Scene/caption restarts and native visibility pauses stay synchronized, without JS timers or new controls.
- Carousel tests 19/19, homepage tests 155/155, full suite 1055 pass plus five known unrelated failures; lint/typecheck/build pass. Existing preview refreshed to `index-Dglc780U.js` with backup, asset/import/alias checks. Shared layouts, scene sources and model-logo fix preserved. `/tmp` has ~5.3 MiB remaining; keep artifacts in `/var/tmp` and check space before future publication. No production/process action or cleanup. Browser/HTTP not checked; exact artifacts and limits are in task verification.

### 2026-09-22 — Coordinated feature crossfade
- Added a 900ms overlapping handover for manual/automatic switches, preserving the outgoing scene and starting the incoming timeline only after entrance. Stationary shared background/divider, native SVG entrance motion, staggered descriptions and softened selector accent replace the immediate timeline handoff. Same-tab clicks cancel autoplay without a visual restart. Existing holds and manual loops remain.
- Carousel tests 22/22, final homepage tests 158/158; full suite 1058 pass with five known unrelated failures. Lint/typecheck/build pass, including final CSS-only refinement checks. Preview is `index-DRnfBfPa.js` with the final scoped carousel CSS applied atomically; manifests/backups and unchanged-source checks verified. Two documented historical screenshots were preserved on disk with original-path symlinks for copy headroom. No production/process action; browser/HTTP not checked. `/tmp` has ~3.5 MiB free. Exact artifacts and limits are in task verification.

### 2026-09-22 — Balance card: use anytime
- Replaced the balance multiplier display with `随时使用` / `Use anytime`, removing the multiplier unit, Astra caption and group-rate row. Kept top-up/usage details and subscription content; actual billing and all animations are unchanged.
- Homepage tests 160/160, full suite 1060 pass with the same five unrelated failures; lint/typecheck/build pass. Preview updated to `index-BNWh8Z9n.js` with asset/import/alias/backup checks. Four documented historical screenshots preserved on disk with verified original-path symlinks for publication space; `/tmp` has ~3.4 MiB free. Production/processes untouched; browser/HTTP not checked. See task verification for artifacts and limits.

### 2026-09-22 — Equal client-button widths
- Codex/Pi links now share two equal-width, content-sized grid columns with centered icon/label groups; responsive sizing and existing links/styles remain.
- Homepage tests 161/161; scoped lint, typecheck, isolated build and diff check pass. Refreshed only the two compiled CSS rules in the existing preview; all other assets unchanged. Backup/manifest/logs in `/var/tmp/sub2api-client-buttons/`. No production/process action or browser/HTTP verification.

### 2026-09-22 — Billing symmetry and scene hold completion
- Both billing cards now share a restrained two-part highlight/caption and exactly two aligned fact rows; removed subscription pricing copy. Moved the intelligence hold after the final signal completes, kept its result visible, and exempted speed's ambient world traffic from the result hold. Header wordmark is about 10% smaller.
- Homepage tests 164/164; scoped lint/typecheck/build pass. Real Chromium checked 12 bilingual responsive layouts, scrolled mobile cards, native ending/hold states, ambient continuity, auto/manual behavior, offscreen pauses and brand size. Preview is `index-BVVYdFUk.js`; verified assets and backups are under `/var/tmp/sub2api-billing-pair/`. Nine prior screenshots preserved with original-path symlinks for tmpfs headroom. No production/process action, new commit/push/tag or change to published `cafecode-v0.0.78`.

### 2026-09-23 — Consolidate production source in custom-prod
- At the owner's request, consolidated the deployed NL request-origin feature
  and latest v0.0.79 history directly in `custom-prod`, preserving the original
  worktree in a private backup and stash. Included previously uncommitted
  stream-cancellation billing source/tests and the current homepage refinements.
  No new NL-only release/tag; unrelated artwork/SQL exports remain untracked.
- Preserved valid drained usage after downstream cancellation/write failure,
  while separately tracking upstream failures to prevent overload/business
  errors from becoming successful billing outcomes. Existing no-overload-retry
  policy, single interruption rescue and authoritative billing paths remain.
- Repaired stale expectations and timezone/wall-clock test fixtures. Full backend
  unit suite passed; focused billing race and isolated origin repository
  integration passed; backend lint 0 issues. Frontend: 150 files / 1073 tests
  passed, typecheck/build passed, lint 0 errors / 12 existing warnings.
- SQL is unchanged versus the deployed origin feature. No production lifecycle,
  proxy/database edits, release tag, image publication or historical rebilling.
  Source push is authorized; production deployment remains separate. Artifacts:
  `/root/backups/custom-prod-consolidation-20260922T235441Z`.

### 2026-09-23 — Presale review fixes
- Matched recovered/successful payments to the server-confirmed presale plan/month;
  preserved uncertain native payments and verified cancellation before QR retry.
- Refunds now recognize source-order early-reset deductions, lock before confirming
  the amount, and preserve the complete accepted quote for provider retries.
- Frontend 1190/1190 tests, typecheck and lint passed (12 existing warnings).
  Focused backend regressions passed; backend lint reported 0 issues. Broader HTTP
  fixtures and disposable-Postgres execution are blocked by sandbox socket access;
  the integration package compiled. No production actions or publication.
- Details: `.trellis/tasks/09-23-presale-review-fixes/verification.md`.
- Publication follow-up: restored permissions allowed the full affected backend
  unit packages and disposable PostgreSQL tests to run. Fixed the newly exposed
  custom-presale activation clear/set collision with exclusive Ent mutation
  branches. User authorized source/tag publication as `cafecode-v0.0.85`, not a
  production update; final results and safety boundaries are in the verification.
