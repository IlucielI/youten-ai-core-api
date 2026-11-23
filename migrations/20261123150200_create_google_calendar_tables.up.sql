-- Create Google Calendar connections and scheduled events tables
CREATE TABLE IF NOT EXISTS user_google_calendar_connections (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    google_user_id VARCHAR(255) NOT NULL,
    google_email VARCHAR(255) NOT NULL,
    encrypted_access_token TEXT,
    encrypted_refresh_token TEXT NOT NULL,
    token_expires_at TIMESTAMPTZ NOT NULL,
    sync_token TEXT,
    is_auto_record_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    lead_time_minutes INT NOT NULL DEFAULT 2,
    last_synced_at TIMESTAMPTZ,
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT uidx_user_google_calendar_user_id UNIQUE (user_id)
);

CREATE INDEX IF NOT EXISTS idx_user_google_calendar_status ON user_google_calendar_connections(status) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS scheduled_calendar_events (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    google_event_id VARCHAR(255) NOT NULL,
    title VARCHAR(500) NOT NULL,
    description TEXT,
    start_time TIMESTAMPTZ NOT NULL,
    end_time TIMESTAMPTZ NOT NULL,
    scheduled_dispatch_at TIMESTAMPTZ NOT NULL,
    meeting_url TEXT NOT NULL,
    meeting_platform VARCHAR(50) NOT NULL,
    organizer_email VARCHAR(255),
    user_response_status VARCHAR(50) NOT NULL DEFAULT 'accepted',
    auto_record_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    dispatch_status VARCHAR(50) NOT NULL DEFAULT 'PENDING',
    bot_session_id UUID REFERENCES bot_sessions(id) ON DELETE SET NULL,
    recording_id UUID REFERENCES recordings(id) ON DELETE SET NULL,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT uidx_user_scheduled_google_event UNIQUE (user_id, google_event_id)
);

CREATE INDEX IF NOT EXISTS idx_sched_cal_events_dispatch ON scheduled_calendar_events(scheduled_dispatch_at, dispatch_status) 
WHERE deleted_at IS NULL AND auto_record_enabled = TRUE;

CREATE INDEX IF NOT EXISTS idx_sched_cal_events_user_time ON scheduled_calendar_events(user_id, start_time ASC) 
WHERE deleted_at IS NULL;

