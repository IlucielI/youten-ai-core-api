package services

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

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
		msg := earlySlice
		if len(msg) > 200 {
			msg = msg[:200]
		}
		t.Errorf("expected early slice to contain early lines, got: %s", msg)
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
	roster := map[string]dtos.SpeakerRosterItem{
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
	roster := map[string]dtos.SpeakerRosterItem{
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

func TestService_DiarizeAndCorrectSegmentsWithLLM_RelativeIndexChunk(t *testing.T) {
	// Simulate 2 chunks (total 30 segments).
	// Chunk 0 is segments [0..25). Chunk 1 is segments [25..30).
	// In Chunk 1, the LLM returns 0-based relative indices [{"index": 0, ...}, {"index": 1, ...}]
	// instead of [{"index": 25, ...}, {"index": 26, ...}].
	mockLLM := &dummyLLM{
		generateChatFunc: func(ctx context.Context, systemPrompt string, messages []dtos.ChatMessageInput, opts dtos.ChatOptions) (*dtos.ChatResponse, error) {
			userContent := messages[0].Content
			if !strings.Contains(userContent, "ROSTER PEMBICARA RESMI") {
				// Pass 1: Roster discovery
				return &dtos.ChatResponse{
					Content: `[{"label": "Speaker 0", "name": "Host", "role": "Host"}, {"label": "Speaker 1", "name": "Budi", "role": "Guest"}]`,
				}, nil
			}
			if strings.Contains(userContent, "25 [") {
				// Pass 2, Chunk 1 [25..30): LLM outputs 0-based relative index for index 25!
				return &dtos.ChatResponse{
					Content: `[
						{"index": 0, "speaker": "Speaker 1", "speaker_name": "Budi", "text": "Halo saya Budi segmen 25."},
						{"index": 1, "speaker": "Speaker 1", "speaker_name": "Budi", "text": "Ini segmen 26."}
					]`,
				}, nil
			}
			// Pass 2, Chunk 0 [0..25)
			return &dtos.ChatResponse{
				Content: `[{"index": 0, "speaker": "Speaker 0", "speaker_name": "Host", "text": "Selamat datang"}]`,
			}, nil
		},
	}

	svc := &Service{llm: mockLLM}
	segments := make([]dtos.SegmentResult, 30)
	for i := range segments {
		segments[i] = dtos.SegmentResult{
			ID:    i,
			Start: float64(i * 10),
			End:   float64(i*10 + 8),
			Text:  fmt.Sprintf("Teks original segmen %d", i),
		}
	}

	svc.diarizeAndCorrectSegmentsWithLLM(context.Background(), segments)

	// Verify Chunk 1 segment 25 correctly mapped from relative index 0
	if segments[25].SpeakerLabel != "Speaker 1" {
		t.Errorf("expected segment 25 SpeakerLabel 'Speaker 1', got %q", segments[25].SpeakerLabel)
	}
	if segments[25].SpeakerName != "Budi" {
		t.Errorf("expected segment 25 SpeakerName 'Budi', got %q", segments[25].SpeakerName)
	}
	if segments[25].Text != "Halo saya Budi segmen 25." {
		t.Errorf("expected corrected text on segment 25, got %q", segments[25].Text)
	}

	// Verify Chunk 1 segment 26 correctly mapped from relative index 1
	if segments[26].SpeakerLabel != "Speaker 1" || segments[26].SpeakerName != "Budi" {
		t.Errorf("expected segment 26 to be Budi/Speaker 1, got %+v", segments[26])
	}
}

func TestService_DiarizeAndCorrectSegmentsWithLLM_MultiChunkOneHourSimulation(t *testing.T) {
	// Simulate 100 segments (~4 chunks of 25) with 4 distinct speakers across chunks
	mockLLM := &dummyLLM{
		generateChatFunc: func(ctx context.Context, systemPrompt string, messages []dtos.ChatMessageInput, opts dtos.ChatOptions) (*dtos.ChatResponse, error) {
			userContent := messages[0].Content
			if strings.Contains(userContent, "ROSTER PEMBICARA RESMI") {
				// Chunk execution
				if strings.Contains(userContent, "0 [0.00s") {
					// Chunk 0
					return &dtos.ChatResponse{
						Content: `[{"index": 0, "speaker": "Speaker 0", "speaker_name": "Alice", "text": "Part 0"}]`,
					}, nil
				} else if strings.Contains(userContent, "25 [250.00s") {
					// Chunk 1
					return &dtos.ChatResponse{
						Content: `[{"index": 25, "speaker": "Speaker 1", "speaker_name": "Bob", "text": "Part 25"}]`,
					}, nil
				} else if strings.Contains(userContent, "50 [500.00s") {
					// Chunk 2
					return &dtos.ChatResponse{
						Content: `[{"index": 50, "speaker": "Speaker 2", "speaker_name": "Charlie", "text": "Part 50"}]`,
					}, nil
				} else if strings.Contains(userContent, "75 [750.00s") {
					// Chunk 3
					return &dtos.ChatResponse{
						Content: `[{"index": 75, "speaker": "Speaker 3", "speaker_name": "David", "text": "Part 75"}]`,
					}, nil
				}
			}
			// Fallback / Roster discovery
			return &dtos.ChatResponse{
				Content: `[
					{"label": "Speaker 0", "name": "Alice", "role": "Host"},
					{"label": "Speaker 1", "name": "Bob", "role": "Guest"},
					{"label": "Speaker 2", "name": "Charlie", "role": "Guest"},
					{"label": "Speaker 3", "name": "David", "role": "Guest"}
				]`,
			}, nil
		},
	}

	svc := &Service{llm: mockLLM}
	segments := make([]dtos.SegmentResult, 100)
	for i := range segments {
		segments[i] = dtos.SegmentResult{
			ID:    i,
			Start: float64(i * 10),
			End:   float64(i*10 + 8),
			Text:  fmt.Sprintf("Original segment text %d", i),
		}
	}

	svc.diarizeAndCorrectSegmentsWithLLM(context.Background(), segments)

	if segments[0].SpeakerName != "Alice" {
		t.Errorf("expected segment 0 Alice, got %s", segments[0].SpeakerName)
	}
	if segments[25].SpeakerName != "Bob" {
		t.Errorf("expected segment 25 Bob, got %s", segments[25].SpeakerName)
	}
	if segments[50].SpeakerName != "Charlie" {
		t.Errorf("expected segment 50 Charlie, got %s", segments[50].SpeakerName)
	}
	if segments[75].SpeakerName != "David" {
		t.Errorf("expected segment 75 David, got %s", segments[75].SpeakerName)
	}
}

func TestPruneGhostSpeakers_PreservesLegitimateTenthSpeaker(t *testing.T) {
	roster := make(map[string]dtos.SpeakerRosterItem)
	for i := 0; i < 10; i++ {
		lbl := fmt.Sprintf("Speaker %d", i)
		roster[lbl] = dtos.SpeakerRosterItem{Label: lbl, Name: lbl, Role: "Participant"}
	}

	// 10 active speakers. Speaker 9 only speaks for 2 segments (30 words, 12 seconds).
	var segments []dtos.SegmentResult
	for i := 0; i < 9; i++ {
		lbl := fmt.Sprintf("Speaker %d", i)
		for j := 0; j < 5; j++ {
			segments = append(segments, dtos.SegmentResult{
				SpeakerLabel: lbl,
				SpeakerName:  lbl,
				Text:         fmt.Sprintf("Diskusi teknis oleh pembicara %d pada sesi diskusi mendalam tentang arsitektur sistem berbasis event driven.", i),
				Start:        float64(i*100 + j*10),
				End:          float64(i*100 + j*10 + 8),
			})
		}
	}
	// Speaker 9 speaks at the very end
	segments = append(segments, dtos.SegmentResult{
		SpeakerLabel: "Speaker 9",
		SpeakerName:  "Speaker 9",
		Text:         "Terima kasih atas diskusinya, saya ingin menanyakan apakah arsitektur ini sudah diuji untuk beban satu jam penuh?",
		Start:        950.0,
		End:          960.0,
	})
	segments = append(segments, dtos.SegmentResult{
		SpeakerLabel: "Speaker 9",
		SpeakerName:  "Speaker 9",
		Text:         "Dan bagaimana dengan konsumsi memori pada saat prosesor melakukan parsing transkrip?",
		Start:        961.0,
		End:          967.0,
	})

	pruned := pruneGhostSpeakers(segments, roster)

	foundSpeaker9 := false
	for _, seg := range pruned {
		if seg.SpeakerLabel == "Speaker 9" {
			foundSpeaker9 = true
			break
		}
	}

	if !foundSpeaker9 {
		t.Errorf("expected Speaker 9 to be preserved as a legitimate 10th speaker, but was pruned")
	}
}

func TestBuildContextualSTTPrompt(t *testing.T) {
	prompt := buildContextualSTTPrompt("[TICS] Data Pipeline Discussion", "id")
	if !strings.Contains(prompt, "Topik: [TICS] Data Pipeline Discussion.") {
		t.Errorf("expected prompt to contain topic, got: %s", prompt)
	}
	if !strings.Contains(prompt, "ClickHouse") || !strings.Contains(prompt, "S3") || !strings.Contains(prompt, "front-end") {
		t.Errorf("expected domain vocabulary in prompt, got: %s", prompt)
	}

	emptyPrompt := buildContextualSTTPrompt("", "id")
	if strings.Contains(emptyPrompt, "Topik:") {
		t.Errorf("expected no topic prefix when title is empty, got: %s", emptyPrompt)
	}
	if !strings.Contains(emptyPrompt, "GitLab") {
		t.Errorf("expected default vocabulary even when title is empty")
	}

	enPrompt := buildContextualSTTPrompt("Quarterly Sync", "en")
	if !strings.Contains(enPrompt, "Topic: Quarterly Sync.") {
		t.Errorf("expected English prompt to contain 'Topic: Quarterly Sync.', got: %s", enPrompt)
	}
	if !strings.Contains(enPrompt, "Technology meeting") || !strings.Contains(enPrompt, "Terms:") {
		t.Errorf("expected English domain vocabulary in prompt, got: %s", enPrompt)
	}
}

func TestCleanLLMJSONResponse(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "plain json array",
			input:    `[{"index": 0, "text": "Halo"}]`,
			expected: `[{"index": 0, "text": "Halo"}]`,
		},
		{
			name:     "markdown fenced json",
			input:    "```json\n[{\"index\": 0, \"text\": \"Halo\"}]\n```",
			expected: `[{"index": 0, "text": "Halo"}]`,
		},
		{
			name:     "wrapped in think tags",
			input:    "<think>Evaluating speech consensus...</think>\n```json\n[{\"index\": 0, \"text\": \"Halo\"}]\n```",
			expected: `[{"index": 0, "text": "Halo"}]`,
		},
		{
			name:     "wrapped in escape tags from thinking model",
			input:    "<escape>[{\"index\": 0, \"text\": \"Halo\"}]</escape>",
			expected: `[{"index": 0, "text": "Halo"}]`,
		},
		{
			name:     "reasoning tags with brackets inside reasoning",
			input:    "<reasoning>Speaker mapping: [Speaker 0 -> Speaker 1]</reasoning>[{\"index\": 0, \"text\": \"Halo\"}]",
			expected: `[{"index": 0, "text": "Halo"}]`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := cleanLLMJSONResponse(tc.input)
			if actual != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, actual)
			}
		})
	}
}

