ALTER TABLE purchase
    ADD COLUMN IF NOT EXISTS stripe_session_id TEXT,
    ADD COLUMN IF NOT EXISTS stripe_checkout_url TEXT;
