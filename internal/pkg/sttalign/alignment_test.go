package sttalign_test

import (
	"strings"
	"testing"

	"code-base-golang/internal/dtos"
	"code-base-golang/internal/pkg/sttalign"
)

func TestCalculateOverlap(t *testing.T) {
	// Normal overlap
	if ov := sttalign.CalculateOverlap(0.0, 5.0, 3.0, 8.0); ov != 2.0 {
		t.Errorf("expected overlap 2.0, got %f", ov)
	}
	// Fully contained
	if ov := sttalign.CalculateOverlap(2.0, 4.0, 0.0, 10.0); ov != 2.0 {
		t.Errorf("expected overlap 2.0, got %f", ov)
	}
	// No overlap (before)
	if ov := sttalign.CalculateOverlap(0.0, 2.0, 3.0, 5.0); ov != 0.0 {
		t.Errorf("expected overlap 0.0, got %f", ov)
	}
	// No overlap (after)
	if ov := sttalign.CalculateOverlap(5.0, 10.0, 0.0, 4.0); ov != 0.0 {
		t.Errorf("expected overlap 0.0, got %f", ov)
	}
}

func TestFindBestAcousticSpeaker(t *testing.T) {
	diarized := []dtos.SegmentResult{
		{Start: 0.0, End: 10.0, SpeakerLabel: "Speaker 0"},
		{Start: 10.5, End: 20.0, SpeakerLabel: "Speaker 1"},
	}

	// Range [2.0, 8.0] is 100% inside Speaker 0
	spk, maxOv, total := sttalign.FindBestAcousticSpeaker(2.0, 8.0, diarized)
	if spk != "Speaker 0" || maxOv != 6.0 || total != 6.0 {
		t.Errorf("expected Speaker 0 with 6.0 overlap, got %s, maxOv=%f, total=%f", spk, maxOv, total)
	}

	// Range [9.0, 15.0] overlaps 1.0s with Speaker 0 and 4.5s with Speaker 1
	spk2, maxOv2, total2 := sttalign.FindBestAcousticSpeaker(9.0, 15.0, diarized)
	if spk2 != "Speaker 1" || maxOv2 != 4.5 || total2 != 5.5 {
		t.Errorf("expected Speaker 1 with 4.5 overlap, got %s, maxOv=%f, total=%f", spk2, maxOv2, total2)
	}
}

func TestAlignSegments_WordLevelTurnSplitting(t *testing.T) {
	// A single Whisper segment spanning [10.0 - 20.0] where:
	// Words 0..3: Putri asks "Apakah ada lowongan accounting?" [10.0 - 13.5] -> Speaker 0
	// Words 4..8: Interviewer answers "Iya, kita membuka posisi tersebut." [14.0 - 19.5] -> Speaker 1
	whisperSeg := dtos.SegmentResult{
		ID:    0,
		Start: 10.0,
		End:   20.0,
		Text:  "Apakah ada lowongan accounting? Iya, kita membuka posisi tersebut.",
		Words: []dtos.WordResult{
			{Word: "Apakah", Start: 10.0, End: 10.6},
			{Word: "ada", Start: 10.7, End: 11.2},
			{Word: "lowongan", Start: 11.3, End: 12.2},
			{Word: "accounting?", Start: 12.3, End: 13.5},
			{Word: "Iya,", Start: 14.0, End: 14.6},
			{Word: "kita", Start: 14.7, End: 15.2},
			{Word: "membuka", Start: 15.3, End: 16.5},
			{Word: "posisi", Start: 16.6, End: 17.8},
			{Word: "tersebut.", Start: 17.9, End: 19.5},
		},
	}

	diarized := []dtos.SegmentResult{
		{Start: 10.0, End: 13.8, SpeakerLabel: "Speaker 0"},
		{Start: 13.9, End: 20.0, SpeakerLabel: "Speaker 1"},
	}

	aligned := sttalign.AlignSegments([]dtos.SegmentResult{whisperSeg}, diarized)

	if len(aligned) != 2 {
		t.Fatalf("expected Whisper segment to be split into 2 segments at speaker turn, got %d", len(aligned))
	}

	// Segment 0: Putri
	if aligned[0].SpeakerLabel != "Speaker 0" {
		t.Errorf("expected segment 0 to be Speaker 0, got %s", aligned[0].SpeakerLabel)
	}
	if !strings.Contains(aligned[0].Text, "Apakah ada lowongan accounting?") {
		t.Errorf("expected segment 0 text to be Putri's question, got %q", aligned[0].Text)
	}
	if aligned[0].Start != 10.0 || aligned[0].End != 13.5 {
		t.Errorf("expected segment 0 time [10.0 - 13.5], got [%.2f - %.2f]", aligned[0].Start, aligned[0].End)
	}

	// Segment 1: Interviewer
	if aligned[1].SpeakerLabel != "Speaker 1" {
		t.Errorf("expected segment 1 to be Speaker 1, got %s", aligned[1].SpeakerLabel)
	}
	if !strings.Contains(aligned[1].Text, "Iya, kita membuka posisi tersebut.") {
		t.Errorf("expected segment 1 text to be Interviewer's answer, got %q", aligned[1].Text)
	}
	if aligned[1].Start != 14.0 || aligned[1].End != 19.5 {
		t.Errorf("expected segment 1 time [14.0 - 19.5], got [%.2f - %.2f]", aligned[1].Start, aligned[1].End)
	}
}