func TestService_DiarizeAndCorrectSegments_SimulasiInterviewJobFair(t *testing.T) {
	// Real-world scenario based on Simulasi_Interview_Kerja_Bahasa_Indonesia.m4a
	// Two speakers: Pewawancara (Speaker 1) and Putri (Speaker 0)
	mockLLM := &dummyLLM{
		generateChatFunc: func(ctx context.Context, systemPrompt string, messages []dtos.ChatMessageInput, opts dtos.ChatOptions) (*dtos.ChatResponse, error) {
			userContent := messages[0].Content
			if strings.Contains(userContent, "ROSTER PEMBICARA RESMI") {
				// Chunk response: strictly preserve acoustic speakers and map Putri's real name
				return &dtos.ChatResponse{
					Content: `[
						{"index": 0, "speaker": "Speaker 1", "speaker_name": "Pewawancara", "text": "Selamat siang, silakan duduk. Bisakah Anda menceritakan tentang diri Anda?"},
						{"index": 1, "speaker": "Speaker 0", "speaker_name": "Putri", "text": "Selamat siang Pak, terima kasih. Nama saya Putri. Saya lulusan S1 Akuntansi dari Universitas Indonesia."},
						{"index": 2, "speaker": "Speaker 1", "speaker_name": "Pewawancara", "text": "Apa kelebihan dan kelemahan terbesar Anda?"},
						{"index": 3, "speaker": "Speaker 0", "speaker_name": "Putri", "text": "Kelebihan saya adalah teliti dalam mengolah data keuangan dan siap bekerja di bawah tekanan."}
					]`,
				}, nil
			}
			// Roster discovery response
			return &dtos.ChatResponse{
				Content: `[
					{"label": "Speaker 1", "name": "Pewawancara", "role": "Interviewer"},
					{"label": "Speaker 0", "name": "Putri", "role": "Pelamar"}
				]`,
			}, nil
		},
	}

	svc := &Service{llm: mockLLM}

	whisperSegments := []dtos.SegmentResult{
		{ID: 0, Start: 0.0, End: 6.0, Text: "Selamat siang silakan duduk bisakah anda menceritakan tentang diri anda", SpeakerLabel: ""},
		{ID: 1, Start: 6.2, End: 15.0, Text: "Selamat siang Pak terima kasih nama saya Putri saya lulusan S1 Akuntansi dari Universitas Indonesia", SpeakerLabel: ""},
		{ID: 2, Start: 15.5, End: 20.0, Text: "Apa kelebihan dan kelemahan terbesar anda", SpeakerLabel: ""},
		{ID: 3, Start: 20.2, End: 30.0, Text: "Kelebihan saya adalah teliti dalam mengolah data keuangan dan siap bekerja di bawah tekanan", SpeakerLabel: ""},
	}

	diarizedSegments := []dtos.SegmentResult{
		{Start: 0.0, End: 6.0, SpeakerLabel: "Speaker 1", Text: "Selamat siang silakan duduk bisakah anda menceritakan tentang diri anda"},
		{Start: 6.1, End: 15.1, SpeakerLabel: "Speaker 0", Text: "Selamat siang Pak terima kasih nama saya Putri saya lulusan S1 Akuntansi dari Universitas Indonesia"},
		{Start: 15.3, End: 20.1, SpeakerLabel: "Speaker 1", Text: "Apa kelebihan dan kelemahan terbesar anda"},
		{Start: 20.1, End: 30.2, SpeakerLabel: "Speaker 0", Text: "Kelebihan saya adalah teliti dalam mengolah data keuangan dan siap bekerja di bawah tekanan"},
	}

	diarizedRef := "Speaker 1 [0.00s - 6.00s]: Selamat siang silakan duduk bisakah anda menceritakan tentang diri anda\n" +
		"Speaker 0 [6.10s - 15.10s]: Selamat siang Pak terima kasih nama saya Putri saya lulusan S1 Akuntansi dari Universitas Indonesia\n" +
		"Speaker 1 [15.30s - 20.10s]: Apa kelebihan dan kelemahan terbesar anda\n" +
		"Speaker 0 [20.10s - 30.20s]: Kelebihan saya adalah teliti dalam mengolah data keuangan dan siap bekerja di bawah tekanan"

	aligned := svc.diarizeAndCorrectSegmentsWithLLM(context.Background(), whisperSegments, diarizedRef, diarizedSegments)

	if len(aligned) != 4 {
		t.Fatalf("expected 4 aligned segments, got %d", len(aligned))
	}

	// Turn 0: Pewawancara (Speaker 1)
	if aligned[0].SpeakerLabel != "Speaker 1" || aligned[0].SpeakerName != "Pewawancara" {
		t.Errorf("turn 0 expected Speaker 1 / Pewawancara, got label=%q name=%q", aligned[0].SpeakerLabel, aligned[0].SpeakerName)
	}

	// Turn 1: Putri (Speaker 0) - MUST NOT BE FLIPPED TO PEWAWANCARA!
	if aligned[1].SpeakerLabel != "Speaker 0" || aligned[1].SpeakerName != "Putri" {
		t.Errorf("turn 1 expected Speaker 0 / Putri, got label=%q name=%q", aligned[1].SpeakerLabel, aligned[1].SpeakerName)
	}

	// Turn 2: Pewawancara (Speaker 1)
	if aligned[2].SpeakerLabel != "Speaker 1" || aligned[2].SpeakerName != "Pewawancara" {
		t.Errorf("turn 2 expected Speaker 1 / Pewawancara, got label=%q name=%q", aligned[2].SpeakerLabel, aligned[2].SpeakerName)
	}

	// Turn 3: Putri (Speaker 0)
	if aligned[3].SpeakerLabel != "Speaker 0" || aligned[3].SpeakerName != "Putri" {
		t.Errorf("turn 3 expected Speaker 0 / Putri, got label=%q name=%q", aligned[3].SpeakerLabel, aligned[3].SpeakerName)
	}
}

