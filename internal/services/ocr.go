package services

import "context"

// LocalOCRExtractor defines the consumer port contract for lightweight local text extraction.
type LocalOCRExtractor interface {
	IsAvailable() bool
	ExtractText(ctx context.Context, imgBytes []byte) (text string, confidence float64, isUsable bool, err error)
}

