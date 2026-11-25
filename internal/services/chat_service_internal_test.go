package services

import (
	"testing"

	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
)

func TestService_PopulateSummaryMetadata_HeadingWhitespaceTrimming(t *testing.T) {
	meta := &dtos.RecordingContextMetadata{}

	summary := &models.Summary{
		StructuredData: models.JSONMap{
			"executive_summary": "High-level overview",
			"action_items":      []interface{}{"Action item 1"},
			"key_decisions":     []interface{}{"Migrate to Go 1.25"},
			"open_issues":       []interface{}{"Pending review"},
		},
	}

	populateSummaryMetadata(meta, summary)

	if meta.SummaryOverview != "High-level overview" {
		t.Errorf("expected summary overview 'High-level overview', got '%s'", meta.SummaryOverview)
	}
	if len(meta.ActionItems) != 1 || meta.ActionItems[0] != "Action item 1" {
		t.Errorf("unexpected action items: %v", meta.ActionItems)
	}
	if len(meta.KeyTopics) != 2 {
		t.Fatalf("expected 2 key topics, got %d: %v", len(meta.KeyTopics), meta.KeyTopics)
	}
	if meta.KeyTopics[0] != "Keputusan Kunci (Key Decisions)" {
		t.Errorf("expected 'Keputusan Kunci (Key Decisions)', got '%s'", meta.KeyTopics[0])
	}
	if meta.KeyTopics[1] != "Isu Terbuka (Open Issues)" {
		t.Errorf("expected 'Isu Terbuka (Open Issues)', got '%s'", meta.KeyTopics[1])
	}
}

func TestComputeLexicalAnalysis_ExactAndVariantCounts(t *testing.T) {
	segments := []models.TranscriptSegment{
		{
			SpeakerLabel: "Speaker 1",
			SpeakerName:  "Speaker 1",
			StartTime:    10.0,
			EndTime:      20.0,
			Text:         "Mungkin kita bisa mulai sekarang, tapi kemungkinan ada kendala VPN.",
		},
		{
			SpeakerLabel: "Speaker 2",
			SpeakerName:  "Speaker 2",
			StartTime:    25.0,
			EndTime:      35.0,
			Text:         "Nggak mungkin sih kalau tanpa akun, mungkin besok baru memungkinkan.",
		},
		{
			SpeakerLabel: "Speaker 1",
			SpeakerName:  "Speaker 1",
			StartTime:    40.0,
			EndTime:      50.0,
			Text:         "Baik, kita lanjut ke topik berikutnya.",
		},
	}

	report, chunks := ComputeLexicalAnalysis(segments, []string{"mungkin"})
	if len(chunks) != 2 {
		t.Fatalf("expected 2 matching segment chunks, got %d", len(chunks))
	}
	if chunks[0].StartTime != 10.0 || chunks[1].StartTime != 25.0 {
		t.Errorf("unexpected chunk timestamps: %+v", chunks)
	}
	if report == "" {
		t.Fatal("expected non-empty lexical analysis report")
	}
}

func TestNormalizeChatRetrievalPlan(t *testing.T) {
	p1 := normalizeChatRetrievalPlan(dtos.ChatRetrievalPlan{
		Strategy:    "lexical_search",
		TargetTerms: []string{"  \"mungkin\" ", "MUNGKIN", ""},
	})
	if p1.Strategy != "lexical_search" || len(p1.TargetTerms) != 1 || p1.TargetTerms[0] != "mungkin" {
		t.Fatalf("unexpected normalized lexical plan: %+v", p1)
	}

	// Empty terms on lexical_search falls back to vector_top_k
	p2 := normalizeChatRetrievalPlan(dtos.ChatRetrievalPlan{
		Strategy:    "lexical_search",
		TargetTerms: []string{"   "},
	})
	if p2.Strategy != "vector_top_k" {
		t.Fatalf("expected fallback to vector_top_k when terms are empty, got %+v", p2)
	}

	p3 := normalizeChatRetrievalPlan(dtos.ChatRetrievalPlan{
		Strategy: "full_transcript",
	})
	if p3.Strategy != "full_transcript" {
		t.Fatalf("expected full_transcript, got %+v", p3)
	}
}

func TestSampleAndMergeChronologicalChunks(t *testing.T) {
	chunks := []dtos.TranscriptChunkData{
		{ChunkIndex: 0, StartTime: 0, EndTime: 10},
		{ChunkIndex: 1, StartTime: 10, EndTime: 20},
		{ChunkIndex: 2, StartTime: 20, EndTime: 30},
	}

	// maxCount == 1 must not divide by zero
	s1 := sampleChronologicalChunks(chunks, 1)
	if len(s1) != 1 || s1[0].ChunkIndex != 0 {
		t.Fatalf("expected 1 sampled chunk at index 0, got %+v", s1)
	}

	// Union coverage across [0, 10] and [10, 20] must cover [2, 18]
	if !isTimeRangeCovered(chunks[:2], 2, 18) {
		t.Error("expected [2, 18] to be covered by union of [0, 10] and [10, 20]")
	}

	// Combining mark in NFD word must remain a single token
	toks := tokenizeUnicodeWords("cafe\u0301 world")
	if len(toks) != 2 || toks[0] != "cafe\u0301" {
		t.Errorf("expected combining mark preserved in single token, got %v", toks)
	}
}
