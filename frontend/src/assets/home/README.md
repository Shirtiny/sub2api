# Homepage artwork and vector wordmark

- `cafe-banner.webp`: byte-for-byte copy of the original root `banner.webp` (2560 × 1024, 188,862 bytes).
- `brand-vectors.json`: precomputed contours for 223 Latin glyphs only. Shared contours and half-unit delta encoding keep the data around 37 KB gzip. Imported synchronously with the homepage component: no font request, font parser, image download, or animation dependency in the wordmark.
- `lora-regular.ttf`: unmodified build-time source only, upright Lora regular 400 from the [Google Fonts distribution](https://fonts.google.com/specimen/Lora), under the included `Lora-OFL.txt`. Downloaded from `https://fonts.gstatic.com/s/lora/v37/0QI6MX1D_JOuGQbT0gvTJPa787weuyJG.ttf`. It is **not loaded by the browser**. Only the existing Latin character repertoire is exported as illustration contours.
- `codex-mark.svg`: **Codex cloud with the `>_` terminal face**, not the generic OpenAI/ChatGPT knot. Matched visually against the [official Codex promotional artwork](https://images.ctfassets.net/kftzwdyauwt9/2Z64goaCYuEmGe825kLJFB/9bc3f606fcceff406082118e9d9a8fb4/codex_share-image_v1.png). Vendored vector from [LobeHub Codex SVG at commit 63c800e](https://github.com/lobehub/lobe-icons/blob/63c800e3db6427b3f156f7b88d426ccb3f7277a1/packages/static-svg/icons/codex.svg), with its MIT notice in `LobeHub-MIT.txt`. No package upgrade was performed.
- `pi-mark.svg`: original downloadable SVG from [pi.dev](https://pi.dev/logo-auto.svg), accessed 2026-09-18.
- `model-marks.json`: monochrome OpenAI/ChatGPT, Claude, Grok and Gemini path data extracted from the already installed `@lobehub/icons` 4.0.2 `es/{OpenAI,Claude,Grok,Gemini}/components/Mono.js`. Covered by the existing `LobeHub-MIT.txt`. Only vector data is imported, not React components or a new runtime dependency. Only ChatGPT is currently rendered; other brand marks remain unused source data. Its finite outline/fill/echo introduction contracts into a brand header before revealing the Luna, Terra, Sol and Astra celestial scenes described below. The design is independently implemented, not copied from Aether.

Regenerate glyph contours with `python tools/generate-home-brand-vectors.py` from `frontend/` in a temporary Python environment with `fonttools`. No Python dependency is added to the application or its build. The generator uses only the vendored font. Product marks remain separate from the glyph atlas.

The header uses the **backend-configured site name**, never a hard-coded Café Shop replacement. Broader upright serif contours and open letter spacing form the cafe sign, without an underline or subtitle. The current Café Shop outline spans about 210 px on desktop. The SVG has no runtime font dependency.

For a name with the required é → S/s → O/o → p letters, `HomeBrandStory.vue` and `brandStory.ts` create a **22-second native SVG story inside the lettering**:

1. The acute accent is replaced by steam **from the first frame onward**, including the loop boundary and reduced-motion rendition. The original accepted single, fuller ribbon and its 2.09–3.30 second unfurling/upward motion are restored exactly, rather than the later two thin, continuously waving strands. The puff rises into the separate hand-drawn cloud and dissipates; a quiet lower steam accent returns underneath it. There are no added branching curls. The cloud's light outline and barely tinted interior gather progressively instead of revealing a flattened solid blob. Only after it has gathered does the small Codex (13 px across) emerge, gaining weight/size and rotating as it falls. The cloud lingers briefly above it instead of simply turning into the icon. A regression fixture recovered from the original preview locks the two steam contours and their timing.
2. Codex's first trajectory accelerates downward from the cloud, without the previous upward launch. The remaining sampled parabolic bounces hit the actual S and o upper contours. Short cloud squash/recoil and bottom-anchored letter compression convey impact; the third arc drops it into the actual p counter. Rendering Codex behind the lettering provides occlusion at the rim.
3. The p's original outer/inner contours continuously reshape into the monochrome P-like portion of pi; its extra leg grows from the counter. This retains the official pi silhouette, but not its multicolor palette.
4. The extra leg stretches into a rounded liquid neck and falling drop, fades away, and the p restores its **exact original contour**. The steam returns to its starting shape at the loop boundary; the old acute accent does not reappear. No drifting endpoints or accumulated letter transforms.

All targets come from the current name's glyph geometry, not screen-coordinate guesses. Matching 96-point paths and cyclic alignment keep contour morph commands compatible. One native SVG clock drives the whole story without a JavaScript frame/interval timer, library, remote font or image request. The stage reserves space above/below for steam and runoff, avoiding layout movement during the loop.

For configured names lacking the required letters, the name remains unchanged and the previous small upper-right `HomeToolGlyph.vue` animation remains as a fallback, now also **monochrome**. It draws the Codex outline/face and pi pieces on a 16-second native SVG clock; it never invents Café Shop lettering.

`toolArtwork.ts` contains exact copies of the vendored Codex compound path and pi paths/colors; regression tests compare them to the original SVG sources. The Codex outer contour is extracted from its first closed subpath; the two face strokes are drawing guides only. The settled mark retains the original compound path and both negative-space cutouts. Product icons are inline paths, not SVG images animated as whole rectangles.

The story and fallback marks animate automatically without a hover/focus pause button or extra tab stop. Fallback marks are decorative, not clickable controls. Both pause in background tabs and resume when visible. Reduced motion removes all animation nodes and restores the full configured name with a static steam accent, never a half-transformed letter; the fallback retains a complete static Codex. Changing a story-enabled name recalculates targets and restarts the native clock. Below 360 px header actions wrap to avoid squeezing the name.

The correct Codex cloud/terminal icon and the original pi mark sit **together below the hero**, as small, static tool references with readable names. They never replace the site identity and make no affiliation or availability promise.

Names outside the supplied Latin glyph set remain readable as static SVG text in local system fonts, without a font download. The link's accessible name always retains the full configured value; unusually long decorative names are bounded to 48 characters.

All branding assets are self-hosted. Product marks are decorative references, not an affiliation or service-availability claim. Original concept images and root artwork remain unchanged.


## SVG celestial model scenes (2026-09-21)

Visual associations were verified against official API model artwork: [Luna](https://developers.openai.com/api/docs/models/gpt-5.6-luna), [Terra](https://developers.openai.com/api/docs/models/gpt-5.6-terra), [Sol](https://developers.openai.com/api/docs/models/gpt-5.6-sol), and [Astra](https://developers.openai.com/api/docs/models/gpt-6-astra). These independently authored SVG scenes are not official OpenAI animations.

The photorealistic Canvas renderer and external surface textures have been removed at the user's request. All four scenes now use inline SVG paths/circles and CSS motion, with no raster artwork, canvas or image loading. Sphere surface geometry is projected into SVG paths at at most 24 updates per second while visible; no work is scheduled when paused or reduced-motion is enabled.

On an identical 320×280 viewBox, the Luna/Terra/Sol body radii are **26 / 48 / 80**, making small/medium/large explicit. These are illustrative display sizes, not an astronomical scale. Following the official PNG visual references (without embedding them), the three bodies are horizontally centered in their cards against a sparse star field. Luna has a clean silver surface with a handful of large, legible geodesic craters under fixed directional lighting; Terra has blue oceans, land silhouettes and drifting, broken cloud clusters and a translucent veil, without latitude rings or diagram meridians; Sol has a bright gold-orange plasma surface with static native SVG turbulence, a bright limb and an irregular filled corona instead of dots, radial icon rays or intersecting surface strokes; Astra has 720 individually drawn SVG stars in rotating spiral arms around a bright center rather than a generic star icon. IDs are unique per instance. Animations pause offscreen/background and stop for reduced motion. The titled section and ChatGPT-to-model reveal sequence are unchanged.

The refined scenes use a clean silver moon with a few prominent crater landmarks, finer tapered terrestrial cloud wisps, and luminous plasma-like solar texture rather than soft polka dots, short squiggles or long intersecting ribbons. Astra is a face-on spiral with equal horizontal/vertical scale, not a flattened ellipse. Its outer radius is now 112 viewBox units (previously 84), and a revolution takes 240 seconds (previously 45); the core glow varies gently over 28 seconds. The solar turbulence seed/frequency remain static; larger projected plasma patches move across the sphere. `earth-geography.json` contains geographic coastline coordinates derived from [Natural Earth 1:110m land](https://github.com/nvkelso/natural-earth-vector/blob/master/geojson/ne_110m_land.geojson), [public domain](https://github.com/nvkelso/natural-earth-vector/blob/master/LICENSE.md). `d3-geo` 3.1.1 is now a pinned direct dependency for robust spherical projection and hemisphere clipping; it was already present transitively. No runtime network request is used. Regenerate the data from the official land GeoJSON with `node tools/generate-home-earth-geography.mjs /path/to/ne_110m_land.geojson` from `frontend/`.

### Spherical rotation correction

The three bodies no longer rotate flat surface groups with CSS. `sphereSurface.ts` uses orthographic projection of longitude/latitude geometry: surface features expand across the center, foreshorten toward the limb, and disappear behind the sphere. The projected axis, body outline and directional illumination remain fixed. Geographic data is normalized to D3’s spherical winding convention. Luna/Terra/Sol complete an illustrative turn in 96/64/80 seconds (not astronomical real-time periods); cloud/plasma layers have a small relative drift. Fine solar noise is stationary, while larger plasma structures rotate on the sphere. Astra’s face-on 240s galaxy rotation is unchanged.

VueUse’s existing RAF lifecycle utility drives SVG path updates with a 24Hz cap. It pauses/resumes without time jumps, stops on unmount, and honors offscreen/background/reduced-motion state. Native SVG remains the rendered output; no WebGL, canvas, raster textures, or frame sequence assets are used.

Earth cloud refinement: twelve differently oriented weather systems form small irregular fragments along curved, interrupted tracks. A translucent outer veil and denser cloud centers share the same spherical drift, leaving clear areas over ocean/land instead of continuous zonal belts. Regression coverage checks cloud fragment widths at four rotation angles.

Lunar landmark refinement: remove mare patches entirely and retain only eight deliberately placed larger geodesic craters around the whole sphere, so each face shows just a few. Clear bowl shading and broken rim highlights distinguish pits from flat gray spots. A restrained 12%-opacity monochrome surface finish adds slight rock-like tonal variation without dense microdetail. Very light rim feathering softens hard model-like edges. Crater geometry still foreshortens/clips correctly with the existing spherical rotation; other models are unchanged.

All three planetary bodies, their clipping paths and the solar corona animation origin share x=160 on the 320-unit viewBox. Sizes, vertical placement and rotation remain unchanged.

Crater-lighting refinement: each crater is projected/rendered independently, so its diagonal bowl gradient and soft basin light are local to that pit rather than spanning all craters. Tapered shadow crescents and much thinner one-sided rim highlights replace blunt ring segments. Eight crater locations/radii, body dimensions and motion are unchanged. The broad surface illumination remains fixed while the projected geometry turns.

Crater-shape refinement: slightly irregular geodesic contours replace perfect circles, with a flatter inset floor and a restrained broken inner terrace. Only the three larger craters receive small two-facet central relief; smaller pits remain simple. Rim thickness varies and tapers to avoid a machined outline. All parts stay attached to the same spherical projection, with unchanged nominal crater positions/radii and no extra dense surface detail.

## Section slider and sticky navigation (2026-09-21)

The public default homepage now has six vertical chapters: welcome, everyday features, supported models, setup, usage/pricing, and the closing/footer. The model showcase follows “Made for your everyday”; its SVG artwork and animation timing are unchanged.

`HomeSectionSlider.vue` owns a page-local native scroll container. The current transition references the continuous-scroll interaction observed read-only in the local Aether homepage (`views/public/Home.vue` and `useSectionAnimations.ts`), but uses an independently written viewport-coverage/staging implementation; no Aether code or assets are copied. Desktop chapters use native vertical scroll snap, with proximity snapping on phone/tablet and enough height for long content. Chapter buttons/guide links use measured native smooth scrolling; the main content never disappears for a timer-delayed reposition. Fast repeated chapter clicks redirect the browser's current smooth scroll without a forced intermediate snap. Wheel/touch and navigation keys may interrupt programmatic scrolling without consuming native input. Skip links preserve focus; incoming hashes and reduced-motion navigation are immediate. No carousel library, global scroll-style mutation, or automatic page advance is added.

The header reserves a constant track and keeps the logo/actions at identical coordinates in both states. Two fixed-size pseudo-element planes crossfade over 500ms: an expanded page-tinted surface and an inset translucent glass panel (24px blur). Insets, radius, blur and shadows never animate, and persistent opacity compositing avoids a layer handoff at the end. Separate 64px entry / 8px exit thresholds avoid threshold chatter. The floating plane has a close contact shadow plus a broader ambient shadow, with a stronger dark-mode variant. Individual `data-reveal` headings, copy, buttons and cards derive their opacity/displacement from continuous viewport coverage, with staggered spatial thresholds rather than a shared full-page fade. An initial 750ms ramp introduces the first chapter; after that only scroll/resize events schedule motion updates. The guide's two columns enter from opposite sides. Over-height chapters retain already-revealed content until the entire chapter leaves, so mobile policy reading does not blink or fade out. Model-card reveal wrappers are separate from the existing brand/celestial animations, so their transforms do not conflict. Reduced-motion mode removes header/reveal/navigation transitions and leaves every content element visible. All sections remain in the DOM, including policy details/footer links; custom HTML/iframe homepages are unchanged.

## Everyday feature SVG carousel (2026-09-21)

The existing three everyday feature cards now have an appended `HomeFeatureCarousel.vue`. It presents three independently authored, code-native SVG scenes (`HomeFeatureScene.vue`): a woven intelligence core with converging signals, three optimized paths with traveling response pulses, and a layered ledger with a verification seal. It reuses the cafe palette in light/dark themes; no external artwork, fonts, libraries, account data or invented billing figures are fetched. The approximate 0.3-second WS first-byte figure is the existing site copy, not a live measurement. All new copy is localized in Chinese and English.

Each scene has an eight-second dwell and a continuous crossfade with slight vertical settling. A CSS progress animation is the sole rotation clock, avoiding background timers and per-frame JS. The carousel only runs when at least 30% visible and the document is visible. Hover pauses its countdown; keyboard/control focus stops automatic rotation until explicitly resumed. Direct buttons and Left/Right/Home/End keys select scenes without taking over the outer chapter navigation. Decorative SVG motion continues while reading or manually selecting a scene; only the current scene animates, and offscreen/background/reduced-motion states suspend it. Reduced motion uses static artwork and manual selection only. Region/group labels, current selection, controlled-panel IDs, a quiet playback control and manual-only live announcements preserve accessible navigation.

The illustration height is capped on desktop so the full section fits a typical 1440×900 viewport without removing existing copy. Narrow/short layouts remain naturally scrollable with the existing long-chapter behavior. The supported-model chapter, SVG brand/planet scenes and fixed-plane glass header are unchanged.

### Unified feature selection and pricing close (2026-09-21)

The original three feature cards now live inside `HomeFeatureCarousel.vue` and are its **only** scene selectors. Their original icons, numbered titles and descriptions are preserved; a native heading button stretches over each card, with controlled-panel/described-by wiring, keyboard selection, active styling and the autoplay progress indicator. Cards and SVG share one bordered surface, with no duplicated lower tab strip. The small playback control sits in the scene corner. Visibility observes the artwork presentation, not the now much taller combined block, so card-only visibility on mobile does not start unseen scene rotation. Per-card and presentation reveal wrappers remain siblings, preserving chapter entry motion without nested transforms.

The homepage now has five chapters. The former standalone closing chapter is a compact callout directly beneath Usage & Pricing's policy cards, retaining its copy and public/auth-aware entry action. The site footer follows within the pricing chapter; it no longer occupies an extra full-height slide. The chapter rail/dock consequently contains five stops and returns to the top from pricing. Long pricing/phone content remains natively scrollable. SVG scene artwork and the fixed-plane glass header are unchanged.

### Roomier chapters and full-width vector scenes (2026-09-21)

The first screen and header retain their prior geometry. All four subsequent chapters now allow additional vertical room beyond a viewport (`available viewport + two chapter-space gutters`), with shared 80–128px desktop / 64px phone section padding. Heading gaps, model-brand/card spacing, setup steps and pricing/callout spacing are expanded rather than fitting everything into one screen. Content can grow further and remains natively scrollable; the five-chapter structure and combined pricing close are unchanged.

All repeated carousel scene captions, headlines, descriptions and footnotes have been removed, including their unused translations. The original three feature cards still contain the only copy and selection controls. `HomeFeatureScene.vue` now draws independently authored full-width tableaux: work-input cards, a layered neural processor and structured output; three network sources, a four-gate transport tunnel and streaming result; authentic model identity, an itemized receipt with verification seal, and an audit trail. These are decorative illustrations, not live/account statistics. The illustration stage is no longer constrained to a small right-hand column or 260px art cap.

Landscape art uses a 1200×520 viewBox with uniform scaling. At phone widths the complete composition rearranges into 600×720 portrait: the main subject above both supporting panels, with rerouted connections. `preserveAspectRatio="xMidYMid meet"` prevents distortion; no panel is cropped or dropped. Motion uses traveling strokes, drawing lines, signal pulses and receipt scanning, with the existing CSS rotation clock, offscreen/background pause and static reduced-motion behavior. No bitmap, external artwork/font, new dependency or per-frame JavaScript rendering was introduced. Existing celestial artwork and physical rotations are unchanged.

### Single-screen desktop correction (2026-09-22)

Removed the previous `viewport + two gutters` minimum-height override, which incorrectly forced all later chapters beyond the screen. The native chapter controller again owns the one-viewport minimum. At desktop widths ≥1024px and heights ≥700px, section gutters/gaps adapt to the available height below the fixed header. The everyday and model sections use that height with intrinsic text rows and flexible illustration rows; minimum-content sizing permits a readable overflow fallback instead of hiding text. SVGs retain their viewBox proportions. Model captions remain outside their flexible artwork area; the smaller settled brand size is shared by static and animated states.

Pricing keeps its complete policies, callout and footer, with tighter desktop card padding, line spacing and callout/footer gaps. No copy is removed, no whole-page transform or zoom is used, and the first-screen/header styles are unchanged. Narrow/short screens retain natural scrolling.

This session's network/browser sandbox prevents an HTTP health check and Chromium screenshot/layout validation. The code, unit tests, typecheck, lint and isolated build were checked, and preview files were refreshed offline using recorded entry aliases. Do not treat this as a verified desktop geometry result; the prepared Playwright check in the task verification must be run when browser access is available.


### Final chapter vertical distribution (2026-09-22)

Following the user's clarification, only Usage & Pricing now fills the available desktop height with a grid: heading and notice above, flexible policy cards in the middle, then the closing callout and footer at the bottom. The cards center their contents vertically and gain bounded vertical padding on taller screens. Horizontal widths, copy, typography, other slides and all animation behavior are unchanged. Intrinsic minimums and the existing desktop media condition preserve readable short/mobile scrolling; this does not restore the rejected viewport-plus-gutters height. Preview files were refreshed offline; browser geometry verification remains unavailable in this sandbox.


### Closing-gap-only correction (2026-09-22)

The user rejected expanding the cards. Restored the original policy-card block layout and padding. The final chapter still fills the available desktop height, but its first three grid rows now size strictly to their content; only the final row is flexible, with Make room for an idea aligned to its bottom. Spare height therefore becomes the gap between the unchanged policy cards and closing callout. The minimum closing gap, narrow/short fallback, copy and other slides are unchanged. This supersedes the card expansion described above.


### Billing chapter and compact setup / request policies (2026-09-22)

The five chapters are now welcome, everyday, supported models, billing, and usage policies. `HomeBillingSection.vue` occupies the former setup chapter, with two intrinsic-height cards covering subscription presales (one month per purchase, usual calendar-month period, discounted pricing) and immediate balance top-ups (usage-based charging and Astra's 0.4–0.5 multiplier). No specific package price or account data is invented.

Your first sip is a compact three-step guide at the start of Usage & Pricing, above enforcement and client/request controls. Its configured documentation link and public/auth-aware entry remain. The former billing card now covers request format, audit-only temporary content storage for one day, and an explicit permanent-retention exception for Cyber requests and policy-violating request content. These are user-provided homepage policy statements, not changes to backend retention/enforcement. Until the user specifies a stricter format rule, the copy neutrally directs clients to follow Codex/Pi and the setup documentation rather than inventing protocol or rejection rules. Both locales carry the same retention policy.

The existing `#getting-started` links/hash resolve to their new enclosing usage chapter without introducing a sixth slide. The controller preserves focus and modifier-click behavior. The final chapter's policy rows remain intrinsic; only the gap before Make room for an idea absorbs spare height. Narrow/short screens remain naturally scrollable. All existing brand, feature and model animations are unchanged.


### Request-control presentation refinement (2026-09-22)

Replaced the three repetitive label/paragraph rows with one concise request-format/audit-only summary and a semantic two-column retention comparison. Ordinary request content is labeled “1 day only”; Cyber and rule-violating request content is labeled “Retained permanently”. The two scopes and durations remain explicit in both languages, with a light divider rather than additional nested cards or a destructive-looking alert. The comparison stacks on narrow phones. No retention policy, backend behavior, chapter order, card stretching or closing-gap logic changed.


### Setup guide alignment and policy separation (2026-09-22)

The compact guide now has a dedicated header row (Chinese title with its English eyebrow, configured documentation link opposite) and a separate full-width row of three equal grid tracks for the numbered steps. This removes the competing title/step/docs columns and empty optional-link track. The guide has a 24–32px responsive bottom gap before Usage Policy. On phones, steps stack and vertical dividers disappear. Copy, links, staged reveals, intrinsic policy-card sizing and the flexible closing gap are preserved.


### Shared Sub2API primary buttons (2026-09-22)

Both homepage entry actions now reuse the application-wide `btn btn-primary` treatment rather than a page-local flat fill. This brings the same primary-token gradient, white text, shadow, hover sheen, pointer-responsive highlight and active feedback used inside Sub2API while preserving homepage dimensions and placement. The local duplicate light/dark button color variables were removed.


### Interactive ChatGPT brand stage (2026-09-22)

The initial ChatGPT-to-model sequence is shorter: the four model cards finish around 3.6 seconds rather than 4.7 seconds. The ChatGPT mark is now a keyboard-accessible button after the intro. In the model stage, activating it expands back to the brand presentation and pauses the hidden celestial scenes. A manually selected brand stage is persistent and never auto-advances; activating the mark again is the only way back to the model cards. This differs intentionally from the one-time automatic first visit.

A transform-isolated wrapper rotates the ChatGPT mark once every 36 seconds in the automatic brand stage, model stage and manually selected brand stage, without interfering with the path-draw or layout transforms. Rotation and all transitions pause offscreen/background and are removed under reduced-motion preferences. The control is disabled only while the first automatic sequence is running and has stage-specific bilingual accessible labels.


### Centered pointer-only ChatGPT switch (2026-09-22)

The ChatGPT mark switch is intentionally pointer-only and has no tooltip, focus treatment, keyboard semantics or visible instruction. In manual brand mode, both the brand and its rotating mark sit at the exact horizontal and vertical center of the bordered presentation area. The brand layer is above the hidden model grid, and the entire grid stops receiving pointer events, so clicking the enlarged mark reliably returns to models. The first automatic sequence and persistent manual behavior remain unchanged.


### Native SVG logo rotation (2026-09-22)

To address jagged edges reported on the compact ChatGPT mark, rotation now uses `animateTransform` on a group inside the SVG rather than CSS rotation on an HTML wrapper containing the whole SVG. Geometric-precision rendering is requested; the original paths, viewBox, 36-second period, displayed size and stage positions are unchanged. No blur filter, raster replacement or per-frame JavaScript was introduced. The existing visibility gate pauses/resumes the SVG clock without resetting it, and the same SVG remains mounted across manual stage changes. Visual improvement still needs browser confirmation in an unrestricted environment.


### Compact feature selectors and ASTRA galaxy journey (2026-09-22)

Made for your everyday now keeps only the icon, bilingual title and sequence number in each feature selector. All three existing descriptions move, unchanged, into their corresponding animation frame and switch with it; accessible description references still point to the matching paragraph. No second tab row or redundant headline has been added.

The intelligence scene is independently redrawn in `HomeAstraJourney.vue`: a dark full-bleed starfield, tilted spiral galaxy, softly lit nucleus, fine galactic dust and perspective-accelerated outward star trails suggesting forward travel. ASTRA is set as a large HTML wordmark above the original description, over a dark legibility gradient. Speed/trust retain their original SVG illustrations, with copy below them inside the same panel. Model-showcase art and the first-screen/header are untouched.

The new vector art has 220 distant stars, 640 galaxy stars, 2,400 fine dust marks batched into three paths, and only 64 moving flight groups. Geometry is deterministic and created once; CSS handles motion and inherits the existing offscreen/background/inactive-scene suspension. There are no remote assets, bitmaps in the UI, invented metrics, new dependencies or per-frame JavaScript. Portrait recomposition moves the focal point; background cropping is intentional, while text remains separate and fully readable. Desktop height-aware rows and natural short/mobile scrolling are preserved.

Offline landscape/portrait SVG renders were visually inspected with the installed librsvg renderer. These are static artwork checks only, not browser screenshots or animation/layout verification; the latter remains pending under the current environment restrictions.

### Looping first-person ASTRA film (2026-09-22)

Supersedes the tilted-galaxy artwork and eight-second automatic feature rotation above. Feature selectors remain compact and manual; the lower-right playback control and countdown have been removed. Hover/focus no longer interrupt the active illustration. Returning to intelligence restarts its artwork and captions together.

The intelligence panel now has a shared 28-second infinite CSS timeline: forward approach through a galactic dust lane, accelerated perspective trails, a foreground nebula passage, then a vector-drawn ASTRA end card. Three localized copy passages appear sequentially between shots rather than a permanent paragraph overlay. The original description remains available to assistive technology, while speed/trust keep their existing visible descriptions. ASTRA is outlined SVG, not a late-loading display font. Stars continue travelling through the end card, and the scene fades back into its opening composition without a frozen terminal frame or automatic tab change.

Artwork uses 260 distant stars, 2,700 dust marks batched into three paths, and two deterministic perspective particle fields (144 cruise / 72 fast). Cloud density is static SVG fractal noise with softened edges; camera/depth/exposure transforms provide motion without animating filter parameters, adding dependencies or using a JavaScript frame loop. Offscreen/background/inactive suspension and the existing system reduced-motion fallback are preserved. Desktop chapter sizing, mobile natural flow, other scenes and model art are unchanged.

Offline librsvg renders were checked for landscape, portrait and representative shot compositions. They are static artwork samples, not verification of browser playback, frame rate or final page geometry.

### Sharper, faster forward flight (2026-09-22)

The film now loops in 14 seconds instead of 28. Removed the turbulence/blur cloud filters and foreground cloud veil; a faint distant gradient and sharp batched dust remain behind the stars. Bright heads and tapered, transparent-tail vector trails provide the main depth cues. Three forward-flight layers (180 cruise, 96 fast, 24 close flybys) now cross a much wider world-space field, with 1.1–4.8s travel cycles instead of the previous 2.6–16s range.

Perspective samples now follow `scale = 1 / (4 - 3.875t)`, avoiding the old center-bound slow approach. Near stars accelerate and leave the viewport before recycling. Cruise remains at full exposure through the ASTRA card, while the faster layers remain visible. All three copy passages and the outlined wordmark use the shorter shared clock; the first passage enters within 0.42s. Reduced caption shading and a lighter vignette keep passing stars visible. Infinite looping, manual feature selection, pause-only-when-hidden behavior and existing accessibility fallback are preserved.

Static landscape/portrait vector samples were inspected after increasing head/trail contrast. Actual browser playback and frame rate remain unverified in the restricted environment.

### Edited shots, quiet copy and batched SVG motion (2026-09-22)

The 14s loop now contains distinct compositions rather than repeating one star field behind three headlines: a left-offset grazing approach, a right-offset fast warp, curved light around a dark gravitational horizon, and restrained moving diffraction lines behind ASTRA. Their overlap windows share the existing editorial clock, with continuous motion during the ending and loop return.

Captions are single brief localized asides (Chinese: `完整能力。`, `思考，无界。`, `抵达更远。`). They use muted 13–16px type, different corner placements and directional drifts. Removed the centered two-line headlines, secondary notes and full-size caption scrim; reduced the final outlined ASTRA size. Original approved feature descriptions remain available to assistive technology, and speed/trust copy is unchanged.

Performance work replaces 300 independently animated nested particle trees with 12 depth sheets, each batching 24 star heads/trails into two paths. Static stars are also batched, and decorative dust was reduced. Actual SSR SVG element count falls from 1,804 to 101 (including the root); this is a structural measurement, not a measured FPS improvement. No per-frame JavaScript, raster replacement, blur filters, dependencies or mass compositor-layer promotion were added. Visibility/inactive suspension and reduced-motion fallback remain intact. Offline shot artwork was inspected in landscape and portrait; browser FPS and final page motion still need live validation.

### Native intelligence: from complexity to a clear structure (2026-09-22)

Replaces the space-flight concept entirely. `HomeIntelligenceScene.vue` now presents one continuous SVG system: code, constraints and dependency fragments settle into place; layered relationship planes reveal connections; secondary routes recede as a selected path leads to a structured result. The nodes and cards stay mounted throughout the loop. This is a capability illustration, not a model reasoning transcript, benchmark or live execution display.

The illustration follows the homepage's warm light/dark palette with thin strokes, translucent layered surfaces and a restrained accent. Three small margin notes read `1. 理解全貌`, `2. 深入关联`, `3. 清晰落地`; ASTRA appears as a small outlined signature in the lower-right margin, rather than replacing the artwork. The 14s infinite cycle and end-phase travelling route signal remain; feature selection is still manual with no playback control or hover pause.

Landscape flows left-to-right. Portrait places the three inputs above the system and the result beneath it, with recomputed ports/routes and `xMidYMid meet` so meaningful content is not cropped. Original accessible feature descriptions, speed/trust scenes, chapter sizing and other homepage sections are preserved. The complete SVG has 83 elements including its root; there are no particles, filters, image assets, added dependencies or per-frame JavaScript. Existing visibility/reduced-motion handling applies. Offline light/dark/portrait geometry samples were inspected; actual browser animation and FPS remain unverified under the environment restriction.

### Restore complete feature descriptions (2026-09-22)

The intelligence description is visible again, verbatim, in an intrinsic text row below the illustration. The numbered phase captions supplement this service information rather than replacing it. All three original descriptions remain in their corresponding feature panels, outside the tabs and timed animation layers.

The caption/signature overlay now belongs to the artwork box only, so it cannot cover the restored paragraph. The intelligence panel uses flexible artwork plus an auto-sized footer row with a restrained separator. Desktop artwork minimum height is reduced to reserve room for the paragraph within the existing chapter height; mobile text remains naturally sized. No copy, illustration, numbering or loop timing was changed.

### Unified right-aligned description footers (2026-09-22)

All three feature panels now separate artwork from the complete description with the same thin top rule and intrinsic footer row. Descriptions are right-aligned with an 820px readable maximum; intelligence retains full-width footer padding while speed/trust keep their existing inset artwork spacing. Copy, animation and tab behavior are unchanged.

### Compact desktop copy and a continuous intelligence route (2026-09-22)

Supersedes the 820px copy limit above: descriptions now use the entire available row, retaining the divider, right alignment and exact original text. Desktop type scales only from 13 to 14px, with tighter line height and 12px vertical intelligence-footer padding. Narrow screens and longer translations can still wrap naturally; there is no truncation, forced nowrap or fixed text height.

All wired cards, nodes and layer geometry are now stationary. Removed independent card/plane translation and system scaling that separated connector endpoints. A single continuous foreground path spans the selected input, association core and result port, sharing geometry with its travelling highlight in both layouts. Background connections remain fully drawn and subdued instead of revealing on competing clocks. Input emphasis hands off to a restrained central halo, then to the result border/check. Removed diagonal secondary crossings and the opaque center tile; the path stays visible through the hub. The original captions, 14s loop, ending signal and manual feature selection remain.

The SVG has 92 elements including its root, with no filters, bitmap assets, dependencies or JavaScript frame loop. Static light/dark/portrait artwork was inspected; browser layout, playback and FPS remain unverified in the restricted environment.

### Living associations before resolution (2026-09-22)

The intelligence scene now reveals its complexity progressively rather than presenting a complete pipeline immediately. Three input fragments acquire content and send offset signals into the network. Three curved, shared-junction association routes unfold around 12 additional small nodes, with cross-links, asynchronous local signals and a responding core. Two intermediate structural alternatives appear during this exploration phase and disappear before the final structured result becomes fully visible. The resolved end-to-end path now starts halfway through the sequence, after association, rather than giving away the outcome at the beginning.

The main composition still follows the existing 14s loop and numbered captions. Shorter CSS detail rhythms (input streams, node accents, a local echo and segmented core ring) add motion within each phase without moving wired cards or endpoints. All geometry is deterministic and tied to shared junctions; portrait source/candidate routes terminate at their actual card edges. This remains an abstract capability illustration, not a representation of hidden reasoning or a live model trace. No copy, footer sizing, tab behavior or other homepage section changed.

Measured SVG size is 138 elements including its root, with no filters, raster assets, extra dependencies or frame-level JavaScript. Existing inactive/offscreen/background suspension and reduced-motion presentation apply to the new details. Offline light/dark/portrait phase compositions were inspected; browser animation, FPS and live HTTP health remain unverified.

### Speed: a global network and a fast first byte (2026-09-22)

`HomeSpeedScene.vue` replaces the old transport-tunnel illustration. An orthographic globe uses the existing licensed Natural Earth coastline data, projected graticules, restrained surface shading and seven illustrative geographic origins. Raised great-circle routes converge at a shared hub; three optimized lanes carry short packet highlights toward a response console, followed by return acknowledgements. A first-byte indicator flashes before the response rows and token marks populate rapidly. The warm monochrome palette and the original complete speed description are unchanged; there are no additional headlines, counters, claimed coverage locations or simulated live metrics.

`speedGlobe.ts` projects the geometry once with the already-installed `d3-geo`. All geographic endpoints lie on the sphere, and all selected routes remain on its visible hemisphere. The coastline does not spin as a flat image. Light moves over fixed, continuous paths; no frame-level JS, bitmap replacement, filters, dependencies or network requests were added. Native CSS motion inherits the existing inactive/offscreen/document-hidden pause rules and reduced-motion fallback. The response sequence loops in 3.2s, with quicker offset geographic packet rhythms.

Landscape places the globe left and response right. Portrait keeps the complete globe above the console and recomputes all three lane endpoints at the console's top edge, rather than shrinking or cropping the landscape. The original trust drawing primitives were preserved while unreachable legacy speed markup/styles were removed from `HomeFeatureScene.vue`. Total speed SVG size is 130 elements including its root. Offline landscape/portrait/light/dark artwork was inspected; actual browser playback, page geometry, FPS and HTTP health remain unverified under the environment restriction.

### Speed: right-hand globe and one readable round trip (2026-09-22)

Supersedes the layout and independent packet clocks above. Desktop now places the client on the left and globe on the right; portrait follows the same source-first order vertically. All three available lanes still attach to the client and globe hub, but only the middle lane carries a request. Only one geographic route activates; the remaining routes and nodes stay subdued as context.

One shared 5.6s CSS clock stages request composition, accelerated delivery, hub-to-target transit, a target echo, the return trip, first-byte illumination and fast streaming rows. Outbound and inbound signals never compete. Small directional nudges, timed echoes, a breathing halo and a response cursor provide activity without moving the wired globe or ports. Delayed response details use backwards fill to prevent first-frame flashes. No extra copy, controls, telemetry, filters, assets, dependencies or frame-level JavaScript were added; the existing description, footer and other tabs are unchanged.

The scene now contains 118 SVG elements including its root. Static landscape/portrait/light/dark vector samples were inspected; browser animation, FPS and live preview HTTP health remain unverified in this restricted environment.

### Speed: Netherlands endpoint and Pi-style response sequence (2026-09-22)

Corrects the network topology above: the owner identifies the server country as the Netherlands. The single server marker now uses a country-level coordinate `[5.3, 52.2]`, an outlined server glyph and a localized `NL` callout. The foreground client connects directly to this marker and receives directly back along the same path, with no onward geographic hop. All seven illustrative global client routes are animated again: staggered 3.6s request/return pairs terminate at the same Netherlands endpoint. Their contrast is quieter than the foreground exchange; origins are not claimed operating PoPs or live telemetry.

The wider left-hand terminal follows the Pi interface's transcript / bottom editor / working-directory and model footer hierarchy. Reference: the README and `docs/images/interactive-mode.png` packaged with locally installed `@earendil-works/pi-coding-agent` 0.84.3; no screenshot was copied into the UI. The small monochrome Pi mark reuses the existing vector paths. Minimal Chinese/English illustrative prompt, status and reply text uses system monospace fonts, not newly downloaded fonts.

The foreground loop is 6.4s: compose and submit -> direct Netherlands round trip -> first-byte receipt -> visible thinking status -> sequentially typed reply. Thinking is only a status, not a fabricated reasoning trace. SVG clipping reveals one reply line at a time; global traffic continues during thinking and output. Existing manual tab selection, hidden/offscreen suspension, reduced-motion fallback, original description/footer and other scenes remain unchanged. Total scene size is 152 SVG elements, with no new dependencies, filters, raster assets, network calls or JS frame loop. Offline bilingual light/dark/portrait samples were inspected; actual browser playback and live HTTP health remain unverified.

### Unbranded streaming long task (2026-09-22)

The Chinese selector now reads `快速响应 · Speed`. The speed illustration's only visible text is `Server`; removed the country label, server rack glyph, Pi artwork/name, ASTRA footer, working directory and example conversation/status text. The geographic endpoint and all seven animated world routes are unchanged. The terminal retains its bottom editor, but input, thinking, replies, tool work and progress are entirely vector graphics. Removed the now-unused scene copy from both locale files; the original service description remains intact.

A 16s loop demonstrates a longer task: input -> direct server exchange -> processing dots -> streaming output -> a graphical three-step tool operation -> a second stream -> completion. Two batches of 18 packet highlights use matching per-chunk delays for the corresponding output segments; data remains in flight while already-received content accumulates. This replaces the post-response text typewriter. The transcript scrolls within a clipped viewport as the task grows, independently of the stationary editor and connected client shell. The global network continues animating throughout.

The former input became complete at 8% of a 6.4s loop and faded out by 10%, then reappeared as a second transcript element. The new input is one persistent SVG group: it completes drawing, holds for 0.64s, then moves continuously into history without an opacity swap. Its reveal has an explicit hidden initial state; resets happen only after the scene fades out. No input font loading or duplicate text rendering remains.

The scene contains 272 simple SVG elements, with no new dependencies, filters, raster assets, network calls or per-frame JS. CSS-only visibility suspension and the completed reduced-motion view are retained. Offline input/stream/tool/scrolled-result artwork was inspected in landscape, portrait and dark mode; these checks do not verify browser playback, FPS or live HTTP health.

### Continuous response connection, no client footer meter (2026-09-22)

Replaced the 36 rapidly travelling output packets with one complete, undashed connection along the existing client-to-server path. It fades in once per response interval and stays steadily lit while the client accumulates output, rather than repeatedly sending short marks. Both output batches, the intermediate tool task, transcript scrolling and all seven global routes retain their existing behavior.

Removed the client's bottom task-progress track, fill and completion tick, along with their unused geometry and keyframes. The tool card's internal progress remains; the client shell and editor have not moved. The resulting scene has 233 SVG elements. Offline landscape/portrait compositions and timing regressions were checked; browser motion and live HTTP health remain unverified.

### A flowing response rather than a switched-on line (2026-09-22)

The response now fills from the Server end toward the client before the first output arrives. One path-length SVG mask exposes a continuous body, holds it open during streaming, then sends its trailing edge into the client near the final output segment. The two batches share the existing 16s task clock; a longer invisible gap between dash repetitions prevents a backwards reveal during reset. There is no whole-line opacity switch or train of separate packets.

A repeating, always-visible gradient moves continuously under this mask: leftward on desktop, upward on portrait. Its matching end stops and one-period translation make the surface loop seamless without gaps in the current. All motion is CSS-only and follows the existing visibility suspension. The client meter stays removed; global traffic, tool work, scrolling and all text are unchanged. Total scene size is 243 SVG elements. Offline fill/flow/drain artwork and source-derived timing checks are not a substitute for live browser playback, which remains unverified.

### Slower illustrative generation pace (2026-09-22)

Slowed the surface current from a 1.2s to a 3.2s period and doubled the leading-edge travel times (0.4s to 0.8s, then 0.32s to 0.64s). The trailing edge now also takes 0.8s / 0.64s to drain. Client fragments arrive every 155ms rather than 140ms and grow over 120ms instead of 80ms; the second batch starts later so it still waits for the stream to arrive. These are illustration timings, not claimed model throughput.

The initial request, first-byte acknowledgment, global network clock and 16s task loop are unchanged. Timing regressions cover both batches, the inter-batch reset and final output before the fluid tail reaches the client. No geometry, text, assets, dependencies or controls changed; browser playback remains unverified.


### Trust scene: requests → individual billing → usage records (2026-09-22)

`HomeTrustScene.vue` presents three distinct user requests, a shared central billing surface and a receipt containing exactly three corresponding usage records. Input/cache/output are **terms inside each request's calculation**, never separate requests or separate receipt entries. An instance-scoped graphical request fingerprint and model mark remain identical across all three stages. There is no visible prose, price numeral, model name or real account data.

The center follows the roles in `UsageLog`, `UsageView.vue` and the token-based billing path: select model unit prices, separate cached input from ordinary input, calculate input/cache-read/output costs, sum them into `total_cost`, then apply that request's effective `rate_multiplier` once to obtain `actual_cost`. Term widths, their summed width and the adjusted result are derived together; the final result is copied unchanged to the matching receipt row. This is an illustrative token/cache-read example, not a pricing quote or an exhaustive engine for image/per-request/tier/cache-creation rules. All authored spans, weights and factors are drawing units. No usage endpoint, real request payload or private account record is fetched for the public homepage.

Permanent cables attach history to the calculator's inlet and the calculated result to the printer. Incoming and outgoing highlights use those exact paths. The outgoing highlight starts only after the multiplier has completed; a receipt row prints only after its charge arrives. Native clips reveal paper upward through a stationary slot in 62px line-height advances, with pauses between records. Each printed row contains identity, model, usage/cache marks and its already-adjusted actual charge. Removed the old receipt-wide rate tag and combined monetary total: the footer now only reconciles the recorded requests, without implying a second deduction.

One 16s CSS clock runs the three requests at 4s offsets. Calculation windows do not overlap; the final result remains visible during the audit. The 296-element vector drawing uses no fonts, raster assets, filters, runtime timers, frame JavaScript, new controls or dependencies. Desktop is 1200×520 with three columns. Portrait is 600×720: history/calculator form a compact upper row, the receipt remains full-size below, and its cable stays outside both objects. Instance-scoped resources, fixed outer transform anchors, inactive/offscreen/background suspension and a complete static reduced-motion state are retained.

Original feature headings, right-aligned description and divider are unchanged. Chinese/English screen-reader summaries explain one request per usage row and explicitly identify the imagery as illustrative. Offline native SVG phase compositions were inspected; these are not browser playback/FPS or live page verification.

### Compact model-brand anchor (2026-09-22)

The second-stage ChatGPT brand scales from `top center`, so its visible top stays at the reserved header inset. Scaling around the center of the original 240px SVG plus wordmark previously left a downward offset that overlapped the cards on shorter screens. The introductory and manually selected brand stages still use scale 1 and remain centered; logo size, rotation, timings and model artwork are unchanged. Regression checks cover the default layout and the 700px-height fitted-layout boundary without claiming browser geometry verification.

### One shared everyday-feature frame (2026-09-22)

All three feature tabs now use the original intelligence frame rules, not only its footer padding. The common zero-padding frame owns the page-color background, full-width canvas and separate right-aligned description below a divider. Canvas rows retain the first tab's 320px base / 400px phone / 200px fitted-desktop minimums. Removed all feature-specific frame/art/footer selectors and negative-margin compensation. The same `feature-frame → feature-art + feature-copy` structure is rendered for each tab; only the SVG artwork and intelligence captions differ. Scene geometry, copy, timing and visibility suspension remain unchanged.

### Automatic feature tour and longer final holds (2026-09-22)

The initial feature tour now advances Intelligence → Speed → Trust → Intelligence. Numbered intelligence captions sit at the lower right, sharing the final ASTRA signature's corner at different times. A native CSS phase clock reaches a completed-result plateau at 12.6s / 14.72s / 14.88s, pauses the artwork for an additional 1.5s, then advances the tour. Offscreen/background visibility pauses the phase clock as well as the scene, without JavaScript interval or timeout timers.

Clicking any feature selector (including the selected one), or selecting with the existing keyboard controls, permanently cancels automatic tab changes for that mounted carousel. The selected scene still loops: after its 1.5s end hold it resumes its original fade/reset tail, then restarts. Each newly selected scene starts from the beginning with its captions synchronized; no inactive scene resumes halfway through. The original 14s / 16s artwork timelines and playback speeds are unchanged, making manually selected loops 15.5s / 17.5s long. Shared tab layout, descriptions and SVG artwork remain intact; no extra controls or visible copy are added.

### Coordinated feature handover (2026-09-22)

Both automatic and manual tab changes now use a 900ms overlapping dissolve. The outgoing scene stays mounted and paused while fading out; the incoming scene waits for its entrance phase before starting its artwork and play clock. A native entrance keyframe supplies a small 12px / .985-scale movement even for freshly mounted SVGs, which cannot rely on a previous computed transform. The phase clock and entrance keyframes pause offscreen/in the background. No blur filters or JavaScript frame timers are added.

The shared stage owns the stationary background; canvases are transparent, the frame/divider no longer moves, and description text follows with a short stagger. The selected-tab accent also fades. Selecting the already active tab only cancels autoplay, avoiding an unnecessary visible scene reset. Reduced motion skips the entrance phase; the longer final hold and continued manual scene loops are unchanged.

### Balance card: use anytime (2026-09-22)

The public balance card now highlights `随时使用` / `Use anytime`, instead of a hard-coded Astra multiplier. Removed the multiplier unit, caption and group-rate fact; the card retains top-up and usage details. Subscription information and its three facts are unchanged. This is a homepage copy change only, not a change to configured rates, billing calculations or the account usage UI.

### Matched billing cards and completed-scene holds (2026-09-22)

Both billing cards now use the same headline/unit/caption structure and two fact rows. Chinese highlights pair `按月 / 订阅` with `按需 / 充值`; the one-month purchase limit remains in the subscription caption, while anytime availability is a small balance caption instead of oversized text. Removed the subscription price-advantage row. Shared type sizes and equal fact tracks align both cards without fixing their overall heights or introducing a numeric unlimited-use claim.

The intelligence hold now starts at 97% of its 14s cycle, after the last route highlight reaches the endpoint at 94% and disappears at 96%. The finished solution, backbone and ASTRA signature stay fully visible through 98%, so the additional 1.5s pause preserves a completed result rather than an in-flight signal or partially faded drawing. Speed's world routes, port echoes and globe breathing remain animated during its result hold; the client task stays completed. These ambient loops still pause when inactive, offscreen or backgrounded. The 900ms crossfade, manual cancellation and manually selected scene loops are retained.

Header wordmarks scale to 90% on desktop and 79% rather than 87.5% on mobile (roughly a 10% reduction). The configured name, SVG letter geometry, steam/Codex/Pi story and header height are unchanged.