func TestService_DiarizeAndCorrectSegments_WordLevelTurnSplitInService(t *testing.T) {
	// When Whisper returns a single segment with words spanning both Putri and Pewawancara
	whisperSeg := dtos.SegmentResult{
		ID:    0,
		Start: 10.0,
		End:   20.0,
		Text:  "Apakah job desk yang diberikan? Untuk job desk kita membuka staff accounting.",
		Words: []dtos.WordResult{
			{Word: "Apakah", Start: 10.0, End: 10.8},
			{Word: "job", Start: 10.9, End: 11.4},
			{Word: "desk", Start: 11.5, End: 12.0},
			{Word: "yang", Start: 12.1, End: 12.5},
			{Word: "diberikan?", Start: 12.6, End: 13.8},
			{Word: "Untuk", Start: 14.2, End: 14.8},
			{Word: "job", Start: 14.9, End: 15.4},
			{Word: "desk", Start: 15.5, End: 16.0},
			{Word: "kita", Start: 16.1, End: 16.6},
			{Word: "membuka", Start: 16.7, End: 17.6},
			{Word: "staff", Start: 17.7, End: 18.5},
			{Word: "accounting.", Start: 18.6, End: 19.8},
		},
	}

	diarizedSegments := []dtos.SegmentResult{
		{Start: 10.0, End: 14.0, SpeakerLabel: "Speaker 0"},
		{Start: 14.1, End: 20.0, SpeakerLabel: "Speaker 1"},
	}

	svc := &Service{llm: nil} // Test acoustic pre-pass without LLM
	splitSegs := svc.diarizeAndCorrectSegmentsWithLLM(context.Background(), []dtos.SegmentResult{whisperSeg}, "", diarizedSegments)

	if len(splitSegs) != 2 {
		t.Fatalf("expected Whisper segment to be split into 2 distinct speaker segments, got %d", len(splitSegs))
	}
	if splitSegs[0].SpeakerLabel != "Speaker 0" {
		t.Errorf("part 0 expected Speaker 0, got %s", splitSegs[0].SpeakerLabel)
	}
	if !strings.Contains(splitSegs[0].Text, "Apakah job desk") {
		t.Errorf("part 0 expected Putri's question, got %q", splitSegs[0].Text)
	}
	if splitSegs[1].SpeakerLabel != "Speaker 1" {
		t.Errorf("part 1 expected Speaker 1, got %s", splitSegs[1].SpeakerLabel)
	}
	if !strings.Contains(splitSegs[1].Text, "Untuk job desk") {
		t.Errorf("part 1 expected Interviewer's answer, got %q", splitSegs[1].Text)
	}
}

