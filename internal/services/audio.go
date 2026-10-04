package services

import (
	"context"
	"io"
)

// AudioExtractionResult represents the output of audio extraction or conversion.
type AudioExtractionResult struct {
	Reader          io.ReadCloser
	Format          string
	DurationSeconds float64
	SizeBytes       int64
}

// AudioExtractor defines the consumer port contract for extracting mono audio from media files.
type AudioExtractor interface {
	// ExtractMonoAudio converts a video or audio stream into 16kHz mono MP3 or WAV optimized for STT.
	ExtractMonoAudio(ctx context.Context, input io.Reader, filename string) (*AudioExtractionResult, error)
}
