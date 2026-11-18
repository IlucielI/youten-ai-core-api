package services

import (
	"strings"
	"testing"
)

func TestExtractFirstLine_UTF8Multibyte(t *testing.T) {
	// 100 fullwidth Japanese characters (each 3 bytes in UTF-8)
	longJapanese := "これは非常に長いプレゼンテーションのスライドタイトルであり、日本語のマルチバイト文字が８０文字以上連続して含まれています。テスト用のテキストです。追加の文章を入れて確実に８０文字を超えるようにします。"
	firstLine := extractFirstLine(longJapanese)
	runes := []rune(firstLine)
	// 80 runes + 3 dots "..." = 83 runes
	if len(runes) != 83 {
		t.Errorf("expected 83 runes for truncated title, got %d", len(runes))
	}
	if !strings.HasSuffix(firstLine, "...") {
		t.Errorf("expected title to end with '...', got: %s", firstLine)
	}
}

func TestExtractFirstLine_Short(t *testing.T) {
	text := "Short Title\nSecond line"
	line := extractFirstLine(text)
	if line != "Short Title" {
		t.Errorf("expected 'Short Title', got: %s", line)
	}
}
