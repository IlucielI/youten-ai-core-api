package strutil_test

import (
	"testing"

	"code-base-golang/internal/pkg/strutil"
)

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		name      string
		title     string
		fallback  string
		maxLength []int
		expected  string
	}{
		{
			name:     "empty title uses fallback",
			title:    "",
			fallback: "fallback_name",
			expected: "fallback_name",
		},
		{
			name:     "whitespace only uses fallback",
			title:    "   \t\n  ",
			fallback: "default",
			expected: "default",
		},
		{
			name:     "clean title with spaces converted to underscores",
			title:    "Quarterly Team Meeting",
			fallback: "recording",
			expected: "Quarterly_Team_Meeting",
		},
		{
			name:     "special characters removed and trimmed",
			title:    "***Project Alpha: Status Update & Next Steps!***",
			fallback: "recording",
			expected: "Project_Alpha_Status_Update_Next_Steps",
		},
		{
			name:      "custom max length limits output",
			title:     "A_Very_Long_Meeting_Title_That_Should_Be_Truncated",
			fallback:  "fallback",
			maxLength: []int{10},
			expected:  "A_Very_Lon",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := strutil.SanitizeFilename(tt.title, tt.fallback, tt.maxLength...)
			if got != tt.expected {
				t.Errorf("SanitizeFilename() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestInferMediaExtension(t *testing.T) {
	tests := []struct {
		contentType string
		expected    string
	}{
		{contentType: "audio/mpeg", expected: ".mp3"},
		{contentType: "audio/mp3; charset=utf-8", expected: ".mp3"},
		{contentType: "audio/wav", expected: ".wav"},
		{contentType: "audio/x-wav", expected: ".wav"},
		{contentType: "audio/wave", expected: ".wav"},
		{contentType: "audio/mp4", expected: ".mp4"},
		{contentType: "video/mp4", expected: ".mp4"},
		{contentType: "audio/m4a", expected: ".m4a"},
		{contentType: "audio/webm", expected: ".webm"},
		{contentType: "video/webm", expected: ".webm"},
		{contentType: "audio/ogg", expected: ".ogg"},
		{contentType: "video/quicktime", expected: ".mov"},
		{contentType: "application/json", expected: ""},
		{contentType: "unknown/type", expected: ""},
		{contentType: "", expected: ""},
	}

	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			got := strutil.InferMediaExtension(tt.contentType)
			if got != tt.expected {
				t.Errorf("InferMediaExtension(%q) = %q, want %q", tt.contentType, got, tt.expected)
			}
		})
	}
}
