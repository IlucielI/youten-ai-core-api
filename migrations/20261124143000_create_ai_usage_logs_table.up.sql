-- Create ai_usage_logs table for comprehensive AI model and token audit
CREATE TABLE IF NOT EXISTS ai_usage_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    workspace_id UUID REFERENCES workspaces(id) ON DELETE SET NULL,
    recording_id UUID REFERENCES recordings(id) ON DELETE SET NULL,
    service_type VARCHAR(50) NOT NULL,
    provider VARCHAR(50) NOT NULL,
    model_name VARCHAR(100) NOT NULL,
    prompt_tokens INT NOT NULL DEFAULT 0,
    completion_tokens INT NOT NULL DEFAULT 0,
    total_tokens INT NOT NULL DEFAULT 0,
    audio_seconds NUMERIC(10, 2) NOT NULL DEFAULT 0.00,
    latency_ms INT NOT NULL DEFAULT 0,
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ai_usage_logs_user_id ON ai_usage_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_ai_usage_logs_recording_id ON ai_usage_logs(recording_id);
CREATE INDEX IF NOT EXISTS idx_ai_usage_logs_workspace_id ON ai_usage_logs(workspace_id);
CREATE INDEX IF NOT EXISTS idx_ai_usage_logs_service_type ON ai_usage_logs(service_type);
CREATE INDEX IF NOT EXISTS idx_ai_usage_logs_created_at ON ai_usage_logs(created_at);

-- Add total_tokens_used to recordings table for instant cached token count lookup
ALTER TABLE recordings ADD COLUMN IF NOT EXISTS total_tokens_used INT NOT NULL DEFAULT 0;
