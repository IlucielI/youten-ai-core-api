-- Add user_id column to inline_comments for verified author ownership
ALTER TABLE inline_comments ADD COLUMN IF NOT EXISTS user_id UUID REFERENCES users(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_comments_user_id ON inline_comments(user_id);
