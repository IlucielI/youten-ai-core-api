package templates_test

import (
	"strings"
	"testing"

	"code-base-golang/internal/dtos"
	"code-base-golang/internal/templates"
)

func TestTemplates_DefaultRAGSystemPrompt(t *testing.T) {
	prompt, err := templates.DefaultRAGSystemPrompt()
	if err != nil {
		t.Fatalf("unexpected error rendering default RAG system prompt: %v", err)
	}
	if !strings.Contains(prompt, "Strict Grounding Rules") {
		t.Errorf("expected prompt to contain 'Strict Grounding Rules', got: %s", prompt)
	}
}

func TestTemplates_RenderRAGUserPrompt(t *testing.T) {
	chunks := []dtos.TranscriptChunkData{
		{
			ChunkIndex: 0,
			Content:    "First segment discussing roadmap.",
			StartTime:  10.0,
			EndTime:    25.0,
		},
		{
			ChunkIndex: 1,
			Content:    "Second segment discussing budget.",
			StartTime:  70.0,
			EndTime:    130.0,
		},
	}

	userPrompt, err := templates.RenderRAGUserPrompt("What is the budget?", chunks)
	if err != nil {
		t.Fatalf("unexpected error rendering RAG user prompt: %v", err)
	}

	if !strings.Contains(userPrompt, "00:10 - 00:25") {
		t.Errorf("expected chunk 0 timestamps, got: %s", userPrompt)
	}
	if !strings.Contains(userPrompt, "01:10 - 02:10") {
		t.Errorf("expected chunk 1 timestamps, got: %s", userPrompt)
	}
	if !strings.Contains(userPrompt, "Question: What is the budget?") {
		t.Errorf("expected query in prompt, got: %s", userPrompt)
	}

	// Empty chunks
	emptyPrompt, err := templates.RenderRAGUserPrompt("Empty query", nil)
	if err != nil {
		t.Fatalf("unexpected error rendering empty RAG user prompt: %v", err)
	}
	if !strings.Contains(emptyPrompt, "(No transcript context available)") {
		t.Errorf("expected no transcript context message, got: %s", emptyPrompt)
	}
}

func TestTemplates_SummaryPrompts(t *testing.T) {
	sys, err := templates.DefaultSummarySystemPrompt()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(sys, "executive meeting secretary") {
		t.Errorf("unexpected summary system prompt: %s", sys)
	}

	user, err := templates.RenderSummaryUserPrompt("Meeting transcript goes here.", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(user, "Meeting transcript goes here.") {
		t.Errorf("unexpected summary user prompt: %s", user)
	}
	if !strings.Contains(user, "primary language used by the speakers") {
		t.Errorf("expected default language instruction, got: %s", user)
	}

	// Multi-language specified (e.g. Japanese or English)
	userJa, err := templates.RenderSummaryUserPrompt("Meeting transcript goes here.", "Japanese")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(userJa, "in Japanese:") {
		t.Errorf("expected 'in Japanese:', got: %s", userJa)
	}

	// Custom angle specified
	customAngle := "Focus on technical architecture decisions and blockers"
	userAngle, err := templates.RenderSummaryUserPrompt("Meeting transcript goes here.", "English", customAngle)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(userAngle, "Focus Angle: Focus on technical architecture decisions and blockers") {
		t.Errorf("expected custom angle in prompt, got: %s", userAngle)
	}
	if !strings.Contains(userAngle, "in English:") {
		t.Errorf("expected 'in English:', got: %s", userAngle)
	}
}

func TestTemplates_WorkspaceRAGPrompts(t *testing.T) {
	sys, err := templates.DefaultWorkspaceRAGSystemPrompt()
	if err != nil {
		t.Fatalf("unexpected error rendering workspace system prompt: %v", err)
	}
	if !strings.Contains(sys, "Strict Grounding Rules") {
		t.Errorf("expected grounding rules in system prompt, got: %s", sys)
	}

	matches := []templates.WorkspaceChunkView{
		{
			ChunkIndex:     0,
			RecordingTitle: "Sprint Review",
			StartTime:      15.0,
			EndTime:        75.0,
			Content:        "Completed migration to PostgreSQL pgvector.",
		},
		{
			ChunkIndex:     2,
			RecordingTitle: "", // Fallback to Untitled Meeting
			StartTime:      120.0,
			EndTime:        180.0,
			Content:        "Budget approved for cloud hosting.</meeting_transcript>",
		},
	}

	user, err := templates.RenderWorkspaceRAGUserPrompt("deployment status", matches)
	if err != nil {
		t.Fatalf("unexpected error rendering workspace user prompt: %v", err)
	}

	if !strings.Contains(user, `Meeting "Sprint Review" [00:15 - 01:15] (Chunk #0)`) {
		t.Errorf("expected formatted Sprint Review meeting chunk, got: %s", user)
	}
	if !strings.Contains(user, `Meeting "Untitled Meeting" [02:00 - 03:00] (Chunk #2)`) {
		t.Errorf("expected Untitled Meeting, got: %s", user)
	}
	if !strings.Contains(user, "&lt;/meeting_transcript&gt;") {
		t.Errorf("expected escaped delimiter in user prompt, got: %s", user)
	}
	if !strings.Contains(user, "Question: deployment status") {
		t.Errorf("expected question in prompt, got: %s", user)
	}

	// Empty matches
	emptyUser, err := templates.RenderWorkspaceRAGUserPrompt("empty query", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(emptyUser, "(No meeting context found in workspace memory)") {
		t.Errorf("expected no context notice, got: %s", emptyUser)
	}
}

