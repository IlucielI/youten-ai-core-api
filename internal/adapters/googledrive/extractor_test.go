package googledrive_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"code-base-golang/internal/adapters/googledrive"
	"code-base-golang/internal/dtos"
)

type mockAudioConverter struct {
	available bool
	duration  float64
	err       error
}

func (m *mockAudioConverter) IsAvailable() bool {
	return m.available
}

func (m *mockAudioConverter) ExtractMonoAudio(ctx context.Context, input io.Reader, filename string) (*dtos.AudioExtractionResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	data, _ := io.ReadAll(input)
	return &dtos.AudioExtractionResult{
		Reader:          io.NopCloser(bytes.NewReader(data)),
		Format:          "mp3",
		DurationSeconds: m.duration,
		SizeBytes:       int64(len(data)),
	}, nil
}

func TestExtractor_Supports(t *testing.T) {
	ext := googledrive.NewExtractor()

	tests := []struct {
		url      string
		expected bool
	}{
		{"https://drive.google.com/file/d/1BxiMVs0XRA5nFMdKvBdBZjgmUUqptlbs74OgvE2upms/view", true},
		{"https://drive.google.com/file/d/1a2b3c4d5e6f7g8h9i0j-k_l/view?usp=sharing", true},
		{"https://drive.google.com/open?id=1a2b3c4d5e6f7g8h9i0j-k_l", true},
		{"https://drive.google.com/uc?id=1a2b3c4d5e6f7g8h9i0j-k_l&export=download", true},
		{"https://docs.google.com/file/d/1a2b3c4d5e6f7g8h9i0j-k_l/edit", true},
		{"http://drive.google.com/file/d/1BxiMVs0XRA5nFMdKvBdBZjgmUUqptlbs74OgvE2upms/view", false}, // Insecure HTTP
		{"https://attacker-drive.google.com/file/d/1a2b3c4d5e6f7g8h9i0j/view", false},
		{"https://drive.google.com.attacker.com/file/d/1a2b3c4d5e6f7g8h9i0j/view", false},
		{"https://youtube.com/watch?v=dQw4w9WgXcQ", false},
		{"https://drive.google.com/file/d/short/view", false}, // ID too short (< 10 chars)
		{"invalid-url", false},
		{"", false},
	}

	for _, tc := range tests {
		t.Run(tc.url, func(t *testing.T) {
			got := ext.Supports(tc.url)
			if got != tc.expected {
				t.Errorf("Supports(%q) = %v; want %v", tc.url, got, tc.expected)
			}
		})
	}
}

func TestExtractor_ExtractID(t *testing.T) {
	ext := googledrive.NewExtractor()

	tests := []struct {
		name        string
		url         string
		expectedID  string
		expectError bool
	}{
		{
			name:        "Standard view link",
			url:         "https://drive.google.com/file/d/1BxiMVs0XRA5nFMdKvBdBZjgmUUqptlbs74OgvE2upms/view",
			expectedID:  "1BxiMVs0XRA5nFMdKvBdBZjgmUUqptlbs74OgvE2upms",
			expectError: false,
		},
		{
			name:        "Open link with query ID",
			url:         "https://drive.google.com/open?id=1a2b3c4d5e6f7g8h9i0jklmnopqrst",
			expectedID:  "1a2b3c4d5e6f7g8h9i0jklmnopqrst",
			expectError: false,
		},
		{
			name:        "Docs host link",
			url:         "https://docs.google.com/file/d/9876543210zyxwvutsrqponmlkjihgfedcba/edit",
			expectedID:  "9876543210zyxwvutsrqponmlkjihgfedcba",
			expectError: false,
		},
		{
			name:        "Foreign domain",
			url:         "https://dropbox.com/s/1234567890/file.mp3",
			expectError: true,
		},
		{
			name:        "Missing ID",
			url:         "https://drive.google.com/drive/my-drive",
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			id, err := ext.ExtractID(tc.url)
			if tc.expectError && err == nil {
				t.Errorf("expected error for url %q, got nil", tc.url)
			}
			if !tc.expectError && err != nil {
				t.Errorf("unexpected error for url %q: %v", tc.url, err)
			}
			if id != tc.expectedID {
				t.Errorf("ExtractID(%q) = %q; want %q", tc.url, id, tc.expectedID)
			}
		})
	}
}

func TestExtractor_NormalizeURL(t *testing.T) {
	ext := googledrive.NewExtractor()

	raw := "https://drive.google.com/open?id=1BxiMVs0XRA5nFMdKvBdBZjgmUUqptlbs74OgvE2upms"
	expected := "https://drive.google.com/file/d/1BxiMVs0XRA5nFMdKvBdBZjgmUUqptlbs74OgvE2upms/view"

	got, err := ext.NormalizeURL(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != expected {
		t.Errorf("NormalizeURL(%q) = %q; want %q", raw, got, expected)
	}

	// Invalid URL should fail
	if _, err := ext.NormalizeURL("https://evil.com"); err == nil {
		t.Error("expected error normalizing untrusted URL")
	}
}

func TestExtractor_FetchMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", `attachment; filename="quarterly_meeting.mp3"`)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ext := googledrive.NewExtractorForTest(server.URL, googledrive.Config{
		Transport: server.Client().Transport,
	})

	meta, err := ext.FetchMetadata(context.Background(), "https://drive.google.com/file/d/1BxiMVs0XRA5nFMdKvBdBZjgmUUqptlbs74OgvE2upms/view")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if meta.ID != "1BxiMVs0XRA5nFMdKvBdBZjgmUUqptlbs74OgvE2upms" {
		t.Errorf("expected ID 1BxiMVs0XRA5nFMdKvBdBZjgmUUqptlbs74OgvE2upms, got %s", meta.ID)
	}
	if meta.Title != "quarterly_meeting.mp3" {
		t.Errorf("expected Title 'quarterly_meeting.mp3', got %q", meta.Title)
	}
}

