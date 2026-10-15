package validations_test

import (
	"strings"
	"testing"

	"code-base-golang/internal/constants"
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
			name: "valid feedback with attachments",
			req: dtos.FeedbackRequest{
				Title:   "UI Alignment bug in dashboard",
				Message: "The sidebar collides with the header on 13 inch screens.",
				Attachments: []dtos.FeedbackAttachment{
					{
						URL:           "https://s3.example.com/feedbacks/img.png",
						Filename:      "screenshot.png",
						ContentType:   "image/png",
						FileSizeBytes: 1024,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid feedback with invalid attachment missing URL",
			req: dtos.FeedbackRequest{
				Title:   "UI Alignment bug in dashboard",
				Message: "The sidebar collides with the header on 13 inch screens.",
				Attachments: []dtos.FeedbackAttachment{
					{
						URL:      "",
						Filename: "screenshot.png",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid feedback exceeding max attachments",
			req: dtos.FeedbackRequest{
				Title:   "Too many screenshots attached",
				Message: "Here are 6 screenshots of the same issue.",
				Attachments: []dtos.FeedbackAttachment{
					{URL: "https://example.com/1.png", Filename: "1.png"},
					{URL: "https://example.com/2.png", Filename: "2.png"},
					{URL: "https://example.com/3.png", Filename: "3.png"},
					{URL: "https://example.com/4.png", Filename: "4.png"},
					{URL: "https://example.com/5.png", Filename: "5.png"},
					{URL: "https://example.com/6.png", Filename: "6.png"},
				},
			},
			wantErr: true,
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

func TestValidateFeedbackPresignRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     dtos.FeedbackPresignRequest
		wantErr bool
	}{
		{
			name: "valid png image",
			req: dtos.FeedbackPresignRequest{
				Filename:      "screenshot.png",
				ContentType:   "image/png",
				FileSizeBytes: 1024 * 1024,
			},
			wantErr: false,
		},
		{
			name: "valid jpeg image with auto-resolved mime from extension",
			req: dtos.FeedbackPresignRequest{
				Filename: "bug_evidence.jpg",
			},
			wantErr: false,
		},
		{
			name: "valid webp image",
			req: dtos.FeedbackPresignRequest{
				Filename:      "capture.webp",
				ContentType:   "image/webp",
				FileSizeBytes: 500000,
			},
			wantErr: false,
		},
		{
			name: "empty filename",
			req: dtos.FeedbackPresignRequest{
				Filename:    "",
				ContentType: "image/png",
			},
			wantErr: true,
		},
		{
			name: "invalid unsupported format",
			req: dtos.FeedbackPresignRequest{
				Filename:    "malicious.exe",
				ContentType: "application/x-msdownload",
			},
			wantErr: true,
		},
		{
			name: "exceeds max attachment size limit",
			req: dtos.FeedbackPresignRequest{
				Filename:      "giant_screenshot.png",
				ContentType:   "image/png",
				FileSizeBytes: int64(constants.MaxFeedbackAttachmentSizeBytes) + 1,
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reqCopy := tc.req
			err := validations.ValidateFeedbackPresignRequest(&reqCopy)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ValidateFeedbackPresignRequest() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
