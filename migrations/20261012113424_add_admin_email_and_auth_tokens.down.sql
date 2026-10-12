DROP TABLE IF EXISTS admin_auth_tokens;
DROP INDEX IF EXISTS idx_admin_users_email;
ALTER TABLE admin_users DROP COLUMN IF EXISTS email;
