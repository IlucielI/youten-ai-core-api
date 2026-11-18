package services

import (
	"strings"
	"testing"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
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

func TestCorrelateVisualSpeakerNames(t *testing.T) {
	keyframes := []dtos.ExtractedKeyframe{
		{TimestampSeconds: 5.0, SequenceOrder: 1},
		{TimestampSeconds: 15.0, SequenceOrder: 2},
		{TimestampSeconds: 25.0, SequenceOrder: 3},
		{TimestampSeconds: 99.0, SequenceOrder: 4}, // no matching segment
	}

	analyses := []*dtos.KeyframeAnalysisResult{
		{
			ActiveSpeaker: "Bayu Anugerah",
			Confidence:    0.95,
		},
		{
			CueType:         constants.VisualCueTypeActiveSpeaker,
			VisibleSpeakers: []string{"Budi Santoso"},
			Confidence:      0.90,
		},
		{
			ActiveSpeaker: "Speaker 2", // generic label, should be ignored
			Confidence:    0.80,
		},
		{
			ActiveSpeaker: "Unmatched Person",
			Confidence:    0.85,
		},
	}

	segments := []models.TranscriptSegment{
		{StartTime: 0.0, EndTime: 10.0, SpeakerLabel: "Speaker 0"},
		{StartTime: 10.5, EndTime: 20.0, SpeakerLabel: "Speaker 1"},
		{StartTime: 20.5, EndTime: 30.0, SpeakerLabel: "Speaker 2"},
	}

	speakerMap := correlateVisualSpeakerNames(keyframes, analyses, segments)

	if len(speakerMap) != 2 {
		t.Fatalf("expected 2 mapped speakers, got %d: %v", len(speakerMap), speakerMap)
	}

	if speakerMap["Speaker 0"] != "Bayu Anugerah" {
		t.Errorf("expected Speaker 0 to be 'Bayu Anugerah', got %q", speakerMap["Speaker 0"])
	}

	if speakerMap["Speaker 1"] != "Budi Santoso" {
		t.Errorf("expected Speaker 1 to be 'Budi Santoso', got %q", speakerMap["Speaker 1"])
	}

	if _, exists := speakerMap["Speaker 2"]; exists {
		t.Errorf("expected Speaker 2 to NOT be mapped from generic label, but got %q", speakerMap["Speaker 2"])
	}
}

func TestCorrelateVisualSpeakerNames_EmptyInputs(t *testing.T) {
	if res := correlateVisualSpeakerNames(nil, nil, nil); res != nil {
		t.Errorf("expected nil for empty inputs, got %v", res)
	}
}

func TestExtractExternalRosterFromAnalytics(t *testing.T) {
	t.Run("nil analytics returns nil", func(t *testing.T) {
		if res := extractExternalRosterFromAnalytics(nil); res != nil {
			t.Errorf("expected nil, got: %v", res)
		}
	})

	t.Run("extracts participants and visual speakers deduplicated", func(t *testing.T) {
		analytics := models.JSONMap{
			"meeting_participants": []map[string]interface{}{
				{"name": "Bayu Anugerah", "email": "bayu@example.com"},
				{"name": "Budi Santoso", "email": "budi@example.com"},
				{"name": "Speaker 1"}, // generic placeholder, should be ignored
			},
			"visual_speakers": map[string]string{
				"Speaker 0": "Bayu Anugerah", // duplicate, should be skipped
				"Speaker 2": "Citra Kirana",
			},
		}

		names := extractExternalRosterFromAnalytics(analytics)
		if len(names) != 3 {
			t.Fatalf("expected 3 unique real names, got %d: %v", len(names), names)
		}

		expected := map[string]bool{"Bayu Anugerah": true, "Budi Santoso": true, "Citra Kirana": true}
		for _, n := range names {
			if !expected[n] {
				t.Errorf("unexpected name extracted: %q", n)
			}
		}
	})
}
