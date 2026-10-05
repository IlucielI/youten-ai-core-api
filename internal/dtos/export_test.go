package dtos_test

import (
	"encoding/json"
	"testing"
	"time"

	"code-base-golang/internal/dtos"
)

func TestExportDTO_JSONExportPayload_Serialization(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	payload := dtos.JSONExportPayload{
		RecordingID:      "rec-123",
		Title:            "Weekly Sync",
		DurationSeconds:  120.5,
		Status:           "ready",
		CreatedAt:        now,
		ExecutiveSummary: "Discussed Q4 roadmap.",
		ActionItems:      []string{"Send deck", "Schedule follow-up"},
		Chapters: []dtos.JSONExportChapter{
			{
				Title:     "Introduction",
				StartTime: 0.0,
				EndTime:   30.0,
				Summary:   "Welcome and agenda.",
			},
		},
		Highlights: []dtos.JSONExportHighlight{
			{
				Title:     "Key Decision",
				StartTime: 15.0,
				EndTime:   25.0,
				Note:      "Approved budget",
			},
		},
		Transcript: []dtos.JSONExportTranscriptSeg{
			{
				Speaker:   "Alice",
				StartTime: 0.0,
				EndTime:   10.0,
				Text:      "Hello everyone.",
			},
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal JSONExportPayload: %v", err)
	}

	var decoded dtos.JSONExportPayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal JSONExportPayload: %v", err)
	}

	if decoded.RecordingID != payload.RecordingID {
		t.Errorf("expected RecordingID %s, got %s", payload.RecordingID, decoded.RecordingID)
	}
	if decoded.Title != payload.Title {
		t.Errorf("expected Title %s, got %s", payload.Title, decoded.Title)
	}
	if len(decoded.Chapters) != 1 || decoded.Chapters[0].Title != "Introduction" {
		t.Errorf("unexpected chapters: %+v", decoded.Chapters)
	}
	if len(decoded.Highlights) != 1 || decoded.Highlights[0].Note != "Approved budget" {
		t.Errorf("unexpected highlights: %+v", decoded.Highlights)
	}
	if len(decoded.Transcript) != 1 || decoded.Transcript[0].Speaker != "Alice" {
		t.Errorf("unexpected transcript: %+v", decoded.Transcript)
	}
}

func TestExportDTO_ExportResult_Structure(t *testing.T) {
	res := dtos.ExportResult{
		Filename:    "meeting_mom.md",
		ContentType: "text/markdown; charset=utf-8",
		Data:        []byte("# Meeting Notes"),
	}

	if res.Filename != "meeting_mom.md" {
		t.Errorf("expected Filename meeting_mom.md, got %s", res.Filename)
	}
	if res.ContentType != "text/markdown; charset=utf-8" {
		t.Errorf("expected ContentType text/markdown; charset=utf-8, got %s", res.ContentType)
	}
	if string(res.Data) != "# Meeting Notes" {
		t.Errorf("expected Data '# Meeting Notes', got %s", string(res.Data))
	}
}