func TestExtractor_ExtractAudio_AudioStreamSuccess(t *testing.T) {
	fakeAudioPayload := "RIFF....WAVEfmt ....datafakeaudio"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Header().Set("Content-Disposition", `attachment; filename="team_sync.mp3"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fakeAudioPayload))
	}))
	defer server.Close()

	ext := googledrive.NewExtractorForTest(server.URL, googledrive.Config{
		Transport:         server.Client().Transport,
		DownloadTimeout:   5 * time.Second,
		MaxAudioSizeBytes: 10 * 1024 * 1024,
	})

	targetURL := "https://drive.google.com/file/d/1BxiMVs0XRA5nFMdKvBdBZjgmUUqptlbs74OgvE2upms/view"
	extracted, err := ext.ExtractAudio(context.Background(), targetURL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer extracted.Close()

	if extracted.Filename != "team_sync.mp3" {
		t.Errorf("expected Filename 'team_sync.mp3', got %q", extracted.Filename)
	}
	if extracted.Title != "team_sync" {
		t.Errorf("expected Title 'team_sync', got %q", extracted.Title)
	}
	if extracted.SizeBytes != int64(len(fakeAudioPayload)) {
		t.Errorf("expected SizeBytes %d, got %d", len(fakeAudioPayload), extracted.SizeBytes)
	}

	readBytes, err := io.ReadAll(extracted)
	if err != nil {
		t.Fatalf("failed to read extracted stream: %v", err)
	}
	if string(readBytes) != fakeAudioPayload {
		t.Errorf("expected payload %q, got %q", fakeAudioPayload, string(readBytes))
	}
}

func TestExtractor_ExtractAudio_VirusWarningBypass(t *testing.T) {
	fakeAudioPayload := "audio-bytes-after-bypass"
	attempt := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt++
		if attempt == 1 {
			// First request: Google Drive returns HTML virus scan warning with confirm token
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`
				<html>
				<body>
				<a href="/uc?export=download&confirm=t_code123&id=1BxiMVs0XRA5nFMdKvBdBZjgmUUqptlbs74OgvE2upms">Download anyway</a>
				</body>
				</html>
			`))
			return
		}

		// Second request: confirmed download returns binary audio
		if r.URL.Query().Get("confirm") != "t_code123" {
			http.Error(w, "missing or bad confirm token", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Header().Set("Content-Disposition", `attachment; filename="large_podcast.mp3"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fakeAudioPayload))
	}))
	defer server.Close()

	ext := googledrive.NewExtractorForTest(server.URL, googledrive.Config{
		Transport: server.Client().Transport,
	})

	targetURL := "https://drive.google.com/file/d/1BxiMVs0XRA5nFMdKvBdBZjgmUUqptlbs74OgvE2upms/view"
	extracted, err := ext.ExtractAudio(context.Background(), targetURL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer extracted.Close()

	if extracted.Filename != "large_podcast.mp3" {
		t.Errorf("expected Filename 'large_podcast.mp3', got %q", extracted.Filename)
	}

	data, _ := io.ReadAll(extracted)
	if string(data) != fakeAudioPayload {
		t.Errorf("expected payload %q, got %q", fakeAudioPayload, string(data))
	}
}

func TestExtractor_ExtractAudio_VideoConversion(t *testing.T) {
	fakeVideoPayload := "fake-mp4-video-stream-content"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Disposition", `attachment; filename="lecture_video.mp4"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fakeVideoPayload))
	}))
	defer server.Close()

	mockConv := &mockAudioConverter{
		available: true,
		duration:  42.5,
	}

	ext := googledrive.NewExtractorForTest(server.URL, googledrive.Config{
		Transport:      server.Client().Transport,
		AudioExtractor: mockConv,
	})

	targetURL := "https://drive.google.com/file/d/1BxiMVs0XRA5nFMdKvBdBZjgmUUqptlbs74OgvE2upms/view"
	extracted, err := ext.ExtractAudio(context.Background(), targetURL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer extracted.Close()

	if extracted.Filename != "lecture_video.mp3" {
		t.Errorf("expected converted filename 'lecture_video.mp3', got %q", extracted.Filename)
	}
	if extracted.DurationSeconds != 42.5 {
		t.Errorf("expected duration 42.5, got %v", extracted.DurationSeconds)
	}
}

func TestExtractor_ExtractAudio_SizeExceeded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("12345678901234567890")) // 20 bytes
	}))
	defer server.Close()

	ext := googledrive.NewExtractorForTest(server.URL, googledrive.Config{
		Transport:         server.Client().Transport,
		MaxAudioSizeBytes: 10, // only 10 bytes allowed
	})

	targetURL := "https://drive.google.com/file/d/1BxiMVs0XRA5nFMdKvBdBZjgmUUqptlbs74OgvE2upms/view"
	_, err := ext.ExtractAudio(context.Background(), targetURL)
	if err == nil {
		t.Fatal("expected size limit error, got nil")
	}
	if !strings.Contains(err.Error(), "exceeds maximum allowed size") {
		t.Errorf("expected size exceeded error, got: %v", err)
	}
}