func TestService_DiarizeAndCorrectSegments_InversionAutoCorrection(t *testing.T) {
	// Simulate LLM hallucinating and systematically inverting Speaker 0 and Speaker 1
	mockLLM := &dummyLLM{
		generateChatFunc: func(ctx context.Context, systemPrompt string, messages []dtos.ChatMessageInput, opts dtos.ChatOptions) (*dtos.ChatResponse, error) {
			userContent := messages[0].Content
			if strings.Contains(userContent, "ROSTER PEMBICARA RESMI") {
				// Inverted LLM hallucination: returns Speaker 1 for Speaker 0's turns, and Speaker 0 for Speaker 1's turns
				return &dtos.ChatResponse{
					Content: `[
						{"index": 0, "speaker": "Speaker 1", "speaker_name": "Speaker 1", "text": "Turn 0"},
						{"index": 1, "speaker": "Speaker 1", "speaker_name": "Speaker 1", "text": "Turn 1"},
						{"index": 2, "speaker": "Speaker 1", "speaker_name": "Speaker 1", "text": "Turn 2"},
						{"index": 3, "speaker": "Speaker 0", "speaker_name": "Speaker 0", "text": "Turn 3"},
						{"index": 4, "speaker": "Speaker 0", "speaker_name": "Speaker 0", "text": "Turn 4"},
						{"index": 5, "speaker": "Speaker 0", "speaker_name": "Speaker 0", "text": "Turn 5"}
					]`,
				}, nil
			}
			return &dtos.ChatResponse{
				Content: `[{"label": "Speaker 0", "name": "Speaker 0"}, {"label": "Speaker 1", "name": "Speaker 1"}]`,
			}, nil
		},
	}

	svc := &Service{llm: mockLLM}

	whisperSegs := make([]dtos.SegmentResult, 6)
	for i := 0; i < 6; i++ {
		whisperSegs[i] = dtos.SegmentResult{
			ID:    i,
			Start: float64(i * 10),
			End:   float64(i*10 + 8),
			Text:  "Turn original",
		}
	}

	// Acoustic ground truth: 0..2 are Speaker 0, 3..5 are Speaker 1
	diarized := []dtos.SegmentResult{
		{Start: 0.0, End: 28.0, SpeakerLabel: "Speaker 0"},
		{Start: 30.0, End: 58.0, SpeakerLabel: "Speaker 1"},
	}

	result := svc.diarizeAndCorrectSegmentsWithLLM(context.Background(), whisperSegs, "", diarized)

	// Verify inversion was detected and reversed back to Speaker 0 for 0..2 and Speaker 1 for 3..5
	for i := 0; i < 3; i++ {
		if result[i].SpeakerLabel != "Speaker 0" {
			t.Errorf("turn %d: expected Speaker 0, got %s", i, result[i].SpeakerLabel)
		}
	}
	for i := 3; i < 6; i++ {
		if result[i].SpeakerLabel != "Speaker 1" {
			t.Errorf("turn %d: expected Speaker 1, got %s", i, result[i].SpeakerLabel)
		}
	}
}

