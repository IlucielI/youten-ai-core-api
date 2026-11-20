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

// AudioSlicer is an optional extension interface for audio extractors capable of silence-aware chunking.
type AudioSlicer interface {
	// SliceAudioOnSilence splits long audio into silence-aligned chunks for parallel STT processing.
	SliceAudioOnSilence(ctx context.Context, input io.Reader, filename string, totalDurationSec float64) ([]dtos.AudioChunk, error)
}
