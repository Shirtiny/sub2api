# Isolated local presale test environment

User approved a local full-console test environment populated only with a safe
copy of Netherlands public settings, subscription plans and non-user-owned groups.

- Dedicated app, PostgreSQL and Redis; no shared production volumes or networks.
- Existing guarded development auto-payment mode; no real money, merchant
  credentials, SMTP settings, upstream credentials, real users or orders.
- Reuse the existing preview port, serving the latest local frontend and proxying
  only to the isolated backend. Mark the UI clearly as TEST / no real payment.
- Provide separate test admin/user credentials and validate purchase, pending
  entitlement, renewal and refund paths without changing production state.
- Preserve existing pending home billing edits. No commit, tag, push or production
  lifecycle/config/data changes authorized by this task.
