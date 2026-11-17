package dtos

import (
	"github.com/google/uuid"
)

// LLMUsage captures token consumption metrics for an LLM call.
type LLMUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatMessageInput represents an input message in a conversation.
type ChatMessageInput struct {
	Role    string `json:"role"` // "system", "user", "assistant"
	Content string `json:"content"`
}

// ChatOptions contains optional tuning parameters for chat completions.
type ChatOptions struct {
	Temperature *float64 `json:"temperature,omitempty"`
	MaxTokens   *int     `json:"max_tokens,omitempty"`
	TopP        *float64 `json:"top_p,omitempty"`
}

// StructuredResponse holds the structured JSON output and token usage from the LLM.
type StructuredResponse struct {
	RawJSON string   `json:"raw_json"`
	Usage   LLMUsage `json:"usage"`
}

// ChatResponse represents the result of a non-streaming chat completion.
type ChatResponse struct {
	Content      string   `json:"content"`
	Role         string   `json:"role"`
	FinishReason string   `json:"finish_reason"`
	Usage        LLMUsage `json:"usage"`
}

// StreamChunk represents a streamed token or error emitted during SSE generation.
type StreamChunk struct {
	Content      string `json:"content"`
	FinishReason string `json:"finish_reason,omitempty"`
	Err          error  `json:"-"`
}

// RecordingContextMetadata provides rich meeting metadata for grounded AI chat copilot synthesis.
type RecordingContextMetadata struct {
	Title             string   `json:"title,omitempty"`
	DurationSeconds   float64  `json:"duration_seconds,omitempty"`
	DurationFormatted string   `json:"duration_formatted,omitempty"`
	SelectedTemplate  string   `json:"selected_template,omitempty"`
	DetectedLanguage  string   `json:"detected_language,omitempty"`
	OutputLanguage    string   `json:"output_language,omitempty"`
	Speakers          []string `json:"speakers,omitempty"`
	SummaryOverview   string   `json:"summary_overview,omitempty"`
	KeyTopics         []string `json:"key_topics,omitempty"`
	ActionItems       []string `json:"action_items,omitempty"`
}

// RecordingChatRequest represents the request payload for interactive RAG chat over a recording.
type RecordingChatRequest struct {
	SessionID           *uuid.UUID         `json:"session_id,omitempty"`
	Message             string             `json:"message"`
	ConversationHistory []ChatMessageInput `json:"conversation_history,omitempty"`
	OwnershipToken      string             `json:"ownership_token,omitempty"`
	ShareToken          string             `json:"share_token,omitempty"`
	SummaryOverview     string             `json:"summary_overview,omitempty"`
	ActionItems         []string           `json:"action_items,omitempty"`
}
