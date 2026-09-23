-- Public waiting-list opt-ins. Emails are normalized before insertion.
-- Joining does not create an account or prove ownership of an email address.
CREATE TABLE IF NOT EXISTS waitlist_entries (
    id BIGSERIAL PRIMARY KEY,
    email VARCHAR(254) NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
