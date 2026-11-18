-- Create video_keyframes table for storing extracted video slides, screen share cues, and visual context
CREATE TABLE IF NOT EXISTS video_keyframes (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    recording_id UUID NOT NULL REFERENCES recordings(id) ON DELETE CASCADE,
    timestamp_seconds DOUBLE PRECISION NOT NULL,
    s3_image_url TEXT NOT NULL,
    cue_type VARCHAR(50) NOT NULL DEFAULT 'SLIDE',
    title VARCHAR(255),
    extracted_text TEXT,
    visual_summary TEXT,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    confidence DOUBLE PRECISION NOT NULL DEFAULT 1.0,
    sequence_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_video_keyframes_recording_time ON video_keyframes(recording_id, timestamp_seconds);
CREATE INDEX IF NOT EXISTS idx_video_keyframes_recording_seq ON video_keyframes(recording_id, sequence_order);
CREATE INDEX IF NOT EXISTS idx_video_keyframes_cue_type ON video_keyframes(cue_type);

