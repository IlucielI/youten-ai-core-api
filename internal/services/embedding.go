package services

import (
	"context"
)

// TranscriptChunkData represents a semantic chunk produced from transcript segments, ready for vector embedding and storage.
type TranscriptChunkData struct {
	ChunkIndex int       `json:"chunk_index"`
	Content    string    `json:"content"`
	StartTime  float64   `json:"start_time"`
	EndTime    float64   `json:"end_time"`
	Embedding  []float32 `json:"embedding,omitempty"`
}

// EmbeddingProvider defines the port for generating dense vector embeddings.
type EmbeddingProvider interface {
	// CreateEmbeddings generates vector embeddings for a batch of text chunks.
	CreateEmbeddings(ctx context.Context, texts []string) ([][]float32, error)
}
