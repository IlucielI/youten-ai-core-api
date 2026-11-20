package audio

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"strings"

	"code-base-golang/internal/dtos"
	"code-base-golang/internal/services"
)

// MockAudioExtractor provides an in-memory test implementation of services.AudioExtractor.
type MockAudioExtractor struct {
	ExtractFn func(ctx context.Context, input io.Reader, filename string) (*dtos.AudioExtractionResult, error)
	SliceFn   func(ctx context.Context, input io.Reader, filename string, totalDurationSec float64) ([]dtos.AudioChunk, error)
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
		SliceFn: func(ctx context.Context, input io.Reader, filename string, totalDurationSec float64) ([]dtos.AudioChunk, error) {
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
			format := strings.TrimPrefix(strings.ToLower(filepath.Ext(filename)), ".")
			if format == "" {
				format = "mp3"
			}
			return []dtos.AudioChunk{{
				Index:           0,
				StartOffsetSec:  0,
				DurationSeconds: totalDurationSec,
				Data:            data,
				Format:          format,
			}}, nil
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

// SliceAudioOnSilence delegates to SliceFn.
func (m *MockAudioExtractor) SliceAudioOnSilence(ctx context.Context, input io.Reader, filename string, totalDurationSec float64) ([]dtos.AudioChunk, error) {
	if m.SliceFn != nil {
		return m.SliceFn(ctx, input, filename, totalDurationSec)
	}
	return nil, nil
}

// Ensure MockAudioExtractor satisfies services.AudioExtractor and services.AudioSlicer at compile time.
var (
	_ services.AudioExtractor = (*MockAudioExtractor)(nil)
	_ services.AudioSlicer    = (*MockAudioExtractor)(nil)
)
