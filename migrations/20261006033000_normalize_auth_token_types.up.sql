-- Normalize auth_tokens type values to lowercase ('refresh', 'reset_password')
UPDATE auth_tokens SET type = 'refresh' WHERE type = 'REFRESH';
UPDATE auth_tokens SET type = 'reset_password' WHERE type = 'RESET_PASSWORD';
