package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
	"code-base-golang/internal/pkg/ctxmeta"
	"code-base-golang/internal/pkg/docx"
	"code-base-golang/internal/pkg/jsonutil"
	"code-base-golang/internal/pkg/strutil"
	"code-base-golang/internal/pkg/timeutil"
)

// ExportRecordingMOM generates a multi-format export of a meeting recording's executive summary,
// action items, chapter breakdown, and diarized transcript for a specific or active summary version.
func (s *Service) ExportRecordingMOM(ctx context.Context, id uuid.UUID, ownershipToken string, format string, version string) (*dtos.ExportResult, error) {
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
	} else if ownershipToken != "" && strutil.ConstantTimeEqual(ownershipToken, rec.OwnershipToken) {
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
	case "docx", "doc":
		expFormat = constants.ExportFormatDocx
	default:
		return nil, constants.ErrBadRequest
	}

	// Fetch requested summary version (or fallback to active summary)
	var summary *models.Summary
	if strings.TrimSpace(version) != "" {
		vStr := strings.TrimSpace(version)
		if targetUUID, err := uuid.Parse(vStr); err == nil {
			sum, err := s.repo.FindSummaryByRecordingAndID(ctx, id, targetUUID)
			if err == nil {
				summary = sum
			}
		} else if targetVer, err := strconv.Atoi(vStr); err == nil {
			sum, err := s.repo.FindSummaryByRecordingAndVersion(ctx, id, targetVer)
			if err == nil {
				summary = sum
			}
		}
	}

	if summary == nil {
		activeSummary, err := s.repo.FindActiveSummaryByRecordingID(ctx, id)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("failed to fetch active summary: %w", err)
		}
		summary = activeSummary
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
	if summary != nil && summary.Version > 0 {
		baseName = fmt.Sprintf("%s_v%d", baseName, summary.Version)
	}

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

	case constants.ExportFormatDocx:
		docxBytes, dErr := buildDocxExport(rec, summary, chapters, highlights, segments)
		if dErr != nil {
			return nil, fmt.Errorf("failed to generate docx export: %w", dErr)
		}
		return &dtos.ExportResult{
			Filename:    baseName + "_mom.docx",
			ContentType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
			Data:        docxBytes,
		}, nil
	}

	return nil, constants.ErrBadRequest
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
		if parsed, _, err := jsonutil.UnmarshalMapLenient(rawMd); err == nil {
			for k, v := range parsed {
				data[k] = v
			}
		}
	}

	return data, rawMd
}

func appendSection(res *dtos.FormattedSummary, heading string, items []string) {
	if len(items) == 0 {
		return
	}
	for i, sec := range res.Sections {
		if sec.Heading == heading {
			res.Sections[i].Items = append(res.Sections[i].Items, items...)
			return
		}
	}
	res.Sections = append(res.Sections, dtos.FormattedSection{
		Heading: heading,
		Items:   items,
	})
}

func appendOverviewParagraph(res *dtos.FormattedSummary, p string) {
	trimmed := strings.TrimSpace(p)
	if trimmed == "" {
		return
	}
	if res.Overview == "" {
		res.Overview = trimmed
		return
	}
	for _, existing := range strings.Split(res.Overview, "\n\n") {
		if strings.TrimSpace(existing) == trimmed {
			return
		}
	}
	res.Overview = res.Overview + "\n\n" + trimmed
}

