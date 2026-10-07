package youtube_test

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"code-base-golang/internal/adapters/youtube"
)

func TestYouTube_IsYouTubeURL(t *testing.T) {
	tests := []struct {
		url      string
		expected bool
	}{
		{"https://www.youtube.com/watch?v=dQw4w9WgXcQ", true},
		{"https://youtu.be/dQw4w9WgXcQ", true},
		{"https://m.youtube.com/watch?v=dQw4w9WgXcQ", true},
		{"https://music.youtube.com/watch?v=dQw4w9WgXcQ", true},
		{"https://www.youtube.com/shorts/dQw4w9WgXcQ", true},
		{"https://www.youtube.com/embed/dQw4w9WgXcQ", true},
		{"http://youtube.com/watch?v=dQw4w9WgXcQ", false}, // Insecure http rejected
		{"https://google.com", false},
		{"https://vimeo.com/123456", false},
		{"https://notyoutube.com/watch?v=123", false},
		{"https://attacker.youtube.com.evil.com", false},
		{"https://sub.youtube.com", false},
		{"invalid-url", false},
		{"", false},
	}

	for _, tc := range tests {
		t.Run(tc.url, func(t *testing.T) {
			res := youtube.IsYouTubeURL(tc.url)
			if res != tc.expected {
				t.Errorf("IsYouTubeURL(%q) = %v; want %v", tc.url, res, tc.expected)
			}
		})
	}
}

func TestYouTube_ExtractVideoID(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		expectedID  string
		expectError bool
	}{
		{
			name:        "Standard desktop watch URL",
			url:         "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
			expectedID:  "dQw4w9WgXcQ",
			expectError: false,
		},
		{
			name:        "Desktop watch URL with additional query params",
			url:         "https://www.youtube.com/watch?v=dQw4w9WgXcQ&feature=share&t=42",
			expectedID:  "dQw4w9WgXcQ",
			expectError: false,
		},
		{
			name:        "Shortened youtu.be link",
			url:         "https://youtu.be/dQw4w9WgXcQ",
			expectedID:  "dQw4w9WgXcQ",
			expectError: false,
		},
		{
			name:        "Shortened link with timestamp",
			url:         "https://youtu.be/dQw4w9WgXcQ?t=10",
			expectedID:  "dQw4w9WgXcQ",
			expectError: false,
		},
		{
			name:        "YouTube Shorts URL",
			url:         "https://www.youtube.com/shorts/dQw4w9WgXcQ",
			expectedID:  "dQw4w9WgXcQ",
			expectError: false,
		},
		{
			name:        "Mobile watch URL",
			url:         "https://m.youtube.com/watch?v=dQw4w9WgXcQ",
			expectedID:  "dQw4w9WgXcQ",
			expectError: false,
		},
		{
			name:        "Music YouTube URL",
			url:         "https://music.youtube.com/watch?v=dQw4w9WgXcQ",
			expectedID:  "dQw4w9WgXcQ",
			expectError: false,
		},
		{
			name:        "Embed URL",
			url:         "https://www.youtube.com/embed/dQw4w9WgXcQ",
			expectedID:  "dQw4w9WgXcQ",
			expectError: false,
		},
		{
			name:        "Old style v/ URL",
			url:         "https://www.youtube.com/v/dQw4w9WgXcQ",
			expectedID:  "dQw4w9WgXcQ",
			expectError: false,
		},
		{
			name:        "Insecure HTTP URL rejected",
			url:         "http://www.youtube.com/watch?v=dQw4w9WgXcQ",
			expectedID:  "",
			expectError: true,
		},
		{
			name:        "Non-YouTube URL",
			url:         "https://vimeo.com/12345678901",
			expectedID:  "",
			expectError: true,
		},
		{
			name:        "Invalid video ID length",
			url:         "https://www.youtube.com/watch?v=short",
			expectedID:  "",
			expectError: true,
		},
		{
			name:        "Empty URL",
			url:         "",
			expectedID:  "",
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			id, err := youtube.ExtractVideoID(tc.url)
			if tc.expectError {
				if err == nil {
					t.Errorf("expected error for URL %q, got nil", tc.url)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error for URL %q: %v", tc.url, err)
				}
				if id != tc.expectedID {
					t.Errorf("ExtractVideoID(%q) = %q; want %q", tc.url, id, tc.expectedID)
				}
			}
		})
	}
}

func TestYouTube_NormalizeURL(t *testing.T) {
	inputs := []string{
		"https://youtu.be/dQw4w9WgXcQ",
		"https://m.youtube.com/watch?v=dQw4w9WgXcQ&t=5",
		"https://www.youtube.com/shorts/dQw4w9WgXcQ",
		"https://www.youtube.com/embed/dQw4w9WgXcQ",
	}

	expected := "https://www.youtube.com/watch?v=dQw4w9WgXcQ"

	for _, in := range inputs {
		norm, err := youtube.NormalizeURL(in)
		if err != nil {
			t.Errorf("NormalizeURL(%q) unexpected error: %v", in, err)
		}
		if norm != expected {
			t.Errorf("NormalizeURL(%q) = %q; want %q", in, norm, expected)
		}
	}
}

