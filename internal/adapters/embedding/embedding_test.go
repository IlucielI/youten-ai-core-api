package embedding_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"code-base-golang/internal/adapters/embedding"
	"code-base-golang/internal/config"
	"code-base-golang/internal/services"
)

func TestOmniRouteEmbedding_CreateEmbeddings_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/embeddings" {
			t.Errorf("expected /embeddings, got %s", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer emb-key" {
			t.Errorf("expected Bearer emb-key, got %s", auth)
		}

		var reqBody map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&reqBody)
		if reqBody["model"] != "cf/@cf/baai/bge-m3" {
			t.Errorf("expected model cf/@cf/baai/bge-m3, got %v", reqBody["model"])
		}

		inputs, _ := reqBody["input"].([]interface{})
		data := make([]map[string]interface{}, len(inputs))
		for i := range inputs {
			data[i] = map[string]interface{}{
				"object":    "embedding",
				"index":     i,
				"embedding": []float32{float32(i) * 0.1, 0.2, 0.3},
			}
		}

		resp := map[string]interface{}{
			"object": "list",
			"data":   data,
			"model":  "cf/@cf/baai/bge-m3",
			"usage": map[string]interface{}{
				"prompt_tokens": 10,
				"total_tokens":  10,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := config.Config{
		LLMBaseURL:     server.URL,
		LLMAPIKey:      "emb-key",
		EmbeddingModel: "cf/@cf/baai/bge-m3",
	}

	adapter := embedding.NewOmniRoute(cfg)

	// Test empty slice
	emptyRes, err := adapter.CreateEmbeddings(context.Background(), nil)
	if err != nil || emptyRes != nil {
		t.Fatalf("expected nil, nil for empty texts: %v, %v", emptyRes, err)
	}

	texts := []string{"First chunk", "Second chunk"}
	vectors, err := adapter.CreateEmbeddings(context.Background(), texts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(vectors) != 2 {
		t.Fatalf("expected 2 vectors, got %d", len(vectors))
	}
	if len(vectors[0]) != 3 || len(vectors[1]) != 3 {
		t.Fatalf("expected 3-dimension vectors, got %d and %d", len(vectors[0]), len(vectors[1]))
	}
	if vectors[0][0] != 0.0 || vectors[1][0] != 0.1 {
		t.Errorf("unexpected vector values: %v, %v", vectors[0], vectors[1])
	}
}

func TestOmniRouteEmbedding_CreateEmbeddings_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "invalid_model"}`))
	}))
	defer server.Close()

	cfg := config.Config{LLMBaseURL: server.URL}
	adapter := embedding.NewOmniRoute(cfg)

	_, err := adapter.CreateEmbeddings(context.Background(), []string{"test"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "status 400") {
		t.Errorf("expected status 400 in error, got %v", err)
	}
}

func TestOmniRouteEmbedding_CreateEmbeddings_IndexOutOfRange(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"object": "list",
			"data": []map[string]interface{}{
				{
					"object":    "embedding",
					"index":     99, // Out of range index
					"embedding": []float32{0.1, 0.2, 0.3},
				},
			},
			"model": "cf/@cf/baai/bge-m3",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := config.Config{LLMBaseURL: server.URL}
	adapter := embedding.NewOmniRoute(cfg)

	_, err := adapter.CreateEmbeddings(context.Background(), []string{"test"})
	if err == nil {
		t.Fatal("expected error for out of range index, got nil")
	}
	if !strings.Contains(err.Error(), "embedding index out of range") {
		t.Errorf("expected 'embedding index out of range' error, got %v", err)
	}
}

func TestMockEmbedding(t *testing.T) {
	mock := embedding.NewMock(512)

	// Empty input
	empty, err := mock.CreateEmbeddings(context.Background(), nil)
	if err != nil || empty != nil {
		t.Fatalf("expected nil, nil: %v, %v", empty, err)
	}

	texts := []string{"Alpha", "Beta"}
	vecs, err := mock.CreateEmbeddings(context.Background(), texts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vecs) != 2 {
		t.Fatalf("expected 2 vectors, got %d", len(vecs))
	}
	if len(vecs[0]) != 512 {
		t.Fatalf("expected 512 dimensions, got %d", len(vecs[0]))
	}

	// Custom hook
	mock.CreateEmbeddingsFunc = func(ctx context.Context, texts []string) ([][]float32, error) {
		return nil, fmt.Errorf("custom embedding error")
	}
	_, err = mock.CreateEmbeddings(context.Background(), texts)
	if err == nil || err.Error() != "custom embedding error" {
		t.Errorf("expected custom error, got %v", err)
	}

	// Verify interface compliance
	var _ services.EmbeddingProvider = mock
}
