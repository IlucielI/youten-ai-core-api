CREATE TABLE IF NOT EXISTS user_roles (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(100) NOT NULL UNIQUE,
    code VARCHAR(50) NOT NULL UNIQUE,
    description TEXT,
    permissions JSONB NOT NULL DEFAULT '[]'::jsonb,
    daily_quota INT NOT NULL DEFAULT 5,
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Seed Default User Roles: Free Member and Pro Member
INSERT INTO user_roles (id, name, code, description, permissions, daily_quota, is_default)
VALUES 
(
    '00000000-0000-0000-0000-000000000020',
    'Free Member',
    'FREE',
    'Standard free tier with default daily quota and baseline access',
    '["recordings:create", "recordings:read", "recordings:chat", "recordings:share", "profile:manage"]'::jsonb,
    5,
    TRUE
),
(
    '00000000-0000-0000-0000-000000000021',
    'Pro Member',
    'PRO',
    'Professional tier with PDF export, workspace memory, vector search, and higher quota',
    '["recordings:*", "workspace:search", "workspace:memory", "export:pdf", "profile:manage"]'::jsonb,
    25,
    FALSE
)
ON CONFLICT (code) DO NOTHING;

-- Add role_id foreign key column to users table
ALTER TABLE users 
ADD COLUMN IF NOT EXISTS role_id UUID REFERENCES user_roles(id) ON DELETE RESTRICT;

CREATE INDEX IF NOT EXISTS idx_users_role_id ON users(role_id);

-- Backfill existing users with default Free Member role dynamically
UPDATE users 
SET role_id = (SELECT id FROM user_roles WHERE is_default = TRUE ORDER BY created_at ASC LIMIT 1) 
WHERE role_id IS NULL;
