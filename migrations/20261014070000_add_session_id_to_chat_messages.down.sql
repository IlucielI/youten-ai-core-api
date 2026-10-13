DROP INDEX IF EXISTS idx_chat_messages_user;
DROP INDEX IF EXISTS idx_chat_messages_session;
ALTER TABLE chat_messages DROP COLUMN IF EXISTS user_id;
ALTER TABLE chat_messages DROP COLUMN IF EXISTS session_id;
