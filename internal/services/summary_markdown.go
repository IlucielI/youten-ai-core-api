package services

import (
	"fmt"
	"strings"
)

// AssembleStructuredMarkdown converts a structured summary JSON map into a rich,
// formatted Markdown document (Executive Overview, Discussion Topics, Open Points, Next Actions).
// If markdown_content is already present in structMap, it is returned directly.
func AssembleStructuredMarkdown(structMap map[string]interface{}, fallbackRaw string) string {
	if len(structMap) == 0 {
		return strings.TrimSpace(fallbackRaw)
	}

	if md, ok := structMap["markdown_content"].(string); ok && strings.TrimSpace(md) != "" {
		return strings.TrimSpace(md)
	}

	hasSections := hasStructuredSections(structMap)

	var sb strings.Builder

	renderTitleAndOverview(&sb, structMap, hasSections)
	renderDiscussionTopics(&sb, structMap)
	renderKeyDecisions(&sb, structMap)
	renderOpenPoints(&sb, structMap)
	renderActionItems(&sb, structMap)
	renderKeyTakeaways(&sb, structMap)

	res := strings.TrimSpace(sb.String())
	if res == "" {
		return strings.TrimSpace(fallbackRaw)
	}
	return res
}

func hasStructuredSections(structMap map[string]interface{}) bool {
	if raw, ok := structMap["discussion_topics"].([]interface{}); ok && len(raw) > 0 {
		return true
	}
	if raw, ok := structMap["core_themes"].([]interface{}); ok && len(raw) > 0 {
		return true
	}
	if raw, ok := structMap["key_decisions"].([]interface{}); ok && len(raw) > 0 {
		return true
	}
	if raw, ok := structMap["decisions_adopted"].([]interface{}); ok && len(raw) > 0 {
		return true
	}
	if raw, ok := structMap["open_points"].([]interface{}); ok && len(raw) > 0 {
		return true
	}
	if raw, ok := structMap["open_issues"].([]interface{}); ok && len(raw) > 0 {
		return true
	}
	if raw, ok := structMap["action_items"].([]interface{}); ok && len(raw) > 0 {
		return true
	}
	if raw, ok := structMap["next_steps"].([]interface{}); ok && len(raw) > 0 {
		return true
	}
	return false
}

func renderTitleAndOverview(sb *strings.Builder, structMap map[string]interface{}, hasSections bool) {
	title := extractFirstString(structMap, "meeting_title", "title", "episode_title", "lecture_title", "song_title")
	if title != "" {
		sb.WriteString(fmt.Sprintf("# %s\n\n", title))
	} else if hasSections {
		sb.WriteString("# Ringkasan Eksekutif\n\n")
	}

	if goal, ok := structMap["meeting_goal"].(string); ok && strings.TrimSpace(goal) != "" {
		sb.WriteString(fmt.Sprintf("**Tujuan Pertemuan:** %s\n\n", strings.TrimSpace(goal)))
	}

	overview := extractFirstString(structMap, "executive_summary", "executive_overview", "show_notes", "course_summary", "context", "abstract_or_summary", "core_message")
	if overview != "" {
		sb.WriteString(fmt.Sprintf("%s\n\n", overview))
	}
}

func renderDiscussionTopics(sb *strings.Builder, structMap map[string]interface{}) {
	if rawTopics, ok := structMap["discussion_topics"].([]interface{}); ok && len(rawTopics) > 0 {
		sb.WriteString("## Sub Pembahasan & Poin Kunci\n\n")
		for idx, raw := range rawTopics {
			topicMap, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			topicName, _ := topicMap["topic"].(string)
			if strings.TrimSpace(topicName) == "" {
				topicName = fmt.Sprintf("Topik %d", idx+1)
			}
			sb.WriteString(fmt.Sprintf("### %d. %s\n", idx+1, strings.TrimSpace(topicName)))

			if rawPoints, ok := topicMap["key_points"].([]interface{}); ok && len(rawPoints) > 0 {
				for _, p := range rawPoints {
					if str := strings.TrimSpace(fmt.Sprintf("%v", p)); str != "" {
						sb.WriteString(fmt.Sprintf("- %s\n", str))
					}
				}
			}
			sb.WriteString("\n")
		}
		return
	}

	// Fallback to core_themes if discussion_topics is not present
	if rawThemes, ok := structMap["core_themes"].([]interface{}); ok && len(rawThemes) > 0 {
		sb.WriteString("## Sub Pembahasan\n\n")
		for idx, raw := range rawThemes {
			themeMap, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			themeName, _ := themeMap["theme"].(string)
			if strings.TrimSpace(themeName) == "" {
				themeName = fmt.Sprintf("Topik %d", idx+1)
			}
			sb.WriteString(fmt.Sprintf("### %d. %s\n", idx+1, strings.TrimSpace(themeName)))

			if rawPoints, ok := themeMap["summary_points"].([]interface{}); ok && len(rawPoints) > 0 {
				for _, p := range rawPoints {
					if str := strings.TrimSpace(fmt.Sprintf("%v", p)); str != "" {
						sb.WriteString(fmt.Sprintf("- %s\n", str))
					}
				}
			}
			sb.WriteString("\n")
		}
	}
}

