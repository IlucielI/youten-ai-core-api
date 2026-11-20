package services

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSplitTranscriptIntoChunks_LongLinesWithoutNewlines(t *testing.T) {
	// A single continuous line of 5,000 characters without newlines
	longLine := strings.Repeat("Kata-kata-panjang-tanpa-newline ", 160)
	maxChunk := 1000
	chunks := splitTranscriptIntoChunks(longLine, maxChunk)

	if len(chunks) < 4 {
		t.Errorf("expected at least 4 chunks, got %d", len(chunks))
	}
	totalChars := 0
	for _, c := range chunks {
		if utf8.RuneCountInString(c) > maxChunk {
			t.Errorf("chunk exceeds maxChunk (%d runes): %d", maxChunk, utf8.RuneCountInString(c))
		}
		totalChars += utf8.RuneCountInString(c)
	}
	// Verify content wasn't silently dropped
	if totalChars < utf8.RuneCountInString(strings.TrimSpace(longLine))-50 {
		t.Errorf("expected near full retention, totalChars=%d, original=%d", totalChars, utf8.RuneCountInString(longLine))
	}
}

func TestDeterministicCondenseChunk_RuneSafeMultibyte(t *testing.T) {
	// Japanese / multibyte characters where each rune is 3 bytes
	multibyteText := "こんにちは世界。これは長いテキストです。\n第二行です。\n第三行です。\n第四行です。\n第五行です。"
	condensed := deterministicCondenseChunk(multibyteText, 15)

	if !utf8.ValidString(condensed) {
		t.Errorf("condensed text is not valid UTF-8: %s", condensed)
	}
	if utf8.RuneCountInString(condensed) > 15 {
		t.Errorf("expected at most 15 runes, got %d runes", utf8.RuneCountInString(condensed))
	}
}

func TestSplitTranscriptIntoChunksWithOverlap_BoundaryContext(t *testing.T) {
	lines := []string{
		"[00:01:00] Alice: Pembahasan pertama mengenai migrasi database.",
		"[00:02:00] Bob: Kami mengusulkan PostgreSQL 16.",
		"[00:03:00] Charlie: Apakah latency replikasi akan berkurang?",
		"[00:04:00] Bob: Ya, replikasi streaming WAL jauh lebih cepat.",
		"[00:05:00] Alice: Baik, kita sepakati migrasi ke PG 16.",
		"[00:06:00] David: Topik kedua adalah refactoring worker pool RabbitMQ.",
		"[00:07:00] Eve: Worker saat ini menggunakan prefetch count 10.",
		"[00:08:00] David: Kita naikkan prefetch menjadi 50 untuk throughput tinggi.",
		"[00:09:00] Alice: Tindakan: David update konfigurasi deployment sebelum Jumat.",
	}
	body := strings.Join(lines, "\n")
	// Chunk size small enough to force 2 or 3 chunks
	maxChunk := 250
	overlap := 150

	chunks := splitTranscriptIntoChunksWithOverlap(body, maxChunk, overlap)
	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d", len(chunks))
	}

	// First chunk must not have overlap
	if chunks[0].overlapText != "" {
		t.Errorf("chunk 0 should have empty overlapText, got: %s", chunks[0].overlapText)
	}

	// Subsequent chunks must have non-empty overlap containing trailing lines from prior chunk
	for i := 1; i < len(chunks); i++ {
		if chunks[i].overlapText == "" {
			t.Errorf("chunk %d should have non-empty overlapText", i)
		}
		// Overlap should be found in prior chunk text
		priorLines := strings.Split(chunks[i-1].text, "\n")
		lastPriorLine := strings.TrimSpace(priorLines[len(priorLines)-1])
		if !strings.Contains(chunks[i].overlapText, lastPriorLine) {
			t.Errorf("chunk %d overlapText %q should contain last prior line %q", i, chunks[i].overlapText, lastPriorLine)
		}
	}
}

func TestSmartDeterministicCondense_PriorityActionItemRetention(t *testing.T) {
	lines := []string{
		"[00:00:10] Alice: Halo rekan-rekan sekalian, selamat pagi.",
		"[00:00:20] Bob: Pagi Alice, apakah suara saya terdengar jelas?",
		"[00:00:30] Charlie: Ya, terdengar jelas sekali.",
		"[00:01:00] Alice: Keputusan: Kita alokasikan budget $5,000 untuk server GPU baru.",
		"[00:01:30] Bob: Bagaimana dengan timeline pengadaan hardware?",
		"[00:02:00] Alice: Action item: Bob hubungi vendor hardware paling lambat 15 Oktober, PIC: Bob.",
		"[00:02:30] Charlie: Baik, mari kita tutup rapat hari ini.",
		"[00:02:40] Bob: Terima kasih semuanya.",
	}
	text := strings.Join(lines, "\n")

	// Constrain budget so that greetings/fillers must be dropped, but priority action item and decision are retained
	maxChars := 220
	condensed := smartDeterministicCondense(text, maxChars)

	if utf8.RuneCountInString(condensed) > maxChars {
		t.Errorf("expected <= %d runes, got %d", maxChars, utf8.RuneCountInString(condensed))
	}

	// Decision and action item MUST be retained!
	if !strings.Contains(condensed, "Keputusan: Kita alokasikan budget") {
		t.Errorf("critical decision was lost during condensation: %s", condensed)
	}
	if !strings.Contains(condensed, "Action item: Bob hubungi vendor") {
		t.Errorf("critical action item was lost during condensation: %s", condensed)
	}

	// Chronological integrity check: Decision must appear BEFORE Action item
	decIdx := strings.Index(condensed, "Keputusan:")
	actIdx := strings.Index(condensed, "Action item:")
	if decIdx >= actIdx {
		t.Errorf("chronological order was violated: decIdx=%d, actIdx=%d", decIdx, actIdx)
	}
}