func TestService_Diarize_AcousticAnchorProtectsAgainstSpeakerSwap(t *testing.T) {
	// Tests that when biometric acoustic confidence is high, LLM cannot swap Speaker 0 to Speaker 1
	mockLLM := &dummyLLM{
		generateChatFunc: func(ctx context.Context, systemPrompt string, messages []dtos.ChatMessageInput, opts dtos.ChatOptions) (*dtos.ChatResponse, error) {
			userContent := messages[0].Content
			if strings.Contains(userContent, "ROSTER PEMBICARA RESMI") {
				// LLM attempts to swap turn 0 to Speaker 1
				return &dtos.ChatResponse{
					Content: `[
						{"index": 0, "speaker": "Speaker 1", "speaker_name": "Speaker 1", "text": "Kita mulai sesi pembahasan arsitektur."}
					]`,
				}, nil
			}
			return &dtos.ChatResponse{
				Content: `[{"label": "Speaker 0", "name": "Pewawancara"}, {"label": "Speaker 1", "name": "Putri"}]`,
			}, nil
		},
	}

	svc := &Service{llm: mockLLM}

	whisperSegs := []dtos.SegmentResult{
		{
			ID:    0,
			Start: 0.0,
			End:   8.0,
			Text:  "Kita mulai sesi pembahasan arsitektur.",
		},
	}

	// Acoustic ground truth has 100% overlap with Speaker 0
	diarized := []dtos.SegmentResult{
		{Start: 0.0, End: 8.0, SpeakerLabel: "Speaker 0", Text: "Kita mulai sesi pembahasan arsitektur."},
	}

	result := svc.diarizeAndCorrectSegmentsWithLLM(context.Background(), whisperSegs, "", diarized)

	if len(result) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(result))
	}
	// Acoustic anchor must reject LLM swap to Speaker 1 and keep Speaker 0!
	if result[0].SpeakerLabel != "Speaker 0" {
		t.Errorf("expected Speaker 0 to be protected by acoustic anchor, got %s", result[0].SpeakerLabel)
	}
}

