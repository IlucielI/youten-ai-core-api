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
