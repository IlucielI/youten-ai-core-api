-- Migration: add_unique_constraint_to_bot_waitlist_email (DOWN)
ALTER TABLE bot_waitlists DROP CONSTRAINT IF EXISTS uq_bot_waitlists_email;