func TestMergeConsecutiveSegments_DifferentSpeakerLabelsNeverMerge(t *testing.T) {
	// Two adjacent segments with different speaker labels but identical or empty names
	// MUST NOT be merged together into a single segment!
	input := []models.TranscriptSegment{
		{
			SpeakerLabel:  "Speaker 0",
			SpeakerName:   "",
			StartTime:     0.0,
			EndTime:       3.0,
			Text:          "Selamat siang.",
			SequenceOrder: 1,
		},
		{
			SpeakerLabel:  "Speaker 1",
			SpeakerName:   "",
			StartTime:     3.2,
			EndTime:       6.0,
			Text:          "Selamat siang Pak.",
			SequenceOrder: 2,
		},
	}

	merged := mergeConsecutiveSegments(input, 1.5, 20.0)

	if len(merged) != 2 {
		t.Fatalf("expected 2 distinct segments for different speakers, got %d (they were erroneously merged!)", len(merged))
	}
	if merged[0].SpeakerLabel != "Speaker 0" {
		t.Errorf("expected segment 0 to have Speaker 0, got %s", merged[0].SpeakerLabel)
	}
	if merged[1].SpeakerLabel != "Speaker 1" {
		t.Errorf("expected segment 1 to have Speaker 1, got %s", merged[1].SpeakerLabel)
	}
}

