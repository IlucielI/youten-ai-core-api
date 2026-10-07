package services

import (
	"context"
	"testing"

	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
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

func TestService_DiarizeAndCorrectSegmentsWithLLM_PropagatesDiscoveredName(t *testing.T) {
	mockLLM := &dummyLLM{
		chatResp: `[
			{"index": 0, "speaker": "Speaker 0", "speaker_name": "Speaker 0", "text": "Selamat pagi Pak."},
			{"index": 1, "speaker": "Speaker 1", "speaker_name": "Speaker 1", "text": "Siapa nama kamu?"},
			{"index": 2, "speaker": "Speaker 0", "speaker_name": "Putri", "text": "Nama saya Putri."},
			{"index": 3, "speaker": "Speaker 0", "speaker_name": "Speaker 0", "text": "Saya lulusan Akuntansi."}
		]`,
	}
	svc := &Service{llm: mockLLM}

	segments := []dtos.SegmentResult{
		{Text: "Selamat pagi Pak.", SpeakerLabel: "Speaker 0"},
		{Text: "Siapa nama kamu?", SpeakerLabel: "Speaker 1"},
		{Text: "Nama saya Putri.", SpeakerLabel: "Speaker 0"},
		{Text: "Saya lulusan Akuntansi.", SpeakerLabel: "Speaker 0"},
	}

	svc.diarizeAndCorrectSegmentsWithLLM(context.Background(), segments)

	// Speaker 0 should now be "Putri" on index 0, 2, and 3
	if segments[0].SpeakerName != "Putri" {
		t.Errorf("expected segment 0 speaker_name 'Putri', got %q", segments[0].SpeakerName)
	}
	if segments[2].SpeakerName != "Putri" {
		t.Errorf("expected segment 2 speaker_name 'Putri', got %q", segments[2].SpeakerName)
	}
	if segments[3].SpeakerName != "Putri" {
		t.Errorf("expected segment 3 speaker_name 'Putri', got %q", segments[3].SpeakerName)
	}
	// Speaker 1 should retain fallback "Speaker 1"
	if segments[1].SpeakerName != "Speaker 1" {
		t.Errorf("expected segment 1 speaker_name 'Speaker 1', got %q", segments[1].SpeakerName)
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

func TestMergeConsecutiveSegments_FragmentedSentence(t *testing.T) {
	input := []models.TranscriptSegment{
		{SpeakerLabel: "Speaker 0", SpeakerName: "Putri", StartTime: 306.0, EndTime: 307.0, Text: "Yang saya ingin tanyakan,"},
		{SpeakerLabel: "Speaker 0", SpeakerName: "Putri", StartTime: 307.0, EndTime: 308.0, Text: "apakah"},
		{SpeakerLabel: "Speaker 0", SpeakerName: "Putri", StartTime: 308.0, EndTime: 310.0, Text: "job desk yang"},
		{SpeakerLabel: "Speaker 0", SpeakerName: "Putri", StartTime: 310.0, EndTime: 313.0, Text: "Bapak berikan untuk saya nantinya?"},
		{SpeakerLabel: "Speaker 1", SpeakerName: "Pewawancara", StartTime: 314.0, EndTime: 320.0, Text: "Untuk job desk kita membuka lowongan staff accounting."},
	}

	merged := mergeConsecutiveSegments(input, 3.0, 45.0)

	if len(merged) != 2 {
		t.Fatalf("expected 2 merged segments (1 for Putri, 1 for Pewawancara), got %d", len(merged))
	}

	// Verify Putri's sentence is unified into one
	expectedText := "Yang saya ingin tanyakan, apakah job desk yang Bapak berikan untuk saya nantinya?"
	if merged[0].Text != expectedText {
		t.Errorf("expected merged text %q, got %q", expectedText, merged[0].Text)
	}
	if merged[0].StartTime != 306.0 || merged[0].EndTime != 313.0 {
		t.Errorf("expected timing 306.0 - 313.0, got %f - %f", merged[0].StartTime, merged[0].EndTime)
	}
	if merged[0].SpeakerName != "Putri" {
		t.Errorf("expected speaker name 'Putri', got %q", merged[0].SpeakerName)
	}
	if merged[0].SequenceOrder != 1 {
		t.Errorf("expected sequence order 1, got %d", merged[0].SequenceOrder)
	}

	// Verify Pewawancara is preserved as distinct turn
	if merged[1].SpeakerName != "Pewawancara" || merged[1].SequenceOrder != 2 {
		t.Errorf("expected Pewawancara with seq 2, got %+v", merged[1])
	}
}

func TestMergeConsecutiveSegments_MaxDurationExceeded(t *testing.T) {
	// If a single speaker talks for longer than maxDuration, split into multiple cards
	input := []models.TranscriptSegment{
		{SpeakerLabel: "Speaker 0", SpeakerName: "Putri", StartTime: 0.0, EndTime: 30.0, Text: "Part 1"},
		{SpeakerLabel: "Speaker 0", SpeakerName: "Putri", StartTime: 30.0, EndTime: 55.0, Text: "Part 2"},
	}

	merged := mergeConsecutiveSegments(input, 3.0, 45.0)

	// Since 30s + 25s = 55s > 45s, they should not be merged
	if len(merged) != 2 {
		t.Fatalf("expected 2 segments due to max duration cap, got %d", len(merged))
	}
}


