-- Create transcript_segments table (word-level diarization for interactive karaoke)
CREATE TABLE IF NOT EXISTS transcript_segments (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    recording_id UUID NOT NULL REFERENCES recordings(id) ON DELETE CASCADE,
    speaker_label VARCHAR(50) NOT NULL DEFAULT 'Speaker 0',
    speaker_name VARCHAR(100) NOT NULL DEFAULT 'Speaker 0',
    start_time DOUBLE PRECISION NOT NULL,
    end_time DOUBLE PRECISION NOT NULL,
    text TEXT NOT NULL,
    words_data JSONB NOT NULL DEFAULT '[]'::jsonb,
    sequence_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_segments_recording_seq ON transcript_segments(recording_id, sequence_order);
CREATE INDEX IF NOT EXISTS idx_segments_start_time ON transcript_segments(start_time);

-- Create transcript_chunks table with pgvector support
CREATE TABLE IF NOT EXISTS transcript_chunks (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    recording_id UUID NOT NULL REFERENCES recordings(id) ON DELETE CASCADE,
    chunk_index INT NOT NULL DEFAULT 0,
    content TEXT NOT NULL,
    start_time DOUBLE PRECISION NOT NULL DEFAULT 0,
    end_time DOUBLE PRECISION NOT NULL DEFAULT 0,
    embedding vector(1024),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_chunks_recording_index ON transcript_chunks(recording_id, chunk_index);

-- Create HNSW Vector index for fast cosine distance approximate nearest neighbor search
CREATE INDEX IF NOT EXISTS idx_chunks_embedding_hnsw 
ON transcript_chunks 
USING hnsw (embedding vector_cosine_ops);
