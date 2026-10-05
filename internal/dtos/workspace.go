package dtos

import (
	"strings"
	"time"

	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/google/uuid"
)

// SemanticSearchQuery defines query parameters for cross-meeting semantic vector search.
type SemanticSearchQuery struct {
	Q         string  `form:"q" json:"q"`
	Limit     int     `form:"limit" json:"limit"`
	Threshold float64 `form:"threshold" json:"threshold"`
}

// Validate validates the SemanticSearchQuery fields.
func (q SemanticSearchQuery) Validate() error {
	trimmed := strings.TrimSpace(q.Q)
	if trimmed == "" {
		return validation.Errors{
			"q": validation.NewError("validation_required", "search query is required and cannot be blank"),
		}
	}

	return validation.ValidateStruct(&q,
		validation.Field(&q.Q, validation.Required, validation.Length(1, 1000)),
		validation.Field(&q.Limit, validation.Min(0), validation.Max(50)),
		validation.Field(&q.Threshold, validation.Min(0.0), validation.Max(1.0)),
	)
}

// SearchResultItem represents an individual matched transcript chunk snippet from a meeting.
type SearchResultItem struct {
	RecordingID    uuid.UUID `json:"recording_id"`
	RecordingTitle string    `json:"recording_title"`
	ChunkIndex     int       `json:"chunk_index"`
	Snippet        string    `json:"snippet"`
	StartTime      float64   `json:"start_time"`
	EndTime        float64   `json:"end_time"`
	Score          float64   `json:"score"`
}

// SemanticSearchResponse defines the envelope containing all semantic search results.
type SemanticSearchResponse struct {
	Query   string             `json:"query"`
	Count   int                `json:"count"`
	Results []SearchResultItem `json:"results"`
}

// WorkspaceAskRequest defines the input payload for cross-meeting workspace memory chat.
type WorkspaceAskRequest struct {
	Question string             `json:"question"`
	History  []ChatMessageInput `json:"history,omitempty"`
}

// Validate validates the WorkspaceAskRequest fields.
func (r WorkspaceAskRequest) Validate() error {
	trimmed := strings.TrimSpace(r.Question)
	if trimmed == "" {
		return validation.Errors{
			"question": validation.NewError("validation_required", "question is required and cannot be blank"),
		}
	}

	return validation.ValidateStruct(&r,
		validation.Field(&r.Question, validation.Required, validation.Length(1, 4000)),
	)
}

// MeetingSourceCitation represents a citation reference to a specific meeting and timestamp segment.
type MeetingSourceCitation struct {
	RecordingID    uuid.UUID `json:"recording_id"`
	RecordingTitle string    `json:"recording_title"`
	ChunkIndex     int       `json:"chunk_index"`
	Snippet        string    `json:"snippet"`
	StartTime      float64   `json:"start_time"`
	EndTime        float64   `json:"end_time"`
}

// WorkspaceAskResponse represents the response containing synthesized answer and meeting citations.
type WorkspaceAskResponse struct {
	Answer  string                  `json:"answer"`
	Sources []MeetingSourceCitation `json:"sources"`
}

// SpeakerSummary represents aggregated participation metrics for a speaker across a user's recordings.
type SpeakerSummary struct {
	Name          string    `json:"name"`
	TotalMeetings int       `json:"total_meetings"`
	TotalTalkTime float64   `json:"total_talk_time"`
	LastActive    time.Time `json:"last_active"`
}

// SpeakerDirectoryResponse defines the envelope returned by the workspace speaker directory endpoint.
type SpeakerDirectoryResponse struct {
	Count    int              `json:"count"`
	Speakers []SpeakerSummary `json:"speakers"`
}


