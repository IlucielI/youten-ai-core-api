package dtos

import (
	"time"

	"github.com/google/uuid"
)

// FeedbackRequest defines the payload for submitting user feedback, bug reports, or suggestions.
type FeedbackRequest struct {
	Title          string                 `json:"title"`
	Category       string                 `json:"category"`
	Rating         int                    `json:"rating"`
	Message        string                 `json:"message"`
	UserName       *string                `json:"user_name,omitempty"`
	UserEmail      *string                `json:"user_email,omitempty"`
	IdempotencyKey *string                `json:"idempotency_key,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// SetDefaults sets sensible fallback defaults for feedback request fields.
func (r *FeedbackRequest) SetDefaults() {
	if r.Category == "" {
		r.Category = "general"
	}
	if r.Rating <= 0 {
		r.Rating = 5
	}
}

// FeedbackResponse represents the returned feedback confirmation data.
type FeedbackResponse struct {
	ID             uuid.UUID              `json:"id"`
	Title          string                 `json:"title"`
	Category       string                 `json:"category"`
	Rating         int                    `json:"rating"`
	Message        string                 `json:"message"`
	Status         string                 `json:"status"`
	UserName       *string                `json:"user_name,omitempty"`
	UserEmail      *string                `json:"user_email,omitempty"`
	IdempotencyKey *string                `json:"idempotency_key,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
}
