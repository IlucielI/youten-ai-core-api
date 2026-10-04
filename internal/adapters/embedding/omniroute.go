package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"code-base-golang/internal/config"
	"code-base-golang/internal/services"
)

// OmniRouteEmbedding implements services.EmbeddingProvider using an OpenAI-compatible /embeddings endpoint.
type OmniRouteEmbedding struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
	batchSize  int
}

// NewOmniRoute creates a new OmniRoute OpenAI-compatible Embedding adapter.
func NewOmniRoute(cfg config.Config, customClient ...*http.Client) *OmniRouteEmbedding {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.LLMBaseURL), "/")
	if baseURL == "" {
		baseURL = "http://localhost:20128/v1"
	}

	model := strings.TrimSpace(cfg.EmbeddingModel)
	if model == "" {
		model = "cf/@cf/baai/bge-m3"
	}

	client := &http.Client{Timeout: 2 * time.Minute}
	if len(customClient) > 0 && customClient[0] != nil {
		client = customClient[0]
	}

	return &OmniRouteEmbedding{
		baseURL:    baseURL,
		apiKey:     strings.TrimSpace(cfg.LLMAPIKey),
		model:      model,
		httpClient: client,
		batchSize:  50,
	}
}

type openAIEmbeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type openAIEmbeddingData struct {
	Index     int       `json:"index"`
	Embedding []float32 `json:"embedding"`
}

type openAIEmbeddingResponse struct {
	Data  []openAIEmbeddingData `json:"data"`
	Model string                `json:"model"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

// CreateEmbeddings produces dense vector representations for the input texts, preserving input order.
func (o *OmniRouteEmbedding) CreateEmbeddings(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	allEmbeddings := make([][]float32, len(texts))

	batchSize := o.batchSize
	if batchSize <= 0 {
		batchSize = 50
	}

	for i := 0; i < len(texts); i += batchSize {
		end := i + batchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch := texts[i:end]

		reqPayload := openAIEmbeddingRequest{
			Model: o.model,
			Input: batch,
		}

		bodyBytes, err := json.Marshal(reqPayload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal embeddings request: %w", err)
		}

		url := fmt.Sprintf("%s/embeddings", o.baseURL)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, fmt.Errorf("failed to create http request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		if o.apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+o.apiKey)
		}

		resp, err := o.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("embedding request failed: %w", err)
		}

		respBytes, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read embedding response: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("embedding api returned status %d: %s", resp.StatusCode, string(respBytes))
		}

		var embResp openAIEmbeddingResponse
		if err := json.Unmarshal(respBytes, &embResp); err != nil {
			return nil, fmt.Errorf("failed to unmarshal embedding response: %w", err)
		}

		if len(embResp.Data) != len(batch) {
			return nil, fmt.Errorf("embedding count mismatch: expected %d, got %d", len(batch), len(embResp.Data))
		}

		for _, item := range embResp.Data {
			targetIdx := i + item.Index
			if targetIdx < 0 || targetIdx >= len(allEmbeddings) {
				return nil, fmt.Errorf("embedding index out of range: %d", targetIdx)
			}
			allEmbeddings[targetIdx] = item.Embedding
		}
	}

	return allEmbeddings, nil
}

// Ensure interface contract
var _ services.EmbeddingProvider = (*OmniRouteEmbedding)(nil)
