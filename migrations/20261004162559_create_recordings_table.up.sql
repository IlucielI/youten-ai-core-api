-- Create recordings table
CREATE TABLE IF NOT EXISTS recordings (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    ownership_token VARCHAR(255) NOT NULL,
    share_token VARCHAR(255) DEFAULT NULL,
    is_share_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    title VARCHAR(255) NOT NULL,
    original_filename VARCHAR(255) NOT NULL,
    file_size_bytes BIGINT NOT NULL DEFAULT 0,
    duration_seconds DOUBLE PRECISION NOT NULL DEFAULT 0,
    audio_url TEXT DEFAULT NULL,
    source_type VARCHAR(50) NOT NULL DEFAULT 'UPLOAD',
    bot_provider VARCHAR(50) DEFAULT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'QUEUED',
    error_message TEXT DEFAULT NULL,
    error_code VARCHAR(100) DEFAULT NULL,
    selected_template VARCHAR(100) NOT NULL DEFAULT 'GENERAL',
    detected_language VARCHAR(50) DEFAULT NULL,
    output_language VARCHAR(50) NOT NULL DEFAULT 'id',
    analytics_data JSONB DEFAULT NULL,
    is_guest BOOLEAN NOT NULL DEFAULT TRUE,
    guest_ip VARCHAR(100) DEFAULT NULL,
    consent_given BOOLEAN NOT NULL DEFAULT FALSE,
    consent_version VARCHAR(50) NOT NULL DEFAULT '1.0',
    consent_at TIMESTAMPTZ DEFAULT NULL,
    expires_at TIMESTAMPTZ DEFAULT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ DEFAULT NULL
);

CREATE INDEX IF NOT EXISTS idx_recordings_user_id ON recordings(user_id);
CREATE INDEX IF NOT EXISTS idx_recordings_ownership_token ON recordings(ownership_token);
CREATE INDEX IF NOT EXISTS idx_recordings_share_token ON recordings(share_token);
CREATE INDEX IF NOT EXISTS idx_recordings_status ON recordings(status);
CREATE INDEX IF NOT EXISTS idx_recordings_expires_at ON recordings(expires_at);
CREATE INDEX IF NOT EXISTS idx_recordings_created_at ON recordings(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_recordings_template ON recordings(selected_template);
