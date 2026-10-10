-- Migration: add_unique_constraint_to_bot_waitlist_email (UP)
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'uq_bot_waitlists_email'
    ) THEN
        ALTER TABLE bot_waitlists ADD CONSTRAINT uq_bot_waitlists_email UNIQUE (email);
    END IF;
END $$;
