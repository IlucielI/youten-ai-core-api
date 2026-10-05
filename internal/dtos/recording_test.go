package dtos_test

import (
	"testing"

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


