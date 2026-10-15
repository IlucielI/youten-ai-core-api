-- Add attachments JSONB column to feedbacks table for screenshots and files
ALTER TABLE feedbacks ADD COLUMN IF NOT EXISTS attachments JSONB DEFAULT '[]'::jsonb;
