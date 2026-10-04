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

	user, err := templates.RenderSummaryUserPrompt("Meeting transcript goes here.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(user, "Meeting transcript goes here.") {
		t.Errorf("unexpected summary user prompt: %s", user)
	}
}
