-- Create inline_comments table (code-review style timestamped comments)
CREATE TABLE IF NOT EXISTS inline_comments (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    recording_id UUID NOT NULL REFERENCES recordings(id) ON DELETE CASCADE,
    segment_id UUID REFERENCES transcript_segments(id) ON DELETE SET NULL,
    timestamp_sec DOUBLE PRECISION NOT NULL DEFAULT 0,
    selected_text TEXT DEFAULT NULL,
    author_name VARCHAR(100) NOT NULL,
    comment_text TEXT NOT NULL,
    parent_id UUID REFERENCES inline_comments(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_comments_recording_time ON inline_comments(recording_id, timestamp_sec);
CREATE INDEX IF NOT EXISTS idx_comments_parent_id ON inline_comments(parent_id);

-- Create chat_messages table (RAG interactive chat with grounded citations)
CREATE TABLE IF NOT EXISTS chat_messages (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    recording_id UUID NOT NULL REFERENCES recordings(id) ON DELETE CASCADE,
    sender_role VARCHAR(20) NOT NULL,
    content TEXT NOT NULL,
    citations JSONB NOT NULL DEFAULT '[]'::jsonb,
    retrieved_chunk_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_chat_messages_recording ON chat_messages(recording_id, created_at ASC);
