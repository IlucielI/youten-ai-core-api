package sttalign_test

import (
	"strings"
	"testing"

	"code-base-golang/internal/dtos"
	"code-base-golang/internal/pkg/sttalign"
)

func TestNormalizeSpeakerLabel(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"SPEAKER_00", "Speaker 0"},
		{"SPEAKER_01", "Speaker 1"},
		{"speaker_0", "Speaker 0"},
		{"speaker 2", "Speaker 2"},
		{"[Speaker 3]", "Speaker 3"},
		{"0", "Speaker 0"},
		{"1", "Speaker 1"},
		{"Pembicara 0", "Speaker 0"},
		{"Pembicara 2", "Speaker 2"},
		{"Caesar", "Caesar"},
		{"Putri", "Putri"},
		{"", "Speaker 0"},
	}

	for _, c := range cases {
		if res := sttalign.NormalizeSpeakerLabel(c.input); res != c.expected {
			t.Errorf("NormalizeSpeakerLabel(%q): expected %q, got %q", c.input, c.expected, res)
		}
	}
}

func TestFindBestAcousticSpeaker_CollarGapBridging(t *testing.T) {
	diarized := []dtos.SegmentResult{
		{Start: 0.0, End: 10.0, SpeakerLabel: "Speaker 1"},
		{Start: 11.5, End: 20.0, SpeakerLabel: "Speaker 1"},
	}

	// Range [10.2, 10.8] falls in the silence gap between 10.0 and 11.5 (zero direct overlap),
	// but is within 0.2s of Speaker 1. Collar bridging should attribute it to Speaker 1!
	spk, maxOv, total := sttalign.FindBestAcousticSpeaker(10.2, 10.8, diarized)
	if spk != "Speaker 1" {
		t.Errorf("expected collar gap to bridge to Speaker 1, got %s", spk)
	}
	if maxOv != 0.0 || total != 0.0 {
		t.Errorf("expected 0 direct overlap for gap, got maxOv=%f, total=%f", maxOv, total)
	}
}

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
	if got := strings.TrimSpace(aligned[0].Text); got != "Apakah ada lowongan accounting?" {
		t.Errorf("expected segment 0 text to be Putri's question, got %q", got)
	}
	if aligned[0].Start != 10.0 || aligned[0].End != 13.5 {
		t.Errorf("expected segment 0 time [10.0 - 13.5], got [%.2f - %.2f]", aligned[0].Start, aligned[0].End)
	}

	// Segment 1: Interviewer
	if aligned[1].SpeakerLabel != "Speaker 1" {
		t.Errorf("expected segment 1 to be Speaker 1, got %s", aligned[1].SpeakerLabel)
	}
	if got := strings.TrimSpace(aligned[1].Text); got != "Iya, kita membuka posisi tersebut." {
		t.Errorf("expected segment 1 text to be Interviewer's answer, got %q", got)
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

func TestAlignSegments_ClauseSplittingWithDiscourseMarkers(t *testing.T) {
	// A segment without punctuation where two speakers converse:
	// "Selamat siang saya terima lamaran terima kasih Pak saya Putri"
	// Early acoustic speaker is Speaker 0, late acoustic speaker is Speaker 1
	wSeg := dtos.SegmentResult{
		ID:    0,
		Start: 0.0,
		End:   8.0,
		Text:  "Selamat siang saya terima lamaran terima kasih Pak saya Putri",
	}

	diarized := []dtos.SegmentResult{
		{Start: 0.0, End: 4.0, SpeakerLabel: "Speaker 0"},
		{Start: 4.0, End: 8.0, SpeakerLabel: "Speaker 1"},
	}

	aligned := sttalign.AlignSegments([]dtos.SegmentResult{wSeg}, diarized)
	if len(aligned) != 2 {
		t.Fatalf("expected discourse marker splitting into 2 segments, got %d", len(aligned))
	}
	if aligned[0].SpeakerLabel != "Speaker 0" {
		t.Errorf("expected segment 0 to be Speaker 0, got %s", aligned[0].SpeakerLabel)
	}
	if aligned[1].SpeakerLabel != "Speaker 1" {
		t.Errorf("expected segment 1 to be Speaker 1, got %s", aligned[1].SpeakerLabel)
	}
	if got := strings.TrimSpace(aligned[0].Text); got != "Selamat siang saya terima lamaran" {
		t.Errorf("expected segment 0 text to be 'Selamat siang saya terima lamaran', got %q", got)
	}
	if got := strings.TrimSpace(aligned[1].Text); got != "terima kasih Pak saya Putri" {
		t.Errorf("expected segment 1 text to be 'terima kasih Pak saya Putri', got %q", got)
	}
}

func TestIsAcousticallyAnchored(t *testing.T) {
	segHighConfidence := dtos.SegmentResult{
		Start:              0.0,
		End:                5.0,
		Text:               "Baik, kita lanjutkan pembahasan arsitektur pipeline.",
		AcousticSpeaker:    "Speaker 0",
		AcousticConfidence: 0.90,
	}

	// Case 1: Proposed is different from high confidence acoustic speaker -> anchored!
	if !sttalign.IsAcousticallyAnchored(segHighConfidence, "Speaker 1") {
		t.Errorf("expected segment to be acoustically anchored against speaker swap")
	}

	// Case 2: Proposed is identical to acoustic speaker -> not anchored (no conflict)
	if sttalign.IsAcousticallyAnchored(segHighConfidence, "Speaker 0") {
		t.Errorf("expected same speaker not to trigger anchor violation")
	}

	// Case 3: Self-introduction exception allows name update
	segSelfIntro := dtos.SegmentResult{
		Start:              0.0,
		End:                3.0,
		Text:               "Halo selamat pagi nama saya Putri calon pelamar.",
		AcousticSpeaker:    "Speaker 0",
		AcousticConfidence: 0.85,
	}
	if sttalign.IsAcousticallyAnchored(segSelfIntro, "Speaker 1") {
		t.Errorf("expected self-introduction text to bypass acoustic anchor")
	}

	// Case 4: Low acoustic confidence (< 0.70) does not anchor
	segLowConf := dtos.SegmentResult{
		Start:              0.0,
		End:                2.0,
		Text:               "Iya oke.",
		AcousticSpeaker:    "Speaker 0",
		AcousticConfidence: 0.45,
	}
	if sttalign.IsAcousticallyAnchored(segLowConf, "Speaker 1") {
		t.Errorf("expected low confidence segment not to be anchored")
	}
}

func TestResolveSpeakerLabelAndName(t *testing.T) {
	roster := map[string]dtos.SpeakerRosterItem{
		"Speaker 0": {Label: "Speaker 0", Name: "Pewawancara", Role: "Interviewer"},
		"Speaker 1": {Label: "Speaker 1", Name: "Putri", Role: "Candidate"},
	}

	// Test 1: LLM outputs person's name as speaker -> resolves to canonical label "Speaker 1"
	lbl1, name1 := sttalign.ResolveSpeakerLabelAndName("Putri", "", roster)
	if lbl1 != "Speaker 1" || name1 != "Putri" {
		t.Errorf("expected ('Speaker 1', 'Putri'), got (%q, %q)", lbl1, name1)
	}

	// Test 2: LLM outputs "speaker_0" -> resolves to "Speaker 0" with roster name "Pewawancara"
	lbl2, name2 := sttalign.ResolveSpeakerLabelAndName("speaker_0", "", roster)
	if lbl2 != "Speaker 0" || name2 != "Pewawancara" {
		t.Errorf("expected ('Speaker 0', 'Pewawancara'), got (%q, %q)", lbl2, name2)
	}

	// Test 3: LLM outputs "0" -> resolves to "Speaker 0"
	lbl3, name3 := sttalign.ResolveSpeakerLabelAndName("0", "", roster)
	if lbl3 != "Speaker 0" || name3 != "Pewawancara" {
		t.Errorf("expected ('Speaker 0', 'Pewawancara'), got (%q, %q)", lbl3, name3)
	}
}

func TestBuildSegmentsFromWords(t *testing.T) {
	words := []dtos.WordResult{
		{Word: "Halo", Start: 0.0, End: 0.4, Speaker: "Speaker 0"},
		{Word: "selamat", Start: 0.5, End: 0.9, Speaker: "Speaker 0"},
		{Word: "siang.", Start: 1.0, End: 1.4, Speaker: "Speaker 0"},
		{Word: "Selamat", Start: 2.0, End: 2.3, Speaker: "Speaker 1"},
		{Word: "siang", Start: 2.4, End: 2.7, Speaker: "Speaker 1"},
		{Word: "Pak.", Start: 2.8, End: 3.0, Speaker: "Speaker 1"},
	}

	segments := sttalign.BuildSegmentsFromWords(words, 1.2, 15.0)
	if len(segments) != 2 {
		t.Fatalf("expected 2 segments from word grouping, got %d", len(segments))
	}

	if segments[0].SpeakerLabel != "Speaker 0" || segments[0].Text != "Halo selamat siang." {
		t.Errorf("unexpected segment 0: %+v", segments[0])
	}
	if segments[1].SpeakerLabel != "Speaker 1" || segments[1].Text != "Selamat siang Pak." {
		t.Errorf("unexpected segment 1: %+v", segments[1])
	}
	if len(segments[0].Words) != 3 || len(segments[1].Words) != 3 {
		t.Errorf("expected 3 words per segment, got %d and %d", len(segments[0].Words), len(segments[1].Words))
	}

	// Test case: Word without speaker tag inherits current speaker instead of switching to "Speaker 0"
	wordsWithEmpty := []dtos.WordResult{
		{Word: "Selamat", Start: 0.0, End: 0.5, Speaker: "Speaker 1"},
		{Word: "pagi", Start: 0.6, End: 1.0, Speaker: ""}, // unlabelled word
		{Word: "semua.", Start: 1.1, End: 1.5, Speaker: "Speaker 1"},
	}
	segmentsEmpty := sttalign.BuildSegmentsFromWords(wordsWithEmpty, 1.2, 15.0)
	if len(segmentsEmpty) != 1 {
		t.Fatalf("expected 1 segment without spurious split, got %d", len(segmentsEmpty))
	}
	if segmentsEmpty[0].SpeakerLabel != "Speaker 1" {
		t.Errorf("expected Speaker 1 to be maintained across unlabelled word, got %s", segmentsEmpty[0].SpeakerLabel)
	}
}

func TestFindBestAcousticSpeaker_IgnoresEmptySpeakerLabels(t *testing.T) {
	diarized := []dtos.SegmentResult{
		{Start: 0.0, End: 5.0, SpeakerLabel: ""}, // Empty speaker label should NOT be treated as Speaker 0
		{Start: 1.0, End: 4.0, SpeakerLabel: "Speaker 1"},
	}

	spk, maxOv, totalOv := sttalign.FindBestAcousticSpeaker(0.5, 4.5, diarized)
	if spk != "Speaker 1" {
		t.Errorf("expected 'Speaker 1', got %q", spk)
	}
	if maxOv <= 0 {
		t.Errorf("expected positive max overlap, got %f", maxOv)
	}
	if totalOv != maxOv {
		t.Errorf("expected total overlap to equal max overlap for Speaker 1, got total=%f, max=%f", totalOv, maxOv)
	}
}

func TestIsNonSpeechToken_BracketedMarkers(t *testing.T) {
	testCases := []struct {
		input    string
		expected bool
	}{
		{"[musik piano]", true},
		{"[applause]", true},
		{"(tertawa)", true},
		{"[tepuk tangan penonton]", true},
		{"(suara batuk)", true},
		{"Halo nama saya Budi", false},
	}

	for _, tc := range testCases {
		res := sttalign.IsNonSpeechToken(tc.input)
		if res != tc.expected {
			t.Errorf("IsNonSpeechToken(%q) = %v, want %v", tc.input, res, tc.expected)
		}
	}
}

func TestDetectCompoundDialogueSplitPoints(t *testing.T) {
	cases := []struct {
		name          string
		text          string
		expectedCount int
	}{
		{
			name:          "Question followed by answer",
			text:          "sudah makan belum? Alhamdulillah sudah Pak",
			expectedCount: 1,
		},
		{
			name:          "Greeting followed by return greeting",
			text:          "Selamat siang Pak, ini surat lamaran dan CV saya. Selamat siang saya terima surat lamaran CV-nya.",
			expectedCount: 1,
		},
		{
			name:          "Candidate answer with interjected interviewer question and answer",
			text:          "Karena sesuai dengan jurusan saya, yaitu akuntansi, saya tertarik melamar di bagian accounting. Lalu, kontribusi apa yang akan kamu lakukan di perusahaan ini? Saya berdedikasi tinggi untuk perusahaan dan saya mampu mengerjakan tugas-tugas akuntansi.",
			expectedCount: 2,
		},
		{
			name:          "Meeting rapid exchange",
			text:          "satu e-wallet. Nah, e-wallet-nya apa? Bebas. Bebas. Oke, nanti kredensial e-wallet-nya itu yang diproduksi itu gimana?",
			expectedCount: 2,
		},
		{
			name:          "Single speaker question with self clarification",
			text:          "Berarti ke mana ya? Berarti K224 atau K226 ya?",
			expectedCount: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			points := sttalign.DetectCompoundDialogueSplitPoints(tc.text)
			if len(points) != tc.expectedCount {
				t.Errorf("expected %d split points, got %d (points: %v)", tc.expectedCount, len(points), points)
			}
		})
	}
}

