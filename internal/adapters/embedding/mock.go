package embedding

import (
	"context"

	"code-base-golang/internal/services"
)

// MockEmbedding provides a mockable implementation of services.EmbeddingProvider for unit tests.
type MockEmbedding struct {
	Dimension            int
	CreateEmbeddingsFunc func(ctx context.Context, texts []string) ([][]float32, error)
}

// NewMock creates a new MockEmbedding instance with the specified vector dimension (default 1024).
func NewMock(dimensionOpt ...int) *MockEmbedding {
	dimension := 1024
	if len(dimensionOpt) > 0 && dimensionOpt[0] > 0 {
		dimension = dimensionOpt[0]
	}
	return &MockEmbedding{Dimension: dimension}
}

// CreateEmbeddings produces mock vectors or executes CreateEmbeddingsFunc.
func (m *MockEmbedding) CreateEmbeddings(ctx context.Context, texts []string) ([][]float32, error) {
	if m.CreateEmbeddingsFunc != nil {
		return m.CreateEmbeddingsFunc(ctx, texts)
	}

	if len(texts) == 0 {
		return nil, nil
	}

	dim := m.Dimension
	if dim <= 0 {
		dim = 1024
	}

	results := make([][]float32, len(texts))
	for i, text := range texts {
		vec := make([]float32, dim)
		// Deterministically seed vector based on text length and index
		val := float32(len(text)%100) / 100.0
		for j := 0; j < dim; j++ {
			vec[j] = val + float32(j)*0.0001
		}
		results[i] = vec
	}

	return results, nil
}

var _ services.EmbeddingProvider = (*MockEmbedding)(nil)
