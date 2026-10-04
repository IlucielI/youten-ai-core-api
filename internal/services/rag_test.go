package services_test

import (
	"strings"
	"testing"

	"code-base-golang/internal/dtos"
	"code-base-golang/internal/services"
)

func TestRAG_FormatTimestamp(t *testing.T) {
	tests := []struct {
		seconds  float64
		expected string
	}{
		{-5.0, "00:00"},
		{0.0, "00:00"},
		{9.0, "00:09"},
		{65.0, "01:05"},
		{3599.0, "59:59"},
		{3600.0, "01:00:00"},
		{3665.0, "01:01:05"},
	}

	for _, tt := range tests {
		got := services.FormatTimestamp(tt.seconds)
		if got != tt.expected {
			t.Errorf("FormatTimestamp(%f): expected %q, got %q", tt.seconds, tt.expected, got)
		}
	}
}

func TestRAG_BuildRAGPrompt(t *testing.T) {
	chunks := []dtos.TranscriptChunkData{
		{
			ChunkIndex: 0,
			Content:    "[Speaker 1]: We decided to launch the product in November.",
			StartTime:  15.0,
			EndTime:    35.0,
		},
		{
			ChunkIndex: 1,
			Content:    "[Speaker 2]: The budget allocated is 50,000 USD.",
			StartTime:  40.0,
			EndTime:    65.0,
		},
	}

	sys, user := services.BuildRAGPrompt("When is the launch date?", chunks)
	if !strings.Contains(sys, "Strict Grounding Rules") {
		t.Error("expected system prompt to contain Strict Grounding Rules")
	}
	if !strings.Contains(user, "00:15 - 00:35") {
		t.Error("expected user prompt to contain chunk 0 formatted timestamp")
	}
	if !strings.Contains(user, "00:40 - 01:05") {
		t.Error("expected user prompt to contain chunk 1 formatted timestamp")
	}
	if !strings.Contains(user, "When is the launch date?") {
		t.Error("expected user prompt to contain query")
	}

	// Test with empty chunks
	_, emptyUser := services.BuildRAGPrompt("Empty query", nil)
	if !strings.Contains(emptyUser, "(No transcript context available)") {
		t.Error("expected notice about no context")
	}
}

func TestRAG_ExtractCitations(t *testing.T) {
	text := "The launch is in November [00:25] and budget is set [01:05]. Another ref to [00:25] and hour [01:15:30]."
	citations := services.ExtractCitations(text)

	if len(citations) != 3 {
		t.Fatalf("expected 3 unique citations, got %d: %v", len(citations), citations)
	}
	if citations[0] != "00:25" || citations[1] != "01:05" || citations[2] != "01:15:30" {
		t.Errorf("unexpected citations slice: %v", citations)
	}

	// No citations
	if res := services.ExtractCitations("No timestamps here at all."); res != nil {
		t.Errorf("expected nil for no citations, got %v", res)
	}
}
