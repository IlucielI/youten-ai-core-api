package services

import (
	"context"

	"code-base-golang/internal/dtos"
)

// VisionAnalyzer defines the consumer port contract for analyzing video keyframe images (OCR + Multimodal LLM).
type VisionAnalyzer interface {
	// AnalyzeKeyframe extracts structured visual context (type, title, text, summary, speakers) from an image.
	AnalyzeKeyframe(ctx context.Context, imgBytes []byte, mimeType string, prompt string) (*dtos.KeyframeAnalysisResult, error)
	// AnalyzeKeyframesBatch analyzes multiple keyframes concurrently or sequentially.
	AnalyzeKeyframesBatch(ctx context.Context, frames []dtos.KeyframeBatchInput) ([]dtos.KeyframeAnalysisResult, error)
}

