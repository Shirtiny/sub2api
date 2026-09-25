# Selectable multiplier for presale renewals

## Requirement
Once next-month renewal is open, subscribers may select another multiplier from
the plan's configured choices. The current term is unchanged; the paid selection
takes effect only at next month's activation and must not reset usage/windows.

## Scope
- Unlock the landing card select while preserving all eligibility restrictions.
- Apply current plan bounds to new presales, using the old multiplier only as an
  in-range default. Customization disabled means 1x; no new configuration.
- Align request validation, creation, coupon previews and checkout pricing.
- Keep immediate/legacy renewals and accepted-order fulfillment unchanged.
- Preserve the previous fourteen-day window and quota-preservation work.
- No production changes, migrations, user-facing policy additions or deployment.

## Validation
Exercise upgrades, downgrades, return to 1x, invalid/range-disabled selections,
coupon price consistency, unchanged current entitlement, activation snapshots,
retained usage/windows, repeated activation and duplicate/early-renewal guards.
