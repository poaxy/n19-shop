ALTER TABLE referral
    DROP CONSTRAINT IF EXISTS referral_not_self_ref;

ALTER TABLE referral
    DROP CONSTRAINT IF EXISTS referral_referee_unique;