func TestService_DiarizeAndCorrectSegmentsWithLLM_ParallelPoolLargeMeeting(t *testing.T) {
	// 150 segments triggers large-meeting chunking (chunkSize = 60 => 3 parallel chunks)
	segments := make([]dtos.SegmentResult, 150)
	for i := range segments {
		segments[i] = dtos.SegmentResult{
			ID:           i,
			Start:        float64(i * 5),
			End:          float64(i*5 + 4),
			SpeakerLabel: fmt.Sprintf("Speaker %d", i%2),
			Text:         fmt.Sprintf("Original speech turn segment %d", i),
		}
	}

	var activeWorkers int32
	var maxObservedWorkers int32

	mockLLM := &dummyLLM{
		generateChatFunc: func(ctx context.Context, systemPrompt string, messages []dtos.ChatMessageInput, opts dtos.ChatOptions) (*dtos.ChatResponse, error) {
			content := messages[0].Content
			if !strings.Contains(content, "ROSTER PEMBICARA RESMI") {
				// Pass 1: Roster discovery
				return &dtos.ChatResponse{
					Content: `[{"label": "Speaker 0", "name": "Budi", "role": "Host"}, {"label": "Speaker 1", "name": "Dewi", "role": "Guest"}]`,
				}, nil
			}

			// Pass 2: Track concurrent worker execution
			cur := atomic.AddInt32(&activeWorkers, 1)
			for {
				old := atomic.LoadInt32(&maxObservedWorkers)
				if cur <= old || atomic.CompareAndSwapInt32(&maxObservedWorkers, old, cur) {
					break
				}
			}
			time.Sleep(20 * time.Millisecond) // simulate short LLM inference latency
			atomic.AddInt32(&activeWorkers, -1)

			// Determine chunk by looking at segment index markers at line starts in prompt
			if strings.Contains(content, "\n120 [") {
				return &dtos.ChatResponse{
					Content: `[{"index": 120, "speaker": "Speaker 0", "speaker_name": "Budi", "text": "Corrected speech turn segment 120"}]`,
				}, nil
			} else if strings.Contains(content, "\n60 [") {
				return &dtos.ChatResponse{
					Content: `[{"index": 60, "speaker": "Speaker 1", "speaker_name": "Dewi", "text": "Corrected speech turn segment 60"}]`,
				}, nil
			} else if strings.Contains(content, "\n0 [") {
				return &dtos.ChatResponse{
					Content: `[{"index": 0, "speaker": "Speaker 0", "speaker_name": "Budi", "text": "Corrected speech turn segment 0"}]`,
				}, nil
			}

			return &dtos.ChatResponse{Content: "[]"}, nil
		},
	}

	svc := &Service{llm: mockLLM}
	result := svc.diarizeAndCorrectSegmentsWithLLM(context.Background(), segments)

	if len(result) != 150 {
		t.Fatalf("expected 150 segments returned, got %d", len(result))
	}

	// Verify corrected segments
	if result[0].Text != "Corrected speech turn segment 0" {
		t.Errorf("expected segment 0 corrected, got %s", result[0].Text)
	}
	if result[60].Text != "Corrected speech turn segment 60" {
		t.Errorf("expected segment 60 corrected, got %s", result[60].Text)
	}
	if result[120].Text != "Corrected speech turn segment 120" {
		t.Errorf("expected segment 120 corrected, got %s", result[120].Text)
	}

	// Verify that chunks were actually executed concurrently across multiple workers
	maxWorkers := atomic.LoadInt32(&maxObservedWorkers)
	if maxWorkers < 2 {
		t.Errorf("expected at least 2 concurrent workers for 3 large chunks, got %d", maxWorkers)
	}

	// Verify canonical name propagation across chunks
	for i, seg := range result {
		if seg.SpeakerLabel == "Speaker 0" && seg.SpeakerName != "Budi" {
			t.Errorf("expected segment %d SpeakerName Budi, got %s", i, seg.SpeakerName)
		}
		if seg.SpeakerLabel == "Speaker 1" && seg.SpeakerName != "Dewi" {
			t.Errorf("expected segment %d SpeakerName Dewi, got %s", i, seg.SpeakerName)
		}
	}
}

