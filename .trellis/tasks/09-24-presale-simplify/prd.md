# Simplify presale configuration and copy

- Public catalog and purchase eligibility depend on presale_enabled + for_sale (with existing valid-group safeguards), not the legacy visibility flag.
- Remove fixed reset-card bonus configuration; new orders no longer copy legacy plan bonuses. Retain historic order snapshots and schema for safe activation/refund compatibility. No official grant automation is introduced.
- Remove administrator presale explanations/preview links, repeated timezone captions and their layout remnants. Keep the business timezone and detailed refund rules unchanged.
- Match Chinese/English plan selection, official reset-card issuance and short daily-refund benefit copy.
- Verify local tests and isolated preview only; no production lifecycle/data changes, commit/tag/push.
