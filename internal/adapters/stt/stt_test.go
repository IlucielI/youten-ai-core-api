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
	"code-base-golang/internal/dtos"
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
			t.Errorf("expected POST method, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/audio/transcriptions") {
			t.Errorf("expected /audio/transcriptions path, got %s", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-api-key" {
			t.Errorf("expected Bearer token, got %s", auth)
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

	res, err := adapter.Transcribe(context.Background(), audioData, "sample.mp3", dtos.STTOptions{
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
	_, err := adapter.Transcribe(context.Background(), bytes.NewReader([]byte("data")), "test.mp3", dtos.STTOptions{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "status 401") {
		t.Errorf("expected status 401 in error, got %v", err)
	}
}

func TestOmniRouteSTT_Transcribe_NilReader(t *testing.T) {
	adapter := stt.NewOmniRoute(config.Config{})
	_, err := adapter.Transcribe(context.Background(), nil, "test.mp3", dtos.STTOptions{})
	if err == nil {
		t.Fatal("expected error for nil reader, got nil")
	}
}

func TestMockSTT(t *testing.T) {
	mock := stt.NewMock()
	res, err := mock.Transcribe(context.Background(), bytes.NewReader([]byte("audio")), "test.mp3", dtos.STTOptions{Language: "id"})
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
	mock.TranscribeFunc = func(ctx context.Context, reader io.Reader, filename string, opts dtos.STTOptions) (*dtos.TranscriptionResult, error) {
		return nil, fmt.Errorf("custom mock error")
	}
	_, err = mock.Transcribe(context.Background(), bytes.NewReader([]byte("audio")), "test.mp3", dtos.STTOptions{})
	if err == nil || err.Error() != "custom mock error" {
		t.Errorf("expected custom mock error, got %v", err)
	}
}

func TestOmniRouteSTT_Transcribe_DiarizedText(t *testing.T) {
	mockResponse := `{
		"text": "Speaker 0: So how long did you stay in line to visit the store?\nSpeaker 1: Twenty minutes.\nSpeaker 0: Was it worth the twenty minute line?\nSpeaker 1: Yes, it was fun.",
		"duration": 45.0,
		"noSpeechDetected": false
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(mockResponse))
	}))
	defer server.Close()

	cfg := config.Config{
		LLMBaseURL: server.URL,
		LLMAPIKey:  "test-key",
		STTModel:   "deepgram/nova-2",
	}

	adapter := stt.NewOmniRoute(cfg)
	res, err := adapter.Transcribe(context.Background(), bytes.NewReader([]byte("audio")), "sample.ogg", dtos.STTOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(res.Segments) != 4 {
		t.Fatalf("expected 4 diarized segments, got %d", len(res.Segments))
	}

	if res.Segments[0].SpeakerLabel != "Speaker 0" {
		t.Errorf("expected Speaker 0, got %s", res.Segments[0].SpeakerLabel)
	}
	if res.Segments[1].SpeakerLabel != "Speaker 1" {
		t.Errorf("expected Speaker 1, got %s", res.Segments[1].SpeakerLabel)
	}
	if res.Segments[0].Text != "So how long did you stay in line to visit the store?" {
		t.Errorf("expected parsed text without prefix, got %q", res.Segments[0].Text)
	}
	if res.Segments[1].Text != "Twenty minutes." {
		t.Errorf("expected parsed text without prefix, got %q", res.Segments[1].Text)
	}
	if res.Language != "en" {
		t.Errorf("expected detected language 'en', got %q", res.Language)
	}
}

func TestOmniRouteSTT_Transcribe_ParallelDualModelDiarization(t *testing.T) {
	primaryResponse := `{
		"task": "transcribe",
		"language": "id",
		"duration": 20.0,
		"text": "Kita mulai pembahasan arsitektur pipeline.",
		"segments": [
			{"id": 0, "start": 0.0, "end": 20.0, "text": "Kita mulai pembahasan arsitektur pipeline."}
		]
	}`

	diarizeResponse := `{
		"task": "transcribe",
		"language": "id",
		"duration": 20.0,
		"text": "Kita mulai pembahasan arsitektur pipeline.",
		"segments": [
			{"id": 0, "start": 0.0, "end": 10.0, "text": "Kita mulai pembahasan arsitektur pipeline.", "speaker": 0},
			{"id": 1, "start": 10.1, "end": 20.0, "text": "Siap, silakan share screen.", "speaker": 1}
		]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseMultipartForm(10 << 20)
		if err != nil {
			t.Errorf("failed to parse multipart form: %v", err)
		}
		model := r.FormValue("model")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if model == "deepgram/nova-3" {
			_, _ = w.Write([]byte(diarizeResponse))
		} else {
			_, _ = w.Write([]byte(primaryResponse))
		}
	}))
	defer server.Close()

	cfg := config.Config{
		LLMBaseURL:      server.URL,
		LLMAPIKey:       "test-key",
		STTModel:        "groq/whisper-large-v3",
		STTDiarizeModel: "deepgram/nova-3",
	}

	adapter := stt.NewOmniRoute(cfg)
	res, err := adapter.Transcribe(context.Background(), bytes.NewReader([]byte("audio-bytes")), "test.mp3", dtos.STTOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(res.DiarizedText, "Speaker 0 [0.00s - 10.00s]:") {
		t.Errorf("expected DiarizedText to contain Speaker 0 timestamp, got: %s", res.DiarizedText)
	}
	if !strings.Contains(res.DiarizedText, "Speaker 1 [10.10s - 20.00s]:") {
		t.Errorf("expected DiarizedText to contain Speaker 1 timestamp, got: %s", res.DiarizedText)
	}
	if len(res.DiarizedSegments) != 2 {
		t.Errorf("expected 2 DiarizedSegments, got %d", len(res.DiarizedSegments))
	}
	if len(res.Segments) == 0 || res.Segments[0].SpeakerLabel == "" {
		t.Errorf("expected primary segments to be aligned with speaker labels, got %+v", res.Segments)
	}
}

func TestOmniRouteSTT_Transcribe_RootWordsDistribution(t *testing.T) {
	// Simulates OpenAI Whisper format: segments have empty words array, but root words array exists
	openAIWhisperResponse := `{
		"task": "transcribe",
		"language": "id",
		"duration": 5.0,
		"text": "Kita mulai pembahasan arsitektur.",
		"words": [
			{"word": "Kita", "start": 0.0, "end": 0.5},
			{"word": "mulai", "start": 0.6, "end": 1.2},
			{"word": "pembahasan", "start": 1.3, "end": 2.2},
			{"word": "arsitektur.", "start": 2.3, "end": 3.5}
		],
		"segments": [
			{"id": 0, "start": 0.0, "end": 4.0, "text": "Kita mulai pembahasan arsitektur."}
		]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(openAIWhisperResponse))
	}))
	defer server.Close()

	cfg := config.Config{
		LLMBaseURL: server.URL,
		STTModel:   "whisper-1",
	}

	adapter := stt.NewOmniRoute(cfg)
	res, err := adapter.Transcribe(context.Background(), bytes.NewReader([]byte("audio")), "test.mp3", dtos.STTOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(res.Segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(res.Segments))
	}
	if len(res.Segments[0].Words) < 4 {
		t.Fatalf("expected at least 4 words distributed into segment, got %d", len(res.Segments[0].Words))
	}
	if res.Segments[0].Words[0].Word != "Kita" {
		t.Errorf("expected first word to be 'Kita', got %s", res.Segments[0].Words[0].Word)
	}
}

func TestOmniRouteSTT_Transcribe_BuildSegmentsFromWords(t *testing.T) {
	// Simulates Deepgram format with no pre-packaged segments, but words with speaker labels
	deepgramWordResponse := `{
		"task": "transcribe",
		"language": "id",
		"duration": 6.0,
		"text": "Selamat siang. Selamat siang juga Pak.",
		"words": [
			{"word": "Selamat", "start": 0.0, "end": 0.4, "speaker": 0},
			{"word": "siang.", "start": 0.5, "end": 0.9, "speaker": 0},
			{"word": "Selamat", "start": 2.0, "end": 2.4, "speaker": 1},
			{"word": "siang", "start": 2.5, "end": 2.8, "speaker": 1},
			{"word": "juga", "start": 2.9, "end": 3.2, "speaker": 1},
			{"word": "Pak.", "start": 3.3, "end": 3.6, "speaker": 1}
		],
		"segments": []
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(deepgramWordResponse))
	}))
	defer server.Close()

	cfg := config.Config{
		LLMBaseURL: server.URL,
		STTModel:   "deepgram/nova-3",
	}

	adapter := stt.NewOmniRoute(cfg)
	res, err := adapter.Transcribe(context.Background(), bytes.NewReader([]byte("audio")), "test.mp3", dtos.STTOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(res.Segments) != 2 {
		t.Fatalf("expected 2 segments constructed from words, got %d", len(res.Segments))
	}
	if res.Segments[0].SpeakerLabel != "Speaker 0" {
		t.Errorf("expected segment 0 to be Speaker 0, got %s", res.Segments[0].SpeakerLabel)
	}
	if res.Segments[1].SpeakerLabel != "Speaker 1" {
		t.Errorf("expected segment 1 to be Speaker 1, got %s", res.Segments[1].SpeakerLabel)
	}
	if res.Segments[0].Text != "Selamat siang." {
		t.Errorf("unexpected segment 0 text: %s", res.Segments[0].Text)
	}
	if res.Segments[1].Text != "Selamat siang juga Pak." {
		t.Errorf("unexpected segment 1 text: %s", res.Segments[1].Text)
	}
}

func TestStitchChunkResults(t *testing.T) {
	chunks := []dtos.AudioChunk{
		{Index: 0, StartOffsetSec: 0.0, DurationSeconds: 300.0},
		{Index: 1, StartOffsetSec: 300.0, DurationSeconds: 300.0},
	}
	results := []*dtos.TranscriptionResult{
		{
			Text:     "First chunk intro.",
			Duration: 290.0,
			Language: "id",
			Segments: []dtos.SegmentResult{
				{
					ID:    0,
					Start: 5.0,
					End:   15.0,
					Text:  "First chunk intro.",
					Words: []dtos.WordResult{
						{Word: "First", Start: 5.0, End: 7.0},
						{Word: "chunk", Start: 7.0, End: 10.0},
						{Word: "intro.", Start: 10.0, End: 15.0},
					},
					SpeakerLabel: "Speaker 0",
				},
			},
		},
		{
			Text:     "Second chunk dialogue.",
			Duration: 280.0,
			Language: "id",
			Segments: []dtos.SegmentResult{
				{
					ID:    0,
					Start: 10.0,
					End:   25.0,
					Text:  "Second chunk dialogue.",
					Words: []dtos.WordResult{
						{Word: "Second", Start: 10.0, End: 15.0},
						{Word: "chunk", Start: 15.0, End: 18.0},
						{Word: "dialogue.", Start: 18.0, End: 25.0},
					},
					SpeakerLabel: "Speaker 1",
				},
			},
		},
	}

	stitched := stt.StitchChunkResults(chunks, results)
	if stitched == nil {
		t.Fatal("expected non-nil stitched result")
	}

	if stitched.Text != "First chunk intro. Second chunk dialogue." {
		t.Errorf("unexpected stitched text: %s", stitched.Text)
	}

	if len(stitched.Segments) != 2 {
		t.Fatalf("expected 2 stitched segments, got %d", len(stitched.Segments))
	}

	// Verify segment 0
	if stitched.Segments[0].Start != 5.0 || stitched.Segments[0].End != 15.0 {
		t.Errorf("expected chunk 0 segment [5, 15], got [%f, %f]", stitched.Segments[0].Start, stitched.Segments[0].End)
	}
	if stitched.Segments[0].Words[0].Start != 5.0 || stitched.Segments[0].Words[0].End != 7.0 {
		t.Errorf("expected word 0 [5, 7], got [%f, %f]", stitched.Segments[0].Words[0].Start, stitched.Segments[0].Words[0].End)
	}

	// Verify segment 1 (offset by 300s: 300 + 10 = 310, 300 + 25 = 325)
	if stitched.Segments[1].Start != 310.0 || stitched.Segments[1].End != 325.0 {
		t.Errorf("expected chunk 1 segment [310, 325], got [%f, %f]", stitched.Segments[1].Start, stitched.Segments[1].End)
	}
	if stitched.Segments[1].Words[0].Start != 310.0 || stitched.Segments[1].Words[0].End != 315.0 {
		t.Errorf("expected word 0 in chunk 1 [310, 315], got [%f, %f]", stitched.Segments[1].Words[0].Start, stitched.Segments[1].Words[0].End)
	}
}

func TestMockSTT_TranscribeChunksParallel(t *testing.T) {
	mock := stt.NewMock()
	chunks := []dtos.AudioChunk{
		{Index: 0, StartOffsetSec: 0.0, DurationSeconds: 10.0, Data: []byte("chunk0")},
		{Index: 1, StartOffsetSec: 10.0, DurationSeconds: 10.0, Data: []byte("chunk1")},
	}

	res, err := mock.TranscribeChunksParallel(context.Background(), chunks, dtos.STTOptions{Language: "id"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil {
		t.Fatal("expected non-nil result")
	}
	if len(res.Segments) != 4 { // 2 segments per chunk * 2 chunks = 4 segments
		t.Errorf("expected 4 stitched segments, got %d", len(res.Segments))
	}
}

