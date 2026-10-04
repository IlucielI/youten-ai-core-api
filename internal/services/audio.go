package services

import (
	"context"
	"io"

	"code-base-golang/internal/dtos"
)

// AudioExtractor defines the consumer port contract for extracting mono audio from media files.
type AudioExtractor interface {
	// ExtractMonoAudio converts a video or audio stream into 16kHz mono MP3 or WAV optimized for STT.
	ExtractMonoAudio(ctx context.Context, input io.Reader, filename string) (*dtos.AudioExtractionResult, error)
}
