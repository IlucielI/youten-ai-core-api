package services

import (
	"fmt"
	"strings"

	"code-base-golang/internal/models"
)

// ChunkerConfig contains tuning parameters for transcript semantic chunking.
type ChunkerConfig struct {
	MaxTokens     int
	OverlapTokens int
}

// DefaultChunkerConfig returns standard RAG chunking parameters (300 tokens, 50 overlap).
func DefaultChunkerConfig() ChunkerConfig {
	return ChunkerConfig{
		MaxTokens:     300,
		OverlapTokens: 50,
	}
}

// EstimateTokens provides a lightweight token estimation for text.
// Uses a standard heuristic of ~0.75 words per token (or ~4 chars per token).
func EstimateTokens(text string) int {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return 0
	}
	words := len(strings.Fields(trimmed))
	// Approximately 1.33 tokens per word
	tokens := (words * 4) / 3
	if tokens < 1 {
		tokens = 1
	}
	return tokens
}

// ChunkTranscriptSegments partitions SegmentResult items into overlapping chunks with precise time boundaries.
func ChunkTranscriptSegments(segments []SegmentResult, cfg ChunkerConfig) []TranscriptChunkData {
	if len(segments) == 0 {
		return nil
	}

	maxTokens := cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 300
	}
	overlapTokens := cfg.OverlapTokens
	if overlapTokens < 0 {
		overlapTokens = 50
	}
	if overlapTokens >= maxTokens {
		overlapTokens = maxTokens / 4
	}

	var chunks []TranscriptChunkData
	chunkIndex := 0

	n := len(segments)
	i := 0

	for i < n {
		var currentSegments []SegmentResult
		currentTokens := 0
		firstIdx := i

		for j := i; j < n; j++ {
			seg := segments[j]
			segText := formatSegmentText(seg.SpeakerLabel, seg.Text)
			segTokens := EstimateTokens(segText)

			// If adding this segment exceeds maxTokens and we already have at least one segment, stop.
			if len(currentSegments) > 0 && currentTokens+segTokens > maxTokens {
				break
			}

			currentSegments = append(currentSegments, seg)
			currentTokens += segTokens
		}

		if len(currentSegments) == 0 {
			// Defensively ensure progress if a single segment is somehow skipped
			currentSegments = append(currentSegments, segments[i])
		}

		// Assemble chunk text and time boundaries
		var sb strings.Builder
		for idx, s := range currentSegments {
			if idx > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(formatSegmentText(s.SpeakerLabel, s.Text))
		}

		chunk := TranscriptChunkData{
			ChunkIndex: chunkIndex,
			Content:    sb.String(),
			StartTime:  currentSegments[0].Start,
			EndTime:    currentSegments[len(currentSegments)-1].End,
		}
		chunks = append(chunks, chunk)
		chunkIndex++

		lastIdx := firstIdx + len(currentSegments) - 1
		if lastIdx >= n-1 {
			// Reached the end of all segments
			break
		}

		// Compute overlap: find trailing segments within overlapTokens
		nextStart := lastIdx + 1
		overlapCount := 0
		accumulatedOverlap := 0

		if overlapTokens > 0 {
			for k := len(currentSegments) - 1; k >= 0; k-- {
				s := currentSegments[k]
				tok := EstimateTokens(formatSegmentText(s.SpeakerLabel, s.Text))
				if accumulatedOverlap+tok > overlapTokens && overlapCount > 0 {
					break
				}
				accumulatedOverlap += tok
				overlapCount++
			}
		}

		// Ensure strictly forward progress (cannot overlap 100% of currentSegments)
		if overlapCount >= len(currentSegments) {
			overlapCount = len(currentSegments) - 1
		}
		if overlapCount < 0 {
			overlapCount = 0
		}

		candidateStart := (firstIdx + len(currentSegments)) - overlapCount
		if candidateStart <= firstIdx {
			candidateStart = firstIdx + 1
		}
		if candidateStart > nextStart {
			candidateStart = nextStart
		}
		i = candidateStart
	}

	return chunks
}

// ChunkModelSegments converts models.TranscriptSegment records into overlapping chunks.
func ChunkModelSegments(segments []models.TranscriptSegment, cfg ChunkerConfig) []TranscriptChunkData {
	if len(segments) == 0 {
		return nil
	}

	results := make([]SegmentResult, len(segments))
	for i, seg := range segments {
		speaker := seg.SpeakerName
		if speaker == "" {
			speaker = seg.SpeakerLabel
		}
		results[i] = SegmentResult{
			ID:           seg.SequenceOrder,
			Start:        seg.StartTime,
			End:          seg.EndTime,
			Text:         seg.Text,
			SpeakerLabel: speaker,
		}
	}

	return ChunkTranscriptSegments(results, cfg)
}

func formatSegmentText(speaker, text string) string {
	cleanText := strings.TrimSpace(text)
	cleanSpeaker := strings.TrimSpace(speaker)
	if cleanSpeaker != "" {
		return fmt.Sprintf("[%s]: %s", cleanSpeaker, cleanText)
	}
	return cleanText
}
