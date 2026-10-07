package jsonutil_test

import (
	"testing"

	"code-base-golang/internal/pkg/jsonutil"
)

func TestRawStringSlice(t *testing.T) {
	tests := []struct {
		name     string
		items    []string
		expected string
	}{
		{
			name:     "nil slice returns empty array JSON",
			items:    nil,
			expected: "[]",
		},
		{
			name:     "empty slice returns empty array JSON",
			items:    []string{},
			expected: "[]",
		},
		{
			name:     "single element slice",
			items:    []string{"00:15"},
			expected: `["00:15"]`,
		},
		{
			name:     "multiple elements slice",
			items:    []string{"chunk-1", "chunk-2", "chunk-3"},
			expected: `["chunk-1","chunk-2","chunk-3"]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(jsonutil.RawStringSlice(tt.items))
			if got != tt.expected {
				t.Errorf("RawStringSlice() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestCleanMarkdownJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "clean JSON unchanged",
			input:    `{"summary": "hello"}`,
			expected: `{"summary": "hello"}`,
		},
		{
			name:     "fenced with ```json and whitespace",
			input:    "```json\n{\"summary\": \"hello\"}\n```",
			expected: `{"summary": "hello"}`,
		},
		{
			name:     "fenced with ``` only",
			input:    "```\n{\"summary\": \"hello\"}\n```",
			expected: `{"summary": "hello"}`,
		},
		{
			name:     "surrounding explanatory text",
			input:    "Here is the summary:\n{\"summary\": \"hello\"}\nHope this helps!",
			expected: `{"summary": "hello"}`,
		},
		{
			name:     "JSON array",
			input:    "```json\n[\"item1\", \"item2\"]\n```",
			expected: `["item1", "item2"]`,
		},
		{
			name:     "malformed input stays as-is for validation error",
			input:    `{malformed json`,
			expected: `{malformed json`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := jsonutil.CleanMarkdownJSON(tt.input)
			if got != tt.expected {
				t.Errorf("CleanMarkdownJSON() = %q, want %q", got, tt.expected)
			}
		})
	}
}