func TestSplitCompoundDialogue_RealInterviewQnA(t *testing.T) {
	wSeg := dtos.SegmentResult{
		ID:           7,
		Start:        50.0,
		End:          52.8,
		Text:         "sudah makan belum? Alhamdulillah sudah Pak",
		SpeakerLabel: "Speaker 0",
	}

	split := sttalign.SplitCompoundDialogue(wSeg, nil)
	if len(split) != 2 {
		t.Fatalf("expected 2 split segments, got %d", len(split))
	}

	if !strings.Contains(split[0].Text, "sudah makan belum?") {
		t.Errorf("expected part 0 to be question, got %q", split[0].Text)
	}
	if split[0].SpeakerLabel != "Speaker 0" {
		t.Errorf("expected question to be Speaker 0, got %q", split[0].SpeakerLabel)
	}

	if !strings.Contains(split[1].Text, "Alhamdulillah sudah Pak") {
		t.Errorf("expected part 1 to be answer, got %q", split[1].Text)
	}
	if split[1].SpeakerLabel != "Speaker 1" {
		t.Errorf("expected answer to be Speaker 1, got %q", split[1].SpeakerLabel)
	}

	if split[0].End != split[1].Start {
		t.Errorf("expected continuous timestamps, got %f vs %f", split[0].End, split[1].Start)
	}
}

