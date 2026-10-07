package services

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
	"code-base-golang/internal/pkg/ctxmeta"
	"code-base-golang/internal/pkg/strutil"
	"code-base-golang/internal/pkg/timeutil"
)

// ExportRecordingMOM generates a multi-format export of a meeting recording's executive summary,
// action items, chapter breakdown, and diarized transcript.
func (s *Service) ExportRecordingMOM(ctx context.Context, id uuid.UUID, ownershipToken string, format string) (*dtos.ExportResult, error) {
	rec, err := s.repo.FindRecordingByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrRecordingNotFound
		}
		return nil, fmt.Errorf("failed to lookup recording: %w", err)
	}

	// Verify authorization: owner via JWT, guest token, or public share
	hasAccess := false
	isAuth := ctxmeta.IsAuthenticated(ctx)
	userID, hasUserID := ctxmeta.GetAuthUserID(ctx)

	if isAuth && hasUserID && rec.UserID != nil && *rec.UserID == userID {
		hasAccess = true
	} else if ownershipToken != "" && subtle.ConstantTimeCompare([]byte(ownershipToken), []byte(rec.OwnershipToken)) == 1 {
		hasAccess = true
	} else if rec.IsShareEnabled {
		hasAccess = true
	}

	if !hasAccess {
		return nil, constants.ErrForbidden
	}

	// Normalize format
	normFormat := strings.ToLower(strings.TrimSpace(format))
	if normFormat == "" {
		normFormat = string(constants.ExportFormatMarkdown)
	}

	var expFormat constants.ExportFormat
	switch normFormat {
	case "markdown", "md":
		expFormat = constants.ExportFormatMarkdown
	case "txt", "text", "plain":
		expFormat = constants.ExportFormatTxt
	case "json":
		expFormat = constants.ExportFormatJSON
	case "pdf":
		expFormat = constants.ExportFormatPDF
	default:
		return nil, constants.ErrBadRequest
	}

	// Fetch related entities (gracefully handling missing or empty records)
	summary, err := s.repo.FindActiveSummaryByRecordingID(ctx, id)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to fetch active summary: %w", err)
	}

	segments, err := s.repo.ListTranscriptSegmentsByRecordingID(ctx, id)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to fetch transcript segments: %w", err)
	}

	chapters, err := s.repo.ListChaptersByRecordingID(ctx, id)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to fetch chapters: %w", err)
	}

	highlights, err := s.repo.ListHighlightsByRecordingID(ctx, id)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to fetch highlights: %w", err)
	}

	baseName := strutil.SanitizeFilename(rec.Title, fmt.Sprintf("recording_%s", rec.ID.String()[:8]))

	switch expFormat {
	case constants.ExportFormatMarkdown:
		data := buildMarkdownExport(rec, summary, chapters, highlights, segments)
		return &dtos.ExportResult{
			Filename:    baseName + "_mom.md",
			ContentType: "text/markdown; charset=utf-8",
			Data:        []byte(data),
		}, nil

	case constants.ExportFormatTxt:
		data := buildTextExport(rec, summary, chapters, highlights, segments)
		return &dtos.ExportResult{
			Filename:    baseName + "_mom.txt",
			ContentType: "text/plain; charset=utf-8",
			Data:        []byte(data),
		}, nil

	case constants.ExportFormatJSON:
		data, jErr := buildJSONExport(rec, summary, chapters, highlights, segments)
		if jErr != nil {
			return nil, fmt.Errorf("failed to marshal export json: %w", jErr)
		}
		return &dtos.ExportResult{
			Filename:    baseName + "_mom.json",
			ContentType: "application/json; charset=utf-8",
			Data:        data,
		}, nil

	case constants.ExportFormatPDF:
		textContent := buildTextExport(rec, summary, chapters, highlights, segments)
		pdfBytes := generateStandardPDF(rec.Title, textContent)
		return &dtos.ExportResult{
			Filename:    baseName + "_mom.pdf",
			ContentType: "application/pdf",
			Data:        pdfBytes,
		}, nil
	}

	return nil, constants.ErrBadRequest
}

type formattedSection struct {
	Heading string
	Items   []string
}

type formattedSummary struct {
	Overview    string
	Sections    []formattedSection
	ActionItems []string
}

