-- Seed default administrator account if not present, and ensure standard credentials hash
INSERT INTO admin_users (id, username, password_hash, full_name, role_id, status)
VALUES (
    '00000000-0000-0000-0000-000000000002',
    'admin',
    '$2a$12$n33YfdVl5YvERU.q3cxT/.06OTgvKT1R2PXSPUFXvx67tSp8HXrq2',
    'Platform Administrator',
    '00000000-0000-0000-0000-000000000001',
    'active'
)
ON CONFLICT (username) DO UPDATE
SET password_hash = EXCLUDED.password_hash,
    full_name = EXCLUDED.full_name,
    status = 'active';
