ALTER TABLE referral
    ADD CONSTRAINT referral_referee_unique UNIQUE (referee_id);

ALTER TABLE referral
    ADD CONSTRAINT referral_not_self_ref CHECK (referrer_id <> referee_id);

