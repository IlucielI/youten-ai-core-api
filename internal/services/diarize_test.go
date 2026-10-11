package services

import (
	"context"
	"fmt"
	"strings"
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

func TestService_DiarizeAndCorrectSegmentsWithLLM_WithAcousticReference(t *testing.T) {
	mockLLM := &dummyLLM{
		chatResp: `[
			{"index": 0, "speaker": "Speaker 1", "speaker_name": "Speaker 1", "text": "Selamat siang, saya terima surat lamaran dan CV-nya."},
			{"index": 1, "speaker": "Speaker 0", "speaker_name": "Putri", "text": "Terima kasih Pak, nama saya Putri."}
		]`,
	}
	svc := &Service{llm: mockLLM}

	segments := []dtos.SegmentResult{
		{Text: "Selamat siang saya terima surat lamaran", SpeakerLabel: ""},
		{Text: "Terima kasih Pak nama saya Putri", SpeakerLabel: ""},
	}

	deepgramRef := "Speaker 1: Selamat siang saya terima surat lamaran dan CV-nya.\nSpeaker 0: Terima kasih Pak nama saya Putri."
	svc.diarizeAndCorrectSegmentsWithLLM(context.Background(), segments, deepgramRef)

	if segments[0].SpeakerLabel != "Speaker 1" {
		t.Errorf("expected Speaker 1 on segment 0, got %s", segments[0].SpeakerLabel)
	}
	if segments[1].SpeakerLabel != "Speaker 0" || segments[1].SpeakerName != "Putri" {
		t.Errorf("expected Speaker 0 and Putri on segment 1, got %+v", segments[1])
	}
}

func TestSliceAcousticReference_ProportionalWindows(t *testing.T) {
	// 50 minutes audio reference (simulate large multi-speaker dialogue)
	var longRef strings.Builder
	for i := 0; i < 200; i++ {
		longRef.WriteString(fmt.Sprintf("Speaker %d: Line %d ini adalah dialog panjang pada rekaman audio podcast.\n", i%3, i))
	}
	refText := longRef.String()

	// 1. Early chunk (minute 2 out of 50 minutes = 120s / 3000s)
	earlySlice := sliceAcousticReference(refText, 60.0, 180.0, 3000.0)
	if !strings.Contains(earlySlice, "Line 0") && !strings.Contains(earlySlice, "Line 1") {
		t.Errorf("expected early slice to contain early lines, got: %s", earlySlice[:200])
	}

	// 2. Late chunk (minute 48 out of 50 minutes = 2880s / 3000s)
	lateSlice := sliceAcousticReference(refText, 2800.0, 2950.0, 3000.0)
	if !strings.Contains(lateSlice, "Line 190") && !strings.Contains(lateSlice, "Line 195") {
		t.Errorf("expected late slice to contain late lines, got: %s", lateSlice)
	}
	// Verify late slice does not begin with Line 0 (preventing truncation bug)
	if strings.HasPrefix(lateSlice, "Speaker 0: Line 0 ") {
		t.Errorf("late slice should not start at the very beginning of the 50-minute audio")
	}
}

func TestPruneGhostSpeakers_RemovesNoiseSpikes(t *testing.T) {
	roster := map[string]SpeakerRosterItem{
		"Speaker 0": {Label: "Speaker 0", Name: "Caesar", Role: "Host"},
		"Speaker 1": {Label: "Speaker 1", Name: "Bernadya", Role: "Guest"},
		"Speaker 2": {Label: "Speaker 2", Name: "Speaker 2", Role: "Speaker"},
		"Speaker 3": {Label: "Speaker 3", Name: "Speaker 3", Role: "Speaker"},
	}

	// Construct dialogue where Speaker 0 and 1 have extensive conversation (over 300 words),
	// but Speaker 2 and 3 are isolated 1-sentence noise spikes (< 10 words each)
	segments := []dtos.SegmentResult{
		{SpeakerLabel: "Speaker 0", SpeakerName: "Caesar", Text: "Selamat datang di podcast hari ini kita kedatangan tamu istimewa Bernadya yang baru saja merilis album terbarunya dan sangat luar biasa diterima oleh para pendengar musik tanah air.", Start: 0.0, End: 15.0},
		{SpeakerLabel: "Speaker 0", SpeakerName: "Caesar", Text: "Bagaimana perasaanmu setelah melihat antusiasme ribuan penonton yang hadir menyanyikan lagumu secara bersama-sama di panggung?", Start: 15.0, End: 25.0},
		{SpeakerLabel: "Speaker 2", SpeakerName: "Speaker 2", Text: "Haha batuk.", Start: 25.5, End: 26.5}, // Noise spike
		{SpeakerLabel: "Speaker 1", SpeakerName: "Bernadya", Text: "Halo Mas Caesar terima kasih banyak sudah mengundang aku ke podcast ini rasanya sangat senang dan tidak menyangka responnya akan sebesar ini.", Start: 27.0, End: 40.0},
		{SpeakerLabel: "Speaker 1", SpeakerName: "Bernadya", Text: "Waktu pertama kali menulis lagu-lagu di album ini aku cuma ingin menuangkan perasaan dan kegelisahan yang aku rasakan tanpa berekspektasi macam-macam.", Start: 40.0, End: 55.0},
		{SpeakerLabel: "Speaker 3", SpeakerName: "Speaker 3", Text: "Suara kursi.", Start: 56.0, End: 57.0}, // Noise spike
		{SpeakerLabel: "Speaker 1", SpeakerName: "Bernadya", Text: "Dan ternyata banyak sekali orang yang merasa relate dengan cerita-cerita yang aku tulis di lagu tersebut.", Start: 58.0, End: 70.0},
	}

	pruned := pruneGhostSpeakers(segments, roster)

	for i, seg := range pruned {
		if seg.SpeakerLabel == "Speaker 2" {
			t.Errorf("segment %d still has ghost Speaker 2: %+v", i, seg)
		}
		if seg.SpeakerLabel == "Speaker 3" {
			t.Errorf("segment %d still has ghost Speaker 3: %+v", i, seg)
		}
	}

	// Verify segment 2 (formerly Speaker 2) was merged into Speaker 0 or adjacent
	if pruned[2].SpeakerLabel != "Speaker 0" && pruned[2].SpeakerLabel != "Speaker 1" {
		t.Errorf("expected segment 2 to be merged into primary speaker, got %s", pruned[2].SpeakerLabel)
	}
}

func TestPruneGhostSpeakers_PreservesLegitimateThirdSpeaker(t *testing.T) {
	roster := map[string]SpeakerRosterItem{
		"Speaker 0": {Label: "Speaker 0", Name: "Host", Role: "Host"},
		"Speaker 1": {Label: "Speaker 1", Name: "Guest 1", Role: "Guest"},
		"Speaker 2": {Label: "Speaker 2", Name: "Guest 2", Role: "Guest"},
	}

	// 3 legitimate speakers with sustained contributions
	segments := []dtos.SegmentResult{
		{SpeakerLabel: "Speaker 0", SpeakerName: "Host", Text: "Selamat datang semuanya di sesi diskusi meja bundar hari ini kita akan membahas arsitektur kecerdasan buatan.", Start: 0.0, End: 15.0},
		{SpeakerLabel: "Speaker 1", SpeakerName: "Guest 1", Text: "Menurut saya pendekatan multi-agent sangat penting untuk memisahkan domain tanggung jawab dan menjaga skalabilitas sistem.", Start: 16.0, End: 30.0},
		{SpeakerLabel: "Speaker 2", SpeakerName: "Guest 2", Text: "Saya setuju sekali dengan pandangan tersebut terutama ketika kita harus memproses ribuan data secara terdistribusi.", Start: 31.0, End: 45.0},
		{SpeakerLabel: "Speaker 2", SpeakerName: "Guest 2", Text: "Di tim kami implementasi RabbitMQ dan event-driven architecture berhasil mengurangi latency secara signifikan.", Start: 46.0, End: 60.0},
		{SpeakerLabel: "Speaker 2", SpeakerName: "Guest 2", Text: "Sehingga beban pemrosesan pipeline transkripsi audio tidak membebani web server secara langsung.", Start: 61.0, End: 75.0},
		{SpeakerLabel: "Speaker 2", SpeakerName: "Guest 2", Text: "Kami juga menerapkan dead-letter queue untuk memastikan setiap pesan yang gagal dapat diulang dengan aman.", Start: 76.0, End: 90.0},
	}

	pruned := pruneGhostSpeakers(segments, roster)

	// Speaker 2 has significant speech and must NOT be pruned
	foundSpeaker2 := false
	for _, seg := range pruned {
		if seg.SpeakerLabel == "Speaker 2" {
			foundSpeaker2 = true
		}
	}
	if !foundSpeaker2 {
		t.Errorf("expected Speaker 2 to be preserved as a legitimate 3rd speaker")
	}
}

func TestExtractSpeakerLabelsFromText(t *testing.T) {
	text := "Speaker 0: Halo selamat pagi\nSpeaker 1: Pagi juga\n[Speaker 2]: Saya ikut bergabung\nPembicara 3: Baik kita mulai"
	labels := extractSpeakerLabelsFromText(text)

	expected := []string{"Speaker 0", "Speaker 1", "Speaker 2", "Speaker 3"}
	if len(labels) != len(expected) {
		t.Fatalf("expected %d labels, got %d: %v", len(expected), len(labels), labels)
	}
	for i, exp := range expected {
		if labels[i] != exp {
			t.Errorf("expected label %d to be %s, got %s", i, exp, labels[i])
		}
	}
}



