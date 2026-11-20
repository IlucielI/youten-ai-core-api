-- 1. Create workspaces table
CREATE TABLE IF NOT EXISTS workspaces (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(150) NOT NULL,
    slug VARCHAR(150) NOT NULL UNIQUE,
    type VARCHAR(30) NOT NULL DEFAULT 'team' CHECK (type IN ('personal', 'team')),
    owner_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    logo_url TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_workspaces_owner_user_id ON workspaces(owner_user_id);
CREATE INDEX IF NOT EXISTS idx_workspaces_deleted_at ON workspaces(deleted_at);

-- 2. Create workspace_members table
CREATE TABLE IF NOT EXISTS workspace_members (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role VARCHAR(30) NOT NULL DEFAULT 'member' CHECK (role IN ('owner', 'admin', 'member', 'viewer')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS uidx_workspace_members_ws_user ON workspace_members(workspace_id, user_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_workspace_members_workspace_id ON workspace_members(workspace_id);
CREATE INDEX IF NOT EXISTS idx_workspace_members_user_id ON workspace_members(user_id);

-- 3. Add workspace_id to recordings table
ALTER TABLE recordings ADD COLUMN IF NOT EXISTS workspace_id UUID REFERENCES workspaces(id) ON DELETE CASCADE;
CREATE INDEX IF NOT EXISTS idx_recordings_workspace_id ON recordings(workspace_id);

-- 4. Backfill personal workspaces for existing users
INSERT INTO workspaces (id, name, slug, type, owner_user_id, created_at, updated_at)
SELECT 
    uuid_generate_v4(),
    COALESCE(NULLIF(TRIM(full_name), ''), 'My') || '''s Workspace',
    'personal-' || id::text,
    'personal',
    id,
    NOW(),
    NOW()
FROM users
ON CONFLICT (slug) DO NOTHING;

-- 5. Backfill workspace_members for owners
INSERT INTO workspace_members (id, workspace_id, user_id, role, created_at, updated_at)
SELECT 
    uuid_generate_v4(),
    w.id,
    w.owner_user_id,
    'owner',
    NOW(),
    NOW()
FROM workspaces w
ON CONFLICT DO NOTHING;

-- 6. Link existing user recordings to their personal workspace
UPDATE recordings r
SET workspace_id = w.id
FROM workspaces w
WHERE r.user_id = w.owner_user_id 
  AND w.type = 'personal'
  AND r.workspace_id IS NULL;