func renderKeyDecisions(sb *strings.Builder, structMap map[string]interface{}) {
	if rawDecisions, ok := structMap["key_decisions"].([]interface{}); ok && len(rawDecisions) > 0 {
		sb.WriteString("## Keputusan Kunci (Key Decisions)\n\n")
		for _, raw := range rawDecisions {
			if decMap, ok := raw.(map[string]interface{}); ok {
				dec, _ := decMap["decision"].(string)
				app, _ := decMap["approved_by"].(string)
				if app != "" {
					sb.WriteString(fmt.Sprintf("- %s *(Disetujui oleh: %s)*\n", dec, app))
				} else {
					sb.WriteString(fmt.Sprintf("- %s\n", dec))
				}
			} else {
				sb.WriteString(fmt.Sprintf("- %v\n", raw))
			}
		}
		sb.WriteString("\n")
		return
	}

	if rawAdopted, ok := structMap["decisions_adopted"].([]interface{}); ok && len(rawAdopted) > 0 {
		sb.WriteString("## Keputusan Diadopsi\n\n")
		for _, raw := range rawAdopted {
			if decMap, ok := raw.(map[string]interface{}); ok {
				dec, _ := decMap["decision"].(string)
				just, _ := decMap["technical_justification"].(string)
				if just != "" {
					sb.WriteString(fmt.Sprintf("- **%s**: %s\n", dec, just))
				} else {
					sb.WriteString(fmt.Sprintf("- %s\n", dec))
				}
			}
		}
		sb.WriteString("\n")
	}
}

func renderOpenPoints(sb *strings.Builder, structMap map[string]interface{}) {
	var points []string

	addPoints := func(key string) {
		rawList, ok := structMap[key].([]interface{})
		if !ok || len(rawList) == 0 {
			return
		}
		for _, raw := range rawList {
			str := strings.TrimSpace(fmt.Sprintf("%v", raw))
			if str == "" {
				continue
			}
			if !sliceContains(points, str) {
				points = append(points, str)
			}
		}
	}

	addPoints("open_points")
	addPoints("open_issues")

	if len(points) == 0 {
		return
	}

	sb.WriteString("## Isu Terbuka (Open Points)\n\n")
	for _, p := range points {
		sb.WriteString(fmt.Sprintf("- [ ] %s\n", p))
	}
	sb.WriteString("\n")
}

func renderActionItems(sb *strings.Builder, structMap map[string]interface{}) {
	rawItems, ok := structMap["action_items"].([]interface{})
	if !ok || len(rawItems) == 0 {
		rawItems, ok = structMap["next_steps"].([]interface{})
	}
	if !ok || len(rawItems) == 0 {
		return
	}

	sb.WriteString("## Tindakan Lanjutan (Next Action)\n\n")
	for _, raw := range rawItems {
		itemMap, ok := raw.(map[string]interface{})
		if !ok {
			if str := strings.TrimSpace(fmt.Sprintf("%v", raw)); str != "" {
				sb.WriteString(fmt.Sprintf("- [ ] %s\n", str))
			}
			continue
		}

		task := extractFirstString(itemMap, "task", "action", "text")
		if task == "" {
			continue
		}

		pic := extractFirstString(itemMap, "pic", "assignee", "owner")
		due := extractFirstString(itemMap, "due_date", "target_date", "target_sprint", "timeline")
		priority, _ := itemMap["priority"].(string)

		var meta []string
		if pic != "" {
			meta = append(meta, fmt.Sprintf("PIC: %s", pic))
		}
		if due != "" {
			meta = append(meta, fmt.Sprintf("Target: %s", due))
		}
		if priority != "" {
			meta = append(meta, fmt.Sprintf("Prioritas: %s", priority))
		}

		if len(meta) > 0 {
			sb.WriteString(fmt.Sprintf("- [ ] **%s** *(%s)*\n", task, strings.Join(meta, ", ")))
		} else {
			sb.WriteString(fmt.Sprintf("- [ ] %s\n", task))
		}
	}
	sb.WriteString("\n")
}

func renderKeyTakeaways(sb *strings.Builder, structMap map[string]interface{}) {
	rawTakeaways, ok := structMap["key_takeaways"].([]interface{})
	if !ok || len(rawTakeaways) == 0 {
		return
	}

	sb.WriteString("## Poin Kunci (Key Takeaways)\n\n")
	for _, raw := range rawTakeaways {
		if str := strings.TrimSpace(fmt.Sprintf("%v", raw)); str != "" {
			sb.WriteString(fmt.Sprintf("- %s\n", str))
		}
	}
	sb.WriteString("\n")
}

func extractFirstString(m map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if val, ok := m[k].(string); ok && strings.TrimSpace(val) != "" {
			return strings.TrimSpace(val)
		}
	}
	return ""
}

func sliceContains(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}