func TestAlignSegments_ClauseSplittingWithoutWordTimestamps(t *testing.T) {
	// Single Whisper segment with no word timestamps
	whisperSeg := dtos.SegmentResult{
		ID:    0,
		Start: 0.0,
		End:   10.0,
		Text:  "Bisakah Anda menceritakan pengalaman Anda? Tentu, saya pernah bekerja di bidang data selama 2 tahun.",
	}

	diarized := []dtos.SegmentResult{
		{Start: 0.0, End: 4.5, SpeakerLabel: "Speaker 1"},
		{Start: 4.8, End: 10.0, SpeakerLabel: "Speaker 0"},
	}

	aligned := sttalign.AlignSegments([]dtos.SegmentResult{whisperSeg}, diarized)

	if len(aligned) != 2 {
		t.Fatalf("expected segment to be split on sentence boundary, got %d", len(aligned))
	}

	if aligned[0].SpeakerLabel != "Speaker 1" || !strings.Contains(aligned[0].Text, "Bisakah Anda menceritakan") {
		t.Errorf("unexpected part 1: %+v", aligned[0])
	}
	if aligned[1].SpeakerLabel != "Speaker 0" || !strings.Contains(aligned[1].Text, "Tentu, saya pernah bekerja") {
		t.Errorf("unexpected part 2: %+v", aligned[1])
	}
}

func TestAlignSegments_MultiSpeakerTenParticipants(t *testing.T) {
	var diarized []dtos.SegmentResult
	for i := 0; i < 10; i++ {
		diarized = append(diarized, dtos.SegmentResult{
			Start:        float64(i * 10),
			End:          float64((i + 1) * 10),
			SpeakerLabel: strings.TrimSpace("Speaker " + string(rune('0'+i))),
		})
	}

	var whisperSegs []dtos.SegmentResult
	for i := 0; i < 10; i++ {
		whisperSegs = append(whisperSegs, dtos.SegmentResult{
			ID:    i,
			Start: float64(i*10 + 1),
			End:   float64(i*10 + 8),
			Text:  "Laporan dari divisi " + string(rune('0'+i)),
		})
	}

	aligned := sttalign.AlignSegments(whisperSegs, diarized)
	if len(aligned) != 10 {
		t.Fatalf("expected 10 aligned segments, got %d", len(aligned))
	}

	for i := 0; i < 10; i++ {
		expectedSpeaker := "Speaker " + string(rune('0'+i))
		if aligned[i].SpeakerLabel != expectedSpeaker {
			t.Errorf("segment %d expected %s, got %s", i, expectedSpeaker, aligned[i].SpeakerLabel)
		}
	}
}

func TestSliceAcousticReferenceByTime_StructuredSegments(t *testing.T) {
	diarized := []dtos.SegmentResult{
		{Start: 0.0, End: 10.0, SpeakerLabel: "Speaker 0", Text: "Opening intro"},
		{Start: 120.0, End: 140.0, SpeakerLabel: "Speaker 1", Text: "Minute 2 discussion"},
		{Start: 2800.0, End: 2820.0, SpeakerLabel: "Speaker 0", Text: "Minute 46 closing"},
	}

	// Request slice for minute 2 [125.0 - 135.0]
	slice := sttalign.SliceAcousticReferenceByTime(diarized, "", 125.0, 135.0, 3000.0, 10.0)

	if !strings.Contains(slice, "Minute 2 discussion") {
		t.Errorf("expected slice to contain minute 2 discussion, got: %s", slice)
	}
	if strings.Contains(slice, "Opening intro") || strings.Contains(slice, "Minute 46 closing") {
		t.Errorf("slice should NOT contain far-away segments, got: %s", slice)
	}
}