func extractOverviewFromData(res *dtos.FormattedSummary, data map[string]interface{}) {
	if mg, ok := data["meeting_goal"].(string); ok && strings.TrimSpace(mg) != "" {
		appendOverviewParagraph(res, fmt.Sprintf("Tujuan Pertemuan: %s", strings.TrimSpace(mg)))
	}

	var musicMeta []string
	if artist, ok := data["artist_or_performers"].(string); ok && strings.TrimSpace(artist) != "" {
		musicMeta = append(musicMeta, fmt.Sprintf("Artis/Penyanyi: %s", strings.TrimSpace(artist)))
	}
	if style, ok := data["musical_style"].(string); ok && strings.TrimSpace(style) != "" {
		musicMeta = append(musicMeta, fmt.Sprintf("Aliran Musik: %s", strings.TrimSpace(style)))
	}
	if len(musicMeta) > 0 {
		appendOverviewParagraph(res, strings.Join(musicMeta, " | "))
	}

	for _, key := range []string{"executive_summary", "executive_overview", "course_summary", "show_notes", "core_message", "context"} {
		if val, ok := data[key].(string); ok && strings.TrimSpace(val) != "" {
			appendOverviewParagraph(res, val)
		}
	}

	if sh, ok := data["sprint_health"].(map[string]interface{}); ok {
		status, _ := sh["status"].(string)
		sum, _ := sh["summary"].(string)
		if status != "" && sum != "" {
			appendOverviewParagraph(res, fmt.Sprintf("Status Sprint: %s\n%s", status, sum))
		} else if sum != "" {
			appendOverviewParagraph(res, sum)
		}
	}

	if cand, ok := data["candidate_name"].(string); ok && cand != "" {
		role, _ := data["target_role"].(string)
		rec, _ := data["recommendation"].(string)
		just, _ := data["justification"].(string)
		appendOverviewParagraph(res, fmt.Sprintf("Kandidat: %s | Posisi: %s\nRekomendasi: %s\n\n%s", cand, role, rec, just))
	}

	if wb, ok := data["wellbeing_assessment"].(map[string]interface{}); ok {
		score, _ := wb["sentiment_score"].(string)
		sum, _ := wb["summary"].(string)
		if score != "" && sum != "" {
			appendOverviewParagraph(res, fmt.Sprintf("Sentimen Kesejahteraan: %s\n%s", score, sum))
		} else if sum != "" {
			appendOverviewParagraph(res, sum)
		}
	}

	if pc, ok := data["prospect_company"].(string); ok && pc != "" {
		st, _ := data["deal_stage_suggested"].(string)
		appendOverviewParagraph(res, fmt.Sprintf("Perusahaan Prospek: %s (Tahap: %s)", pc, st))
	}
}

func appendDiscussionTopics(res *dtos.FormattedSummary, data map[string]interface{}) {
	rawTopics, ok := data["discussion_topics"].([]interface{})
	heading := "Sub Pembahasan & Poin Kunci"
	if !ok || len(rawTopics) == 0 {
		rawTopics, ok = data["core_themes"].([]interface{})
		if ok && len(rawTopics) > 0 {
			heading = "Sub Pembahasan (Core Themes)"
		}
	}
	if !ok || len(rawTopics) == 0 {
		rawTopics, ok = data["topic_notes"].([]interface{})
		if ok && len(rawTopics) > 0 {
			heading = "Catatan Topik Kuliah (Topic Notes)"
		}
	}
	if !ok || len(rawTopics) == 0 {
		return
	}

	var items []string
	for idx, t := range rawTopics {
		tm, ok := t.(map[string]interface{})
		if !ok {
			continue
		}
		top := extractFirstString(tm, "topic", "theme", "title", "name")
		if top == "" {
			top = fmt.Sprintf("Topik %d", idx+1)
		}

		rawPts, ok := tm["key_points"].([]interface{})
		if !ok {
			rawPts, _ = tm["summary_points"].([]interface{})
		}

		if len(rawPts) > 0 {
			var pts []string
			for _, p := range rawPts {
				if p != nil && strings.TrimSpace(fmt.Sprintf("%v", p)) != "" {
					pts = append(pts, fmt.Sprintf("  - %v", p))
				}
			}
			if len(pts) > 0 {
				items = append(items, fmt.Sprintf("**%s**\n%s", top, strings.Join(pts, "\n")))
			} else {
				items = append(items, fmt.Sprintf("**%s**", top))
			}
		} else {
			items = append(items, fmt.Sprintf("**%s**", top))
		}
	}

	appendSection(res, heading, items)
}

