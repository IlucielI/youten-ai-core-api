package services

import (
	"context"
	"testing"

	"code-base-golang/internal/dtos"
)

func TestService_NeedsDiarization(t *testing.T) {
	svc := &Service{}

	// Case 1: Empty or default speaker labels -> needs diarization
	segments1 := []dtos.SegmentResult{
		{Text: "Halo", SpeakerLabel: ""},
		{Text: "Hai", SpeakerLabel: "Speaker 0"},
	}
	if !svc.needsDiarization(segments1) {
		t.Errorf("expected needsDiarization to be true for empty/default labels")
	}

	// Case 2: Already distinct speaker labels -> does not need diarization
	segments2 := []dtos.SegmentResult{
		{Text: "Halo", SpeakerLabel: "Speaker 0"},
		{Text: "Hai", SpeakerLabel: "Speaker 1"},
	}
	if svc.needsDiarization(segments2) {
		t.Errorf("expected needsDiarization to be false when distinct labels exist")
	}
}

func TestService_DiarizeSegmentsWithLLM(t *testing.T) {
	mockLLM := &dummyLLM{
		chatResp: `{"0": "Speaker 0", "1": "Speaker 1"}`,
	}
	svc := &Service{llm: mockLLM}

	segments := []dtos.SegmentResult{
		{Text: "Selamat pagi Pak", SpeakerLabel: ""},
		{Text: "Selamat pagi silakan duduk", SpeakerLabel: ""},
	}

	svc.diarizeSegmentsWithLLM(context.Background(), segments)

	if segments[0].SpeakerLabel != "Speaker 0" {
		t.Errorf("expected Speaker 0, got %s", segments[0].SpeakerLabel)
	}
	if segments[1].SpeakerLabel != "Speaker 1" {
		t.Errorf("expected Speaker 1, got %s", segments[1].SpeakerLabel)
	}
}

func TestService_DiarizeSegmentsWithLLM_EmptyOrNil(t *testing.T) {
	svc := &Service{llm: nil}
	segments := []dtos.SegmentResult{
		{Text: "Halo", SpeakerLabel: ""},
	}
	// Should not panic when LLM is nil or segments <= 1
	svc.diarizeSegmentsWithLLM(context.Background(), segments)
	if segments[0].SpeakerLabel != "" {
		t.Errorf("expected SpeakerLabel to remain unchanged")
	}
}
