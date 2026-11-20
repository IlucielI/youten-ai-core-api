package dtos

import "io"

// AudioExtractionResult represents the output of audio extraction or conversion.
type AudioExtractionResult struct {
	Reader          io.ReadCloser
	Format          string
	DurationSeconds float64
	SizeBytes       int64
}

// AudioChunk represents a time-bounded slice of an audio recording with its start offset.
type AudioChunk struct {
	Index           int     `json:"index"`
	StartOffsetSec  float64 `json:"start_offset_sec"`
	DurationSeconds float64 `json:"duration_seconds"`
	Data            []byte  `json:"data"`
	Format          string  `json:"format"`
}
