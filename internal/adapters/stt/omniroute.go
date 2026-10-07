package stt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"code-base-golang/internal/config"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/pkg/strutil"
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
	if err := writer.WriteField("diarize", "true"); err != nil {
		return nil, err
	}

	if opts.Language != "" && strings.ToLower(strings.TrimSpace(opts.Language)) != "auto" {
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

		text := s.Text
		speaker := ""
		if match := speakerPrefixRegex.FindStringSubmatch(strings.TrimSpace(text)); len(match) == 3 {
			speaker = strings.TrimSpace(match[1])
			text = strings.TrimSpace(match[2])
		}

		segments[i] = dtos.SegmentResult{
			ID:               s.ID,
			Seek:             s.Seek,
			Start:            s.Start,
			End:              s.End,
			Text:             text,
			SpeakerLabel:     speaker,
			Tokens:           s.Tokens,
			Temperature:      s.Temperature,
			AvgLogprob:       s.AvgLogprob,
			CompressionRatio: s.CompressionRatio,
			NoSpeechProb:     s.NoSpeechProb,
			Words:            words,
		}
	}

	duration := parsed.Duration
	if duration <= 0 && opts.AudioDuration > 0 {
		duration = opts.AudioDuration
	}

	// Defensive fallback if audio returned single text without segmented array or diarized text block
	if len(segments) == 0 && parsed.Text != "" {
		diarized := parseDiarizedTextSegments(parsed.Text, duration)
		if len(diarized) > 0 {
			segments = diarized
		} else {
			segments = append(segments, dtos.SegmentResult{
				ID:           0,
				Start:        0,
				End:          duration,
				Text:         parsed.Text,
				SpeakerLabel: "Speaker 0",
			})
		}
	}

	lang := strings.TrimSpace(parsed.Language)
	if lang == "" {
		lang = strutil.DetectLanguage(parsed.Text, "id")
	} else {
		lang = strutil.NormalizeLanguageCode(lang, "id")
	}

	return &dtos.TranscriptionResult{
		Text:     parsed.Text,
		Language: lang,
		Duration: duration,
		Segments: segments,
	}, nil
}

// Ensure OmniRouteSTT satisfies services.STTProvider at compile time.
var _ services.STTProvider = (*OmniRouteSTT)(nil)

var speakerPrefixRegex = regexp.MustCompile(`^(?:\[)?(Speaker\s*\d+|Pembicara\s*\d+)(?:\])?\s*:\s*(.*)$`)

// parseDiarizedTextSegments splits text containing "Speaker X:" lines into structured segments
// and computes proportional timestamps based on audio duration.
func parseDiarizedTextSegments(fullText string, duration float64) []dtos.SegmentResult {
	lines := strings.Split(fullText, "\n")
	type parsedLine struct {
		speaker string
		text    string
	}
	var items []parsedLine

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if match := speakerPrefixRegex.FindStringSubmatch(trimmed); len(match) == 3 {
			items = append(items, parsedLine{
				speaker: strings.TrimSpace(match[1]),
				text:    strings.TrimSpace(match[2]),
			})
		} else {
			if len(items) > 0 {
				items[len(items)-1].text += " " + trimmed
			} else {
				items = append(items, parsedLine{
					speaker: "Speaker 0",
					text:    trimmed,
				})
			}
		}
	}

	if len(items) == 0 {
		return nil
	}

	totalChars := 0
	for _, it := range items {
		totalChars += len([]rune(it.text))
	}
	if totalChars == 0 {
		totalChars = 1
	}

	res := make([]dtos.SegmentResult, len(items))
	runningChars := 0
	for i, it := range items {
		charLen := len([]rune(it.text))
		var start, end float64
		if duration > 0 {
			start = (float64(runningChars) / float64(totalChars)) * duration
			end = (float64(runningChars+charLen) / float64(totalChars)) * duration
		} else {
			start = float64(i * 3)
			end = float64((i + 1) * 3)
		}
		runningChars += charLen

		res[i] = dtos.SegmentResult{
			ID:           i,
			Start:        start,
			End:          end,
			Text:         it.text,
			SpeakerLabel: it.speaker,
		}
	}
	return res
}