func appendLectureSections(res *dtos.FormattedSummary, data map[string]interface{}) {
	if lo, ok := data["learning_objectives"].([]interface{}); ok && len(lo) > 0 {
		var items []string
		for _, o := range lo {
			if str := strings.TrimSpace(fmt.Sprintf("%v", o)); str != "" {
				items = append(items, fmt.Sprintf("• %s", str))
			}
		}
		appendSection(res, "Tujuan Pembelajaran (Learning Objectives)", items)
	}

	if cc, ok := data["core_concepts"].([]interface{}); ok && len(cc) > 0 {
		var items []string
		for _, c := range cc {
			cm, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			cp := extractFirstString(cm, "concept", "term", "name")
			exp := extractFirstString(cm, "explanation", "description")
			ex := extractFirstString(cm, "examples", "example")
			if cp == "" && exp == "" {
				continue
			}
			line := fmt.Sprintf("• %s: %s", cp, exp)
			if ex != "" {
				line = fmt.Sprintf("%s (Contoh: %s)", line, ex)
			}
			items = append(items, line)
		}
		appendSection(res, "Konsep & Teori Utama", items)
	}

	if tg, ok := data["technical_glossary"].([]interface{}); ok && len(tg) > 0 {
		var items []string
		for _, g := range tg {
			gm, ok := g.(map[string]interface{})
			if !ok {
				continue
			}
			term := extractFirstString(gm, "term", "word")
			def := extractFirstString(gm, "definition", "meaning")
			if term != "" || def != "" {
				items = append(items, fmt.Sprintf("• %s: %s", term, def))
			}
		}
		appendSection(res, "Glosarium Istilah Teknis", items)
	}

	if erq, ok := data["exam_review_questions"].([]interface{}); ok && len(erq) > 0 {
		var items []string
		for _, q := range erq {
			qm, ok := q.(map[string]interface{})
			if !ok {
				if str := strings.TrimSpace(fmt.Sprintf("%v", q)); str != "" {
					items = append(items, fmt.Sprintf("• %s", str))
				}
				continue
			}
			ques := extractFirstString(qm, "question")
			hint := extractFirstString(qm, "hint_or_key_answer", "answer", "hint")
			if ques == "" {
				continue
			}
			if hint != "" {
				items = append(items, fmt.Sprintf("• %s\n  - Petunjuk / Kunci Jawaban: %s", ques, hint))
			} else {
				items = append(items, fmt.Sprintf("• %s", ques))
			}
		}
		appendSection(res, "Pertanyaan Ujian & Diskusi", items)
	}
}

func appendTechReviewSections(res *dtos.FormattedSummary, data map[string]interface{}) {
	if da, ok := data["decisions_adopted"].([]interface{}); ok && len(da) > 0 {
		var items []string
		for _, d := range da {
			dm, ok := d.(map[string]interface{})
			if !ok {
				continue
			}
			dec := extractFirstString(dm, "decision", "text")
			just := extractFirstString(dm, "technical_justification", "justification")
			if dec != "" && just != "" {
				items = append(items, fmt.Sprintf("• %s: %s", dec, just))
			} else if dec != "" {
				items = append(items, fmt.Sprintf("• %s", dec))
			}
		}
		appendSection(res, "Keputusan Diadopsi", items)
	}

	if ra, ok := data["rejected_alternatives"].([]interface{}); ok && len(ra) > 0 {
		var items []string
		for _, a := range ra {
			am, ok := a.(map[string]interface{})
			if !ok {
				continue
			}
			alt := extractFirstString(am, "alternative", "option")
			reason := extractFirstString(am, "rejection_reason", "reason")
			if alt != "" && reason != "" {
				items = append(items, fmt.Sprintf("• %s (Alasan Penolakan: %s)", alt, reason))
			} else if alt != "" {
				items = append(items, fmt.Sprintf("• %s", alt))
			}
		}
		appendSection(res, "Alternatif yang Ditolak (Rejected Alternatives)", items)
	}

	if nfr, ok := data["nfr_assessment"].(map[string]interface{}); ok && len(nfr) > 0 {
		var items []string
		if sec := extractFirstString(nfr, "security"); sec != "" {
			items = append(items, fmt.Sprintf("• Keamanan (Security): %s", sec))
		}
		if perf := extractFirstString(nfr, "performance_scalability", "performance"); perf != "" {
			items = append(items, fmt.Sprintf("• Performa & Skalabilitas: %s", perf))
		}
		if rel := extractFirstString(nfr, "reliability_resilience", "reliability"); rel != "" {
			items = append(items, fmt.Sprintf("• Keandalan & Ketahanan: %s", rel))
		}
		appendSection(res, "Penilaian Non-Functional Requirements (NFR)", items)
	}

	if td := extractFirstString(data, "technical_debt_impact"); td != "" {
		appendSection(res, "Dampak Hutang Teknis (Technical Debt Impact)", []string{fmt.Sprintf("• %s", td)})
	}
}