func TestSliceAcousticReferenceByTime_ParsedStringTimestamps(t *testing.T) {
	rawRef := "Speaker 0 [0.00s - 10.00s]: Opening intro\n" +
		"Speaker 1 [120.00s - 140.00s]: Minute 2 discussion\n" +
		"Speaker 0 [2800.00s - 2820.00s]: Minute 46 closing\n"

	slice := sttalign.SliceAcousticReferenceByTime(nil, rawRef, 125.0, 135.0, 3000.0, 10.0)

	if !strings.Contains(slice, "Minute 2 discussion") {
		t.Errorf("expected parsed string slice to contain minute 2 discussion, got: %s", slice)
	}
	if strings.Contains(slice, "Minute 46 closing") {
		t.Errorf("slice should NOT contain minute 46, got: %s", slice)
	}
}

func TestDetectAndCorrectSpeakerInversion(t *testing.T) {
	// Original acoustic assignments
	original := []dtos.SegmentResult{
		{AcousticSpeaker: "Speaker 0"},
		{AcousticSpeaker: "Speaker 0"},
		{AcousticSpeaker: "Speaker 0"},
		{AcousticSpeaker: "Speaker 1"},
		{AcousticSpeaker: "Speaker 1"},
		{AcousticSpeaker: "Speaker 1"},
	}

	// LLM hallucinates an inverted swap:
	// Speaker 0 -> Speaker 1
	// Speaker 1 -> Speaker 0
	proposed := []dtos.SegmentResult{
		{SpeakerLabel: "Speaker 1", SpeakerName: "Speaker 1"},
		{SpeakerLabel: "Speaker 1", SpeakerName: "Speaker 1"},
		{SpeakerLabel: "Speaker 1", SpeakerName: "Speaker 1"},
		{SpeakerLabel: "Speaker 0", SpeakerName: "Speaker 0"},
		{SpeakerLabel: "Speaker 0", SpeakerName: "Speaker 0"},
		{SpeakerLabel: "Speaker 0", SpeakerName: "Speaker 0"},
	}

	corrected := sttalign.DetectAndCorrectSpeakerInversion(original, proposed)

	// Inversion should be detected and reversed back to Speaker 0 for first 3, Speaker 1 for last 3
	for i := 0; i < 3; i++ {
		if corrected[i].SpeakerLabel != "Speaker 0" {
			t.Errorf("segment %d expected Speaker 0 after inversion correction, got %s", i, corrected[i].SpeakerLabel)
		}
	}
	for i := 3; i < 6; i++ {
		if corrected[i].SpeakerLabel != "Speaker 1" {
			t.Errorf("segment %d expected Speaker 1 after inversion correction, got %s", i, corrected[i].SpeakerLabel)
		}
	}
}

func TestFilterHallucinationsAndNoise(t *testing.T) {
	input := []dtos.SegmentResult{
		{Start: 0.0, End: 3.0, Text: "[Musik]"},
		{Start: 3.5, End: 8.0, Text: "Selamat pagi semuanya."},
		{Start: 8.2, End: 8.5, Text: "(applause)"},
		{Start: 8.6, End: 12.0, Text: "Hari ini kita mulai sesi evaluasi."},
		{Start: 12.1, End: 13.0, Text: "Hari ini kita mulai sesi evaluasi."}, // Stutter duplicate loop
	}

	filtered := sttalign.FilterHallucinationsAndNoise(input)

	if len(filtered) != 2 {
		t.Fatalf("expected 2 legitimate speech segments, got %d", len(filtered))
	}
	if filtered[0].Text != "Selamat pagi semuanya." {
		t.Errorf("expected segment 0 to be 'Selamat pagi semuanya.', got %q", filtered[0].Text)
	}
	if filtered[1].Text != "Hari ini kita mulai sesi evaluasi." {
		t.Errorf("expected segment 1 to be 'Hari ini kita mulai sesi evaluasi.', got %q", filtered[1].Text)
	}
}

func TestIsNonSpeechToken(t *testing.T) {
	cases := []struct {
		text     string
		expected bool
	}{
		{"[musik]", true},
		{"[Music]", true},
		{"(applause)", true},
		{"(tertawa)", true},
		{"...", true},
		{"", true},
		{"Selamat siang rekan-rekan", false},
		{"Kami akan menjelaskan arsitektur", false},
	}

	for _, c := range cases {
		if res := sttalign.IsNonSpeechToken(c.text); res != c.expected {
			t.Errorf("IsNonSpeechToken(%q): expected %v, got %v", c.text, c.expected, res)
		}
	}
}
