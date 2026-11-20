package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
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
	maxWorkers int
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

	timeout := 2 * time.Minute
	client := &http.Client{Timeout: timeout}
	if len(customClient) > 0 && customClient[0] != nil {
		client = customClient[0]
	}

	maxWorkers := cfg.MaxEmbeddingWorkers
	if maxWorkers <= 0 {
		maxWorkers = 4
	}

	return &OmniRouteEmbedding{
		baseURL:    baseURL,
		apiKey:     strings.TrimSpace(cfg.LLMAPIKey),
		model:      model,
		httpClient: client,
		batchSize:  50,
		maxWorkers: maxWorkers,
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

type embeddingBatchJob struct {
	startIdx int
	texts    []string
}

// CreateEmbeddings produces dense vector representations for the input texts, preserving input order.
// Uses an elastic worker pool when multiple batches are present, scaling concurrency up to maxWorkers
// and immediately returning to zero worker goroutines upon completion.
func (o *OmniRouteEmbedding) CreateEmbeddings(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	start := time.Now()
	allEmbeddings := make([][]float32, len(texts))

	batchSize := o.batchSize
	if batchSize <= 0 {
		batchSize = 50
	}

	// 1. Single batch fast path: execute synchronously without worker pool overhead
	if len(texts) <= batchSize {
		log.Printf("[EMBEDDING START] generating embeddings for %d texts (single batch, model=%s)", len(texts), o.model)
		data, err := o.fetchBatchEmbeddings(ctx, texts)
		if err != nil {
			return nil, err
		}
		for _, item := range data {
			if item.Index < 0 || item.Index >= len(allEmbeddings) {
				return nil, fmt.Errorf("embedding index out of range: %d", item.Index)
			}
			allEmbeddings[item.Index] = item.Embedding
		}
		log.Printf("[EMBEDDING SUCCESS] generated %d embeddings in %s", len(allEmbeddings), time.Since(start).Round(time.Millisecond))
		return allEmbeddings, nil
	}

	// 2. Partition into batch jobs
	var jobs []embeddingBatchJob
	for i := 0; i < len(texts); i += batchSize {
		end := i + batchSize
		if end > len(texts) {
			end = len(texts)
		}
		jobs = append(jobs, embeddingBatchJob{
			startIdx: i,
			texts:    texts[i:end],
		})
	}

	maxWorkers := o.maxWorkers
	if maxWorkers <= 0 {
		maxWorkers = 4
	}
	numWorkers := len(jobs)
	if numWorkers > maxWorkers {
		numWorkers = maxWorkers
	}

	log.Printf("[EMBEDDING START] generating embeddings for %d texts (batches=%d, workers=%d, model=%s)",
		len(texts), len(jobs), numWorkers, o.model)

	jobChan := make(chan embeddingBatchJob, len(jobs))
	for _, job := range jobs {
		jobChan <- job
	}
	close(jobChan)

	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		wg      sync.WaitGroup
		errOnce sync.Once
		firstErr error
	)

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobChan {
				select {
				case <-workerCtx.Done():
					return
				default:
				}

				data, err := o.fetchBatchEmbeddings(workerCtx, job.texts)
				if err != nil {
					errOnce.Do(func() {
						firstErr = fmt.Errorf("batch at offset %d failed: %w", job.startIdx, err)
						cancel()
					})
					return
				}

				for _, item := range data {
					targetIdx := job.startIdx + item.Index
					if targetIdx < 0 || targetIdx >= len(allEmbeddings) {
						errOnce.Do(func() {
							firstErr = fmt.Errorf("embedding index out of range: %d", targetIdx)
							cancel()
						})
						return
					}
					allEmbeddings[targetIdx] = item.Embedding
				}
			}
		}()
	}

	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}

	log.Printf("[EMBEDDING SUCCESS] generated %d embeddings in %s", len(allEmbeddings), time.Since(start).Round(time.Millisecond))
	return allEmbeddings, nil
}

// fetchBatchEmbeddings executes an individual HTTP embedding batch request against OmniRoute.
func (o *OmniRouteEmbedding) fetchBatchEmbeddings(ctx context.Context, batch []string) ([]openAIEmbeddingData, error) {
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
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
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

	return embResp.Data, nil
}

// Ensure interface contract
var _ services.EmbeddingProvider = (*OmniRouteEmbedding)(nil)