func appendMOMDynamicsAndPerspectives(res *dtos.FormattedSummary, data map[string]interface{}) {
	if dyn, ok := data["meeting_dynamics"].(map[string]interface{}); ok && len(dyn) > 0 {
		var items []string
		if score := extractFirstString(dyn, "consensus_score"); score != "" {
			items = append(items, fmt.Sprintf("• Skor Konsensus: %s", score))
		}
		if traj := extractFirstString(dyn, "sentiment_trajectory"); traj != "" {
			items = append(items, fmt.Sprintf("• Trajektori Sentimen: %s", traj))
		}
		if stt, ok := dyn["speaker_talk_time"].([]interface{}); ok && len(stt) > 0 {
			items = append(items, "• Distribusi Waktu Bicara:")
			for _, s := range stt {
				sm, ok := s.(map[string]interface{})
				if !ok {
					continue
				}
				spk := extractFirstString(sm, "speaker")
				var pct float64
				if pNum, ok := sm["percentage"].(float64); ok {
					pct = pNum
				}
				var dur float64
				if dNum, ok := sm["duration_minutes"].(float64); ok {
					dur = dNum
				}
				if spk != "" {
					items = append(items, fmt.Sprintf("  - %s: %.1f%% (%.1f menit)", spk, pct, dur))
				}
			}
		}
		appendSection(res, "Dinamika Pertemuan (Meeting Dynamics)", items)
	}

	if sp, ok := data["stakeholder_perspectives"].(map[string]interface{}); ok && len(sp) > 0 {
		var items []string
		if eb := extractFirstString(sp, "executive_brief"); eb != "" {
			items = append(items, fmt.Sprintf("• Ringkasan Eksekutif: %s", eb))
		}
		if ef := extractFirstString(sp, "engineering_focus"); ef != "" {
			items = append(items, fmt.Sprintf("• Fokus Rekayasa (Engineering): %s", ef))
		}
		if pd := extractFirstString(sp, "product_delivery"); pd != "" {
			items = append(items, fmt.Sprintf("• Pengiriman Produk (Product Delivery): %s", pd))
		}
		appendSection(res, "Perspektif Pemangku Kepentingan (Stakeholder Perspectives)", items)
	}

	if nm := extractFirstString(data, "next_meeting"); nm != "" {
		appendSection(res, "Agenda Pertemuan Berikutnya (Next Meeting)", []string{fmt.Sprintf("• %s", nm)})
	}
}

func appendKeyDecisions(res *dtos.FormattedSummary, data map[string]interface{}) {
	kd, ok := data["key_decisions"].([]interface{})
	if !ok || len(kd) == 0 {
		return
	}

	var items []string
	for _, d := range kd {
		dm, ok := d.(map[string]interface{})
		if !ok {
			if str := strings.TrimSpace(fmt.Sprintf("%v", d)); str != "" {
				items = append(items, fmt.Sprintf("• %s", str))
			}
			continue
		}
		dec := extractFirstString(dm, "decision", "text")
		if dec == "" {
			continue
		}

		app := extractFirstString(dm, "approved_by")
		trigger := extractFirstString(dm, "root_cause_trigger")
		contested := extractFirstString(dm, "contested_points")
		impact := extractFirstString(dm, "quantitative_impact")

		itemStr := fmt.Sprintf("• %s", dec)
		var subDetails []string
		if app != "" {
			subDetails = append(subDetails, fmt.Sprintf("  - Disetujui: %s", app))
		}
		if trigger != "" {
			subDetails = append(subDetails, fmt.Sprintf("  - Pemicu Masalah: %s", trigger))
		}
		if contested != "" {
			subDetails = append(subDetails, fmt.Sprintf("  - Poin Perdebatan: %s", contested))
		}
		if impact != "" {
			subDetails = append(subDetails, fmt.Sprintf("  - Dampak Kuantitatif: %s", impact))
		}

		if len(subDetails) > 0 {
			itemStr = fmt.Sprintf("%s\n%s", itemStr, strings.Join(subDetails, "\n"))
		}
		items = append(items, itemStr)
	}

	appendSection(res, "Keputusan Kunci (Key Decisions)", items)
}

