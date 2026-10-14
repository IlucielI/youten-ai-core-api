DROP INDEX IF EXISTS idx_bot_sessions_deleted_at;
ALTER TABLE bot_sessions DROP COLUMN IF EXISTS deleted_at;
