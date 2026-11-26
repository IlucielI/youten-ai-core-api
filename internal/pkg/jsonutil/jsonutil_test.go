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

func TestRepairJSONAndUnmarshalMapLenient(t *testing.T) {
	tests := []struct {
		name            string
		input           string
		wantTitle       string
		wantNextMeeting string
		wantErr         bool
	}{
		{
			name:            "repairs mismatched array bracket closing object and preserves subsequent fields",
			input:           `{"meeting_title": "Titan QA", "meeting_dynamics": {"consensus_score": "85%", "speaker_talk_time": [{"speaker": "Ki"}], ], "next_meeting": "TBD"}`,
			wantTitle:       "Titan QA",
			wantNextMeeting: "TBD",
		},
		{
			name:      "repairs trailing commas in nested objects and arrays",
			input:     "```json\n{\"meeting_title\": \"Sprint Sync\", \"open_issues\": [\"Bug 1\", ],}\n```",
			wantTitle: "Sprint Sync",
		},
		{
			name:            "preserves brackets inside string literals",
			input:           `{"meeting_title": "Topic [v1] {draft}", "meeting_dynamics": {"consensus_score": "90%"], "next_meeting": "Friday"}`,
			wantTitle:       "Topic [v1] {draft}",
			wantNextMeeting: "Friday",
		},
		{
			name:      "repairs unterminated string ending with trailing backslash",
			input:     `{"meeting_title": "Trailing backslash\`,
			wantTitle: `Trailing backslash\`,
		},
		{
			name:    "unrecoverable garbage returns error",
			input:   `not json at all`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _, err := jsonutil.UnmarshalMapLenient(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalMapLenient() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if got, _ := m["meeting_title"].(string); got != tt.wantTitle {
					t.Errorf("meeting_title = %q, want %q", got, tt.wantTitle)
				}
				if tt.wantNextMeeting != "" {
					if got, _ := m["next_meeting"].(string); got != tt.wantNextMeeting {
						t.Errorf("next_meeting = %q, want %q", got, tt.wantNextMeeting)
					}
				}
			}
		})
	}
}