func TestService_BuildModelTranscriptSegments(t *testing.T) {
	recID := uuid.New()
	svc := &Service{}

	t.Run("converts valid segments and ignores music/noise", func(t *testing.T) {
		input := []dtos.SegmentResult{
			{Start: 0.0, End: 2.0, SpeakerLabel: "Speaker 0", Text: "Halo dunia"},
			{Start: 2.1, End: 4.0, SpeakerLabel: "Speaker 0", Text: "[Musik]"},
			{Start: 4.5, End: 7.0, SpeakerLabel: "Speaker 1", SpeakerName: "Andi", Text: "Halo juga"},
		}

		res := svc.buildModelTranscriptSegments(recID, input, "", 10.0)
		if len(res) != 2 {
			t.Fatalf("expected 2 segments after noise filter, got %d", len(res))
		}
		if res[0].Text != "Halo dunia" || res[0].SpeakerLabel != "Speaker 0" {
			t.Errorf("unexpected segment 0: %+v", res[0])
		}
		if res[1].Text != "Halo juga" || res[1].SpeakerName != "Andi" {
			t.Errorf("unexpected segment 1: %+v", res[1])
		}
	})

	t.Run("fallback to single segment when segments empty", func(t *testing.T) {
		res := svc.buildModelTranscriptSegments(recID, nil, "Ini adalah teks tanpa segmen waktu.", 15.0)
		if len(res) != 1 {
			t.Fatalf("expected 1 fallback segment, got %d", len(res))
		}
		if res[0].Text != "Ini adalah teks tanpa segmen waktu." || res[0].EndTime != 15.0 {
			t.Errorf("unexpected fallback segment: %+v", res[0])
		}
	})
}

