package templates

import (
	"bytes"
	"embed"
	"fmt"
	"strings"
	"text/template"

	"code-base-golang/internal/dtos"
)

//go:embed prompts/*.tmpl
var promptFS embed.FS

var parsedTemplates = template.Must(template.ParseFS(promptFS, "prompts/*.tmpl"))

// RAGChunkView wraps dtos.TranscriptChunkData with preformatted timestamp strings for template rendering.
type RAGChunkView struct {
	ChunkIndex     int
	Content        string
	StartFormatted string
	EndFormatted   string
}

// RAGUserData holds input parameters for rag_user.tmpl.
type RAGUserData struct {
	Query  string
	Chunks []RAGChunkView
}

// SummaryUserData holds input parameters for summary_user.tmpl.
type SummaryUserData struct {
	Language       string
	TranscriptBody string
	CustomAngle    string
}

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

// DefaultRAGSystemPrompt returns the static grounded RAG system prompt string.
func DefaultRAGSystemPrompt() (string, error) {
	var buf bytes.Buffer
	if err := parsedTemplates.ExecuteTemplate(&buf, "rag_system.tmpl", nil); err != nil {
		return "", fmt.Errorf("execute rag_system.tmpl: %w", err)
	}
	return strings.TrimSpace(buf.String()), nil
}

// RenderRAGUserPrompt formats the query and retrieved context chunks into the grounded RAG user prompt.
func RenderRAGUserPrompt(query string, chunks []dtos.TranscriptChunkData) (string, error) {
	var chunkViews []RAGChunkView
	for _, c := range chunks {
		chunkViews = append(chunkViews, RAGChunkView{
			ChunkIndex:     c.ChunkIndex,
			Content:        strings.TrimSpace(c.Content),
			StartFormatted: FormatTimestamp(c.StartTime),
			EndFormatted:   FormatTimestamp(c.EndTime),
		})
	}

	data := RAGUserData{
		Query:  strings.TrimSpace(query),
		Chunks: chunkViews,
	}

	var buf bytes.Buffer
	if err := parsedTemplates.ExecuteTemplate(&buf, "rag_user.tmpl", data); err != nil {
		return "", fmt.Errorf("execute rag_user.tmpl: %w", err)
	}
	return strings.TrimSpace(buf.String()), nil
}

// RenderRAGPrompts returns both system and user prompts ready for LLM consumption.
func RenderRAGPrompts(query string, chunks []dtos.TranscriptChunkData) (systemPrompt, userPrompt string, err error) {
	systemPrompt, err = DefaultRAGSystemPrompt()
	if err != nil {
		return "", "", err
	}
	userPrompt, err = RenderRAGUserPrompt(query, chunks)
	if err != nil {
		return "", "", err
	}
	return systemPrompt, userPrompt, nil
}

// DefaultSummarySystemPrompt returns the default executive summarizer system prompt.
func DefaultSummarySystemPrompt() (string, error) {
	var buf bytes.Buffer
	if err := parsedTemplates.ExecuteTemplate(&buf, "summary_system.tmpl", nil); err != nil {
		return "", fmt.Errorf("execute summary_system.tmpl: %w", err)
	}
	return strings.TrimSpace(buf.String()), nil
}

// RenderSummaryUserPrompt renders the user prompt containing the transcript body to be summarized.
// targetLanguage specifies the requested output language (e.g. "Indonesian", "English", "id", "en"), or empty for speaker's primary language.
// customAngle optionally specifies a particular analytical lens or focus area (e.g. "Focus on technical architecture and blockers").
func RenderSummaryUserPrompt(transcriptBody string, targetLanguage string, customAngle ...string) (string, error) {
	var angle string
	if len(customAngle) > 0 {
		angle = strings.TrimSpace(customAngle[0])
	}

	data := SummaryUserData{
		Language:       strings.TrimSpace(targetLanguage),
		TranscriptBody: strings.TrimSpace(transcriptBody),
		CustomAngle:    angle,
	}
	var buf bytes.Buffer
	if err := parsedTemplates.ExecuteTemplate(&buf, "summary_user.tmpl", data); err != nil {
		return "", fmt.Errorf("execute summary_user.tmpl: %w", err)
	}
	return strings.TrimSpace(buf.String()), nil
}