func appendPodcastSections(res *dtos.FormattedSummary, data map[string]interface{}) {
	if goMap, ok := data["guest_overview"].(map[string]interface{}); ok && len(goMap) > 0 {
		var items []string
		name := extractFirstString(goMap, "guest_name", "name")
		title := extractFirstString(goMap, "guest_title", "title", "role")
		bio := extractFirstString(goMap, "bio_or_background", "bio", "background")
		if name != "" {
			items = append(items, fmt.Sprintf("• Nama: %s", name))
		}
		if title != "" {
			items = append(items, fmt.Sprintf("• Peran / Jabatan: %s", title))
		}
		if bio != "" {
			items = append(items, fmt.Sprintf("• Profil / Latar Belakang: %s", bio))
		}
		appendSection(res, "Profil Narasumber (Guest Overview)", items)
	}

	if tc, ok := data["topic_chapters"].([]interface{}); ok && len(tc) > 0 {
		var items []string
		for _, c := range tc {
			cm, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			ts := extractFirstString(cm, "timestamp", "time")
			top := extractFirstString(cm, "topic", "title")
			sm := extractFirstString(cm, "summary")
			if ts != "" && top != "" {
				items = append(items, fmt.Sprintf("[%s] %s: %s", ts, top, sm))
			} else if top != "" {
				items = append(items, fmt.Sprintf("• %s: %s", top, sm))
			}
		}
		appendSection(res, "Topik Pembahasan (Chapters)", items)
	}

	var rawQuotes []interface{}
	if gq, ok := data["golden_quotes"].([]interface{}); ok && len(gq) > 0 {
		rawQuotes = gq
	} else if nq, ok := data["notable_quotes"].([]interface{}); ok && len(nq) > 0 {
		rawQuotes = nq
	}
	if len(rawQuotes) > 0 {
		var items []string
		for _, q := range rawQuotes {
			qm, ok := q.(map[string]interface{})
			if !ok {
				continue
			}
			quote := extractFirstString(qm, "quote", "text")
			spk := extractFirstString(qm, "speaker", "author")
			ctxStr := extractFirstString(qm, "context")
			if quote == "" {
				continue
			}
			line := fmt.Sprintf("\"%s\"", quote)
			if spk != "" {
				line = fmt.Sprintf("%s — %s", line, spk)
			}
			if ctxStr != "" {
				line = fmt.Sprintf("%s (Konteks: %s)", line, ctxStr)
			}
			items = append(items, line)
		}
		appendSection(res, "Kutipan Berkesan (Quotes)", items)
	}
}

func appendMusicSections(res *dtos.FormattedSummary, data map[string]interface{}) {
	if sl, ok := data["structured_lyrics"].([]interface{}); ok && len(sl) > 0 {
		var items []string
		for _, l := range sl {
			lm, ok := l.(map[string]interface{})
			if !ok {
				continue
			}
			sec := extractFirstString(lm, "section", "part")
			lyr := extractFirstString(lm, "lyrics", "text")
			if sec != "" && lyr != "" {
				items = append(items, fmt.Sprintf("[%s]\n%s", sec, lyr))
			} else if lyr != "" {
				items = append(items, lyr)
			}
		}
		appendSection(res, "Struktur Lirik Lagu", items)
	}

	if et, ok := data["emotional_tone"].(map[string]interface{}); ok && len(et) > 0 {
		var items []string
		mood := extractFirstString(et, "primary_mood", "mood")
		energy := extractFirstString(et, "energy_level", "energy")
		sent := extractFirstString(et, "sentiment_analysis", "sentiment")
		if mood != "" || energy != "" {
			items = append(items, fmt.Sprintf("• Suasana Hati: %s, Tingkat Energi: %s", mood, energy))
		}
		if sent != "" {
			items = append(items, fmt.Sprintf("• Analisis Sentimen: %s", sent))
		}
		appendSection(res, "Analisis Suasana & Emosi", items)
	}
}

func appendGeneralSections(res *dtos.FormattedSummary, data map[string]interface{}) {
	if rr, ok := data["referenced_resources"].([]interface{}); ok && len(rr) > 0 {
		var items []string
		for _, r := range rr {
			if str := strings.TrimSpace(fmt.Sprintf("%v", r)); str != "" {
				items = append(items, fmt.Sprintf("• %s", str))
			}
		}
		appendSection(res, "Referensi & Sumber Daya (Referenced Resources)", items)
	}

	if kt, ok := data["key_takeaways"].([]interface{}); ok && len(kt) > 0 {
		var items []string
		for _, t := range kt {
			if str := strings.TrimSpace(fmt.Sprintf("%v", t)); str != "" {
				items = append(items, fmt.Sprintf("• %s", str))
			}
		}
		appendSection(res, "Poin Kunci (Key Takeaways)", items)
	}
}

func appendOpenPoints(res *dtos.FormattedSummary, data map[string]interface{}) {
	addList := func(key, heading string) {
		rawList, ok := data[key].([]interface{})
		if !ok || len(rawList) == 0 {
			return
		}
		var items []string
		for _, raw := range rawList {
			if raw == nil {
				continue
			}
			str := strings.TrimSpace(fmt.Sprintf("%v", raw))
			if str == "" {
				continue
			}
			items = append(items, fmt.Sprintf("• %s", str))
		}
		appendSection(res, heading, items)
	}

	addList("open_issues", "Isu Terbuka (Open Issues)")
	addList("open_points", "Isu Terbuka (Open Points)")
}