func parseSummaryData(summary *models.Summary) (map[string]interface{}, string) {
	if summary == nil {
		return nil, ""
	}

	data := make(map[string]interface{})
	if summary.StructuredData != nil {
		for k, v := range summary.StructuredData {
			data[k] = v
		}
	}

	rawMd := strings.TrimSpace(summary.MarkdownContent)
	// If StructuredData is empty or contains only meta keys, but MarkdownContent is JSON, parse it
	if (len(data) == 0 || (len(data) == 1 && (data["invalid_key"] != nil || data["$schema"] != nil))) && strings.HasPrefix(rawMd, "{") {
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(rawMd), &parsed); err == nil {
			for k, v := range parsed {
				data[k] = v
			}
		}
	}

	return data, rawMd
}

func formatSummaryContent(summary *models.Summary) formattedSummary {
	var res formattedSummary
	if summary == nil {
		return res
	}

	data, rawMd := parseSummaryData(summary)

	// 1. Daily Standup Schema
	if sh, ok := data["sprint_health"].(map[string]interface{}); ok {
		status, _ := sh["status"].(string)
		sum, _ := sh["summary"].(string)
		if status != "" && sum != "" {
			res.Overview = fmt.Sprintf("Status Sprint: %s\n%s", status, sum)
		} else if sum != "" {
			res.Overview = sum
		}
	}

	if mu, ok := data["member_updates"].([]interface{}); ok && len(mu) > 0 {
		var items []string
		for _, item := range mu {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			name, _ := m["member_name"].(string)
			if name == "" {
				name = "Anggota Tim"
			}
			items = append(items, fmt.Sprintf("• %s:", name))
			if y, ok := m["yesterday"].([]interface{}); ok && len(y) > 0 {
				items = append(items, "  - Kemarin:")
				for _, val := range y {
					items = append(items, fmt.Sprintf("    * %v", val))
				}
			}
			if t, ok := m["today"].([]interface{}); ok && len(t) > 0 {
				items = append(items, "  - Hari Ini:")
				for _, val := range t {
					items = append(items, fmt.Sprintf("    * %v", val))
				}
			}
			if b, ok := m["blockers"].([]interface{}); ok && len(b) > 0 {
				items = append(items, "  - Kendala:")
				for _, val := range b {
					items = append(items, fmt.Sprintf("    * %v", val))
					res.ActionItems = append(res.ActionItems, fmt.Sprintf("Kendala (%s): %v", name, val))
				}
			}
		}
		if len(items) > 0 {
			res.Sections = append(res.Sections, formattedSection{
				Heading: "Pembaruan Anggota (Member Updates)",
				Items:   items,
			})
		}
	}

	if cb, ok := data["critical_blockers"].([]interface{}); ok && len(cb) > 0 {
		var items []string
		for _, b := range cb {
			items = append(items, fmt.Sprintf("• %v", b))
			res.ActionItems = append(res.ActionItems, fmt.Sprintf("Kendala Kritis: %v", b))
		}
		res.Sections = append(res.Sections, formattedSection{
			Heading: "Kendala Kritis (Critical Blockers)",
			Items:   items,
		})
	}

	if pld, ok := data["parking_lot_discussions"].([]interface{}); ok && len(pld) > 0 {
		var items []string
		for _, p := range pld {
			if pm, ok := p.(map[string]interface{}); ok {
				topic, _ := pm["topic"].(string)
				parts, _ := pm["participants"].([]interface{})
				partStr := ""
				if len(parts) > 0 {
					var pNames []string
					for _, pt := range parts {
						pNames = append(pNames, fmt.Sprint(pt))
					}
					partStr = fmt.Sprintf(" (Peserta: %s)", strings.Join(pNames, ", "))
				}
				items = append(items, fmt.Sprintf("• %s%s", topic, partStr))
			} else {
				items = append(items, fmt.Sprintf("• %v", p))
			}
		}
		res.Sections = append(res.Sections, formattedSection{
			Heading: "Diskusi Lanjutan (Parking Lot)",
			Items:   items,
		})
	}

	// 2. Interview Schema
	if cand, ok := data["candidate_name"].(string); ok && cand != "" {
		role, _ := data["target_role"].(string)
		rec, _ := data["recommendation"].(string)
		just, _ := data["justification"].(string)
		res.Overview = fmt.Sprintf("Kandidat: %s | Posisi: %s\nRekomendasi: %s\n\n%s", cand, role, rec, just)
	}
	if str, ok := data["strengths"].([]interface{}); ok && len(str) > 0 {
		var items []string
		for _, s := range str {
			items = append(items, fmt.Sprintf("• %v", s))
		}
		res.Sections = append(res.Sections, formattedSection{
			Heading: "Kekuatan Utama (Strengths)",
			Items:   items,
		})
	}
	if con, ok := data["concerns"].([]interface{}); ok && len(con) > 0 {
		var items []string
		for _, c := range con {
			items = append(items, fmt.Sprintf("• %v", c))
		}
		res.Sections = append(res.Sections, formattedSection{
			Heading: "Catatan & Kekhawatiran (Concerns)",
			Items:   items,
		})
	}
	if cs, ok := data["competency_scores"].([]interface{}); ok && len(cs) > 0 {
		var items []string
		for _, c := range cs {
			if cm, ok := c.(map[string]interface{}); ok {
				comp, _ := cm["competency"].(string)
				rating, _ := cm["rating"].(string)
				evidence, _ := cm["evidence"].(string)
				items = append(items, fmt.Sprintf("• %s [%s]: %s", comp, rating, evidence))
			}
		}
		res.Sections = append(res.Sections, formattedSection{
			Heading: "Penilaian Kompetensi",
			Items:   items,
		})
	}

	// 3. One on One Schema
	if wb, ok := data["wellbeing_assessment"].(map[string]interface{}); ok {
		score, _ := wb["sentiment_score"].(string)
		sum, _ := wb["summary"].(string)
		if res.Overview == "" {
			res.Overview = fmt.Sprintf("Sentimen Kesejahteraan: %s\n%s", score, sum)
		}
	}
	if kw, ok := data["key_wins"].([]interface{}); ok && len(kw) > 0 {
		var items []string
		for _, w := range kw {
			items = append(items, fmt.Sprintf("• %v", w))
		}
		res.Sections = append(res.Sections, formattedSection{Heading: "Pencapaian Utama (Key Wins)", Items: items})
	}
	if bl, ok := data["blockers"].([]interface{}); ok && len(bl) > 0 {
		var items []string
		for _, b := range bl {
			items = append(items, fmt.Sprintf("• %v", b))
			res.ActionItems = append(res.ActionItems, fmt.Sprintf("Kendala 1-on-1: %v", b))
		}
		res.Sections = append(res.Sections, formattedSection{Heading: "Kendala (Blockers)", Items: items})
	}
	if com, ok := data["commitments"].([]interface{}); ok && len(com) > 0 {
		for _, c := range com {
			if cm, ok := c.(map[string]interface{}); ok {
				act, _ := cm["action_item"].(string)
				party, _ := cm["party"].(string)
				tl, _ := cm["timeline"].(string)
				res.ActionItems = append(res.ActionItems, fmt.Sprintf("[%s] %s (%s)", party, act, tl))
			}
		}
	}

	// 4. MOM / Standard Meeting Schema
	if mg, ok := data["meeting_goal"].(string); ok && strings.TrimSpace(mg) != "" {
		if res.Overview == "" {
			res.Overview = fmt.Sprintf("Tujuan Rapat: %s", mg)
		} else {
			res.Overview = fmt.Sprintf("Tujuan Rapat: %s\n\n%s", mg, res.Overview)
		}
	}
	if es, ok := data["executive_summary"].(string); ok && strings.TrimSpace(es) != "" {
		if res.Overview == "" {
			res.Overview = strings.TrimSpace(es)
		} else if !strings.Contains(res.Overview, strings.TrimSpace(es)) {
			res.Overview = res.Overview + "\n\n" + strings.TrimSpace(es)
		}
	}
	if kd, ok := data["key_decisions"].([]interface{}); ok && len(kd) > 0 {
		var items []string
		for _, d := range kd {
			if dm, ok := d.(map[string]interface{}); ok {
				dec, _ := dm["decision"].(string)
				app, _ := dm["approved_by"].(string)
				if app != "" {
					items = append(items, fmt.Sprintf("• %s (Disetujui: %s)", dec, app))
				} else {
					items = append(items, fmt.Sprintf("• %s", dec))
				}
			} else {
				items = append(items, fmt.Sprintf("• %v", d))
			}
		}
		res.Sections = append(res.Sections, formattedSection{Heading: "Keputusan Kunci (Key Decisions)", Items: items})
	}
	if oi, ok := data["open_issues"].([]interface{}); ok && len(oi) > 0 {
		var items []string
		for _, o := range oi {
			items = append(items, fmt.Sprintf("• %v", o))
		}
		res.Sections = append(res.Sections, formattedSection{Heading: "Isu Terbuka (Open Issues)", Items: items})
	}

	// 5. Tech Review Schema
	if ctxStr, ok := data["context"].(string); ok && ctxStr != "" && res.Overview == "" {
		res.Overview = ctxStr
	}
	if da, ok := data["decisions_adopted"].([]interface{}); ok && len(da) > 0 {
		var items []string
		for _, d := range da {
			if dm, ok := d.(map[string]interface{}); ok {
				dec, _ := dm["decision"].(string)
				just, _ := dm["technical_justification"].(string)
				items = append(items, fmt.Sprintf("• %s: %s", dec, just))
			}
		}
		res.Sections = append(res.Sections, formattedSection{Heading: "Keputusan Diadopsi", Items: items})
	}

	// 6. Sales Discovery Schema
	if pc, ok := data["prospect_company"].(string); ok && pc != "" && res.Overview == "" {
		st, _ := data["deal_stage_suggested"].(string)
		res.Overview = fmt.Sprintf("Perusahaan Prospek: %s (Tahap: %s)", pc, st)
	}
	if pp, ok := data["pain_points"].([]interface{}); ok && len(pp) > 0 {
		var items []string
		for _, p := range pp {
			if pm, ok := p.(map[string]interface{}); ok {
				pain, _ := pm["pain"].(string)
				cost, _ := pm["cost_of_inaction"].(string)
				items = append(items, fmt.Sprintf("• %s (Biaya Inaksi: %s)", pain, cost))
			}
		}
		res.Sections = append(res.Sections, formattedSection{Heading: "Titik Kendala (Pain Points)", Items: items})
	}
	if ns, ok := data["next_steps"].([]interface{}); ok && len(ns) > 0 {
		for _, n := range ns {
			if nm, ok := n.(map[string]interface{}); ok {
				act, _ := nm["action"].(string)
				own, _ := nm["owner"].(string)
				dt, _ := nm["target_date"].(string)
				res.ActionItems = append(res.ActionItems, fmt.Sprintf("%s (PIC: %s, Target: %s)", act, own, dt))
			}
		}
	}

	// Action Items from standard fields
	for _, item := range extractActionItems(summary) {
		found := false
		for _, existing := range res.ActionItems {
			if existing == item {
				found = true
				break
			}
		}
		if !found {
			res.ActionItems = append(res.ActionItems, item)
		}
	}

	// Fallback for Overview: use MarkdownContent only if it is NOT a JSON string
	if res.Overview == "" {
		if rawMd != "" && !strings.HasPrefix(rawMd, "{") {
			res.Overview = rawMd
		} else if len(data) > 0 {
			var fallbackItems []string
			for k, v := range data {
				if k == "$schema" || k == "invalid_key" {
					continue
				}
				if str, ok := v.(string); ok && strings.TrimSpace(str) != "" {
					fallbackItems = append(fallbackItems, fmt.Sprintf("%s: %s", strings.ReplaceAll(k, "_", " "), str))
				}
			}
			if len(fallbackItems) > 0 {
				res.Overview = strings.Join(fallbackItems, "\n\n")
			}
		}
	}

	return res
}

