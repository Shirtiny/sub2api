-- Codex alpha/search standalone web search billing, in USD per successful call.
-- NULL keeps the application default ($0.01/call); 0 explicitly makes it free.
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS web_search_price_per_call NUMERIC(20,8);
