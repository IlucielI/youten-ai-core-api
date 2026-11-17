package services

import (
	"regexp"

	"code-base-golang/internal/dtos"
	"code-base-golang/internal/pkg/timeutil"
	"code-base-golang/internal/templates"
)

var citationRegex = regexp.MustCompile(`\[(\d{1,2}:\d{2}(?::\d{2})?)(?:\s*-\s*(\d{1,2}:\d{2}(?::\d{2})?))?\]`)

// FormatTimestamp converts seconds into a human-readable [MM:SS] or [HH:MM:SS] string.
func FormatTimestamp(seconds float64) string {
	return timeutil.FormatTimestamp(seconds)
}

// BuildRAGPrompt constructs system and user prompts for grounded Q&A over transcript chunks using internal/templates.
func BuildRAGPrompt(query string, chunks []dtos.TranscriptChunkData, meta ...*dtos.RecordingContextMetadata) (systemPrompt string, userPrompt string) {
	sys, user, err := templates.RenderRAGPrompts(query, chunks, meta...)
	if err != nil {
		// Fallback in the improbable event of template execution error
		return "You are an advanced AI assistant. Strict Grounding Rules apply.", "Question: " + query
	}
	return sys, user
}

// ExtractCitations parses all [MM:SS] or [HH:MM:SS] timestamp citations (including range formats) from a generated text.
func ExtractCitations(text string) []string {
	matches := citationRegex.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	var citations []string
	for _, match := range matches {
		if len(match) > 1 && match[1] != "" {
			c1 := match[1]
			if !seen[c1] {
				seen[c1] = true
				citations = append(citations, c1)
			}
		}
		if len(match) > 2 && match[2] != "" {
			c2 := match[2]
			if !seen[c2] {
				seen[c2] = true
				citations = append(citations, c2)
			}
		}
	}
	return citations
}
