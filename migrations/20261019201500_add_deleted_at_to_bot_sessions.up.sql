ALTER TABLE bot_sessions ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS idx_bot_sessions_deleted_at ON bot_sessions(deleted_at);