func extractActionItems(summary *models.Summary) []string {
	if summary == nil || summary.StructuredData == nil {
		return nil
	}
	var rawItems interface{}
	if ai, ok := summary.StructuredData["action_items"]; ok {
		rawItems = ai
	} else if kai, ok := summary.StructuredData["key_action_items"]; ok {
		rawItems = kai
	}
	if rawItems == nil {
		return nil
	}

	var results []string
	switch items := rawItems.(type) {
	case []interface{}:
		for _, item := range items {
			switch v := item.(type) {
			case string:
				if s := strings.TrimSpace(v); s != "" {
					results = append(results, s)
				}
			case map[string]interface{}:
				if task, ok := v["task"].(string); ok && strings.TrimSpace(task) != "" {
					pic, _ := v["pic"].(string)
					assignee, _ := v["assignee"].(string)
					owner := pic
					if owner == "" {
						owner = assignee
					}
					if owner != "" {
						results = append(results, fmt.Sprintf("%s (PIC: %s)", task, owner))
					} else {
						results = append(results, task)
					}
				} else if text, ok := v["text"].(string); ok && strings.TrimSpace(text) != "" {
					results = append(results, text)
				}
			}
		}
	}
	return results
}

func extractExecutiveSummary(summary *models.Summary) string {
	if summary == nil {
		return ""
	}
	formatted := formatSummaryContent(summary)
	if formatted.Overview != "" {
		return formatted.Overview
	}
	if len(formatted.Sections) > 0 {
		return strings.Join(formatted.Sections[0].Items, "\n")
	}
	return ""
}

