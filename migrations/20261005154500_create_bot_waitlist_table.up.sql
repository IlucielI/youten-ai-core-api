-- Migration: create_bot_waitlist_table (UP)
CREATE TABLE IF NOT EXISTS bot_waitlists (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(255) NOT NULL,
    platform VARCHAR(50) NOT NULL DEFAULT 'google_meet',
    company_size VARCHAR(50) NOT NULL DEFAULT '1-10',
    status VARCHAR(50) NOT NULL DEFAULT 'PENDING',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_bot_waitlists_email ON bot_waitlists(LOWER(email));
