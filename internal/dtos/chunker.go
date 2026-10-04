package dtos

// TranscriptChunkData represents a semantic chunk produced from transcript segments, ready for vector embedding and storage.
type TranscriptChunkData struct {
	ChunkIndex int       `json:"chunk_index"`
	Content    string    `json:"content"`
	StartTime  float64   `json:"start_time"`
	EndTime    float64   `json:"end_time"`
	Embedding  []float32 `json:"embedding,omitempty"`
}

// ChunkerConfig contains tuning parameters for transcript semantic chunking.
type ChunkerConfig struct {
	MaxTokens     int
	OverlapTokens int
}

// DefaultChunkerConfig returns standard RAG chunking parameters (300 tokens, 50 overlap).
func DefaultChunkerConfig() ChunkerConfig {
	return ChunkerConfig{
		MaxTokens:     300,
		OverlapTokens: 50,
	}
}
