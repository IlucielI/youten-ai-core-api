package services

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
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
					assignee, _ := v["assignee"].(string)
					if assignee != "" {
						results = append(results, fmt.Sprintf("%s (Assignee: %s)", task, assignee))
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
	if summary.StructuredData != nil {
		if es, ok := summary.StructuredData["executive_summary"].(string); ok && strings.TrimSpace(es) != "" {
			return strings.TrimSpace(es)
		}
	}
	if strings.TrimSpace(summary.MarkdownContent) != "" {
		return strings.TrimSpace(summary.MarkdownContent)
	}
	return ""
}

func buildMarkdownExport(rec *models.Recording, summary *models.Summary, chapters []models.Chapter, highlights []models.Highlight, segments []models.TranscriptSegment) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# %s\n\n", rec.Title))
	sb.WriteString(fmt.Sprintf("- **Date:** %s\n", rec.CreatedAt.Format("2006-01-02 15:04:05 MST")))
	sb.WriteString(fmt.Sprintf("- **Duration:** %s\n", timeutil.FormatTimestamp(rec.DurationSeconds)))
	sb.WriteString(fmt.Sprintf("- **Status:** %s\n\n", rec.Status))

	execSummary := extractExecutiveSummary(summary)
	if execSummary != "" {
		sb.WriteString("## Executive Summary\n\n")
		sb.WriteString(execSummary + "\n\n")
	}

	actionItems := extractActionItems(summary)
	if len(actionItems) > 0 {
		sb.WriteString("## Key Action Items\n\n")
		for _, item := range actionItems {
			sb.WriteString(fmt.Sprintf("- [ ] %s\n", item))
		}
		sb.WriteString("\n")
	}

	if len(chapters) > 0 {
		sb.WriteString("## Chapter Breakdown\n\n")
		for i, ch := range chapters {
			sb.WriteString(fmt.Sprintf("### %d. %s (%s - %s)\n\n", i+1, ch.Title, timeutil.FormatTimestamp(ch.StartTime), timeutil.FormatTimestamp(ch.EndTime)))
			if ch.Summary != "" {
				sb.WriteString(ch.Summary + "\n\n")
			}
		}
	}

	if len(highlights) > 0 {
		sb.WriteString("## Key Highlights\n\n")
		for _, h := range highlights {
			title := "Highlight"
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

	if len(segments) > 0 {
		sb.WriteString("## Diarized Transcript\n\n")
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

	return sb.String()
}

func buildTextExport(rec *models.Recording, summary *models.Summary, chapters []models.Chapter, highlights []models.Highlight, segments []models.TranscriptSegment) string {
	var sb strings.Builder

	border := strings.Repeat("=", 72)
	divider := strings.Repeat("-", 72)

	sb.WriteString(border + "\n")
	sb.WriteString("MINUTES OF MEETING: " + strings.ToUpper(rec.Title) + "\n")
	sb.WriteString(border + "\n\n")

	sb.WriteString(fmt.Sprintf("Date:     %s\n", rec.CreatedAt.Format("2006-01-02 15:04:05 MST")))
	sb.WriteString(fmt.Sprintf("Duration: %s\n", timeutil.FormatTimestamp(rec.DurationSeconds)))
	sb.WriteString(fmt.Sprintf("Status:   %s\n\n", rec.Status))

	execSummary := extractExecutiveSummary(summary)
	if execSummary != "" {
		sb.WriteString(divider + "\n")
		sb.WriteString("EXECUTIVE SUMMARY\n")
		sb.WriteString(divider + "\n")
		sb.WriteString(execSummary + "\n\n")
	}

	actionItems := extractActionItems(summary)
	if len(actionItems) > 0 {
		sb.WriteString(divider + "\n")
		sb.WriteString("ACTION ITEMS\n")
		sb.WriteString(divider + "\n")
		for _, item := range actionItems {
			sb.WriteString(fmt.Sprintf("[ ] %s\n", item))
		}
		sb.WriteString("\n")
	}

	if len(chapters) > 0 {
		sb.WriteString(divider + "\n")
		sb.WriteString("CHAPTERS\n")
		sb.WriteString(divider + "\n")
		for i, ch := range chapters {
			sb.WriteString(fmt.Sprintf("%d. %s [%s - %s]\n", i+1, ch.Title, timeutil.FormatTimestamp(ch.StartTime), timeutil.FormatTimestamp(ch.EndTime)))
			if ch.Summary != "" {
				sb.WriteString(fmt.Sprintf("   %s\n", ch.Summary))
			}
		}
		sb.WriteString("\n")
	}

	if len(highlights) > 0 {
		sb.WriteString(divider + "\n")
		sb.WriteString("KEY HIGHLIGHTS\n")
		sb.WriteString(divider + "\n")
		for _, h := range highlights {
			title := "Highlight"
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

	if len(segments) > 0 {
		sb.WriteString(divider + "\n")
		sb.WriteString("DIARIZED TRANSCRIPT\n")
		sb.WriteString(divider + "\n")
		for _, seg := range segments {
			speaker := seg.SpeakerName
			if speaker == "" {
				speaker = seg.SpeakerLabel
			}
			if speaker == "" {
				speaker = "Speaker"
			}
			sb.WriteString(fmt.Sprintf("[%s - %s] %s: %s\n\n", timeutil.FormatTimestamp(seg.StartTime), timeutil.FormatTimestamp(seg.EndTime), speaker, seg.Text))
		}
	}

	return sb.String()
}

func buildJSONExport(rec *models.Recording, summary *models.Summary, chapters []models.Chapter, highlights []models.Highlight, segments []models.TranscriptSegment) ([]byte, error) {
	payload := dtos.JSONExportPayload{
		RecordingID:      rec.ID.String(),
		Title:            rec.Title,
		DurationSeconds:  rec.DurationSeconds,
		Status:           rec.Status,
		CreatedAt:        rec.CreatedAt,
		ExecutiveSummary: extractExecutiveSummary(summary),
		ActionItems:      extractActionItems(summary),
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

// generateStandardPDF constructs a valid, dependency-free PDF 1.4 byte document containing
// the exported MOM text.
func generateStandardPDF(title string, textContent string) []byte {
	// Clean text and break into lines
	sanitized := regexp.MustCompile(`[^\x20-\x7E\n]`).ReplaceAllString(textContent, " ")
	rawLines := strings.Split(sanitized, "\n")

	var wrappedLines []string
	for _, l := range rawLines {
		line := strings.TrimRight(l, " \r")
		for len(line) > 85 {
			wrappedLines = append(wrappedLines, line[:85])
			line = line[85:]
		}
		wrappedLines = append(wrappedLines, line)
	}

	// Maximum 50 lines per page
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
