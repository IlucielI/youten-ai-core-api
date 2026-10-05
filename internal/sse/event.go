package sse

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"code-base-golang/internal/models"
)

// Event represents an individual Server-Sent Event frame.
type Event struct {
	ID      string
	Event   string
	Data    any
	Retry   time.Duration
	Comment string
}

// ProgressEvent defines the JSON payload streamed during pipeline execution.
type ProgressEvent struct {
	RecordingID  string    `json:"recording_id"`
	Status       string    `json:"status"`
	Stage        string    `json:"stage"`
	Progress     int       `json:"progress"`
	ErrorCode    *string   `json:"error_code,omitempty"`
	ErrorMessage *string   `json:"error_message,omitempty"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ChatTokenEvent defines the payload for an individual streamed token.
type ChatTokenEvent struct {
	Token string `json:"token"`
}

// ChatDoneEvent defines the completion payload with extracted citations and retrieved chunk IDs.
type ChatDoneEvent struct {
	MessageID         string   `json:"message_id"`
	Content           string   `json:"content"`
	Citations         []string `json:"citations"`
	RetrievedChunkIDs []string `json:"retrieved_chunk_ids"`
}

// ChatErrorEvent defines the payload when streaming encounters an error.
type ChatErrorEvent struct {
	Error string `json:"error"`
}

// MapStatusToProgress maps a recording status string to a readable stage name and progress percentage.
func MapStatusToProgress(status string) (stage string, progress int) {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case models.RecordingStatusPending:
		return "PENDING", 5
	case models.RecordingStatusQueued:
		return "QUEUED", 5
	case models.RecordingStatusValidating:
		return "VALIDATING", 15
	case models.RecordingStatusExtracting:
		return "EXTRACTING", 30
	case models.RecordingStatusTranscribing:
		return "TRANSCRIBING", 55
	case models.RecordingStatusSummarizing:
		return "SUMMARIZING", 75
	case models.RecordingStatusIndexing:
		return "INDEXING", 90
	case models.RecordingStatusCompleted:
		return "COMPLETED", 100
	case models.RecordingStatusFailed:
		return "FAILED", 100
	default:
		return status, 0
	}
}

// IsTerminalStatus returns true if the status indicates pipeline completion or failure.
func IsTerminalStatus(status string) bool {
	s := strings.ToUpper(strings.TrimSpace(status))
	return s == models.RecordingStatusCompleted || s == models.RecordingStatusFailed
}

// Encode serializes an Event into standard W3C text/event-stream format.
func Encode(e Event) ([]byte, error) {
	var buf bytes.Buffer

	// SSE Comment line (used for keep-alive pings)
	if e.Comment != "" {
		buf.WriteString(": ")
		buf.WriteString(e.Comment)
		buf.WriteString("\n\n")
		return buf.Bytes(), nil
	}

	if e.ID != "" {
		buf.WriteString("id: ")
		buf.WriteString(e.ID)
		buf.WriteByte('\n')
	}

	if e.Event != "" {
		buf.WriteString("event: ")
		buf.WriteString(e.Event)
		buf.WriteByte('\n')
	}

	if e.Retry > 0 {
		buf.WriteString("retry: ")
		buf.WriteString(strconv.FormatInt(e.Retry.Milliseconds(), 10))
		buf.WriteByte('\n')
	}

	if e.Data != nil {
		var dataBytes []byte
		switch v := e.Data.(type) {
		case []byte:
			dataBytes = v
		case string:
			dataBytes = []byte(v)
		default:
			marshaled, err := json.Marshal(v)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal sse data: %w", err)
			}
			dataBytes = marshaled
		}

		lines := bytes.Split(dataBytes, []byte("\n"))
		for _, line := range lines {
			buf.WriteString("data: ")
			buf.Write(line)
			buf.WriteByte('\n')
		}
	}

	buf.WriteByte('\n')
	return buf.Bytes(), nil
}
