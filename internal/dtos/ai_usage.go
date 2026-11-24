package dtos

import (
	"time"

	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/go-ozzo/ozzo-validation/v4/is"
	"github.com/google/uuid"
)

// AIUsageRecordInput represents the input payload when logging AI model execution.
type AIUsageRecordInput struct {
	UserID           *uuid.UUID             `json:"user_id,omitempty"`
	WorkspaceID      *uuid.UUID             `json:"workspace_id,omitempty"`
	RecordingID      *uuid.UUID             `json:"recording_id,omitempty"`
	ServiceType      string                 `json:"service_type"`
	Provider         string                 `json:"provider"`
	ModelName        string                 `json:"model_name"`
	PromptTokens     int                    `json:"prompt_tokens"`
	CompletionTokens int                    `json:"completion_tokens"`
	TotalTokens      int                    `json:"total_tokens"`
	AudioSeconds     float64                `json:"audio_seconds"`
	LatencyMs        int                    `json:"latency_ms"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
}

// AdminAIUsageFilter encapsulates query parameters for searching and filtering AI usage logs.
type AdminAIUsageFilter struct {
	UserID      *string `form:"user_id"`
	WorkspaceID *string `form:"workspace_id"`
	RecordingID *string `form:"recording_id"`
	ServiceType string  `form:"service_type"`
	Provider    string  `form:"provider"`
	ModelName   string  `form:"model_name"`
	StartDate   string  `form:"start_date"`
	EndDate     string  `form:"end_date"`
	Page        int     `form:"page"`
	Limit       int     `form:"limit"`
}

// SetDefaults assigns baseline pagination values.
func (f *AdminAIUsageFilter) SetDefaults() {
	if f.Page <= 0 {
		f.Page = 1
	}
	if f.Limit <= 0 {
		f.Limit = 20
	} else if f.Limit > 100 {
		f.Limit = 100
	}
}

// Validate validates the filter inputs.
func (f AdminAIUsageFilter) Validate() error {
	return validation.ValidateStruct(&f,
		validation.Field(&f.UserID, validation.When(f.UserID != nil && *f.UserID != "", is.UUID)),
		validation.Field(&f.WorkspaceID, validation.When(f.WorkspaceID != nil && *f.WorkspaceID != "", is.UUID)),
		validation.Field(&f.RecordingID, validation.When(f.RecordingID != nil && *f.RecordingID != "", is.UUID)),
	)
}

// AdminAIUsageItemDTO represents a single log entry in the admin audit response.
type AdminAIUsageItemDTO struct {
	ID               uuid.UUID              `json:"id"`
	UserID           *uuid.UUID             `json:"user_id,omitempty"`
	UserEmail        *string                `json:"user_email,omitempty"`
	WorkspaceID      *uuid.UUID             `json:"workspace_id,omitempty"`
	RecordingID      *uuid.UUID             `json:"recording_id,omitempty"`
	RecordingTitle   *string                `json:"recording_title,omitempty"`
	ServiceType      string                 `json:"service_type"`
	Provider         string                 `json:"provider"`
	ModelName        string                 `json:"model_name"`
	PromptTokens     int                    `json:"prompt_tokens"`
	CompletionTokens int                    `json:"completion_tokens"`
	TotalTokens      int                    `json:"total_tokens"`
	AudioSeconds     float64                `json:"audio_seconds"`
	LatencyMs        int                    `json:"latency_ms"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt        time.Time              `json:"created_at"`
}

// AIUsageSummaryDTO contains aggregated statistics across matching records.
type AIUsageSummaryDTO struct {
	TotalPromptTokens     int64   `json:"total_prompt_tokens"`
	TotalCompletionTokens int64   `json:"total_completion_tokens"`
	TotalTokens           int64   `json:"total_tokens"`
	TotalAudioSeconds     float64 `json:"total_audio_seconds"`
}

// AdminAIUsageResponse wraps the paginated audit logs with an overall summary.
type AdminAIUsageResponse struct {
	Items      []AdminAIUsageItemDTO `json:"items"`
	Summary    AIUsageSummaryDTO     `json:"summary"`
	Pagination PaginationMeta        `json:"pagination"`
}
