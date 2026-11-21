CREATE TABLE IF NOT EXISTS workspace_invitations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    email VARCHAR(255) NOT NULL,
    invite_token VARCHAR(64) NOT NULL,
    role VARCHAR(32) NOT NULL DEFAULT 'member',
    invited_by_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    accepted_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_workspace_invitations_workspace_email ON workspace_invitations(workspace_id, email) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uidx_workspace_invitations_token ON workspace_invitations(invite_token) WHERE deleted_at IS NULL;
