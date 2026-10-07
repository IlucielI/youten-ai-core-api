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

func TestService_DiarizeAndCorrectSegmentsWithLLM_ArrayFormat(t *testing.T) {
	mockLLM := &dummyLLM{
		chatResp: `[
			{"index": 0, "speaker": "Speaker 1", "speaker_name": "Pak Budi", "text": "Bisakah anda bekerja di bawah tekanan?"},
			{"index": 1, "speaker": "Speaker 0", "speaker_name": "Speaker 0", "text": "Bisa, siap bekerja di bawah tekanan."},
			{"index": 2, "speaker": "Speaker 0", "speaker_name": "Speaker 0", "text": "Gaji 5 sampai 6 juta."}
		]`,
	}
	svc := &Service{llm: mockLLM}

	segments := []dtos.SegmentResult{
		{Text: "Bisakah anda bekerja di botol kanan?", SpeakerLabel: ""},
		{Text: "Bisa, siap bekerja di botol kanan.", SpeakerLabel: ""},
		{Text: "Gaji 56 juta.", SpeakerLabel: ""},
	}

	svc.diarizeAndCorrectSegmentsWithLLM(context.Background(), segments)

	if segments[0].SpeakerLabel != "Speaker 1" || segments[0].SpeakerName != "Pak Budi" || segments[0].Text != "Bisakah anda bekerja di bawah tekanan?" {
		t.Errorf("unexpected segment 0: %+v", segments[0])
	}
	if segments[1].SpeakerLabel != "Speaker 0" || segments[1].SpeakerName != "Speaker 0" || segments[1].Text != "Bisa, siap bekerja di bawah tekanan." {
		t.Errorf("unexpected segment 1: %+v", segments[1])
	}
	if segments[2].SpeakerLabel != "Speaker 0" || segments[2].SpeakerName != "Speaker 0" || segments[2].Text != "Gaji 5 sampai 6 juta." {
		t.Errorf("unexpected segment 2: %+v", segments[2])
	}
}

func TestService_DiarizeAndCorrectSegmentsWithLLM_MapFormatFallback(t *testing.T) {
	mockLLM := &dummyLLM{
		chatResp: `{"0": "Speaker 0", "1": "Speaker 1"}`,
	}
	svc := &Service{llm: mockLLM}

	segments := []dtos.SegmentResult{
		{Text: "Selamat pagi Pak", SpeakerLabel: ""},
		{Text: "Selamat pagi silakan duduk", SpeakerLabel: ""},
	}

	svc.diarizeAndCorrectSegmentsWithLLM(context.Background(), segments)

	if segments[0].SpeakerLabel != "Speaker 0" {
		t.Errorf("expected Speaker 0, got %s", segments[0].SpeakerLabel)
	}
	if segments[1].SpeakerLabel != "Speaker 1" {
		t.Errorf("expected Speaker 1, got %s", segments[1].SpeakerLabel)
	}
}

func TestService_DiarizeAndCorrectSegmentsWithLLM_EmptyOrNil(t *testing.T) {
	svc := &Service{llm: nil}
	segments := []dtos.SegmentResult{
		{Text: "Halo", SpeakerLabel: ""},
	}
	// Should not panic when LLM is nil
	svc.diarizeAndCorrectSegmentsWithLLM(context.Background(), segments)
	if segments[0].SpeakerLabel != "" {
		t.Errorf("expected SpeakerLabel to remain unchanged")
	}
}

