package audio

import (
	"bytes"
	"context"
	"io"

	"code-base-golang/internal/dtos"
	"code-base-golang/internal/services"
)

// MockAudioExtractor provides an in-memory test implementation of services.AudioExtractor.
type MockAudioExtractor struct {
	ExtractFn func(ctx context.Context, input io.Reader, filename string) (*dtos.AudioExtractionResult, error)
}

// NewMock creates a new MockAudioExtractor with default pass-through behavior.
func NewMock() *MockAudioExtractor {
	return &MockAudioExtractor{
		ExtractFn: func(ctx context.Context, input io.Reader, filename string) (*dtos.AudioExtractionResult, error) {
			if input == nil {
				return nil, ErrEmptyInput
			}
			data, err := io.ReadAll(input)
			if err != nil {
				return nil, err
			}
			if len(data) == 0 {
				return nil, ErrEmptyInput
			}
			return &dtos.AudioExtractionResult{
				Reader:          io.NopCloser(bytes.NewReader(data)),
				Format:          "mp3",
				DurationSeconds: 120.0,
				SizeBytes:       int64(len(data)),
			}, nil
		},
	}
}

// ExtractMonoAudio delegates to ExtractFn.
func (m *MockAudioExtractor) ExtractMonoAudio(ctx context.Context, input io.Reader, filename string) (*dtos.AudioExtractionResult, error) {
	if m.ExtractFn != nil {
		return m.ExtractFn(ctx, input, filename)
	}
	return nil, nil
}

// Ensure MockAudioExtractor satisfies services.AudioExtractor at compile time.
var _ services.AudioExtractor = (*MockAudioExtractor)(nil)
