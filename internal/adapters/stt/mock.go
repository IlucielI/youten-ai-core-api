package stt

import (
	"context"
	"io"

	"code-base-golang/internal/dtos"
	"code-base-golang/internal/services"
)

// MockSTT provides a mockable implementation of services.STTProvider for unit and offline tests.
type MockSTT struct {
	TranscribeFunc func(ctx context.Context, reader io.Reader, filename string, opts dtos.STTOptions) (*dtos.TranscriptionResult, error)
}

// NewMock creates a new MockSTT with realistic default responses.
func NewMock() *MockSTT {
	return &MockSTT{}
}

// Transcribe executes TranscribeFunc if provided, or returns realistic sample transcript data.
func (m *MockSTT) Transcribe(ctx context.Context, reader io.Reader, filename string, opts dtos.STTOptions) (*dtos.TranscriptionResult, error) {
	if m.TranscribeFunc != nil {
		return m.TranscribeFunc(ctx, reader, filename, opts)
	}

	lang := opts.Language
	if lang == "" {
		lang = "english"
	}

	return &dtos.TranscriptionResult{
		Text:     "Welcome to today's project review meeting. We are on track for Q4.",
		Language: lang,
		Duration: 12.5,
		Segments: []dtos.SegmentResult{
			{
				ID:           0,
				Start:        0.0,
				End:          5.5,
				Text:         "Welcome to today's project review meeting.",
				SpeakerLabel: "SPEAKER_00",
				Words: []dtos.WordResult{
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
				Start:        5.5,
				End:          12.5,
				Text:         "We are on track for Q4 deliverables.",
				SpeakerLabel: "SPEAKER_01",
				Words: []dtos.WordResult{
					{Word: "We", Start: 5.5, End: 6.0},
					{Word: "are", Start: 6.0, End: 6.5},
					{Word: "on", Start: 6.5, End: 7.0},
					{Word: "track", Start: 7.0, End: 8.0},
					{Word: "for", Start: 8.0, End: 8.5},
					{Word: "Q4", Start: 8.5, End: 9.5},
					{Word: "deliverables.", Start: 9.5, End: 12.5},
				},
			},
		},
	}, nil
}

// Ensure MockSTT satisfies services.STTProvider at compile time.
var _ services.STTProvider = (*MockSTT)(nil)
