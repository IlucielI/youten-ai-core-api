package services

import (
	"context"
	"io"

	"code-base-golang/internal/dtos"
)

// VisualExtractor defines the consumer port contract for extracting keyframes from video streams.
type VisualExtractor interface {
	// ExtractKeyframes extracts candidate visual keyframes using scene-change detection and audio anchors.
	ExtractKeyframes(ctx context.Context, input io.Reader, filename string, opts dtos.KeyframeExtractionOptions) ([]dtos.ExtractedKeyframe, error)
}

