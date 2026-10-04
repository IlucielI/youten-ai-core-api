package services

import (
	"fmt"
	"regexp"
	"strings"
)

var citationRegex = regexp.MustCompile(`\[(\d{1,2}:\d{2}(?::\d{2})?)\]`)

// FormatTimestamp converts seconds into a human-readable [MM:SS] or [HH:MM:SS] string.
func FormatTimestamp(seconds float64) string {
	if seconds < 0 {
		seconds = 0
	}
	totalSec := int(seconds)
	hours := totalSec / 3600
	minutes := (totalSec % 3600) / 60
	secs := totalSec % 60

	if hours > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, secs)
	}
	return fmt.Sprintf("%02d:%02d", minutes, secs)
}

// BuildRAGPrompt constructs system and user prompts for grounded Q&A over transcript chunks.
func BuildRAGPrompt(query string, chunks []TranscriptChunkData) (systemPrompt string, userPrompt string) {
	systemPrompt = `You are a helpful, accurate AI assistant for audio and video recording transcripts.
Your task is to answer the user's question using ONLY the provided transcript context chunks below.

Strict Grounding Rules:
1. Cite the exact timestamp range or start time for every claim using the format [MM:SS] (or [HH:MM:SS] for recordings over an hour).
2. If the context does not contain enough information to answer the question, state clearly: "I cannot find information about this in the recording transcript."
3. Do NOT extrapolate, hallucinate, or assume facts not explicitly mentioned in the context.
4. Keep answers concise, factual, and well-structured.`

	var sb strings.Builder
	sb.WriteString("Transcript Context:\n")
	if len(chunks) == 0 {
		sb.WriteString("(No transcript context available)\n")
	} else {
		for _, c := range chunks {
			startStr := FormatTimestamp(c.StartTime)
			endStr := FormatTimestamp(c.EndTime)
			sb.WriteString(fmt.Sprintf("\n--- Context [%s - %s] (Chunk #%d) ---\n", startStr, endStr, c.ChunkIndex))
			sb.WriteString(strings.TrimSpace(c.Content))
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\nQuestion: ")
	sb.WriteString(strings.TrimSpace(query))
	sb.WriteString("\nAnswer:")

	userPrompt = sb.String()
	return systemPrompt, userPrompt
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
