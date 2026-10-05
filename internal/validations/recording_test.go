package validations_test

import (
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/validations"
	"strings"
	"testing"
)

func TestRecordingDTO_PresignUploadRequest_Validate(t *testing.T) {
	req := dtos.PresignUploadRequest{
		Filename: "meeting.mp4",
	}
	if err := validations.ValidatePresignUploadRequest(&req); err != nil {
		t.Errorf("expected valid presign request, got %v", err)
	}

	missingFilename := dtos.PresignUploadRequest{}
	if err := validations.ValidatePresignUploadRequest(&missingFilename); err == nil {
		t.Error("expected error for missing filename, got nil")
	}

	longFilename := dtos.PresignUploadRequest{
		Filename: string(make([]byte, 300)),
	}
	if err := validations.ValidatePresignUploadRequest(&longFilename); err == nil {
		t.Error("expected error for filename exceeding 255 chars, got nil")
	}
}

func TestRecordingDTO_UploadRecordingRequest_Validate(t *testing.T) {
	req := dtos.UploadRecordingRequest{
		Filename:  "recording.mp3",
		ObjectKey: "recordings/123/recording.mp3",
		Title:     "Valid Title",
		Template:  "MOM",
		Language:  "en",
	}
	if err := validations.ValidateUploadRecordingRequest(&req); err != nil {
		t.Errorf("expected valid request, got %v", err)
	}

	missingFilenameReq := dtos.UploadRecordingRequest{
		Title: "No file",
	}
	if err := validations.ValidateUploadRecordingRequest(&missingFilenameReq); err == nil {
		t.Error("expected error for missing filename, got nil")
	}

	longTitleReq := dtos.UploadRecordingRequest{
		Filename: "recording.mp3",
		Title:    string(make([]byte, 300)),
	}
	if err := validations.ValidateUploadRecordingRequest(&longTitleReq); err == nil {
		t.Error("expected error for title exceeding 255 chars, got nil")
	}
}

func TestRecordingDTO_ImportURLRequest_Validate(t *testing.T) {
	validReq := dtos.ImportURLRequest{
		URL:      "https://example.com/audio.mp3",
		Title:    "Podcast Episode",
		Template: "GENERAL",
		Language: "id",
	}
	if err := validations.ValidateImportURLRequest(&validReq); err != nil {
		t.Errorf("expected valid request, got %v", err)
	}

	missingURL := dtos.ImportURLRequest{
		Title: "No URL",
	}
	if err := validations.ValidateImportURLRequest(&missingURL); err == nil {
		t.Error("expected error for missing URL, got nil")
	}

	invalidURL := dtos.ImportURLRequest{
		URL: "not-a-valid-url",
	}
	if err := validations.ValidateImportURLRequest(&invalidURL); err == nil {
		t.Error("expected error for invalid URL format, got nil")
	}

	longTitle := dtos.ImportURLRequest{
		URL:   "https://example.com/stream.wav",
		Title: string(make([]byte, 300)),
	}
	if err := validations.ValidateImportURLRequest(&longTitle); err == nil {
		t.Error("expected error for title exceeding 255 chars, got nil")
	}
}

func TestRecordingDTO_ClaimRecordingRequest_Validation(t *testing.T) {
	validReq := dtos.ClaimRecordingRequest{
		OwnershipToken: "secret-token-123",
	}
	if err := validations.ValidateClaimRecordingRequest(&validReq); err != nil {
		t.Errorf("expected valid claim request, got %v", err)
	}

	missingToken := dtos.ClaimRecordingRequest{
		OwnershipToken: "",
	}
	if err := validations.ValidateClaimRecordingRequest(&missingToken); err == nil {
		t.Error("expected error for missing ownership_token, got nil")
	}

	tooLongToken := dtos.ClaimRecordingRequest{
		OwnershipToken: string(make([]byte, 256)),
	}
	if err := validations.ValidateClaimRecordingRequest(&tooLongToken); err == nil {
		t.Error("expected error for ownership_token exceeding 255 chars, got nil")
	}
}

func TestRecordingDTO_BulkClaimRequest_Validation(t *testing.T) {
	validReq := dtos.BulkClaimRequest{
		Tokens: []string{"tok-1", "tok-2"},
	}
	if err := validations.ValidateBulkClaimRequest(&validReq); err != nil {
		t.Errorf("expected valid bulk claim request, got %v", err)
	}

	emptyTokens := dtos.BulkClaimRequest{
		Tokens: []string{},
	}
	if err := validations.ValidateBulkClaimRequest(&emptyTokens); err == nil {
		t.Error("expected error for empty tokens, got nil")
	}

	tooManyTokens := dtos.BulkClaimRequest{
		Tokens: make([]string, 101),
	}
	if err := validations.ValidateBulkClaimRequest(&tooManyTokens); err == nil {
		t.Error("expected error for tokens exceeding 100 items, got nil")
	}

	blankToken := dtos.BulkClaimRequest{
		Tokens: []string{"valid-token", ""},
	}
	if err := validations.ValidateBulkClaimRequest(&blankToken); err == nil {
		t.Error("expected error for empty token in tokens slice, got nil")
	}

	oversizedToken := dtos.BulkClaimRequest{
		Tokens: []string{strings.Repeat("a", 256)},
	}
	if err := validations.ValidateBulkClaimRequest(&oversizedToken); err == nil {
		t.Error("expected error for token exceeding 255 chars, got nil")
	}
}

