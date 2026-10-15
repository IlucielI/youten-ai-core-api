-- Revert attachments column from feedbacks table
ALTER TABLE feedbacks DROP COLUMN IF EXISTS attachments;
