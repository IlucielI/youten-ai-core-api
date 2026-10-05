package dtos

import (
	"time"
)

// ExportResult contains the generated file data and metadata for HTTP response streaming.
type ExportResult struct {
	Filename    string
	ContentType string
	Data        []byte
}

// JSONExportPayload defines the structured JSON export document schema for meeting recordings.
type JSONExportPayload struct {
	RecordingID      string                    `json:"recording_id"`
	Title            string                    `json:"title"`
	DurationSeconds  float64                   `json:"duration_seconds"`
	Status           string                    `json:"status"`
	CreatedAt        time.Time                 `json:"created_at"`
	ExecutiveSummary string                    `json:"executive_summary,omitempty"`
	ActionItems      []string                  `json:"action_items,omitempty"`
	Chapters         []JSONExportChapter       `json:"chapters,omitempty"`
	Highlights       []JSONExportHighlight     `json:"highlights,omitempty"`
	Transcript       []JSONExportTranscriptSeg `json:"transcript,omitempty"`
}

// JSONExportChapter represents a single discussion chapter within an exported JSON document.
type JSONExportChapter struct {
	Title     string  `json:"title"`
	StartTime float64 `json:"start_time"`
	EndTime   float64 `json:"end_time"`
	Summary   string  `json:"summary"`
}

// JSONExportHighlight represents a key highlight within an exported JSON document.
type JSONExportHighlight struct {
	Title     string  `json:"title,omitempty"`
	StartTime float64 `json:"start_time"`
	EndTime   float64 `json:"end_time"`
	Note      string  `json:"note,omitempty"`
}

// JSONExportTranscriptSeg represents a timestamped transcript segment within an exported JSON document.
type JSONExportTranscriptSeg struct {
	Speaker   string  `json:"speaker"`
	StartTime float64 `json:"start_time"`
	EndTime   float64 `json:"end_time"`
	Text      string  `json:"text"`
}