func TestSplitCompoundDialogue_ThreeTurnsInterviewerInterjection(t *testing.T) {
	wSeg := dtos.SegmentResult{
		ID:           15,
		Start:        180.0,
		End:          200.0,
		Text:         "Karena sesuai dengan jurusan saya, yaitu akuntansi, saya tertarik melamar di bagian accounting. Lalu, kontribusi apa yang akan kamu lakukan di perusahaan ini? Saya berdedikasi tinggi untuk perusahaan dan saya mampu mengerjakan tugas-tugas akuntansi.",
		SpeakerLabel: "Speaker 1",
	}

	split := sttalign.SplitCompoundDialogue(wSeg, nil)
	if len(split) != 3 {
		t.Fatalf("expected 3 split segments, got %d", len(split))
	}

	// Turn 1: Candidate answer
	if !strings.Contains(split[0].Text, "jurusan saya") || split[0].SpeakerLabel != "Speaker 1" {
		t.Errorf("expected turn 1 candidate Speaker 1, got spk=%q text=%q", split[0].SpeakerLabel, split[0].Text)
	}
	// Turn 2: Interviewer question
	if !strings.Contains(split[1].Text, "kontribusi apa") || split[1].SpeakerLabel != "Speaker 0" {
		t.Errorf("expected turn 2 interviewer Speaker 0, got spk=%q text=%q", split[1].SpeakerLabel, split[1].Text)
	}
	// Turn 3: Candidate answer
	if !strings.Contains(split[2].Text, "berdedikasi tinggi") || split[2].SpeakerLabel != "Speaker 1" {
		t.Errorf("expected turn 3 candidate Speaker 1, got spk=%q text=%q", split[2].SpeakerLabel, split[2].Text)
	}
}

