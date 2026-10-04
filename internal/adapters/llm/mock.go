package llm

import (
	"context"

	"code-base-golang/internal/services"
)

// MockLLM provides a mockable implementation of services.LLMProvider for unit and offline testing.
type MockLLM struct {
	GenerateStructuredFunc   func(ctx context.Context, systemPrompt string, userPrompt string, schema map[string]interface{}) (*services.StructuredResponse, error)
	GenerateChatResponseFunc func(ctx context.Context, systemPrompt string, messages []services.ChatMessageInput, opts services.ChatOptions) (*services.ChatResponse, error)
	StreamChatResponseFunc   func(ctx context.Context, systemPrompt string, messages []services.ChatMessageInput, opts services.ChatOptions) (<-chan services.StreamChunk, error)
}

// NewMock creates a new MockLLM instance.
func NewMock() *MockLLM {
	return &MockLLM{}
}

// GenerateStructured executes GenerateStructuredFunc or returns a canned JSON summary.
func (m *MockLLM) GenerateStructured(ctx context.Context, systemPrompt string, userPrompt string, schema map[string]interface{}) (*services.StructuredResponse, error) {
	if m.GenerateStructuredFunc != nil {
		return m.GenerateStructuredFunc(ctx, systemPrompt, userPrompt, schema)
	}

	return &services.StructuredResponse{
		RawJSON: `{"title": "Sprint Planning Recap", "overview": "Team reviewed sprint commitments and milestones.", "action_items": [{"task": "Deploy vector db", "owner": "Bayu"}]}`,
		Usage: services.LLMUsage{
			PromptTokens:     120,
			CompletionTokens: 65,
			TotalTokens:      185,
		},
	}, nil
}

// GenerateChatResponse executes GenerateChatResponseFunc or returns a canned conversational answer.
func (m *MockLLM) GenerateChatResponse(ctx context.Context, systemPrompt string, messages []services.ChatMessageInput, opts services.ChatOptions) (*services.ChatResponse, error) {
	if m.GenerateChatResponseFunc != nil {
		return m.GenerateChatResponseFunc(ctx, systemPrompt, messages, opts)
	}

	return &services.ChatResponse{
		Content:      "According to the transcript [00:15], the sprint planning was successfully concluded.",
		Role:         "assistant",
		FinishReason: "stop",
		Usage: services.LLMUsage{
			PromptTokens:     50,
			CompletionTokens: 20,
			TotalTokens:      70,
		},
	}, nil
}

// StreamChatResponse executes StreamChatResponseFunc or streams mock tokens.
func (m *MockLLM) StreamChatResponse(ctx context.Context, systemPrompt string, messages []services.ChatMessageInput, opts services.ChatOptions) (<-chan services.StreamChunk, error) {
	if m.StreamChatResponseFunc != nil {
		return m.StreamChatResponseFunc(ctx, systemPrompt, messages, opts)
	}

	out := make(chan services.StreamChunk, 5)
	go func() {
		defer close(out)
		tokens := []string{"Hello", " from", " mock", " stream", "!"}
		for _, tok := range tokens {
			select {
			case <-ctx.Done():
				out <- services.StreamChunk{Err: ctx.Err()}
				return
			default:
				out <- services.StreamChunk{Content: tok}
			}
		}
		out <- services.StreamChunk{FinishReason: "stop"}
	}()

	return out, nil
}
