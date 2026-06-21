# Complete cafe coupon feature

## Goal
Complete the cafe coupon feature on `feat-cafe-quan` so it meets product expectations across membership benefits UI, payment application flow, backend security, and admin configuration.

## Requirements
- Membership benefits UI must show a cafe coupon action for eligible users.
- If the current period coupon was already claimed, the UI must show claimed state and remaining days until next claim instead of a generic claim button.
- Cafe coupon usage rules must be configurable by membership level.
- Coupon types must support both cash coupons and discount coupons.
- Payment recharge/subscription flows must allow applying a cafe coupon above the pay button and show validated discount results.
- Coupon validation must enforce ownership/transferability rules, coupon state, expiry rules, and strict membership eligibility.
- The payment success lifecycle must not allow stale coupon discounts to survive invalid state transitions.
- Admin settings must expose cafe coupon configuration in the membership/benefits section.

## Product Decisions
- Transferability is fully configurable by membership level.
- Coupon validity should be fixed to month-end, independent from claim-cycle semantics.

## Acceptance Criteria
- [ ] Eligible membership users can see the cafe coupon entry in membership benefits without depending on affiliate feature visibility.
- [ ] If a user has already claimed the current period coupon, the membership UI shows claimed state and remaining days until next claim.
- [ ] The claim/status API returns enough data for frontend claimed-state rendering.
- [ ] Admin settings support per-level cafe coupon config for enabled, type, amount, claim period, month-end validity behavior, and whether others may use the coupon.
- [ ] Recharge and subscription payment flows apply either cash or discount coupons server-side and display amounts consistently with backend fee calculation.
- [ ] Claim/apply/order flows strictly validate active user status, current membership eligibility, coupon state, expiry, ownership/transferability, and config applicability.
- [ ] Current-period used/void coupons are not returned as a valid claimed coupon result.
- [ ] Coupon lifecycle remains consistent across order creation, cancel/expire release, and paid-order recovery.
- [ ] Focused backend/frontend tests cover the new state, validation, and lifecycle paths.

## Technical Notes
- Backend currently has `CafeCouponConfig` but only supports `enabled`, `type`, `value`, and `period`.
- Backend currently enforces self-use only; transferability needs to become config-driven.
- Existing claim service tracks `AlreadyClaimed` internally but handler/frontend drop that signal.
- Payment preview amount and final order pay amount currently diverge when recharge fees apply.
- Current payment-success path validates provider amount but does not fully revalidate coupon eligibility.
- Current admin frontend settings page does not expose `cafe_coupon_config`.

## Planned Implementation
1. Extend cafe coupon config/data contract
   - Add per-level transferability and month-end validity metadata to config/view/admin API types.
   - Expose claim response/status fields needed for claimed-state UI.
2. Harden backend coupon validation and lifecycle
   - Tighten claim existing-coupon checks.
   - Centralize reusable eligibility validation for claim/apply/order/payment-success.
   - Review cancel/expire/recovery interactions and prevent coupon reuse inconsistencies.
3. Complete frontend membership and payment UX
   - Show claim/claimed/remaining-days states in membership benefits.
   - Remove incorrect affiliate gating.
   - Align displayed payable amount and payment-method availability with backend fee-adjusted values.
4. Complete admin settings UI
   - Add cafe coupon controls to membership benefits section.
   - Keep request/response typing aligned with backend.
5. Verify with focused tests
   - Backend unit tests for claimed-used coupon, expiry, inactive user, downgrade/config changes, transferability, and payment recovery cases.
   - Frontend tests for membership state rendering and payment coupon amount display.
