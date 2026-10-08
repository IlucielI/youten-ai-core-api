CREATE TABLE IF NOT EXISTS client_apps (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    client_id VARCHAR(100) NOT NULL UNIQUE,
    client_secret_hash VARCHAR(255) NOT NULL,
    name VARCHAR(150) NOT NULL,
    description TEXT,
    allowed_scopes JSONB NOT NULL DEFAULT '[]'::jsonb,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO client_apps (id, client_id, client_secret_hash, name, description, allowed_scopes, is_active)
VALUES (
    '00000000-0000-0000-0000-000000000010',
    'client-app',
    '$2a$12$o0q4I1Rt.97j0oRCsIzVf.hwCwGWoaF7Rk0T7.4G6f8uLkzjw0l0u',
    'Official Web App & CMS Client Gateway',
    'Gateway identity for official client applications',
    '["recordings:create", "recordings:read", "recordings:chat", "auth:claim", "waitlist:join"]'::jsonb,
    TRUE
) ON CONFLICT (client_id) DO NOTHING;
