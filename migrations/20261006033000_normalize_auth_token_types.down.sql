-- Revert auth_tokens type values to uppercase ('REFRESH', 'RESET_PASSWORD')
UPDATE auth_tokens SET type = 'REFRESH' WHERE type = 'refresh';
UPDATE auth_tokens SET type = 'RESET_PASSWORD' WHERE type = 'reset_password';
