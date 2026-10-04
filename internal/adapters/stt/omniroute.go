package stt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"code-base-golang/internal/config"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/services"
)

// OmniRouteSTT implements services.STTProvider using an OpenAI-compatible audio transcription endpoint.
type OmniRouteSTT struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

// NewOmniRoute creates a new OmniRoute OpenAI-compatible STT adapter.
func NewOmniRoute(cfg config.Config, customClient ...*http.Client) *OmniRouteSTT {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.LLMBaseURL), "/")
	if baseURL == "" {
		baseURL = "http://localhost:20128/v1"
	}

	model := strings.TrimSpace(cfg.STTModel)
	if model == "" {
		model = "whisper-1"
	}

	client := &http.Client{Timeout: 5 * time.Minute}
	if len(customClient) > 0 && customClient[0] != nil {
		client = customClient[0]
	}

	return &OmniRouteSTT{
		baseURL:    baseURL,
		apiKey:     strings.TrimSpace(cfg.LLMAPIKey),
		model:      model,
		httpClient: client,
	}
}

type openAIVerboseJSON struct {
	Language string  `json:"language"`
	Duration float64 `json:"duration"`
	Text     string  `json:"text"`
	Segments []struct {
		ID               int     `json:"id"`
		Seek             int     `json:"seek"`
		Start            float64 `json:"start"`
		End              float64 `json:"end"`
		Text             string  `json:"text"`
		Tokens           []int   `json:"tokens"`
		Temperature      float64 `json:"temperature"`
		AvgLogprob       float64 `json:"avg_logprob"`
		CompressionRatio float64 `json:"compression_ratio"`
		NoSpeechProb     float64 `json:"no_speech_prob"`
		Words            []struct {
			Word        string  `json:"word"`
			Start       float64 `json:"start"`
			End         float64 `json:"end"`
			Probability float64 `json:"probability,omitempty"`
		} `json:"words"`
	} `json:"segments"`
}

// Transcribe streams audio to the transcription endpoint and decodes the verbose JSON response.
func (o *OmniRouteSTT) Transcribe(ctx context.Context, reader io.Reader, filename string, opts dtos.STTOptions) (*dtos.TranscriptionResult, error) {
	if reader == nil {
		return nil, fmt.Errorf("audio reader cannot be nil")
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	name := strings.TrimSpace(filename)
	if name == "" {
		name = "audio.mp3"
	}

	part, err := writer.CreateFormFile("file", name)
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}

	if _, err := io.Copy(part, reader); err != nil {
		return nil, fmt.Errorf("failed to copy audio stream to form: %w", err)
	}

	if err := writer.WriteField("model", o.model); err != nil {
		return nil, err
	}
	if err := writer.WriteField("response_format", "verbose_json"); err != nil {
		return nil, err
	}
	if err := writer.WriteField("timestamp_granularities[]", "segment"); err != nil {
		return nil, err
	}
	if err := writer.WriteField("timestamp_granularities[]", "word"); err != nil {
		return nil, err
	}

	if opts.Language != "" {
		if err := writer.WriteField("language", opts.Language); err != nil {
			return nil, err
		}
	}
	if opts.Prompt != "" {
		if err := writer.WriteField("prompt", opts.Prompt); err != nil {
			return nil, err
		}
	}
	if opts.Temperature > 0 {
		if err := writer.WriteField("temperature", strconv.FormatFloat(opts.Temperature, 'f', 2, 64)); err != nil {
			return nil, err
		}
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	url := fmt.Sprintf("%s/audio/transcriptions", o.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	if o.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+o.apiKey)
	}

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("stt request error: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("stt api returned status %d: %s", resp.StatusCode, string(respBytes))
	}

	var parsed openAIVerboseJSON
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse verbose json response: %w", err)
	}

	segments := make([]dtos.SegmentResult, len(parsed.Segments))
	for i, s := range parsed.Segments {
		words := make([]dtos.WordResult, len(s.Words))
		for wIdx, w := range s.Words {
			words[wIdx] = dtos.WordResult{
				Word:        w.Word,
				Start:       w.Start,
				End:         w.End,
				Probability: w.Probability,
			}
		}

		segments[i] = dtos.SegmentResult{
			ID:               s.ID,
			Seek:             s.Seek,
			Start:            s.Start,
			End:              s.End,
			Text:             s.Text,
			Tokens:           s.Tokens,
			Temperature:      s.Temperature,
			AvgLogprob:       s.AvgLogprob,
			CompressionRatio: s.CompressionRatio,
			NoSpeechProb:     s.NoSpeechProb,
			Words:            words,
		}
	}

	// Defensive fallback if audio returned single text without segmented array
	if len(segments) == 0 && parsed.Text != "" {
		segments = append(segments, dtos.SegmentResult{
			ID:    0,
			Start: 0,
			End:   parsed.Duration,
			Text:  parsed.Text,
		})
	}

	return &dtos.TranscriptionResult{
		Text:     parsed.Text,
		Language: parsed.Language,
		Duration: parsed.Duration,
		Segments: segments,
	}, nil
}

// Ensure OmniRouteSTT satisfies services.STTProvider at compile time.
var _ services.STTProvider = (*OmniRouteSTT)(nil)
