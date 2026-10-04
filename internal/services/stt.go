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
