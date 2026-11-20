package services

import (
	"context"
	"io"

	"code-base-golang/internal/dtos"
)

// STTProvider defines the port for speech-to-text audio transcription.
type STTProvider interface {
	Transcribe(ctx context.Context, reader io.Reader, filename string, opts dtos.STTOptions) (*dtos.TranscriptionResult, error)
}

// ParallelSTTProvider is an optional extension interface for STT providers that support parallel chunk dispatch.
type ParallelSTTProvider interface {
	STTProvider

	// TranscribeChunksParallel dispatches multiple audio chunks concurrently and stitches results into a single chronological transcript.
	TranscribeChunksParallel(ctx context.Context, chunks []dtos.AudioChunk, opts dtos.STTOptions) (*dtos.TranscriptionResult, error)
}