func TestYouTube_ValidateBinaryPath(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Valid binary
	validBin := filepath.Join(tempDir, "yt-dlp")
	script := "#!/bin/sh\nexit 0\n"
	if err := os.WriteFile(validBin, []byte(script), 0755); err != nil {
		t.Fatalf("failed to create valid binary: %v", err)
	}

	ext, err := youtube.NewExtractorWithConfig(youtube.Config{BinaryPath: validBin})
	if err != nil {
		t.Fatalf("unexpected error creating extractor: %v", err)
	}
	if ext == nil {
		t.Fatal("expected non-nil extractor")
	}

	// 2. Reject binary with invalid name
	wrongName := filepath.Join(tempDir, "ffmpeg")
	_ = os.WriteFile(wrongName, []byte(script), 0755)
	_, err = youtube.NewExtractorWithConfig(youtube.Config{BinaryPath: wrongName})
	if !errors.Is(err, youtube.ErrInvalidBinaryPath) {
		t.Errorf("expected ErrInvalidBinaryPath for wrong name, got: %v", err)
	}

	// 3. Reject non-executable
	nonExec := filepath.Join(tempDir, "sub", "yt-dlp")
	_ = os.MkdirAll(filepath.Dir(nonExec), 0755)
	_ = os.WriteFile(nonExec, []byte(script), 0644)
	_, err = youtube.NewExtractorWithConfig(youtube.Config{BinaryPath: nonExec})
	if !errors.Is(err, youtube.ErrInvalidBinaryPath) {
		t.Errorf("expected ErrInvalidBinaryPath for non-executable, got: %v", err)
	}

	// 4. Reject symlinks
	symlink := filepath.Join(tempDir, "link", "yt-dlp")
	_ = os.MkdirAll(filepath.Dir(symlink), 0755)
	_ = os.Symlink(validBin, symlink)
	_, err = youtube.NewExtractorWithConfig(youtube.Config{BinaryPath: symlink})
	if !errors.Is(err, youtube.ErrInvalidBinaryPath) {
		t.Errorf("expected ErrInvalidBinaryPath for symlink, got: %v", err)
	}

	// 5. Reject world-writable files
	worldWritable := filepath.Join(tempDir, "ww", "yt-dlp")
	_ = os.MkdirAll(filepath.Dir(worldWritable), 0755)
	_ = os.WriteFile(worldWritable, []byte(script), 0755)
	_ = os.Chmod(worldWritable, 0777)
	_, err = youtube.NewExtractorWithConfig(youtube.Config{BinaryPath: worldWritable})
	if !errors.Is(err, youtube.ErrInvalidBinaryPath) {
		t.Errorf("expected ErrInvalidBinaryPath for world-writable file, got: %v", err)
	}
}

func TestYouTube_FetchMetadata_Integration(t *testing.T) {
	tempDir := t.TempDir()
	fakeBin := filepath.Join(tempDir, "yt-dlp")

	script := `#!/bin/sh
if [ "$1" = "-j" ]; then
    echo '{"id":"dQw4w9WgXcQ","title":"Rick Astley - Never Gonna Give You Up","duration":213}'
    exit 0
fi
exit 1
`
	if err := os.WriteFile(fakeBin, []byte(script), 0755); err != nil {
		t.Fatalf("failed to write fake binary: %v", err)
	}

	ext, err := youtube.NewExtractorWithConfig(youtube.Config{BinaryPath: fakeBin})
	if err != nil {
		t.Fatalf("unexpected error creating extractor: %v", err)
	}

	ctx := context.Background()
	meta, err := ext.FetchMetadata(ctx, "https://www.youtube.com/watch?v=dQw4w9WgXcQ")
	if err != nil {
		t.Fatalf("FetchMetadata failed: %v", err)
	}

	if meta.ID != "dQw4w9WgXcQ" {
		t.Errorf("expected ID 'dQw4w9WgXcQ', got %q", meta.ID)
	}
	if meta.Title != "Rick Astley - Never Gonna Give You Up" {
		t.Errorf("expected Title match, got %q", meta.Title)
	}
	if meta.Duration != 213 {
		t.Errorf("expected Duration 213, got %v", meta.Duration)
	}
}

