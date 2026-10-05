DROP INDEX IF EXISTS idx_comments_user_id;
ALTER TABLE inline_comments DROP COLUMN IF EXISTS user_id;
