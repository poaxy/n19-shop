ALTER TABLE purchase
    DROP COLUMN IF EXISTS stripe_session_id,
    DROP COLUMN IF EXISTS stripe_checkout_url;
