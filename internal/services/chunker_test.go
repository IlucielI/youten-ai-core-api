package services_test

import (
	"fmt"
	"testing"

	"code-base-golang/internal/models"
	"code-base-golang/internal/services"
)

func TestChunker_EstimateTokens(t *testing.T) {
	if got := services.EstimateTokens(""); got != 0 {
		t.Errorf("expected 0 for empty string, got %d", got)
	}
	if got := services.EstimateTokens("hello"); got < 1 {
		t.Errorf("expected >= 1 for single word, got %d", got)
	}
	text := "This is a simple test sentence for estimating token counts accurately."
	got := services.EstimateTokens(text)
	if got <= 5 || got >= 30 {
		t.Errorf("unexpected token estimate for sample text: %d", got)
	}
}

func TestChunker_ChunkTranscriptSegments(t *testing.T) {
	// Empty slice test
	if chunks := services.ChunkTranscriptSegments(nil, services.DefaultChunkerConfig()); chunks != nil {
		t.Errorf("expected nil for empty segments, got %v", chunks)
	}

	// Create 10 sample segments
	var segments []services.SegmentResult
	for i := 0; i < 10; i++ {
		segments = append(segments, services.SegmentResult{
			ID:           i,
			Start:        float64(i * 10),
			End:          float64((i + 1) * 10),
			SpeakerLabel: fmt.Sprintf("SPEAKER_0%d", i%2),
			Text:         fmt.Sprintf("Segment content number %d discussing technical topics in detail.", i),
		})
	}

	cfg := services.ChunkerConfig{
		MaxTokens:     30,
		OverlapTokens: 10,
	}

	chunks := services.ChunkTranscriptSegments(segments, cfg)
	if len(chunks) == 0 {
		t.Fatal("expected at least one chunk")
	}

	// Verify sequential chunk indices and valid time boundaries
	for i, c := range chunks {
		if c.ChunkIndex != i {
			t.Errorf("expected chunk index %d, got %d", i, c.ChunkIndex)
		}
		if c.StartTime > c.EndTime {
			t.Errorf("invalid timestamp range: start %f > end %f", c.StartTime, c.EndTime)
		}
		if c.Content == "" {
			t.Errorf("empty chunk content at index %d", i)
		}
	}

	// Verify first chunk starts at segment 0's start time
	if chunks[0].StartTime != 0.0 {
		t.Errorf("expected first chunk start 0.0, got %f", chunks[0].StartTime)
	}

	// Verify last chunk ends at segment 9's end time
	lastChunk := chunks[len(chunks)-1]
	if lastChunk.EndTime != 100.0 {
		t.Errorf("expected last chunk end 100.0, got %f", lastChunk.EndTime)
	}
}

func TestChunker_ChunkModelSegments(t *testing.T) {
	var modelSegments []models.TranscriptSegment
	for i := 0; i < 5; i++ {
		modelSegments = append(modelSegments, models.TranscriptSegment{
			SequenceOrder: i,
			StartTime:     float64(i * 5),
			EndTime:       float64((i + 1) * 5),
			SpeakerName:   "Alice",
			Text:          fmt.Sprintf("Line %d by Alice", i),
		})
	}

	chunks := services.ChunkModelSegments(modelSegments, services.ChunkerConfig{
		MaxTokens:     50,
		OverlapTokens: 10,
	})

	if len(chunks) == 0 {
		t.Fatal("expected non-empty chunks from model segments")
	}
	if chunks[0].StartTime != 0.0 {
		t.Errorf("expected start time 0.0, got %f", chunks[0].StartTime)
	}
}

func TestChunker_EdgeCases(t *testing.T) {
	// Single massive segment larger than maxTokens
	segments := []services.SegmentResult{
		{
			ID:    0,
			Start: 0,
			End:   60,
			Text:  "A very very long text that exceeds the limit by having lots and lots of words repeated multiple times to ensure chunking proceeds without infinite loop.",
		},
		{
			ID:    1,
			Start: 60,
			End:   120,
			Text:  "Second segment after the huge one.",
		},
	}

	chunks := services.ChunkTranscriptSegments(segments, services.ChunkerConfig{
		MaxTokens:     5,
		OverlapTokens: 1,
	})

	if len(chunks) < 2 {
		t.Errorf("expected at least 2 chunks, got %d", len(chunks))
	}
}

func TestChunker_ZeroOverlap(t *testing.T) {
	segments := []services.SegmentResult{
		{ID: 0, Start: 0, End: 10, Text: "Segment 1 text content"},
		{ID: 1, Start: 10, End: 20, Text: "Segment 2 text content"},
		{ID: 2, Start: 20, End: 30, Text: "Segment 3 text content"},
		{ID: 3, Start: 30, End: 40, Text: "Segment 4 text content"},
	}

	chunks := services.ChunkTranscriptSegments(segments, services.ChunkerConfig{
		MaxTokens:     5,
		OverlapTokens: 0,
	})

	// With 0 overlap, chunks must not repeat any segments
	if len(chunks) != 4 {
		t.Fatalf("expected 4 chunks for 0 overlap, got %d", len(chunks))
	}
	seenContents := make(map[string]bool)
	for i, c := range chunks {
		expectedStart := float64(i * 10)
		expectedEnd := float64((i + 1) * 10)
		if c.StartTime != expectedStart || c.EndTime != expectedEnd {
			t.Errorf("chunk %d: expected [%f - %f], got [%f - %f]", i, expectedStart, expectedEnd, c.StartTime, c.EndTime)
		}
		if seenContents[c.Content] {
			t.Errorf("chunk %d has duplicate repeated content: %s", i, c.Content)
		}
		seenContents[c.Content] = true
	}
}