func TestRecordingDTO_ShareToggleRequest_Validation(t *testing.T) {
	enabled := true
	validReq := dtos.ShareToggleRequest{
		IsShareEnabled: &enabled,
	}
	if err := validations.ValidateShareToggleRequest(&validReq); err != nil {
		t.Errorf("expected valid share toggle request, got %v", err)
	}

	disabled := false
	validReqDisabled := dtos.ShareToggleRequest{
		IsShareEnabled: &disabled,
	}
	if err := validations.ValidateShareToggleRequest(&validReqDisabled); err != nil {
		t.Errorf("expected valid disabled share toggle request, got %v", err)
	}

	nilReq := dtos.ShareToggleRequest{
		IsShareEnabled: nil,
	}
	if err := validations.ValidateShareToggleRequest(&nilReq); err == nil {
		t.Error("expected error for nil IsShareEnabled, got nil")
	}
}

func TestRecordingDTO_UpdateSpeakersRequest_Validation(t *testing.T) {
	tests := []struct {
		name    string
		req     dtos.UpdateSpeakersRequest
		wantErr bool
	}{
		{
			name: "valid request",
			req: dtos.UpdateSpeakersRequest{
				Speakers: map[string]string{
					"SPEAKER_00": "Bayu",
					"SPEAKER_01": "Alice",
				},
			},
			wantErr: false,
		},
		{
			name: "empty speakers map",
			req: dtos.UpdateSpeakersRequest{
				Speakers: map[string]string{},
			},
			wantErr: true,
		},
		{
			name: "nil speakers map",
			req: dtos.UpdateSpeakersRequest{
				Speakers: nil,
			},
			wantErr: true,
		},
		{
			name: "blank speaker label",
			req: dtos.UpdateSpeakersRequest{
				Speakers: map[string]string{
					"   ": "Bayu",
				},
			},
			wantErr: true,
		},
		{
			name: "too long speaker label (>50 chars)",
			req: dtos.UpdateSpeakersRequest{
				Speakers: map[string]string{
					strings.Repeat("s", 51): "Bayu",
				},
			},
			wantErr: true,
		},
		{
			name: "blank speaker name",
			req: dtos.UpdateSpeakersRequest{
				Speakers: map[string]string{
					"SPEAKER_00": "   ",
				},
			},
			wantErr: true,
		},
		{
			name: "too long speaker name (>100 chars)",
			req: dtos.UpdateSpeakersRequest{
				Speakers: map[string]string{
					"SPEAKER_00": strings.Repeat("n", 101),
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validations.ValidateUpdateSpeakersRequest(&tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRecordingDTO_RegenerateSummaryRequest_Validation(t *testing.T) {
	validAngle := "Focus on technical architecture and blockers"
	tooLongAngle := strings.Repeat("a", 2001)
	tooLongCategory := strings.Repeat("c", 101)

	tests := []struct {
		name    string
		req     dtos.RegenerateSummaryRequest
		wantErr bool
	}{
		{
			name: "valid empty request (defaults)",
			req:  dtos.RegenerateSummaryRequest{},
		},
		{
			name: "valid request with template and custom angle",
			req: dtos.RegenerateSummaryRequest{
				TemplateCategory: "MOM",
				CustomAngle:      &validAngle,
				OwnershipToken:   "token-123",
			},
			wantErr: false,
		},
		{
			name: "template category exceeds 100 chars",
			req: dtos.RegenerateSummaryRequest{
				TemplateCategory: tooLongCategory,
			},
			wantErr: true,
		},
		{
			name: "custom angle exceeds 2000 chars",
			req: dtos.RegenerateSummaryRequest{
				CustomAngle: &tooLongAngle,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validations.ValidateRegenerateSummaryRequest(&tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRecordingDTO_CreateCommentRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     dtos.CreateCommentRequest
		wantErr bool
	}{
		{
			name: "valid request",
			req: dtos.CreateCommentRequest{
				TimestampSec: 12.5,
				CommentText:  "Great point on architecture!",
				AuthorName:   "Alice",
			},
			wantErr: false,
		},
		{
			name: "empty comment text",
			req: dtos.CreateCommentRequest{
				TimestampSec: 0,
				CommentText:  "",
				AuthorName:   "Alice",
			},
			wantErr: true,
		},
		{
			name: "negative timestamp",
			req: dtos.CreateCommentRequest{
				TimestampSec: -1.0,
				CommentText:  "Valid comment",
				AuthorName:   "Alice",
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validations.ValidateCreateCommentRequest(&tc.req)
			if (err != nil) != tc.wantErr {
				t.Fatalf("expected error %v, got %v", tc.wantErr, err)
			}
		})
	}
}
