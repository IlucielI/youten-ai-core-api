package stt_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"code-base-golang/internal/adapters/stt"
	"code-base-golang/internal/config"
	"code-base-golang/internal/services"
)

func TestOmniRouteSTT_Transcribe_Success(t *testing.T) {
	mockResponse := `{
		"task": "transcribe",
		"language": "english",
		"duration": 7.5,
		"text": "Hello world from test.",
		"segments": [
			{
				"id": 0,
				"seek": 0,
				"start": 0.0,
				"end": 7.5,
				"text": "Hello world from test.",
				"tokens": [101, 102],
				"words": [
					{"word": "Hello", "start": 0.0, "end": 2.0, "probability": 0.98},
					{"word": "world", "start": 2.1, "end": 4.0, "probability": 0.95}
				]
			}
		]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/audio/transcriptions" {
			t.Errorf("expected path /audio/transcriptions, got %s", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-api-key" {
			t.Errorf("expected Bearer test-api-key, got %s", auth)
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			t.Errorf("expected multipart/form-data, got %s", r.Header.Get("Content-Type"))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(mockResponse))
	}))
	defer server.Close()

	cfg := config.Config{
		LLMBaseURL: server.URL,
		LLMAPIKey:  "test-api-key",
		STTModel:   "whisper-1",
	}

	adapter := stt.NewOmniRoute(cfg)
	audioData := bytes.NewReader([]byte("fake-audio-bytes"))

	res, err := adapter.Transcribe(context.Background(), audioData, "sample.mp3", services.STTOptions{
		Language:    "en",
		Prompt:      "Test prompt",
		Temperature: 0.2,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Text != "Hello world from test." {
		t.Errorf("expected text 'Hello world from test.', got %q", res.Text)
	}
	if res.Duration != 7.5 {
		t.Errorf("expected duration 7.5, got %f", res.Duration)
	}
	if len(res.Segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(res.Segments))
	}
	if len(res.Segments[0].Words) != 2 {
		t.Fatalf("expected 2 words, got %d", len(res.Segments[0].Words))
	}
	if res.Segments[0].Words[0].Word != "Hello" {
		t.Errorf("expected first word 'Hello', got %s", res.Segments[0].Words[0].Word)
	}
}

func TestOmniRouteSTT_Transcribe_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error": "invalid_api_key"}`))
	}))
	defer server.Close()

	cfg := config.Config{
		LLMBaseURL: server.URL,
		LLMAPIKey:  "wrong-key",
	}

	adapter := stt.NewOmniRoute(cfg)
	_, err := adapter.Transcribe(context.Background(), bytes.NewReader([]byte("data")), "test.mp3", services.STTOptions{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "status 401") {
		t.Errorf("expected status 401 in error, got %v", err)
	}
}

func TestOmniRouteSTT_Transcribe_NilReader(t *testing.T) {
	adapter := stt.NewOmniRoute(config.Config{})
	_, err := adapter.Transcribe(context.Background(), nil, "test.mp3", services.STTOptions{})
	if err == nil {
		t.Fatal("expected error for nil reader, got nil")
	}
}

func TestMockSTT(t *testing.T) {
	mock := stt.NewMock()
	res, err := mock.Transcribe(context.Background(), bytes.NewReader([]byte("audio")), "test.mp3", services.STTOptions{Language: "id"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Language != "id" {
		t.Errorf("expected language id, got %s", res.Language)
	}
	if len(res.Segments) != 2 {
		t.Errorf("expected 2 segments, got %d", len(res.Segments))
	}

	// Custom func test
	mock.TranscribeFunc = func(ctx context.Context, reader io.Reader, filename string, opts services.STTOptions) (*services.TranscriptionResult, error) {
		return nil, fmt.Errorf("custom mock error")
	}
	_, err = mock.Transcribe(context.Background(), bytes.NewReader([]byte("audio")), "test.mp3", services.STTOptions{})
	if err == nil || err.Error() != "custom mock error" {
		t.Errorf("expected custom mock error, got %v", err)
	}
}