func TestEnforceDialogueRoleConsistency(t *testing.T) {
	// A dialogue where Candidate self-introduces as Putri in segment 2.
	// But segment 4 (an interviewer question) was mistakenly labeled as Speaker 1 (Putri),
	// and segment 5 (candidate answer) was mistakenly labeled as Speaker 0.
	segments := []dtos.SegmentResult{
		{ID: 0, SpeakerLabel: "Speaker 0", SpeakerName: "Speaker 0", Text: "Selamat siang, silakan duduk."},
		{ID: 1, SpeakerLabel: "Speaker 0", SpeakerName: "Speaker 0", Text: "Sebelumnya, perkenalkan diri kamu secara singkat."},
		{ID: 2, SpeakerLabel: "Speaker 1", SpeakerName: "Putri Riskip Alokatimah", Text: "Perkenalkan nama saya Putri Riskip Alokatimah, jurusan akuntansi."},
		{ID: 3, SpeakerLabel: "Speaker 0", SpeakerName: "Speaker 0", Text: "Gimana tadi perjalanannya ke sini?"},
		{ID: 4, SpeakerLabel: "Speaker 1", SpeakerName: "Putri Riskip Alokatimah", Text: "Lalu, kontribusi apa yang akan kamu lakukan di perusahaan ini?"}, // Mistake: Interviewer question given to Putri
		{ID: 5, SpeakerLabel: "Speaker 0", SpeakerName: "Speaker 0", Text: "Saya berdedikasi tinggi untuk perusahaan dan saya mampu mengerjakan tugas akuntansi."}, // Mistake: Candidate answer given to Interviewer
	}

	corrected := sttalign.EnforceDialogueRoleConsistency(segments)

	// Segment 4 should be corrected to Speaker 0 (Interviewer)
	if corrected[4].SpeakerLabel != "Speaker 0" {
		t.Errorf("expected segment 4 to be corrected to Speaker 0, got %q", corrected[4].SpeakerLabel)
	}

	// Segment 5 should be corrected to Speaker 1 (Putri)
	if corrected[5].SpeakerLabel != "Speaker 1" || corrected[5].SpeakerName != "Putri Riskip Alokatimah" {
		t.Errorf("expected segment 5 to be corrected to Speaker 1 (Putri), got spk=%q name=%q", corrected[5].SpeakerLabel, corrected[5].SpeakerName)
	}
}

