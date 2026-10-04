package services

import (
	"context"

	"code-base-golang/internal/dtos"
)

// LLMProvider defines the port for OpenAI-compatible text generation, structured JSON extraction, and streaming chat.
type LLMProvider interface {
	// GenerateStructured generates a response guaranteed to conform to the requested JSON schema.
	GenerateStructured(ctx context.Context, systemPrompt string, userPrompt string, schema map[string]interface{}) (*dtos.StructuredResponse, error)

	// GenerateChatResponse performs a non-streaming conversational completion.
	GenerateChatResponse(ctx context.Context, systemPrompt string, messages []dtos.ChatMessageInput, opts dtos.ChatOptions) (*dtos.ChatResponse, error)

	// StreamChatResponse streams tokens via a channel for typewriter effect (SSE).
	StreamChatResponse(ctx context.Context, systemPrompt string, messages []dtos.ChatMessageInput, opts dtos.ChatOptions) (<-chan dtos.StreamChunk, error)
}
