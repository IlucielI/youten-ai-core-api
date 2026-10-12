-- Add email column to admin_users table
ALTER TABLE admin_users ADD COLUMN IF NOT EXISTS email VARCHAR(255) DEFAULT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_admin_users_email ON admin_users(email) WHERE deleted_at IS NULL;

-- Update default admin user email if not set
UPDATE admin_users SET email = 'admin@youten.ai' WHERE username = 'admin' AND email IS NULL;

-- Create admin_auth_tokens table for admin reset tokens and session tokens
CREATE TABLE IF NOT EXISTS admin_auth_tokens (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    admin_id UUID NOT NULL REFERENCES admin_users(id) ON DELETE CASCADE,
    type VARCHAR(50) NOT NULL,
    token_hash VARCHAR(255) NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ DEFAULT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_admin_auth_tokens_admin_id ON admin_auth_tokens(admin_id);
CREATE INDEX IF NOT EXISTS idx_admin_auth_tokens_hash ON admin_auth_tokens(token_hash);
CREATE INDEX IF NOT EXISTS idx_admin_auth_tokens_type ON admin_auth_tokens(type);