func TestDetectCompoundDialogueSplitPoints_GreetingAfterSentence(t *testing.T) {
	text := "Saya ingin bertanya tentang pekerjaan Anda. Selamat pagi, apa kabar?"
	pts := sttalign.DetectCompoundDialogueSplitPoints(text)
	if len(pts) == 0 {
		t.Fatalf("expected split point before greeting, got none")
	}
	// Verify split separates sentence and greeting
	part1 := strings.TrimSpace(text[:pts[0]])
	part2 := strings.TrimSpace(text[pts[0]:])
	if !strings.HasPrefix(part2, "Selamat pagi") {
		t.Errorf("expected part 2 to start with greeting, got %q (part 1=%q)", part2, part1)
	}
}

func TestAssignDialogueTurnSpeakers_ConsistentAcousticPreserved(t *testing.T) {
	// A compound dialogue where acoustic diarization firmly verified that both parts were spoken by Speaker 0
	wSeg := dtos.SegmentResult{
		ID:           1,
		Start:        0.0,
		End:          10.0,
		Text:         "sudah makan belum? Alhamdulillah sudah Pak",
		SpeakerLabel: "Speaker 0",
	}

	diarized := []dtos.SegmentResult{
		{Start: 0.0, End: 5.0, SpeakerLabel: "Speaker 0"},
		{Start: 5.0, End: 10.0, SpeakerLabel: "Speaker 0"},
	}

	split := sttalign.SplitCompoundDialogue(wSeg, diarized)
	if len(split) != 2 {
		t.Fatalf("expected 2 split segments, got %d", len(split))
	}

	// Because all parts are acoustically attributed to Speaker 0 with acoustic diarization,
	// the acoustic speaker must be preserved rather than flipped by text heuristic
	if split[0].SpeakerLabel != "Speaker 0" || split[1].SpeakerLabel != "Speaker 0" {
		t.Errorf("expected both parts to preserve acoustic Speaker 0, got [%s, %s]", split[0].SpeakerLabel, split[1].SpeakerLabel)
	}
}

