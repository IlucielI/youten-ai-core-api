package llm_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"code-base-golang/internal/adapters/llm"
	"code-base-golang/internal/config"
	"code-base-golang/internal/services"
)

func TestOmniRouteLLM_GenerateStructured(t *testing.T) {
	mockResponse := `{
		"id": "chatcmpl-test",
		"choices": [
			{
				"index": 0,
				"message": {
					"role": "assistant",
					"content": "{\"summary\": \"Great discussion about roadmap.\"}"
				},
				"finish_reason": "stop"
			}
		],
		"usage": {
			"prompt_tokens": 100,
			"completion_tokens": 30,
			"total_tokens": 130
		}
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/chat/completions" {
			t.Errorf("expected /chat/completions, got %s", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-key" {
			t.Errorf("expected Bearer test-key, got %s", auth)
		}

		var reqBody map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&reqBody)
		if reqBody["model"] != "gpt-4o-mini" {
			t.Errorf("expected model gpt-4o-mini, got %v", reqBody["model"])
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(mockResponse))
	}))
	defer server.Close()

	cfg := config.Config{
		LLMBaseURL: server.URL,
		LLMAPIKey:  "test-key",
		LLMModel:   "gpt-4o-mini",
	}

	adapter := llm.NewOmniRoute(cfg)
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"summary": map[string]interface{}{"type": "string"},
		},
	}

	resp, err := adapter.GenerateStructured(context.Background(), "System prompt", "User prompt", schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(resp.RawJSON, "Great discussion about roadmap.") {
		t.Errorf("unexpected raw JSON: %s", resp.RawJSON)
	}
	if resp.Usage.TotalTokens != 130 {
		t.Errorf("expected 130 total tokens, got %d", resp.Usage.TotalTokens)
	}
}

func TestOmniRouteLLM_GenerateChatResponse(t *testing.T) {
	mockResponse := `{
		"id": "chatcmpl-chat",
		"choices": [
			{
				"index": 0,
				"message": {
					"role": "assistant",
					"content": "Here is the answer to your question."
				},
				"finish_reason": "stop"
			}
		],
		"usage": {
			"prompt_tokens": 40,
			"completion_tokens": 15,
			"total_tokens": 55
		}
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(mockResponse))
	}))
	defer server.Close()

	cfg := config.Config{LLMBaseURL: server.URL}
	adapter := llm.NewOmniRoute(cfg)

	temp := 0.7
	maxTok := 256
	resp, err := adapter.GenerateChatResponse(context.Background(), "System", []services.ChatMessageInput{
		{Role: "user", Content: "Hello"},
	}, services.ChatOptions{
		Temperature: &temp,
		MaxTokens:   &maxTok,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Content != "Here is the answer to your question." {
		t.Errorf("unexpected content: %s", resp.Content)
	}
	if resp.FinishReason != "stop" {
		t.Errorf("expected finish_reason stop, got %s", resp.FinishReason)
	}
}

func TestOmniRouteLLM_StreamChatResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		events := []string{
			`data: {"choices":[{"delta":{"content":"Hello"},"finish_reason":""}]}` + "\n\n",
			`data: {"choices":[{"delta":{"content":" world"},"finish_reason":""}]}` + "\n\n",
			`data: {"choices":[{"delta":{"content":""},"finish_reason":"stop"}]}` + "\n\n",
			"data: [DONE]\n\n",
		}

		for _, ev := range events {
			_, _ = w.Write([]byte(ev))
			flusher.Flush()
		}
	}))
	defer server.Close()

	cfg := config.Config{LLMBaseURL: server.URL}
	adapter := llm.NewOmniRoute(cfg)

	ch, err := adapter.StreamChatResponse(context.Background(), "System", []services.ChatMessageInput{
		{Role: "user", Content: "Stream please"},
	}, services.ChatOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var sb strings.Builder
	var lastFinish string
	for chunk := range ch {
		if chunk.Err != nil {
			t.Fatalf("stream chunk error: %v", chunk.Err)
		}
		sb.WriteString(chunk.Content)
		if chunk.FinishReason != "" {
			lastFinish = chunk.FinishReason
		}
	}

	if sb.String() != "Hello world" {
		t.Errorf("expected accumulated 'Hello world', got %q", sb.String())
	}
	if lastFinish != "stop" {
		t.Errorf("expected finish_reason stop, got %q", lastFinish)
	}
}

func TestOmniRouteLLM_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "model_overloaded"}`))
	}))
	defer server.Close()

	cfg := config.Config{LLMBaseURL: server.URL}
	adapter := llm.NewOmniRoute(cfg)

	_, err := adapter.GenerateStructured(context.Background(), "", "Hello", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "status 500") {
		t.Errorf("expected status 500 in error, got %v", err)
	}
}

func TestMockLLM(t *testing.T) {
	mock := llm.NewMock()

	// Default structured
	sRes, err := mock.GenerateStructured(context.Background(), "", "", nil)
	if err != nil || !strings.Contains(sRes.RawJSON, "Sprint Planning Recap") {
		t.Errorf("unexpected default structured response: %v, %v", sRes, err)
	}

	// Default chat
	cRes, err := mock.GenerateChatResponse(context.Background(), "", nil, services.ChatOptions{})
	if err != nil || !strings.Contains(cRes.Content, "sprint planning") {
		t.Errorf("unexpected default chat response: %v, %v", cRes, err)
	}

	// Default stream
	stream, err := mock.StreamChatResponse(context.Background(), "", nil, services.ChatOptions{})
	if err != nil {
		t.Fatalf("unexpected stream error: %v", err)
	}
	var streamed string
	for chunk := range stream {
		streamed += chunk.Content
	}
	if streamed != "Hello from mock stream!" {
		t.Errorf("unexpected streamed text: %q", streamed)
	}

	// Custom hook
	mock.GenerateChatResponseFunc = func(ctx context.Context, systemPrompt string, messages []services.ChatMessageInput, opts services.ChatOptions) (*services.ChatResponse, error) {
		return nil, fmt.Errorf("custom hook error")
	}
	_, err = mock.GenerateChatResponse(context.Background(), "", nil, services.ChatOptions{})
	if err == nil || err.Error() != "custom hook error" {
		t.Errorf("expected custom hook error, got %v", err)
	}
}

func TestOmniRouteLLM_StreamContextCancel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"first\"}}]}\n\n"))
		flusher.Flush()
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())

	cfg := config.Config{LLMBaseURL: server.URL}
	adapter := llm.NewOmniRoute(cfg)

	ch, err := adapter.StreamChatResponse(ctx, "", nil, services.ChatOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	chunk := <-ch
	if chunk.Content != "first" {
		t.Errorf("expected 'first', got %q", chunk.Content)
	}

	cancel()

	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-time.After(1 * time.Second):
			t.Fatal("stream channel did not close within timeout after cancellation")
		}
	}
}
