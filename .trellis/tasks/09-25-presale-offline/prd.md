# Administrator offline presale handling

User approved cancellation and offline refund recording in order management.

- Separate cancel-without-money from recording an already-completed external refund.
- Review versioned customer/plan/multiplier/term, require reason and explicit consent;
  refunds require actual currency amount and an external reference.
- Cancel only the correct entitlement, preserve all usage/window counters, release
  the reservation for another eligible purchase, and keep payment/audit facts.
- No provider call, no new schema, no live-order manipulation or deployment.
- Serialize activation/refund/cancellation, reject stale forms and uncertain online
  refunds, make identical replays safe, reverse earned points atomically.
- Localized administrator controls and cancellation display in user records.

Preserve earlier uncommitted renewal-window, usage-preservation and multiplier work.
