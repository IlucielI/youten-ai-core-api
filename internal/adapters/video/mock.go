package video

import (
	"context"
	"io"

	"code-base-golang/internal/dtos"
	"code-base-golang/internal/services"
)

// MockVisualExtractor provides an in-memory test implementation of services.VisualExtractor.
type MockVisualExtractor struct {
	ExtractFn func(ctx context.Context, input io.Reader, filename string, opts dtos.KeyframeExtractionOptions) ([]dtos.ExtractedKeyframe, error)
}

// NewMockVisualExtractor creates a new MockVisualExtractor with configurable behavior.
func NewMockVisualExtractor() *MockVisualExtractor {
	return &MockVisualExtractor{
		ExtractFn: func(ctx context.Context, input io.Reader, filename string, opts dtos.KeyframeExtractionOptions) ([]dtos.ExtractedKeyframe, error) {
			if input == nil {
				return nil, ErrEmptyInput
			}
			return []dtos.ExtractedKeyframe{
				{
					TimestampSeconds: 10.0,
					ImageBytes:       []byte("fake-jpeg-frame-1"),
					MimeType:         "image/jpeg",
					DHash:            123456789,
					SequenceOrder:    1,
				},
				{
					TimestampSeconds: 45.0,
					ImageBytes:       []byte("fake-jpeg-frame-2"),
					MimeType:         "image/jpeg",
					DHash:            987654321,
					SequenceOrder:    2,
				},
			}, nil
		},
	}
}

// ExtractKeyframes delegates to ExtractFn.
func (m *MockVisualExtractor) ExtractKeyframes(ctx context.Context, input io.Reader, filename string, opts dtos.KeyframeExtractionOptions) ([]dtos.ExtractedKeyframe, error) {
	if m.ExtractFn != nil {
		return m.ExtractFn(ctx, input, filename, opts)
	}
	return nil, nil
}

var _ services.VisualExtractor = (*MockVisualExtractor)(nil)