func TestEnforceDialogueRoleConsistency_HighAcousticConfidencePreserved(t *testing.T) {
	// Candidate introduction establishes Speaker 1 as candidate and Speaker 0 as interviewer
	segments := []dtos.SegmentResult{
		{ID: 0, SpeakerLabel: "Speaker 0", Text: "Perkenalkan diri anda secara singkat."},
		{ID: 1, SpeakerLabel: "Speaker 1", Text: "Perkenalkan nama saya Putri Riskip Alokatimah."},
		// Segment 2 is an interviewer remark with high acoustic confidence (>= 0.70)
		{
			ID:                 2,
			SpeakerLabel:       "Speaker 0",
			AcousticSpeaker:    "Speaker 0",
			AcousticConfidence: 0.95,
			Text:               "Ya, saya bersedia mendengar penjelasan Anda.", // Contains phrase "ya saya bersedia"
		},
	}

	corrected := sttalign.EnforceDialogueRoleConsistency(segments)

	// Segment 2 must NOT be overridden to Speaker 1 (candidate) because it is anchored by high acoustic confidence
	if corrected[2].SpeakerLabel != "Speaker 0" {
		t.Errorf("expected high-confidence acoustic speaker to be preserved as Speaker 0, got %q", corrected[2].SpeakerLabel)
	}
}

func TestAlignSegments_CompoundDialoguePreservesSpeakerFallback(t *testing.T) {
	wSeg := dtos.SegmentResult{
		ID:           1,
		Start:        0.0,
		End:          8.0,
		Text:         "sudah makan belum? Alhamdulillah sudah Pak",
		SpeakerLabel: "Speaker 0",
	}

	diarized := []dtos.SegmentResult{
		{Start: 0.0, End: 8.0, SpeakerLabel: "Speaker 0"},
	}

	aligned := sttalign.AlignSegments([]dtos.SegmentResult{wSeg}, diarized)
	if len(aligned) != 2 {
		t.Fatalf("expected 2 aligned segments, got %d", len(aligned))
	}

	for i, seg := range aligned {
		if seg.SpeakerLabel != "Speaker 0" {
			t.Errorf("segment %d expected SpeakerLabel 'Speaker 0', got %q", i, seg.SpeakerLabel)
		}
	}
}

func TestEnforceDialogueRoleConsistency_InterviewerPromptTakesPrecedence(t *testing.T) {
	// Candidate introduces as Speaker 1, Interviewer speaks as Speaker 0
	segments := []dtos.SegmentResult{
		{ID: 0, SpeakerLabel: "Speaker 1", Text: "Perkenalkan nama saya Budi."},
		// Interviewer prompt preceded by exclamation mark
		{ID: 1, SpeakerLabel: "Speaker 0", Text: "Terima kasih! Lingkungan kerja seperti apa yang anda sukai?"},
		// Segment 2: candidate response with trailing question mistakenly attributed to Interviewer (Speaker 0)
		{ID: 2, SpeakerLabel: "Speaker 0", Text: "Saya berdedikasi tinggi untuk perusahaan. Apakah ada tes teknikal berikutnya?"},
	}

	corrected := sttalign.EnforceDialogueRoleConsistency(segments)

	if corrected[0].SpeakerLabel != "Speaker 1" {
		t.Errorf("expected segment 0 to be Candidate Speaker 1, got %q", corrected[0].SpeakerLabel)
	}
	if corrected[1].SpeakerLabel != "Speaker 0" {
		t.Errorf("expected segment 1 to be Interviewer Speaker 0, got %q", corrected[1].SpeakerLabel)
	}
	if corrected[2].SpeakerLabel != "Speaker 1" {
		t.Errorf("expected segment 2 to be corrected to Candidate Speaker 1, got %q", corrected[2].SpeakerLabel)
	}
}

func TestSplitCompoundDialogue_QuestionStarterWithoutQuestionMark(t *testing.T) {
	// ASR omitted question mark (used period), but previous turn has a clear question starter
	wSeg := dtos.SegmentResult{
		ID:           1,
		Start:        0.0,
		End:          8.0,
		Text:         "gimana tadi perjalanannya. alhamdulillah lancar pak",
		SpeakerLabel: "Speaker 0",
	}

	split := sttalign.SplitCompoundDialogue(wSeg, nil)
	if len(split) != 2 {
		t.Fatalf("expected 2 split turns, got %d", len(split))
	}
	if split[0].SpeakerLabel != "Speaker 0" {
		t.Errorf("expected first turn to be questioner Speaker 0, got %q", split[0].SpeakerLabel)
	}
	if split[1].SpeakerLabel != "Speaker 1" {
		t.Errorf("expected second turn to alternate to respondent Speaker 1, got %q", split[1].SpeakerLabel)
	}
}