func TestYouTube_ExtractAudio_Integration(t *testing.T) {
	tempDir := t.TempDir()
	fakeBin := filepath.Join(tempDir, "yt-dlp")

	sampleAudio := []byte("audio-sample-data-1234567890")

	// yt-dlp script locates -o template argument to place output files
	script := `#!/bin/sh
outDir=""
while [ $# -gt 0 ]; do
  if [ "$1" = "-o" ]; then
    shift
    outDir=$(dirname "$1")
    break
  fi
  shift
done
if [ -n "$outDir" ]; then
  echo '{"id":"dQw4w9WgXcQ","title":"Test Audio","duration":100}' > "$outDir/audio.info.json"
  echo -n "audio-sample-data-1234567890" > "$outDir/audio.m4a"
fi
exit 0
`
	if err := os.WriteFile(fakeBin, []byte(script), 0755); err != nil {
		t.Fatalf("failed to write fake binary: %v", err)
	}

	jobTempDir := filepath.Join(tempDir, "jobs")
	_ = os.MkdirAll(jobTempDir, 0755)

	ext, err := youtube.NewExtractorWithConfig(youtube.Config{
		BinaryPath: fakeBin,
		TempDir:    jobTempDir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	extracted, err := ext.ExtractAudio(ctx, "https://www.youtube.com/watch?v=dQw4w9WgXcQ")
	if err != nil {
		t.Fatalf("ExtractAudio failed: %v", err)
	}

	if extracted.Title != "Test Audio" {
		t.Errorf("expected title 'Test Audio', got %q", extracted.Title)
	}
	if extracted.ContentType != "audio/m4a" {
		t.Errorf("expected audio/m4a, got %q", extracted.ContentType)
	}
	if extracted.SizeBytes != int64(len(sampleAudio)) {
		t.Errorf("expected SizeBytes %d, got %d", len(sampleAudio), extracted.SizeBytes)
	}

	// Read stream directly
	body, err := io.ReadAll(extracted)
	if err != nil {
		t.Fatalf("failed to read audio: %v", err)
	}
	if string(body) != string(sampleAudio) {
		t.Errorf("content mismatch: got %q, want %q", string(body), string(sampleAudio))
	}

	// Close stream and assert temp dir is removed
	if err := extracted.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}

	entries, err := os.ReadDir(jobTempDir)
	if err != nil {
		t.Fatalf("failed to read job dir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries in temp directory, found %d", len(entries))
	}
}

func TestYouTube_ValidateDomainBelongsToYouTube(t *testing.T) {
	validHosts := []string{
		"https://youtube.com/watch?v=dQw4w9WgXcQ",
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		"https://m.youtube.com/watch?v=dQw4w9WgXcQ",
		"https://youtu.be/dQw4w9WgXcQ",
		"https://music.youtube.com/watch?v=dQw4w9WgXcQ",
	}

	for _, u := range validHosts {
		if !youtube.IsYouTubeURL(u) {
			t.Errorf("expected %q to be recognized as YouTube URL", u)
		}
	}

	invalidHosts := []string{
		"https://google.com/watch?v=dQw4w9WgXcQ",
		"https://fakeyoutube.com/watch?v=dQw4w9WgXcQ",
		"https://youtube.com.attacker.com/watch?v=dQw4w9WgXcQ",
		"https://attacker.youtube.com/watch?v=dQw4w9WgXcQ",
		"https://localhost/watch?v=dQw4w9WgXcQ",
	}

	for _, u := range invalidHosts {
		if youtube.IsYouTubeURL(u) {
			t.Errorf("expected %q NOT to be recognized as YouTube URL", u)
		}
	}
}

func TestYouTube_MediaLinkExtractor_Interface(t *testing.T) {
	tempDir := t.TempDir()
	fakeBin := filepath.Join(tempDir, "yt-dlp")
	if err := os.WriteFile(fakeBin, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatalf("failed to write fake binary: %v", err)
	}

	ext, err := youtube.NewExtractorWithConfig(youtube.Config{
		BinaryPath: fakeBin,
	})
	if err != nil {
		t.Fatalf("unexpected NewExtractor error: %v", err)
	}

	validURL := "https://www.youtube.com/watch?v=dQw4w9WgXcQ"
	if !ext.Supports(validURL) {
		t.Errorf("expected Supports(%q) to be true", validURL)
	}

	invalidURL := "https://example.com/video"
	if ext.Supports(invalidURL) {
		t.Errorf("expected Supports(%q) to be false", invalidURL)
	}

	id, err := ext.ExtractID(validURL)
	if err != nil {
		t.Fatalf("unexpected ExtractID error: %v", err)
	}
	if id != "dQw4w9WgXcQ" {
		t.Errorf("ExtractID(%q) = %q, want dQw4w9WgXcQ", validURL, id)
	}

	_, err = ext.ExtractID(invalidURL)
	if err == nil {
		t.Errorf("expected ExtractID on invalid URL to return error, got nil")
	}
}
