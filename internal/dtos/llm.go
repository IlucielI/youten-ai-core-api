package dtos

import (
	"strings"

	validation "github.com/go-ozzo/ozzo-validation/v4"
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

// RecordingChatRequest represents the request payload for interactive RAG chat over a recording.
type RecordingChatRequest struct {
	Message             string             `json:"message"`
	ConversationHistory []ChatMessageInput `json:"conversation_history,omitempty"`
	OwnershipToken      string             `json:"ownership_token,omitempty"`
}

// Validate checks request constraints for RecordingChatRequest.
func (r RecordingChatRequest) Validate() error {
	return validation.ValidateStruct(&r,
		validation.Field(&r.Message,
			validation.Required,
			validation.Length(1, 4000),
			validation.By(func(value interface{}) error {
				s, ok := value.(string)
				if !ok || strings.TrimSpace(s) == "" {
					return validation.NewError("validation_required", "message cannot be empty or blank")
				}
				return nil
			}),
		),
	)
}

