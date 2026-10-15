package validations_test

import (
	"strings"
	"testing"

	"code-base-golang/internal/dtos"
	"code-base-golang/internal/validations"
)

func TestValidateFeedbackRequest(t *testing.T) {
	validName := "Alice Bob"
	validEmail := "alice@example.com"
	invalidEmail := "not-an-email"
	validKey := "idem-key-12345"

	tests := []struct {
		name    string
		req     dtos.FeedbackRequest
		wantErr bool
	}{
		{
			name: "valid complete feedback",
			req: dtos.FeedbackRequest{
				Title:          "Export feature failed",
				Category:       "bug",
				Rating:         3,
				Message:        "When clicking export as PDF, the loading spinner never stops.",
				UserName:       &validName,
				UserEmail:      &validEmail,
				IdempotencyKey: &validKey,
			},
			wantErr: false,
		},
		{
			name: "valid minimal feedback with defaults",
			req: dtos.FeedbackRequest{
				Title:   "Great transcription accuracy!",
				Message: "The AI summary was remarkably accurate for my team standup.",
			},
			wantErr: false,
		},
		{
			name: "empty title",
			req: dtos.FeedbackRequest{
				Title:   "",
				Message: "Detailed feedback message with enough length.",
			},
			wantErr: true,
		},
		{
			name: "too short title (< 4 chars)",
			req: dtos.FeedbackRequest{
				Title:   "Bug",
				Message: "Detailed feedback message with enough length.",
			},
			wantErr: true,
		},
		{
			name: "too short message (< 10 chars)",
			req: dtos.FeedbackRequest{
				Title:   "Valid Title",
				Message: "Too short",
			},
			wantErr: true,
		},
		{
			name: "invalid rating (> 5)",
			req: dtos.FeedbackRequest{
				Title:   "Valid Title",
				Message: "Detailed feedback message with enough length.",
				Rating:  6,
			},
			wantErr: true,
		},
		{
			name: "invalid category",
			req: dtos.FeedbackRequest{
				Title:    "Valid Title",
				Category: "unknown_category",
				Message:  "Detailed feedback message with enough length.",
			},
			wantErr: true,
		},
		{
			name: "invalid email format",
			req: dtos.FeedbackRequest{
				Title:     "Valid Title",
				Message:   "Detailed feedback message with enough length.",
				UserEmail: &invalidEmail,
			},
			wantErr: true,
		},
		{
			name: "message exceeding max length",
			req: dtos.FeedbackRequest{
				Title:   "Valid Title",
				Message: strings.Repeat("a", 5001),
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reqCopy := tc.req
			err := validations.ValidateFeedbackRequest(&reqCopy)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ValidateFeedbackRequest() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
