package services

import (
	"context"
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

// LLMProvider defines the port for OpenAI-compatible text generation, structured JSON extraction, and streaming chat.
type LLMProvider interface {
	// GenerateStructured generates a response guaranteed to conform to the requested JSON schema.
	GenerateStructured(ctx context.Context, systemPrompt string, userPrompt string, schema map[string]interface{}) (*StructuredResponse, error)

	// GenerateChatResponse performs a non-streaming conversational completion.
	GenerateChatResponse(ctx context.Context, systemPrompt string, messages []ChatMessageInput, opts ChatOptions) (*ChatResponse, error)

	// StreamChatResponse streams tokens via a channel for typewriter effect (SSE).
	StreamChatResponse(ctx context.Context, systemPrompt string, messages []ChatMessageInput, opts ChatOptions) (<-chan StreamChunk, error)
}
