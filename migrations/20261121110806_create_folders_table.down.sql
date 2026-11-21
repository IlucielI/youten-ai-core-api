DROP INDEX IF EXISTS idx_recordings_folder_id;
ALTER TABLE recordings DROP COLUMN IF EXISTS folder_id;
DROP TABLE IF EXISTS folders;