func computeExportAnalytics(segments []models.TranscriptSegment) *dtos.JSONExportAnalytics {
	if len(segments) == 0 {
		return nil
	}

	type spkStat struct {
		speaker   string
		duration  float64
		wordCount int
		turnCount int
	}

	spkMap := make(map[string]*spkStat)
	var order []string
	var totalDuration float64
	var totalWords int
	var totalTurns int

	for _, seg := range segments {
		dur := seg.EndTime - seg.StartTime
		if dur < 0 {
			dur = 0
		}
		words := len(strings.Fields(seg.Text))
		spk := seg.SpeakerName
		if spk == "" {
			spk = seg.SpeakerLabel
		}
		if spk == "" {
			spk = "Speaker"
		}

		totalDuration += dur
		totalWords += words
		totalTurns++

		stat, ok := spkMap[spk]
		if !ok {
			stat = &spkStat{speaker: spk}
			spkMap[spk] = stat
			order = append(order, spk)
		}
		stat.duration += dur
		stat.wordCount += words
		stat.turnCount++
	}

	sort.SliceStable(order, func(i, j int) bool {
		return spkMap[order[i]].duration > spkMap[order[j]].duration
	})

	var speakerStats []dtos.JSONExportSpeakerStat
	for _, name := range order {
		st := spkMap[name]
		var ratio float64
		if totalDuration > 0 {
			ratio = (st.duration / totalDuration) * 100.0
		}
		speakerStats = append(speakerStats, dtos.JSONExportSpeakerStat{
			Speaker:       st.speaker,
			Duration:      st.duration,
			TalkTimeRatio: math.Round(ratio*10) / 10,
			TurnCount:     st.turnCount,
			WordCount:     st.wordCount,
		})
	}

	return &dtos.JSONExportAnalytics{
		TotalSpeechDuration: totalDuration,
		TotalWords:          totalWords,
		TotalTurns:          totalTurns,
		SpeakerStats:        speakerStats,
	}
}

