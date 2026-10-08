-- Down migration: do not drop default admin if user references exist
DELETE FROM admin_users WHERE username = 'admin' AND id = '00000000-0000-0000-0000-000000000002';
