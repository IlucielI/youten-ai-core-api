-- Revert extended domain templates
DELETE FROM templates WHERE category_key IN ('PODCAST', 'LECTURE', 'MUSIC_LYRICS');
