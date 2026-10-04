-- Create summaries table (versioned, high-density structured analysis)
CREATE TABLE IF NOT EXISTS summaries (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    recording_id UUID NOT NULL REFERENCES recordings(id) ON DELETE CASCADE,
    template_category VARCHAR(100) NOT NULL,
    custom_angle TEXT DEFAULT NULL,
    version INT NOT NULL DEFAULT 1,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    structured_data JSONB NOT NULL DEFAULT '{}'::jsonb,
    markdown_content TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_summaries_recording_active ON summaries(recording_id, is_active);
CREATE INDEX IF NOT EXISTS idx_summaries_version ON summaries(recording_id, version);

-- Create chapters table (automatic topic segmentation)
CREATE TABLE IF NOT EXISTS chapters (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    recording_id UUID NOT NULL REFERENCES recordings(id) ON DELETE CASCADE,
    title VARCHAR(255) NOT NULL,
    start_time DOUBLE PRECISION NOT NULL,
    end_time DOUBLE PRECISION NOT NULL,
    summary TEXT NOT NULL DEFAULT '',
    sequence_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_chapters_recording_seq ON chapters(recording_id, sequence_order);

-- Create highlights table (key moments & audio clips)
CREATE TABLE IF NOT EXISTS highlights (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    recording_id UUID NOT NULL REFERENCES recordings(id) ON DELETE CASCADE,
    start_time DOUBLE PRECISION NOT NULL,
    end_time DOUBLE PRECISION NOT NULL,
    title VARCHAR(255) DEFAULT NULL,
    note TEXT DEFAULT NULL,
    source VARCHAR(50) NOT NULL DEFAULT 'manual',
    clip_url TEXT DEFAULT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_highlights_recording_id ON highlights(recording_id);