func appendLegacyTemplateSections(res *dtos.FormattedSummary, data map[string]interface{}) {
	if mu, ok := data["member_updates"].([]interface{}); ok && len(mu) > 0 {
		var items []string
		for _, item := range mu {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			name := extractFirstString(m, "member_name")
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
		appendSection(res, "Pembaruan Anggota (Member Updates)", items)
	}

	if cb, ok := data["critical_blockers"].([]interface{}); ok && len(cb) > 0 {
		var items []string
		for _, b := range cb {
			items = append(items, fmt.Sprintf("• %v", b))
			res.ActionItems = append(res.ActionItems, fmt.Sprintf("Kendala Kritis: %v", b))
		}
		appendSection(res, "Kendala Kritis (Critical Blockers)", items)
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
		appendSection(res, "Diskusi Lanjutan (Parking Lot)", items)
	}

	if str, ok := data["strengths"].([]interface{}); ok && len(str) > 0 {
		var items []string
		for _, s := range str {
			items = append(items, fmt.Sprintf("• %v", s))
		}
		appendSection(res, "Kekuatan Utama (Strengths)", items)
	}
	if con, ok := data["concerns"].([]interface{}); ok && len(con) > 0 {
		var items []string
		for _, c := range con {
			items = append(items, fmt.Sprintf("• %v", c))
		}
		appendSection(res, "Catatan & Kekhawatiran (Concerns)", items)
	}
	if cs, ok := data["competency_scores"].([]interface{}); ok && len(cs) > 0 {
		var items []string
		for _, c := range cs {
			if cm, ok := c.(map[string]interface{}); ok {
				comp := extractFirstString(cm, "competency")
				rating := extractFirstString(cm, "rating")
				evidence := extractFirstString(cm, "evidence")
				items = append(items, fmt.Sprintf("• %s [%s]: %s", comp, rating, evidence))
			}
		}
		appendSection(res, "Penilaian Kompetensi", items)
	}

	if kw, ok := data["key_wins"].([]interface{}); ok && len(kw) > 0 {
		var items []string
		for _, w := range kw {
			items = append(items, fmt.Sprintf("• %v", w))
		}
		appendSection(res, "Pencapaian Utama (Key Wins)", items)
	}
	if bl, ok := data["blockers"].([]interface{}); ok && len(bl) > 0 {
		var items []string
		for _, b := range bl {
			items = append(items, fmt.Sprintf("• %v", b))
			res.ActionItems = append(res.ActionItems, fmt.Sprintf("Kendala 1-on-1: %v", b))
		}
		appendSection(res, "Kendala (Blockers)", items)
	}
	if com, ok := data["commitments"].([]interface{}); ok && len(com) > 0 {
		for _, c := range com {
			if cm, ok := c.(map[string]interface{}); ok {
				act := extractFirstString(cm, "action_item", "task")
				party := extractFirstString(cm, "party", "owner")
				tl := extractFirstString(cm, "timeline", "target_date")
				res.ActionItems = append(res.ActionItems, fmt.Sprintf("[%s] %s (%s)", party, act, tl))
			}
		}
	}

	if pp, ok := data["pain_points"].([]interface{}); ok && len(pp) > 0 {
		var items []string
		for _, p := range pp {
			if pm, ok := p.(map[string]interface{}); ok {
				pain := extractFirstString(pm, "pain")
				cost := extractFirstString(pm, "cost_of_inaction")
				items = append(items, fmt.Sprintf("• %s (Biaya Inaksi: %s)", pain, cost))
			}
		}
		appendSection(res, "Titik Kendala (Pain Points)", items)
	}
}

func formatSummaryContent(summary *models.Summary) dtos.FormattedSummary {
	var res dtos.FormattedSummary
	if summary == nil {
		return res
	}

	data, rawMd := parseSummaryData(summary)

	extractOverviewFromData(&res, data)
	appendDiscussionTopics(&res, data)
	appendLectureSections(&res, data)
	appendPodcastSections(&res, data)
	appendMusicSections(&res, data)
	appendTechReviewSections(&res, data)
	appendMOMDynamicsAndPerspectives(&res, data)
	appendKeyDecisions(&res, data)
	appendGeneralSections(&res, data)
	appendOpenPoints(&res, data)
	appendLegacyTemplateSections(&res, data)

	// Action items from standard fields
	for _, item := range extractActionItems(summary) {
		if !sliceContains(res.ActionItems, item) {
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
	} else if ns, ok := summary.StructuredData["next_steps"]; ok {
		rawItems = ns
	}
	if rawItems == nil {
		return nil
	}

	var results []string
	items, ok := rawItems.([]interface{})
	if !ok {
		return nil
	}

	for _, item := range items {
		switch v := item.(type) {
		case string:
			if s := strings.TrimSpace(v); s != "" {
				results = append(results, s)
			}
		case map[string]interface{}:
			task := extractFirstString(v, "task", "action", "text", "action_item")
			if task == "" {
				continue
			}

			priority := strings.TrimSpace(extractFirstString(v, "priority"))
			owner := extractFirstString(v, "pic", "assignee", "owner", "party")
			target := extractFirstString(v, "due_date", "target_date", "target_sprint", "timeline")
			dod := extractFirstString(v, "definition_of_done")
			cost := extractFirstString(v, "cost_of_inaction")

			var meta []string
			if owner != "" {
				meta = append(meta, fmt.Sprintf("PIC: %s", owner))
			}
			if target != "" {
				meta = append(meta, fmt.Sprintf("Target: %s", target))
			}
			if dod != "" {
				meta = append(meta, fmt.Sprintf("DoD: %s", dod))
			}
			if cost != "" {
				meta = append(meta, fmt.Sprintf("Biaya Inaksi: %s", cost))
			}

			line := task
			if priority != "" && !strings.HasPrefix(strings.ToUpper(line), "["+strings.ToUpper(priority)+"]") {
				line = fmt.Sprintf("[%s] %s", priority, line)
			}
			if len(meta) > 0 {
				line = fmt.Sprintf("%s (%s)", line, strings.Join(meta, ", "))
			}
			results = append(results, line)
		}
	}
	return results
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
	sb.WriteString(fmt.Sprintf("- **Status:** %s\n", rec.Status))
	if summary != nil {
		if summary.CustomAngle != nil && strings.TrimSpace(*summary.CustomAngle) != "" {
			sb.WriteString(fmt.Sprintf("- **Sudut Pandang Khusus:** %s\n", strings.TrimSpace(*summary.CustomAngle)))
		}
		if summary.Version > 0 {
			templateLabel := summary.TemplateCategory
			if templateLabel == "" {
				templateLabel = "General"
			}
			sb.WriteString(fmt.Sprintf("- **Versi Ringkasan:** Versi %d (%s)\n", summary.Version, templateLabel))
		}
	}
	sb.WriteString("\n")

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
	sb.WriteString(fmt.Sprintf("Status:   %s\n", rec.Status))
	if summary != nil {
		if summary.CustomAngle != nil && strings.TrimSpace(*summary.CustomAngle) != "" {
			sb.WriteString(fmt.Sprintf("Sudut Pandang Khusus: %s\n", strings.TrimSpace(*summary.CustomAngle)))
		}
		if summary.Version > 0 {
			templateLabel := summary.TemplateCategory
			if templateLabel == "" {
				templateLabel = "General"
			}
			sb.WriteString(fmt.Sprintf("Versi:    Versi %d (%s)\n", summary.Version, templateLabel))
		}
	}
	sb.WriteString("\n")

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
	summaryVersion := 0
	templateCategory := ""
	customAngle := ""
	var structuredMap map[string]interface{}
	if summary != nil {
		summaryVersion = summary.Version
		templateCategory = summary.TemplateCategory
		if summary.CustomAngle != nil {
			customAngle = strings.TrimSpace(*summary.CustomAngle)
		}
		data, _ := parseSummaryData(summary)
		if len(data) > 0 {
			structuredMap = data
		}
	}

	payload := dtos.JSONExportPayload{
		RecordingID:      rec.ID.String(),
		Title:            rec.Title,
		DurationSeconds:  rec.DurationSeconds,
		Status:           rec.Status,
		CreatedAt:        rec.CreatedAt,
		ExecutiveSummary: formatted.Overview,
		CustomAngle:      customAngle,
		SummaryVersion:   summaryVersion,
		TemplateCategory: templateCategory,
		Sections:         formatted.Sections,
		ActionItems:      formatted.ActionItems,
		StructuredData:   structuredMap,
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

// buildDocxExport compiles a comprehensive, executive-ready Office Open XML (.docx) document
// containing meeting metadata, executive summary, action items, chapters, and diarized transcript.
func buildDocxExport(rec *models.Recording, summary *models.Summary, chapters []models.Chapter, highlights []models.Highlight, segments []models.TranscriptSegment) ([]byte, error) {
	doc := docx.New()

	// 1. Title
	doc.AddTitle(rec.Title)

	// 2. Metadata Table
	metaHeaders := []string{"Informasi Rapat", "Detail"}
	metaRows := [][]string{
		{"Tanggal Rapat", rec.CreatedAt.Format("02 January 2006 15:04 MST")},
		{"Durasi Rapat", timeutil.FormatTimestamp(rec.DurationSeconds)},
		{"Status", rec.Status},
	}
	if summary != nil {
		if summary.CustomAngle != nil && strings.TrimSpace(*summary.CustomAngle) != "" {
			metaRows = append(metaRows, []string{"Sudut Pandang Khusus", strings.TrimSpace(*summary.CustomAngle)})
		}
		if summary.Version > 0 {
			templateLabel := summary.TemplateCategory
			if templateLabel == "" {
				templateLabel = "General"
			}
			metaRows = append(metaRows, []string{"Versi Ringkasan", fmt.Sprintf("Versi %d (%s)", summary.Version, templateLabel)})
		}
	}
	doc.AddTable(metaHeaders, metaRows)

	// 3. Ringkasan Eksekutif
	if summary != nil {
		doc.AddHeading1("1. Ringkasan Pertemuan (Summary)")
		formatted := formatSummaryContent(summary)

		if strings.TrimSpace(formatted.Overview) != "" {
			doc.AddCallout("Ikhtisar Pertemuan", formatted.Overview)
		}

		for _, sec := range formatted.Sections {
			if strings.TrimSpace(sec.Heading) != "" {
				doc.AddHeading2(sec.Heading)
			}
			for _, item := range sec.Items {
				doc.AddBulletItem(item)
			}
		}

		// 4. Rencana Tindak Lanjut (Action Items)
		actionItems := formatted.ActionItems
		if len(actionItems) == 0 {
			actionItems = extractActionItems(summary)
		}
		if len(actionItems) > 0 {
			doc.AddHeading2("Tindak Lanjut & Action Items")
			actionHeaders := []string{"No", "Tugas / Rencana Tindak Lanjut", "Status"}
			actionRows := make([][]string, len(actionItems))
			for i, ai := range actionItems {
				actionRows[i] = []string{fmt.Sprintf("%d", i+1), ai, "[ ] Pending"}
			}
			doc.AddTable(actionHeaders, actionRows)
		}
	}

	// 5. Pembagian Topik / Chapters
	if len(chapters) > 0 {
		doc.AddHeading1("2. Pembagian Topik & Bab (Chapter Breakdown)")
		chapterHeaders := []string{"Waktu", "Topik", "Ringkasan Pembahasan"}
		chapterRows := make([][]string, len(chapters))
		for i, ch := range chapters {
			timeRange := fmt.Sprintf("%s - %s", timeutil.FormatTimestamp(ch.StartTime), timeutil.FormatTimestamp(ch.EndTime))
			chapterRows[i] = []string{timeRange, ch.Title, ch.Summary}
		}
		doc.AddTable(chapterHeaders, chapterRows)
	}

	// 6. Sorotan Utama (Highlights)
	if len(highlights) > 0 {
		doc.AddHeading1("3. Sorotan Utama (Key Highlights)")
		for _, hl := range highlights {
			title := "Sorotan"
			if hl.Title != nil && *hl.Title != "" {
				title = *hl.Title
			}
			note := ""
			if hl.Note != nil && *hl.Note != "" {
				note = fmt.Sprintf(": %s", *hl.Note)
			}
			text := fmt.Sprintf("[%s] %s%s", timeutil.FormatTimestamp(hl.StartTime), title, note)
			doc.AddBulletItem(text)
		}
	}

	// 7. Transkrip Percakapan Lengkap
	if len(segments) > 0 {
		doc.AddHeading1("4. Transkrip Percakapan Lengkap (Diarized Transcript)")
		for _, seg := range segments {
			speaker := seg.SpeakerName
			if speaker == "" {
				speaker = seg.SpeakerLabel
			}
			if speaker == "" {
				speaker = "Speaker"
			}
			timeStr := fmt.Sprintf("%s - %s", timeutil.FormatTimestamp(seg.StartTime), timeutil.FormatTimestamp(seg.EndTime))
			doc.AddTranscriptTurn(speaker, timeStr, seg.Text)
		}
	}

	return doc.Bytes()
}


