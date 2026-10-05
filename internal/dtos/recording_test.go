package dtos_test

import (
	"testing"
	"time"

	"code-base-golang/internal/dtos"
)

func TestRecordingDTO_IsValidMediaMIME(t *testing.T) {
	testCases := []struct {
		name        string
		contentType string
		filename    string
		expected    bool
	}{
		{"mp3 audio", "audio/mpeg", "recording.mp3", true},
		{"mp3 audio alt", "audio/mp3", "recording.mp3", true},
		{"wav audio", "audio/wav", "meeting.wav", true},
		{"x-wav audio", "audio/x-wav", "meeting.wav", true},
		{"wave audio", "audio/wave", "meeting.wav", true},
		{"mp4 audio", "audio/mp4", "audio.mp4", true},
		{"mp4 video", "video/mp4", "video.mp4", true},
		{"m4a audio", "audio/m4a", "voice.m4a", true},
		{"x-m4a audio", "audio/x-m4a", "voice.m4a", true},
		{"webm audio", "audio/webm", "session.webm", true},
		{"webm video", "video/webm", "session.webm", true},
		{"ogg audio", "audio/ogg", "music.ogg", true},
		{"mov video", "video/quicktime", "clip.mov", true},
		{"with charset parameter", "audio/mpeg; charset=binary", "song.mp3", true},
		{"octet-stream with valid extension", "application/octet-stream", "raw.mp3", true},
		{"empty content-type with valid extension", "", "fallback.wav", true},
		{"octet-stream with invalid extension", "application/octet-stream", "doc.pdf", false},
		{"invalid image png", "image/png", "photo.png", false},
		{"invalid text plain", "text/plain", "notes.txt", false},
		{"invalid executable", "application/x-msdownload", "app.exe", false},
		{"spoofed MIME with exe extension", "audio/mpeg", "malware.exe", false},
		{"spoofed MIME with pdf extension", "video/mp4", "document.pdf", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := dtos.IsValidMediaMIME(tc.contentType, tc.filename)
			if result != tc.expected {
				t.Errorf("expected IsValidMediaMIME(%q, %q) = %v, got %v", tc.contentType, tc.filename, tc.expected, result)
			}
		})
	}
}

