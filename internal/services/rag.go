package services

import (
	"regexp"

	"code-base-golang/internal/dtos"
	"code-base-golang/internal/templates"
)

var citationRegex = regexp.MustCompile(`\[(\d{1,2}:\d{2}(?::\d{2})?)\]`)

// FormatTimestamp converts seconds into a human-readable [MM:SS] or [HH:MM:SS] string.
func FormatTimestamp(seconds float64) string {
	return templates.FormatTimestamp(seconds)
}

// BuildRAGPrompt constructs system and user prompts for grounded Q&A over transcript chunks using internal/templates.
func BuildRAGPrompt(query string, chunks []dtos.TranscriptChunkData) (systemPrompt string, userPrompt string) {
	sys, user, err := templates.RenderRAGPrompts(query, chunks)
	if err != nil {
		// Fallback in the improbable event of template execution error
		return "You are a helpful, accurate AI assistant. Strict Grounding Rules apply.", "Question: " + query
	}
	return sys, user
}

// ExtractCitations parses all [MM:SS] or [HH:MM:SS] timestamp citations from a generated text.
func ExtractCitations(text string) []string {
	matches := citationRegex.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	var citations []string
	for _, match := range matches {
		if len(match) > 1 {
			citation := match[1]
			if !seen[citation] {
				seen[citation] = true
				citations = append(citations, citation)
			}
		}
	}
	return citations
}
