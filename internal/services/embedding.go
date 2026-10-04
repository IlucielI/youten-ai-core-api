package services

import (
	"context"
)

// EmbeddingProvider defines the port for generating dense vector embeddings.
type EmbeddingProvider interface {
	// CreateEmbeddings generates vector embeddings for a batch of text chunks.
	CreateEmbeddings(ctx context.Context, texts []string) ([][]float32, error)
}