func TestRecordingDTO_PresignUploadRequest_Validate(t *testing.T) {
	req := dtos.PresignUploadRequest{
		Filename: "meeting.mp4",
	}
	if err := req.Validate(); err != nil {
		t.Errorf("expected valid presign request, got %v", err)
	}

	missingFilename := dtos.PresignUploadRequest{}
	if err := missingFilename.Validate(); err == nil {
		t.Error("expected error for missing filename, got nil")
	}

	longFilename := dtos.PresignUploadRequest{
		Filename: string(make([]byte, 300)),
	}
	if err := longFilename.Validate(); err == nil {
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
	if err := req.Validate(); err != nil {
		t.Errorf("expected valid request, got %v", err)
	}

	missingFilenameReq := dtos.UploadRecordingRequest{
		Title: "No file",
	}
	if err := missingFilenameReq.Validate(); err == nil {
		t.Error("expected error for missing filename, got nil")
	}

	longTitleReq := dtos.UploadRecordingRequest{
		Filename: "recording.mp3",
		Title:    string(make([]byte, 300)),
	}
	if err := longTitleReq.Validate(); err == nil {
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
	if err := validReq.Validate(); err != nil {
		t.Errorf("expected valid request, got %v", err)
	}

	missingURL := dtos.ImportURLRequest{
		Title: "No URL",
	}
	if err := missingURL.Validate(); err == nil {
		t.Error("expected error for missing URL, got nil")
	}

	invalidURL := dtos.ImportURLRequest{
		URL: "not-a-valid-url",
	}
	if err := invalidURL.Validate(); err == nil {
		t.Error("expected error for invalid URL format, got nil")
	}

	longTitle := dtos.ImportURLRequest{
		URL:   "https://example.com/stream.wav",
		Title: string(make([]byte, 300)),
	}
	if err := longTitle.Validate(); err == nil {
		t.Error("expected error for title exceeding 255 chars, got nil")
	}
}

func TestRecordingDTO_RecordingDetailResponse_Structure(t *testing.T) {
	playbackURL := "https://s3.example.com/presigned-url"
	activeSummary := &dtos.SummaryDTO{
		ID:               "sum-1",
		TemplateCategory: "MOM",
		Version:          1,
		IsActive:         true,
		MarkdownContent:  "## Summary Content",
	}

	detail := dtos.RecordingDetailResponse{
		ID:               "rec-123",
		Title:            "Executive Sync",
		OriginalFilename: "sync.mp3",
		FileSizeBytes:    1024,
		DurationSeconds:  120.5,
		PlaybackURL:      &playbackURL,
		Status:           "COMPLETED",
		SelectedTemplate: "MOM",
		OutputLanguage:   "en",
		IsGuest:          false,
		Segments: []dtos.TranscriptSegmentDTO{
			{
				ID:           "seg-1",
				SpeakerLabel: "Speaker 0",
				SpeakerName:  "Alice",
				StartTime:    0.0,
				EndTime:      5.5,
				Text:         "Good morning everyone",
			},
		},
		ActiveSummary: activeSummary,
		Chapters: []dtos.ChapterDTO{
			{
				ID:        "chap-1",
				Title:     "Opening remarks",
				StartTime: 0.0,
				EndTime:   60.0,
				Summary:   "Introductions and welcome",
			},
		},
		Highlights: []dtos.HighlightDTO{
			{
				ID:        "hl-1",
				StartTime: 10.0,
				EndTime:   15.0,
				Source:    "manual",
			},
		},
	}

	if detail.ID != "rec-123" {
		t.Errorf("expected ID rec-123, got %s", detail.ID)
	}
	if len(detail.Segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(detail.Segments))
	}
	if detail.ActiveSummary == nil || detail.ActiveSummary.TemplateCategory != "MOM" {
		t.Error("expected active summary with MOM template")
	}
	if len(detail.Chapters) != 1 {
		t.Errorf("expected 1 chapter, got %d", len(detail.Chapters))
	}
	if len(detail.Highlights) != 1 {
		t.Errorf("expected 1 highlight, got %d", len(detail.Highlights))
	}
}

func TestRecordingDTO_RecordingFilterQuery_SetDefaults(t *testing.T) {
	// Zero values should be populated with defaults
	q := dtos.RecordingFilterQuery{}
	q.SetDefaults()

	if q.Page != 1 {
		t.Errorf("expected default page 1, got %d", q.Page)
	}
	if q.Limit != 10 {
		t.Errorf("expected default limit 10, got %d", q.Limit)
	}
	if q.SortBy != "created_at" {
		t.Errorf("expected default sort_by created_at, got %s", q.SortBy)
	}
	if q.SortOrder != "desc" {
		t.Errorf("expected default sort_order desc, got %s", q.SortOrder)
	}

	// Custom valid values should be preserved
	qCustom := dtos.RecordingFilterQuery{
		Page:      3,
		Limit:     25,
		SortBy:    "title",
		SortOrder: "asc",
	}
	qCustom.SetDefaults()

	if qCustom.Page != 3 || qCustom.Limit != 25 || qCustom.SortBy != "title" || qCustom.SortOrder != "asc" {
		t.Errorf("unexpected customized filter values: %+v", qCustom)
	}

	// Limit capped at 100
	qOver := dtos.RecordingFilterQuery{
		Limit: 500,
	}
	qOver.SetDefaults()
	if qOver.Limit != 100 {
		t.Errorf("expected limit capped at 100, got %d", qOver.Limit)
	}
}

func TestRecordingDTO_RecordingListResponse_Structure(t *testing.T) {
	resp := dtos.RecordingListResponse{
		Items: []dtos.RecordingListItem{
			{
				ID:               "rec-1",
				Title:            "Sprint Review",
				OriginalFilename: "review.mp3",
				FileSizeBytes:    5000,
				DurationSeconds:  60.0,
				Status:           "COMPLETED",
				SelectedTemplate: "MOM",
				OutputLanguage:   "en",
			},
		},
		Pagination: dtos.PaginationMeta{
			CurrentPage: 1,
			PageSize:    10,
			TotalItems:  1,
			TotalPages:  1,
		},
	}

	if len(resp.Items) != 1 || resp.Items[0].Title != "Sprint Review" {
		t.Errorf("unexpected list response items: %+v", resp.Items)
	}
	if resp.Pagination.TotalItems != 1 {
		t.Errorf("expected total items 1, got %d", resp.Pagination.TotalItems)
	}
}

func TestRecordingDTO_ClaimRecordingRequest_Validation(t *testing.T) {
	validReq := dtos.ClaimRecordingRequest{
		OwnershipToken: "secret-token-123",
	}
	if err := validReq.Validate(); err != nil {
		t.Errorf("expected valid claim request, got %v", err)
	}

	missingToken := dtos.ClaimRecordingRequest{
		OwnershipToken: "",
	}
	if err := missingToken.Validate(); err == nil {
		t.Error("expected error for missing ownership_token, got nil")
	}

	tooLongToken := dtos.ClaimRecordingRequest{
		OwnershipToken: string(make([]byte, 256)),
	}
	if err := tooLongToken.Validate(); err == nil {
		t.Error("expected error for ownership_token exceeding 255 chars, got nil")
	}
}

func TestRecordingDTO_BulkClaimRequest_Validation(t *testing.T) {
	validReq := dtos.BulkClaimRequest{
		Tokens: []string{"tok-1", "tok-2"},
	}
	if err := validReq.Validate(); err != nil {
		t.Errorf("expected valid bulk claim request, got %v", err)
	}

	emptyTokens := dtos.BulkClaimRequest{
		Tokens: []string{},
	}
	if err := emptyTokens.Validate(); err == nil {
		t.Error("expected error for empty tokens, got nil")
	}

	tooManyTokens := dtos.BulkClaimRequest{
		Tokens: make([]string, 101),
	}
	if err := tooManyTokens.Validate(); err == nil {
		t.Error("expected error for tokens exceeding 100 items, got nil")
	}
}

func TestRecordingDTO_BulkClaimResponse_Structure(t *testing.T) {
	resp := dtos.BulkClaimResponse{
		ClaimedCount: 2,
		RecordingIDs: []string{"00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000002"},
	}
	if resp.ClaimedCount != 2 {
		t.Errorf("expected ClaimedCount 2, got %d", resp.ClaimedCount)
	}
	if len(resp.RecordingIDs) != 2 {
		t.Errorf("expected 2 recording IDs, got %d", len(resp.RecordingIDs))
	}
}

func TestRecordingDTO_ShareToggleRequest_Validation(t *testing.T) {
	enabled := true
	validReq := dtos.ShareToggleRequest{
		IsShareEnabled: &enabled,
	}
	if err := validReq.Validate(); err != nil {
		t.Errorf("expected valid share toggle request, got %v", err)
	}

	disabled := false
	validReqDisabled := dtos.ShareToggleRequest{
		IsShareEnabled: &disabled,
	}
	if err := validReqDisabled.Validate(); err != nil {
		t.Errorf("expected valid disabled share toggle request, got %v", err)
	}

	nilReq := dtos.ShareToggleRequest{
		IsShareEnabled: nil,
	}
	if err := nilReq.Validate(); err == nil {
		t.Error("expected error for nil IsShareEnabled, got nil")
	}
}

func TestRecordingDTO_ShareToggleResponse_Structure(t *testing.T) {
	tok := "share-123"
	url := "/v1/recordings/shared/share-123"
	resp := dtos.ShareToggleResponse{
		IsShareEnabled: true,
		ShareToken:     &tok,
		ShareURL:       &url,
	}
	if !resp.IsShareEnabled {
		t.Error("expected IsShareEnabled true")
	}
	if resp.ShareToken == nil || *resp.ShareToken != tok {
		t.Errorf("expected ShareToken %s, got %v", tok, resp.ShareToken)
	}
	if resp.ShareURL == nil || *resp.ShareURL != url {
		t.Errorf("expected ShareURL %s, got %v", url, resp.ShareURL)
	}
}

func TestRecordingDTO_SharedRecordingResponse_Structure(t *testing.T) {
	resp := dtos.SharedRecordingResponse{
		ID:               "rec-123",
		Title:            "Shared Title",
		DurationSeconds:  120.5,
		SelectedTemplate: "GENERAL",
		OutputLanguage:   "id",
		Segments:         []dtos.TranscriptSegmentDTO{},
		Chapters:         []dtos.ChapterDTO{},
		Highlights:       []dtos.HighlightDTO{},
	}
	if resp.ID != "rec-123" {
		t.Errorf("expected ID 'rec-123', got %s", resp.ID)
	}
	if resp.Title != "Shared Title" {
		t.Errorf("expected Title 'Shared Title', got %s", resp.Title)
	}
	if resp.DurationSeconds != 120.5 {
		t.Errorf("expected DurationSeconds 120.5, got %f", resp.DurationSeconds)
	}
}

func TestRecordingDTO_RetryRecordingRequest_Structure(t *testing.T) {
	req := dtos.RetryRecordingRequest{
		OwnershipToken: "custom-token",
	}
	if req.OwnershipToken != "custom-token" {
		t.Errorf("expected OwnershipToken 'custom-token', got %s", req.OwnershipToken)
	}
}

func TestRecordingDTO_RetryRecordingResponse_Structure(t *testing.T) {
	now := time.Now()
	resp := dtos.RetryRecordingResponse{
		ID:        "rec-123",
		Status:    "QUEUED",
		Stage:     "EXTRACTING",
		Message:   "pipeline retry initiated successfully",
		UpdatedAt: now,
	}
	if resp.ID != "rec-123" {
		t.Errorf("expected ID 'rec-123', got %s", resp.ID)
	}
	if resp.Status != "QUEUED" {
		t.Errorf("expected Status 'QUEUED', got %s", resp.Status)
	}
	if resp.Stage != "EXTRACTING" {
		t.Errorf("expected Stage 'EXTRACTING', got %s", resp.Stage)
	}
}
