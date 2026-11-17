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
