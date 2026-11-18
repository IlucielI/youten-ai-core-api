package llm

import (
	"context"

	"code-base-golang/internal/dtos"
	"code-base-golang/internal/services"
)

// MockLLM provides a mockable implementation of services.LLMProvider for unit and offline testing.
type MockLLM struct {
	GenerateStructuredFunc   func(ctx context.Context, systemPrompt string, userPrompt string, schema map[string]interface{}) (*dtos.StructuredResponse, error)
	GenerateChatResponseFunc func(ctx context.Context, systemPrompt string, messages []dtos.ChatMessageInput, opts dtos.ChatOptions) (*dtos.ChatResponse, error)
	StreamChatResponseFunc   func(ctx context.Context, systemPrompt string, messages []dtos.ChatMessageInput, opts dtos.ChatOptions) (<-chan dtos.StreamChunk, error)
	AnalyzeKeyframeFunc      func(ctx context.Context, imgBytes []byte, mimeType string, prompt string) (*dtos.KeyframeAnalysisResult, error)
	AnalyzeKeyframesBatchFunc func(ctx context.Context, frames []dtos.KeyframeBatchInput) ([]dtos.KeyframeAnalysisResult, error)
}

// NewMock creates a new MockLLM instance.
func NewMock() *MockLLM {
	return &MockLLM{}
}

// GenerateStructured executes GenerateStructuredFunc or returns a canned JSON summary.
func (m *MockLLM) GenerateStructured(ctx context.Context, systemPrompt string, userPrompt string, schema map[string]interface{}) (*dtos.StructuredResponse, error) {
	if m.GenerateStructuredFunc != nil {
		return m.GenerateStructuredFunc(ctx, systemPrompt, userPrompt, schema)
	}

	return &dtos.StructuredResponse{
		RawJSON: `{"title": "Sprint Planning Recap", "overview": "Team reviewed sprint commitments and milestones.", "action_items": [{"task": "Deploy vector db", "owner": "Bayu"}]}`,
		Usage: dtos.LLMUsage{
			PromptTokens:     120,
			CompletionTokens: 65,
			TotalTokens:      185,
		},
	}, nil
}

// GenerateChatResponse executes GenerateChatResponseFunc or returns a canned conversational answer.
func (m *MockLLM) GenerateChatResponse(ctx context.Context, systemPrompt string, messages []dtos.ChatMessageInput, opts dtos.ChatOptions) (*dtos.ChatResponse, error) {
	if m.GenerateChatResponseFunc != nil {
		return m.GenerateChatResponseFunc(ctx, systemPrompt, messages, opts)
	}

	return &dtos.ChatResponse{
		Content:      "According to the transcript [00:15], the sprint planning was successfully concluded.",
		Role:         "assistant",
		FinishReason: "stop",
		Usage: dtos.LLMUsage{
			PromptTokens:     50,
			CompletionTokens: 20,
			TotalTokens:      70,
		},
	}, nil
}

// StreamChatResponse executes StreamChatResponseFunc or streams mock tokens.
func (m *MockLLM) StreamChatResponse(ctx context.Context, systemPrompt string, messages []dtos.ChatMessageInput, opts dtos.ChatOptions) (<-chan dtos.StreamChunk, error) {
	if m.StreamChatResponseFunc != nil {
		return m.StreamChatResponseFunc(ctx, systemPrompt, messages, opts)
	}

	out := make(chan dtos.StreamChunk, 5)
	go func() {
		defer close(out)
		tokens := []string{"Hello", " from", " mock", " stream", "!"}
		for _, tok := range tokens {
			select {
			case <-ctx.Done():
				out <- dtos.StreamChunk{Err: ctx.Err()}
				return
			default:
				out <- dtos.StreamChunk{Content: tok}
			}
		}
		out <- dtos.StreamChunk{FinishReason: "stop"}
	}()

	return out, nil
}

// AnalyzeKeyframe executes AnalyzeKeyframeFunc or returns a canned analysis result.
func (m *MockLLM) AnalyzeKeyframe(ctx context.Context, imgBytes []byte, mimeType string, prompt string) (*dtos.KeyframeAnalysisResult, error) {
	if m.AnalyzeKeyframeFunc != nil {
		return m.AnalyzeKeyframeFunc(ctx, imgBytes, mimeType, prompt)
	}
	return &dtos.KeyframeAnalysisResult{
		CueType:       "SLIDE",
		Title:         "System Architecture Overview",
		ExtractedText: "1. Client Web App\n2. Core Go API\n3. PostgreSQL & Vector Database",
		VisualSummary: "Architecture diagram showing backend components.",
		Confidence:    0.95,
	}, nil
}

// AnalyzeKeyframesBatch executes AnalyzeKeyframesBatchFunc or maps AnalyzeKeyframe over frames.
func (m *MockLLM) AnalyzeKeyframesBatch(ctx context.Context, frames []dtos.KeyframeBatchInput) ([]dtos.KeyframeAnalysisResult, error) {
	if m.AnalyzeKeyframesBatchFunc != nil {
		return m.AnalyzeKeyframesBatchFunc(ctx, frames)
	}
	var results []dtos.KeyframeAnalysisResult
	for _, f := range frames {
		res, err := m.AnalyzeKeyframe(ctx, f.ImageBytes, f.MimeType, "")
		if err != nil {
			return nil, err
		}
		res.SequenceOrder = f.SequenceOrder
		res.TimestampSeconds = f.TimestampSeconds
		results = append(results, *res)
	}
	return results, nil
}

// Ensure MockLLM satisfies services.LLMProvider and services.VisionAnalyzer at compile time.
var (
	_ services.LLMProvider    = (*MockLLM)(nil)
	_ services.VisionAnalyzer = (*MockLLM)(nil)
)