// buildMarkdownExport renders the document strictly in the requested order:
// 1. Ringkasan (Summary & Action Items)
// 2. Transkrip (Diarized Transcript)
// 3. Sorotan & Bab (Highlights & Chapter Breakdown)
// 4. Analitik (Conversation & Speaker Analytics)
func buildMarkdownExport(rec *models.Recording, summary *models.Summary, chapters []models.Chapter, highlights []models.Highlight, segments []models.TranscriptSegment) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# %s\n\n", rec.Title))
	sb.WriteString(fmt.Sprintf("- **Tanggal:** %s\n", rec.CreatedAt.Format("2006-01-02 15:04:05 MST")))
	sb.WriteString(fmt.Sprintf("- **Durasi:** %s\n", timeutil.FormatTimestamp(rec.DurationSeconds)))
	sb.WriteString(fmt.Sprintf("- **Status:** %s\n\n", rec.Status))

	// 1. RINGKASAN
	sb.WriteString("## 1. Ringkasan Pertemuan (Summary)\n\n")
	formatted := formatSummaryContent(summary)
	if formatted.Overview != "" {
		sb.WriteString(formatted.Overview + "\n\n")
	}
	for _, sec := range formatted.Sections {
		sb.WriteString(fmt.Sprintf("### %s\n\n", sec.Heading))
		for _, item := range sec.Items {
			sb.WriteString(item + "\n")
		}
		sb.WriteString("\n")
	}
	if len(formatted.ActionItems) > 0 {
		sb.WriteString("### Tindak Lanjut & Action Items\n\n")
		for _, item := range formatted.ActionItems {
			sb.WriteString(fmt.Sprintf("- [ ] %s\n", item))
		}
		sb.WriteString("\n")
	}

	// 2. TRANSKRIP
	if len(segments) > 0 {
		sb.WriteString("## 2. Transkrip Percakapan (Diarized Transcript)\n\n")
		for _, seg := range segments {
			speaker := seg.SpeakerName
			if speaker == "" {
				speaker = seg.SpeakerLabel
			}
			if speaker == "" {
				speaker = "Speaker"
			}
			timeRange := fmt.Sprintf("%s - %s", timeutil.FormatTimestamp(seg.StartTime), timeutil.FormatTimestamp(seg.EndTime))
			sb.WriteString(fmt.Sprintf("**%s (%s):** %s\n\n", speaker, timeRange, seg.Text))
		}
	}

	// 3. SOROTAN & BAB
	if len(highlights) > 0 || len(chapters) > 0 {
		sb.WriteString("## 3. Sorotan & Pembahasan Bab (Highlights & Chapters)\n\n")
		if len(highlights) > 0 {
			sb.WriteString("### Sorotan Utama (Key Highlights)\n\n")
			for _, h := range highlights {
				title := "Sorotan"
				if h.Title != nil && *h.Title != "" {
					title = *h.Title
				}
				note := ""
				if h.Note != nil && *h.Note != "" {
					note = fmt.Sprintf(": %s", *h.Note)
				}
				sb.WriteString(fmt.Sprintf("- **[%s] %s**%s\n", timeutil.FormatTimestamp(h.StartTime), title, note))
			}
			sb.WriteString("\n")
		}
		if len(chapters) > 0 {
			sb.WriteString("### Pembahasan Bab (Chapter Breakdown)\n\n")
			for i, ch := range chapters {
				sb.WriteString(fmt.Sprintf("#### %d. %s (%s - %s)\n\n", i+1, ch.Title, timeutil.FormatTimestamp(ch.StartTime), timeutil.FormatTimestamp(ch.EndTime)))
				if ch.Summary != "" {
					sb.WriteString(ch.Summary + "\n\n")
				}
			}
		}
	}

	// 4. ANALITIK
	analytics := computeExportAnalytics(segments)
	if analytics != nil {
		sb.WriteString("## 4. Analitik Percakapan (Conversation Analytics)\n\n")
		sb.WriteString(fmt.Sprintf("- **Total Durasi Bicara:** %s\n", timeutil.FormatTimestamp(analytics.TotalSpeechDuration)))
		sb.WriteString(fmt.Sprintf("- **Total Kata:** %d kata\n", analytics.TotalWords))
		sb.WriteString(fmt.Sprintf("- **Total Giliran Bicara:** %d giliran\n\n", analytics.TotalTurns))

		if len(analytics.SpeakerStats) > 0 {
			sb.WriteString("### Partisipasi Pembicara\n\n")
			sb.WriteString("| Pembicara | Durasi Bicara | Porsi Bicara | Giliran | Jumlah Kata |\n")
			sb.WriteString("| :--- | :--- | :--- | :--- | :--- |\n")
			for _, st := range analytics.SpeakerStats {
				sb.WriteString(fmt.Sprintf("| %s | %s | %.1f%% | %d | %d |\n",
					st.Speaker,
					timeutil.FormatTimestamp(st.Duration),
					st.TalkTimeRatio,
					st.TurnCount,
					st.WordCount,
				))
			}
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// buildTextExport renders plain text strictly in the requested order:
// 1. Ringkasan (Summary & Action Items)
// 2. Transkrip (Diarized Transcript)
// 3. Sorotan & Bab (Highlights & Chapters)
// 4. Analitik (Conversation & Speaker Analytics)
func buildTextExport(rec *models.Recording, summary *models.Summary, chapters []models.Chapter, highlights []models.Highlight, segments []models.TranscriptSegment) string {
	var sb strings.Builder

	border := strings.Repeat("=", 72)
	divider := strings.Repeat("-", 72)

	sb.WriteString(border + "\n")
	sb.WriteString("MINUTES OF MEETING: " + strings.ToUpper(rec.Title) + "\n")
	sb.WriteString(border + "\n\n")

	sb.WriteString(fmt.Sprintf("Tanggal:  %s\n", rec.CreatedAt.Format("2006-01-02 15:04:05 MST")))
	sb.WriteString(fmt.Sprintf("Durasi:   %s\n", timeutil.FormatTimestamp(rec.DurationSeconds)))
	sb.WriteString(fmt.Sprintf("Status:   %s\n\n", rec.Status))

	// 1. RINGKASAN
	sb.WriteString(divider + "\n")
	sb.WriteString("1. RINGKASAN PERTEMUAN (SUMMARY)\n")
	sb.WriteString(divider + "\n")
	formatted := formatSummaryContent(summary)
	if formatted.Overview != "" {
		sb.WriteString(formatted.Overview + "\n\n")
	}
	for _, sec := range formatted.Sections {
		sb.WriteString(strings.ToUpper(sec.Heading) + ":\n")
		for _, item := range sec.Items {
			sb.WriteString(item + "\n")
		}
		sb.WriteString("\n")
	}
	if len(formatted.ActionItems) > 0 {
		sb.WriteString("TINDAK LANJUT & ACTION ITEMS:\n")
		for _, item := range formatted.ActionItems {
			sb.WriteString(fmt.Sprintf("[ ] %s\n", item))
		}
		sb.WriteString("\n")
	}

	// 2. TRANSKRIP
	if len(segments) > 0 {
		sb.WriteString(divider + "\n")
		sb.WriteString("2. TRANSKRIP PERCAKAPAN (DIARIZED TRANSCRIPT)\n")
		sb.WriteString(divider + "\n")
		for _, seg := range segments {
			speaker := seg.SpeakerName
			if speaker == "" {
				speaker = seg.SpeakerLabel
			}
			if speaker == "" {
				speaker = "Speaker"
			}
			sb.WriteString(fmt.Sprintf("[%s - %s] %s: %s\n\n",
				timeutil.FormatTimestamp(seg.StartTime),
				timeutil.FormatTimestamp(seg.EndTime),
				speaker,
				seg.Text,
			))
		}
	}

	// 3. SOROTAN & BAB
	if len(highlights) > 0 || len(chapters) > 0 {
		sb.WriteString(divider + "\n")
		sb.WriteString("3. SOROTAN & PEMBAHASAN BAB (HIGHLIGHTS & CHAPTERS)\n")
		sb.WriteString(divider + "\n")
		if len(highlights) > 0 {
			sb.WriteString("SOROTAN UTAMA (KEY HIGHLIGHTS):\n")
			for _, h := range highlights {
				title := "Sorotan"
				if h.Title != nil && *h.Title != "" {
					title = *h.Title
				}
				note := ""
				if h.Note != nil && *h.Note != "" {
					note = " - " + *h.Note
				}
				sb.WriteString(fmt.Sprintf("* [%s] %s%s\n", timeutil.FormatTimestamp(h.StartTime), title, note))
			}
			sb.WriteString("\n")
		}
		if len(chapters) > 0 {
			sb.WriteString("PEMBAHASAN BAB (CHAPTER BREAKDOWN):\n")
			for i, ch := range chapters {
				sb.WriteString(fmt.Sprintf("%d. %s [%s - %s]\n", i+1, ch.Title, timeutil.FormatTimestamp(ch.StartTime), timeutil.FormatTimestamp(ch.EndTime)))
				if ch.Summary != "" {
					sb.WriteString(fmt.Sprintf("   %s\n", ch.Summary))
				}
			}
			sb.WriteString("\n")
		}
	}

	// 4. ANALITIK
	analytics := computeExportAnalytics(segments)
	if analytics != nil {
		sb.WriteString(divider + "\n")
		sb.WriteString("4. ANALITIK PERCAKAPAN (CONVERSATION ANALYTICS)\n")
		sb.WriteString(divider + "\n")
		sb.WriteString(fmt.Sprintf("Total Durasi Bicara: %s\n", timeutil.FormatTimestamp(analytics.TotalSpeechDuration)))
		sb.WriteString(fmt.Sprintf("Total Kata:          %d kata\n", analytics.TotalWords))
		sb.WriteString(fmt.Sprintf("Total Giliran:       %d giliran\n\n", analytics.TotalTurns))

		if len(analytics.SpeakerStats) > 0 {
			sb.WriteString("PARTISIPASI PEMBICARA:\n")
			for _, st := range analytics.SpeakerStats {
				sb.WriteString(fmt.Sprintf("- %s: %s (%.1f%%) | %d giliran | %d kata\n",
					st.Speaker,
					timeutil.FormatTimestamp(st.Duration),
					st.TalkTimeRatio,
					st.TurnCount,
					st.WordCount,
				))
			}
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

func buildJSONExport(rec *models.Recording, summary *models.Summary, chapters []models.Chapter, highlights []models.Highlight, segments []models.TranscriptSegment) ([]byte, error) {
	formatted := formatSummaryContent(summary)
	payload := dtos.JSONExportPayload{
		RecordingID:      rec.ID.String(),
		Title:            rec.Title,
		DurationSeconds:  rec.DurationSeconds,
		Status:           rec.Status,
		CreatedAt:        rec.CreatedAt,
		ExecutiveSummary: formatted.Overview,
		ActionItems:      formatted.ActionItems,
		Analytics:        computeExportAnalytics(segments),
	}

	for _, ch := range chapters {
		payload.Chapters = append(payload.Chapters, dtos.JSONExportChapter{
			Title:     ch.Title,
			StartTime: ch.StartTime,
			EndTime:   ch.EndTime,
			Summary:   ch.Summary,
		})
	}

	for _, h := range highlights {
		title := ""
		if h.Title != nil {
			title = *h.Title
		}
		note := ""
		if h.Note != nil {
			note = *h.Note
		}
		payload.Highlights = append(payload.Highlights, dtos.JSONExportHighlight{
			Title:     title,
			StartTime: h.StartTime,
			EndTime:   h.EndTime,
			Note:      note,
		})
	}

	for _, s := range segments {
		speaker := s.SpeakerName
		if speaker == "" {
			speaker = s.SpeakerLabel
		}
		if speaker == "" {
			speaker = "Speaker"
		}
		payload.Transcript = append(payload.Transcript, dtos.JSONExportTranscriptSeg{
			Speaker:   speaker,
			StartTime: s.StartTime,
			EndTime:   s.EndTime,
			Text:      s.Text,
		})
	}

	return json.MarshalIndent(payload, "", "  ")
}

// WrapTextLine cleanly wraps lines at whitespace boundaries without mid-word splits.
func WrapTextLine(line string, maxLen int) []string {
	line = strings.TrimRight(line, " \r\t")
	if len(line) <= maxLen {
		return []string{line}
	}

	words := strings.Fields(line)
	if len(words) == 0 {
		return []string{""}
	}

	var lines []string
	var current strings.Builder

	for _, w := range words {
		for len(w) > maxLen {
			if current.Len() > 0 {
				lines = append(lines, current.String())
				current.Reset()
			}
			lines = append(lines, w[:maxLen])
			w = w[maxLen:]
		}

		if w == "" {
			continue
		}

		if current.Len() == 0 {
			current.WriteString(w)
		} else if current.Len()+1+len(w) <= maxLen {
			current.WriteString(" ")
			current.WriteString(w)
		} else {
			lines = append(lines, current.String())
			current.Reset()
			current.WriteString(w)
		}
	}

	if current.Len() > 0 {
		lines = append(lines, current.String())
	}

	return lines
}

// generateStandardPDF constructs a valid, dependency-free PDF 1.4 byte document containing
// the exported MOM text with neat word wrapping and margins.
func generateStandardPDF(title string, textContent string) []byte {
	// Clean text and break into lines safely
	sanitized := regexp.MustCompile(`[^\x20-\x7E\n]`).ReplaceAllString(textContent, " ")
	rawLines := strings.Split(sanitized, "\n")

	var wrappedLines []string
	for _, l := range rawLines {
		wrapped := WrapTextLine(l, 80)
		wrappedLines = append(wrappedLines, wrapped...)
	}

	// Maximum 48 lines per page for comfortable letter margins
	const linesPerPage = 48
	var pages [][]string
	for i := 0; i < len(wrappedLines); i += linesPerPage {
		end := i + linesPerPage
		if end > len(wrappedLines) {
			end = len(wrappedLines)
		}
		pages = append(pages, wrappedLines[i:end])
	}
	if len(pages) == 0 {
		pages = append(pages, []string{"Minutes of Meeting", title})
	}

	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n%\xE2\xE3\xCF\xD3\n")

	var offsets []int

	// Helper to track object offsets
	writeObj := func(num int, content string) {
		offsets = append(offsets, buf.Len())
		buf.WriteString(fmt.Sprintf("%d 0 obj\n%s\nendobj\n", num, content))
	}

	// 1: Catalog
	writeObj(1, "<< /Type /Catalog /Pages 2 0 R >>")

	// Pre-calculate page and content object numbers
	numPages := len(pages)
	var pageObjIDs []string
	for i := 0; i < numPages; i++ {
		pageObjIDs = append(pageObjIDs, fmt.Sprintf("%d 0 R", 3+i*2))
	}

	// 2: Pages root
	writeObj(2, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(pageObjIDs, " "), numPages))

	// Font object ID is after all pages and contents
	fontObjID := 3 + numPages*2

	// Render each page and its content stream
	for i, pageLines := range pages {
		pageID := 3 + i*2
		contentID := pageID + 1

		// Page object
		writeObj(pageID, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 %d 0 R >> >> /Contents %d 0 R >>", fontObjID, contentID))

		// Stream content
		var stream bytes.Buffer
		stream.WriteString("BT\n/F1 10 Tf\n14 TL\n50 750 Td\n")
		for _, line := range pageLines {
			escaped := strings.ReplaceAll(line, "\\", "\\\\")
			escaped = strings.ReplaceAll(escaped, "(", "\\(")
			escaped = strings.ReplaceAll(escaped, ")", "\\)")
			stream.WriteString(fmt.Sprintf("(%s) '\n", escaped))
		}
		stream.WriteString("ET\n")

		streamBytes := stream.Bytes()
		contentObj := fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(streamBytes), streamBytes)
		writeObj(contentID, contentObj)
	}

	// Font object
	writeObj(fontObjID, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")

	// Cross-reference table (xref)
	xrefOffset := buf.Len()
	totalObjs := fontObjID + 1

	buf.WriteString(fmt.Sprintf("xref\n0 %d\n", totalObjs))
	buf.WriteString("0000000000 65535 f \n")
	for _, offset := range offsets {
		buf.WriteString(fmt.Sprintf("%010d 00000 n \n", offset))
	}

	// Trailer
	buf.WriteString(fmt.Sprintf("trailer\n<< /Size %d /Root 1 0 R >>\n", totalObjs))
	buf.WriteString(fmt.Sprintf("startxref\n%d\n%%%%EOF\n", xrefOffset))

	return buf.Bytes()
}

