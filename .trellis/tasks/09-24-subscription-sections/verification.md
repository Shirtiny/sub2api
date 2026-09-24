# Subscription lifecycle sections

- Current active terms appear first, with a green heading and existing quota,
  renewal and reset controls. Classification checks both status and the actual
  start/expiry window; future or expired terms cannot appear as available now.
- Upcoming presales follow with a warm clock heading, an explicit not-yet-
  available hint, and the existing compact start/end cards. Activation issues
  remain visible here with their existing error state.
- Activated/cancelled/refunding/expired presale records live in a collapsed
  "Presale orders & refunds" section, retaining original order/refund actions.
  Other non-current subscription rows are separately expandable at the bottom.
- Successful presale refund requests refresh the current subscription list, so a
  cancelled active term does not continue to appear usable. Loading errors are
  not rendered as an empty subscription list. Empty active state is compact.
- English and Chinese labels are synchronized; no backend/payment logic changed.

Validation:
- Frontend typecheck and isolated preview build passed.
- Full frontend suite: 164 files, 1262 tests passed.
- Lint: 0 errors (existing unrelated warnings only).
- Browser at 1440/390/320px: real pending order #9 and browser-only mixed lifecycle
  fixtures verify active-first order, pending-only list, accessible folded records,
  retained refund entry and no horizontal overflow. No payment/refund mutations.
- Screenshots: /opt/stacks/sub2api-test/artifacts/subscriptions-sections-*.
- Only test preview assets refreshed. No production changes, commit, tag or push.

## Follow-up: count-free headings and arrival motion

Removed item counts from all four section headings. Actual card information
(multipliers, reset allowances, amounts and dates) is unchanged. Only genuinely
pending presales get a slow warm glow, travelling light and a breathing
status indicator. Active, cancelled and activation-error records remain static.
These are CSS opacity/transform effects with no progress claim, timers, layout
movement or pointer interception. Motion preference is respected without adding UI.

Focused 28 tests, lint, typecheck and preview build passed. Browser checks at
1440/390/320px confirm count-free headings, running arrival animations, stable card
geometry, no horizontal overflow, static history and reduced-motion fallback.
Evidence: /opt/stacks/sub2api-test/artifacts/subscriptions-arrival-*.

The arrival treatment was subsequently strengthened after visual feedback: a
permanent warm frame/left accent, tinted surface, larger outlined status badge,
highlighted start date and a soft diagonal sheen across the card. Distinction is
visible even with animation disabled. The sheen fades at both edges (no moving
hard-edged panel), is clipped in its own decorative layer, and text/controls stay
fixed. Only pending-card presentation changed; no lifecycle or payment changes.
Evidence: /opt/stacks/sub2api-test/artifacts/subscriptions-arrival-emphasis-*.

At the user's subsequent request, reverted the stronger treatment to the previous
subtle upper-edge glint, warm breathing background and status ripple. Removed the
extra sheen layer and restored original borders, badge sizing and date colors.
Count-free section headings and all lifecycle/refund behavior remain unchanged.
Only local source and isolated preview assets are restored; no production rollback.
