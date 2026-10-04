package stt

import (
	"context"
	"io"

	"code-base-golang/internal/services"
)

// MockSTT provides a mockable implementation of services.STTProvider for unit and offline tests.
type MockSTT struct {
	TranscribeFunc func(ctx context.Context, reader io.Reader, filename string, opts services.STTOptions) (*services.TranscriptionResult, error)
}

// NewMock creates a new MockSTT with realistic default responses.
func NewMock() *MockSTT {
	return &MockSTT{}
}

// Transcribe executes TranscribeFunc if provided, or returns realistic sample transcript data.
func (m *MockSTT) Transcribe(ctx context.Context, reader io.Reader, filename string, opts services.STTOptions) (*services.TranscriptionResult, error) {
	if m.TranscribeFunc != nil {
		return m.TranscribeFunc(ctx, reader, filename, opts)
	}

	lang := opts.Language
	if lang == "" {
		lang = "english"
	}

	return &services.TranscriptionResult{
		Text:     "Welcome to today's project review meeting. We are on track for Q4.",
		Language: lang,
		Duration: 12.5,
		Segments: []services.SegmentResult{
			{
				ID:           0,
				Start:        0.0,
				End:          5.5,
				Text:         "Welcome to today's project review meeting.",
				SpeakerLabel: "SPEAKER_00",
				Words: []services.WordResult{
					{Word: "Welcome", Start: 0.0, End: 0.8},
					{Word: "to", Start: 0.8, End: 1.0},
					{Word: "today's", Start: 1.0, End: 1.8},
					{Word: "project", Start: 1.8, End: 2.5},
					{Word: "review", Start: 2.5, End: 3.2},
					{Word: "meeting.", Start: 3.2, End: 5.5},
				},
			},
			{
				ID:           1,
				Start:        6.0,
				End:          12.5,
				Text:         "We are on track for Q4.",
				SpeakerLabel: "SPEAKER_01",
				Words: []services.WordResult{
					{Word: "We", Start: 6.0, End: 6.5},
					{Word: "are", Start: 6.5, End: 7.2},
					{Word: "on", Start: 7.2, End: 7.8},
					{Word: "track", Start: 7.8, End: 9.0},
					{Word: "for", Start: 9.0, End: 9.5},
					{Word: "Q4.", Start: 9.5, End: 12.5},
				},
			},
		},
	}, nil
}
