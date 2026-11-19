package services_test

import (
	"strings"
	"testing"

	"code-base-golang/internal/services"
)

func TestAssembleStructuredMarkdown(t *testing.T) {
	t.Run("returns empty or raw fallback on empty map", func(t *testing.T) {
		res := services.AssembleStructuredMarkdown(nil, "fallback raw")
		if res != "fallback raw" {
			t.Errorf("expected 'fallback raw', got '%s'", res)
		}
	})

	t.Run("returns existing markdown_content if already present", func(t *testing.T) {
		data := map[string]interface{}{
			"markdown_content": "# Direct Markdown Content",
			"executive_summary": "Ignored because markdown_content exists",
		}
		res := services.AssembleStructuredMarkdown(data, "fallback")
		if res != "# Direct Markdown Content" {
			t.Errorf("expected direct markdown content, got '%s'", res)
		}
	})

	t.Run("assembles full markdown with topics, open points, and action items", func(t *testing.T) {
		data := map[string]interface{}{
			"meeting_title":     "Sprint Architecture Review",
			"meeting_goal":      "Finalize Q4 Database Architecture",
			"executive_summary": "Discussed migration to PostgreSQL and caching layer.",
			"discussion_topics": []interface{}{
				map[string]interface{}{
					"topic": "PostgreSQL Partitioning",
					"key_points": []interface{}{
						"Partition by recording date",
						"Add composite indexes",
					},
				},
				map[string]interface{}{
					"topic": "Redis Cache Invalidation",
					"key_points": []interface{}{
						"Use pub/sub channel for cross-node invalidation",
					},
				},
			},
			"open_points": []interface{}{
				"Need approval from DevOps for multi-region Redis cluster",
				"Pending benchmark results under 10k RPS",
			},
			"action_items": []interface{}{
				map[string]interface{}{
					"task":     "Draft migration script",
					"pic":      "Alice",
					"due_date": "2026-11-25",
					"priority": "P0",
				},
				map[string]interface{}{
					"task":     "Run load test",
					"pic":      "Bob",
					"due_date": "2026-11-28",
					"priority": "P1",
				},
			},
			"key_decisions": []interface{}{
				map[string]interface{}{
					"decision":    "Adopt PostgreSQL pgvector for semantic search",
					"approved_by": "Tech Lead",
				},
			},
			"key_takeaways": []interface{}{
				"All microservices must respect single-responsibility boundary",
			},
		}

		res := services.AssembleStructuredMarkdown(data, "fallback")

		if !strings.Contains(res, "# Sprint Architecture Review") {
			t.Errorf("expected title in markdown, got:\n%s", res)
		}
		if !strings.Contains(res, "**Tujuan Pertemuan:** Finalize Q4 Database Architecture") {
			t.Errorf("expected goal in markdown, got:\n%s", res)
		}
		if !strings.Contains(res, "## Sub Pembahasan & Poin Kunci") {
			t.Errorf("expected discussion topics heading, got:\n%s", res)
		}
		if !strings.Contains(res, "### 1. PostgreSQL Partitioning") || !strings.Contains(res, "- Partition by recording date") {
			t.Errorf("expected topic 1 and key points, got:\n%s", res)
		}
		if !strings.Contains(res, "## Isu Terbuka (Open Points)") || !strings.Contains(res, "- [ ] Need approval from DevOps") {
			t.Errorf("expected open points section, got:\n%s", res)
		}
		if !strings.Contains(res, "## Tindakan Lanjutan (Next Action)") {
			t.Errorf("expected action items heading, got:\n%s", res)
		}
		if !strings.Contains(res, "**Draft migration script** *(PIC: Alice, Target: 2026-11-25, Prioritas: P0)*") {
			t.Errorf("expected formatted action item with metadata, got:\n%s", res)
		}
		if !strings.Contains(res, "## Keputusan Kunci (Key Decisions)") || !strings.Contains(res, "Adopt PostgreSQL pgvector") {
			t.Errorf("expected key decisions section, got:\n%s", res)
		}
		if !strings.Contains(res, "## Poin Kunci (Key Takeaways)") {
			t.Errorf("expected key takeaways section, got:\n%s", res)
		}
	})

	t.Run("falls back to core_themes when discussion_topics is absent", func(t *testing.T) {
		data := map[string]interface{}{
			"executive_overview": "Overview of general meeting",
			"core_themes": []interface{}{
				map[string]interface{}{
					"theme": "Budget Allocation",
					"summary_points": []interface{}{
						"Allocated $50k for infrastructure",
					},
				},
			},
		}
		res := services.AssembleStructuredMarkdown(data, "fallback")
		if !strings.Contains(res, "## Sub Pembahasan") || !strings.Contains(res, "### 1. Budget Allocation") {
			t.Errorf("expected fallback core_themes rendering, got:\n%s", res)
		}
	})
}

