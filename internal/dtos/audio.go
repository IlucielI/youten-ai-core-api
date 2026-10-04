package dtos

import "io"

// AudioExtractionResult represents the output of audio extraction or conversion.
type AudioExtractionResult struct {
	Reader          io.ReadCloser
	Format          string
	DurationSeconds float64
	SizeBytes       int64
}