func TestSplitCompoundDialogue_AlternateSpeakerFromDiarization(t *testing.T) {
	// Diarization has Speaker 0 and Speaker 2
	wSeg := dtos.SegmentResult{
		ID:           1,
		Start:        0.0,
		End:          8.0,
		Text:         "apakah anda bersedia? Ya saya bersedia",
		SpeakerLabel: "Speaker 0",
	}
	diarized := []dtos.SegmentResult{
		{Start: 0.0, End: 4.0, SpeakerLabel: "Speaker 0"},
		{Start: 4.0, End: 8.0, SpeakerLabel: "Speaker 2"},
	}

	split := sttalign.SplitCompoundDialogue(wSeg, diarized)
	if len(split) != 2 {
		t.Fatalf("expected 2 split turns, got %d", len(split))
	}
	if split[0].SpeakerLabel != "Speaker 0" {
		t.Errorf("expected first turn to be Speaker 0, got %q", split[0].SpeakerLabel)
	}
	if split[1].SpeakerLabel != "Speaker 2" {
		t.Errorf("expected second turn to be alternate Speaker 2, got %q", split[1].SpeakerLabel)
	}
}

func TestEnforceDialogueRoleConsistency_StrongAcousticPreservesAcousticSpeakerWhenEmpty(t *testing.T) {
	segments := []dtos.SegmentResult{
		{
			ID:                 0,
			SpeakerLabel:       "",
			AcousticSpeaker:    "Speaker 2",
			AcousticConfidence: 0.85,
			Text:               "Pertemuan ini berjalan sangat tertib dan sesuai agenda.",
		},
		{
			ID:           1,
			SpeakerLabel: "Speaker 0",
			Text:         "Lingkungan kerja seperti apa yang anda sukai?",
		},
		{
			ID:           2,
			SpeakerLabel: "Speaker 1",
			Text:         "Nama saya Budi, saya berdedikasi tinggi untuk perusahaan.",
		},
	}

	corrected := sttalign.EnforceDialogueRoleConsistency(segments)
	if corrected[0].SpeakerLabel != "Speaker 2" {
		t.Errorf("expected empty SpeakerLabel with strong acoustic anchor to be populated with %q, got %q", "Speaker 2", corrected[0].SpeakerLabel)
	}
}

func TestIsCandidateIdentityMarker_ExcludesNarrativeJobApplication(t *testing.T) {
	if !sttalign.IsCandidateIdentityMarker("Nama saya Budi, saya lulusan informatika.") {
		t.Errorf("expected true for self-introduction")
	}
	if sttalign.IsCandidateIdentityMarker("Saya melamar pekerjaan ini karena tertarik dengan reputasi perusahaan.") {
		t.Errorf("expected false for narrative job application action")
	}
}

func TestDetectCompoundDialogueSplitPoints_ShortQuestionStarter(t *testing.T) {
	points := sttalign.DetectCompoundDialogueSplitPoints("Bisakah kamu? Ya, saya bisa membantu menyelesaikan pekerjaan ini dengan baik.")
	if len(points) == 0 {
		t.Errorf("expected split point after short question starter 'Bisakah kamu?'")
	}
}

func TestIsInterviewerPromptMarker_ReportedSpeechExcluded(t *testing.T) {
	if sttalign.IsInterviewerPromptMarker("saya ditanya, apakah anda bersedia bekerja lembur?") {
		t.Errorf("expected false for reported speech question following comma")
	}
	if !sttalign.IsInterviewerPromptMarker("Apakah anda bersedia bekerja lembur?") {
		t.Errorf("expected true for direct interviewer question")
	}
	if !sttalign.IsInterviewerPromptMarker("Lalu, apakah anda bersedia bekerja lembur?") {
		t.Errorf("expected true for conversational conjunction + interviewer question")
	}
}
