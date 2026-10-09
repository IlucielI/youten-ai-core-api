-- Create bot_sessions table for tracking real-time voice bot lifecycles
CREATE TABLE IF NOT EXISTS bot_sessions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    recording_id UUID NOT NULL REFERENCES recordings(id) ON DELETE CASCADE,
    provider VARCHAR(50) NOT NULL,
    meeting_url TEXT,
    external_session_id VARCHAR(255),
    channel_id VARCHAR(100),
    guild_id VARCHAR(100),
    status VARCHAR(50) NOT NULL DEFAULT 'DISPATCHED',
    error_message TEXT,
    started_at TIMESTAMPTZ,
    ended_at TIMESTAMPTZ,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_bot_sessions_recording_id ON bot_sessions(recording_id);
CREATE INDEX IF NOT EXISTS idx_bot_sessions_status ON bot_sessions(status);
CREATE INDEX IF NOT EXISTS idx_bot_sessions_provider ON bot_sessions(provider);
