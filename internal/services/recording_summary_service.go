// recording_summary_service.go — transcript speaker edits and summary version use-cases.
package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
	"code-base-golang/internal/queue"
	"code-base-golang/internal/pkg/apperror"
	"code-base-golang/internal/pkg/jsonutil"
	"code-base-golang/internal/pkg/sttalign"
	"code-base-golang/internal/templates"
)

// UpdateTranscriptSpeakers verifies recording ownership and batch-updates speaker names
// across matching transcript segments.
func (s *Service) UpdateTranscriptSpeakers(ctx context.Context, id uuid.UUID, ownershipToken string, req dtos.UpdateSpeakersRequest) (*dtos.UpdateSpeakersResponse, error) {
	rec, err := s.repo.FindRecordingByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrRecordingNotFound
		}
		return nil, fmt.Errorf("failed to lookup recording: %w", err)
	}

	// Ownership verification (owner or ownership token).
	if !s.authorizeRecordingAccess(ctx, rec, ownershipToken, dtos.RecordingAccessPolicy{}) {
		return nil, constants.ErrForbidden
	}

	cleanedSpeakers := make(map[string]string, len(req.Speakers))
	for label, name := range req.Speakers {
		cleanedSpeakers[strings.TrimSpace(label)] = strings.TrimSpace(name)
	}

	updatedCount, err := s.repo.UpdateTranscriptSpeakerNames(ctx, rec.ID, cleanedSpeakers)
	if err != nil {
		return nil, fmt.Errorf("failed to update transcript speaker names: %w", err)
	}

	return &dtos.UpdateSpeakersResponse{
		UpdatedCount: int(updatedCount),
		Speakers:     cleanedSpeakers,
	}, nil
}

// UpdateTranscriptSegment updates the text and/or speaker of a single transcript segment and dispatches async chunk re-indexing.
func (s *Service) UpdateTranscriptSegment(ctx context.Context, id uuid.UUID, segmentID uuid.UUID, ownershipToken string, req dtos.UpdateTranscriptSegmentRequest) (*dtos.TranscriptSegmentDTO, error) {
	rec, err := s.repo.FindRecordingByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrRecordingNotFound
		}
		return nil, fmt.Errorf("failed to lookup recording: %w", err)
	}

	// Ownership verification (owner or ownership token).
	if !s.authorizeRecordingAccess(ctx, rec, ownershipToken, dtos.RecordingAccessPolicy{}) {
		return nil, constants.ErrForbidden
	}

	cleanedText := strings.TrimSpace(req.Text)
	updatedSeg, err := s.repo.UpdateTranscriptSegmentText(ctx, rec.ID, segmentID, cleanedText)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrTranscriptSegmentNotFound
		}
		return nil, fmt.Errorf("failed to update transcript segment: %w", err)
	}

	if req.SpeakerLabel != nil || req.SpeakerName != nil {
		updates := make(map[string]any)
		if req.SpeakerLabel != nil && strings.TrimSpace(*req.SpeakerLabel) != "" {
			normSpk := sttalign.NormalizeSpeakerLabel(*req.SpeakerLabel)
			updates["speaker_label"] = normSpk
		}
		if req.SpeakerName != nil && strings.TrimSpace(*req.SpeakerName) != "" {
			name := strings.TrimSpace(*req.SpeakerName)
			updates["speaker_name"] = name
		}
		if len(updates) > 0 && s.repo.DB() != nil {
			if err := s.repo.DB().WithContext(ctx).Model(&models.TranscriptSegment{}).Where("id = ? AND recording_id = ?", segmentID, rec.ID).Updates(updates).Error; err != nil {
				return nil, fmt.Errorf("failed to update transcript segment speaker: %w", err)
			}
			if spk, ok := updates["speaker_label"].(string); ok {
				updatedSeg.SpeakerLabel = spk
			}
			if name, ok := updates["speaker_name"].(string); ok {
				updatedSeg.SpeakerName = name
			}
		}
	}

	// Trigger async re-indexing so RAG chunk embeddings stay in sync
	_ = s.publishEvent(ctx, constants.TopicRecordingIndex, queue.RecordingPipelinePayload{
		RecordingID: rec.ID,
	})

	return &dtos.TranscriptSegmentDTO{
		ID:            updatedSeg.ID.String(),
		SpeakerLabel:  updatedSeg.SpeakerLabel,
		SpeakerName:   updatedSeg.SpeakerName,
		StartTime:     updatedSeg.StartTime,
		EndTime:       updatedSeg.EndTime,
		Text:          updatedSeg.Text,
		WordsData:     updatedSeg.WordsData,
		SequenceOrder: updatedSeg.SequenceOrder,
	}, nil
}

// RegenerateSummary generates a new structured summary version for a recording with optional template category and custom angle.
func (s *Service) RegenerateSummary(ctx context.Context, id uuid.UUID, ownershipToken string, req dtos.RegenerateSummaryRequest) (*dtos.SummaryVersionResponse, error) {
	rec, err := s.repo.FindRecordingByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrRecordingNotFound
		}
		return nil, fmt.Errorf("failed to lookup recording: %w", err)
	}

	// Verify ownership (owner or ownership token).
	if !s.authorizeRecordingAccess(ctx, rec, ownershipToken, dtos.RecordingAccessPolicy{}) {
		return nil, constants.ErrForbidden
	}

	// Conflict protection: check if currently actively processing
	switch strings.ToUpper(strings.TrimSpace(rec.Status)) {
	case models.RecordingStatusPending,
		models.RecordingStatusQueued,
		models.RecordingStatusValidating,
		models.RecordingStatusExtracting,
		models.RecordingStatusTranscribing,
		models.RecordingStatusSummarizing,
		models.RecordingStatusIndexing:
		return nil, constants.ErrConflictProcessing
	}

	// Enforce version cap: If total summary versions >= 5, return 409 SUMMARY_VERSION_LIMIT
	count, err := s.repo.CountSummaryVersions(ctx, rec.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to count summary versions: %w", err)
	}
	if count >= 5 {
		return nil, constants.ErrSummaryVersionLimit
	}

	// Retrieve transcript segments
	segments, err := s.repo.ListTranscriptSegmentsByRecordingID(ctx, rec.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to list transcript segments: %w", err)
	}
	if len(segments) == 0 {
		return nil, apperror.New(http.StatusBadRequest, "NO_TRANSCRIPT", "cannot regenerate summary without transcript")
	}

	// Build transcript text
	var sb strings.Builder
	for _, seg := range segments {
		startStr := templates.FormatTimestamp(seg.StartTime)
		endStr := templates.FormatTimestamp(seg.EndTime)
		sb.WriteString(fmt.Sprintf("[%s - %s] %s: %s\n", startStr, endStr, seg.SpeakerName, seg.Text))
	}
	transcriptBody := sb.String()

	// Determine template
	templateKey := strings.ToUpper(strings.TrimSpace(req.TemplateCategory))
	if templateKey == "" {
		templateKey = strings.ToUpper(strings.TrimSpace(rec.SelectedTemplate))
	}
	if templateKey == "" {
		templateKey = constants.TemplateKeyGeneral
	}

	template, err := s.repo.FindTemplateByCategoryKey(ctx, templateKey)
	if err != nil || template == nil {
		template, _ = s.repo.FindTemplateByCategoryKey(ctx, constants.TemplateKeyGeneral)
	}

	systemPrompt, err := templates.DefaultSummarySystemPrompt()
	if err != nil {
		systemPrompt = "You are a professional executive meeting secretary and structured summarizer."
	}
	if template != nil && template.Prompt != "" {
		systemPrompt = template.Prompt
	} else if defPrompt := templates.DefaultPromptForTemplate(templateKey); defPrompt != "" {
		systemPrompt = defPrompt
	}

	targetLang := rec.OutputLanguage
	if targetLang == "" && rec.DetectedLanguage != nil {
		targetLang = *rec.DetectedLanguage
	}

	var customAngle string
	if req.CustomAngle != nil {
		customAngle = strings.TrimSpace(*req.CustomAngle)
	}

	var visualCueViews []templates.VisualCueView
	if keyframes, err := s.repo.ListVideoKeyframesByRecordingID(ctx, rec.ID); err == nil && len(keyframes) > 0 {
		for _, kf := range keyframes {
			visualCueViews = append(visualCueViews, templates.VisualCueView{
				TimestampFormatted: templates.FormatTimestamp(kf.TimestampSeconds),
				CueType:            kf.CueType,
				Title:              kf.Title,
				ExtractedText:      kf.ExtractedText,
				VisualSummary:      kf.VisualSummary,
			})
		}
	}

	var schema map[string]interface{}
	if template != nil && len(template.OutputSchema) > 0 {
		schema = map[string]interface{}(template.OutputSchema)
	} else {
		schema = templates.DefaultSchemaForTemplate(templateKey)
	}

	structuredRes, lastErr := s.generateStructuredSummary(
		ctx,
		uuid.Nil,
		systemPrompt,
		transcriptBody,
		targetLang,
		customAngle,
		visualCueViews,
		schema,
	)
	if lastErr != nil {
		if errors.Is(lastErr, context.Canceled) || errors.Is(lastErr, context.DeadlineExceeded) {
			return nil, lastErr
		}
		return nil, fmt.Errorf("llm generation failed: %w", lastErr)
	}
	if structuredRes == nil {
		return nil, errors.New("llm returned nil structured response")
	}

	cleanedJSON := jsonutil.CleanMarkdownJSON(structuredRes.RawJSON)
	var structMap map[string]interface{}
	if err := json.Unmarshal([]byte(cleanedJSON), &structMap); err != nil {
		return nil, fmt.Errorf("invalid structured response JSON: %w", err)
	}

	markdownContent := AssembleStructuredMarkdown(structMap, cleanedJSON)

	var customAnglePtr *string
	if req.CustomAngle != nil && strings.TrimSpace(*req.CustomAngle) != "" {
		trimmed := strings.TrimSpace(*req.CustomAngle)
		customAnglePtr = &trimmed
	}

	newSummary := models.Summary{
		ID:               uuid.New(),
		RecordingID:      rec.ID,
		TemplateCategory: templateKey,
		CustomAngle:      customAnglePtr,
		StructuredData:   models.JSONMap(structMap),
		MarkdownContent:  markdownContent,
	}

	// If recording was previously in FAILED status, restore it to COMPLETED before saving new summary
	if rec.Status == models.RecordingStatusFailed {
		if err := s.repo.UpdateRecordingStatus(ctx, rec.ID, models.RecordingStatusCompleted, nil, nil); err != nil {
			return nil, fmt.Errorf("failed to restore recording status to completed: %w", err)
		}
	}

	if err := s.repo.SaveNewSummaryVersion(ctx, &newSummary); err != nil {
		return nil, fmt.Errorf("failed to save summary version: %w", err)
	}

	resp := toSummaryVersionResponse(newSummary)
	return &resp, nil
}

// ListSummaryVersions retrieves all summary versions for a recording ordered by version ASC.
func (s *Service) ListSummaryVersions(ctx context.Context, id uuid.UUID, ownershipToken string) ([]dtos.SummaryVersionResponse, error) {
	rec, err := s.repo.FindRecordingByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrRecordingNotFound
		}
		return nil, fmt.Errorf("failed to lookup recording: %w", err)
	}

	// Verify ownership (owner or ownership token).
	if !s.authorizeRecordingAccess(ctx, rec, ownershipToken, dtos.RecordingAccessPolicy{}) {
		return nil, constants.ErrForbidden
	}

	summaries, err := s.repo.ListSummaryVersions(ctx, rec.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to list summary versions: %w", err)
	}

	result := make([]dtos.SummaryVersionResponse, 0, len(summaries))
	for _, sum := range summaries {
		result = append(result, toSummaryVersionResponse(sum))
	}

	return result, nil
}

// ActivateSummaryVersion activates a specific historical summary version for a recording.
func (s *Service) ActivateSummaryVersion(ctx context.Context, id uuid.UUID, versionID string, ownershipToken string) (*dtos.SummaryVersionResponse, error) {
	rec, err := s.repo.FindRecordingByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrRecordingNotFound
		}
		return nil, fmt.Errorf("failed to lookup recording: %w", err)
	}

	// Verify ownership (owner or ownership token).
	if !s.authorizeRecordingAccess(ctx, rec, ownershipToken, dtos.RecordingAccessPolicy{}) {
		return nil, constants.ErrForbidden
	}

	activated, err := s.repo.ActivateSummaryVersion(ctx, rec.ID, versionID)
	if err != nil {
		return nil, err
	}

	resp := toSummaryVersionResponse(*activated)
	return &resp, nil
}

const (
	maxDirectSummaryTranscriptChars = 16000
	summaryMapChunkMaxChars         = 14000
	maxSummaryCondenseWorkers       = 4
	minCondensedChunkChars          = 1500
	maxCondensedChunkChars          = 3500
)

type summaryChunkJob struct {
	index int
	text  string
}

func (s *Service) generateStructuredSummary(
	ctx context.Context,
	recordingID uuid.UUID,
	systemPrompt string,
	transcriptBody string,
	targetLang string,
	customAngle string,
	visualCueViews []templates.VisualCueView,
	schema map[string]interface{},
) (*dtos.StructuredResponse, error) {
	effectiveTranscript := s.condenseTranscriptForSummary(ctx, recordingID, transcriptBody, targetLang)

	var structuredRes *dtos.StructuredResponse
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		userPrompt := renderSummaryPromptOrFallback(effectiveTranscript, targetLang, customAngle, visualCueViews)
		structuredRes, lastErr = s.llm.GenerateStructured(ctx, systemPrompt, userPrompt, schema)
		if lastErr == nil && structuredRes != nil {
			return structuredRes, nil
		}
		if lastErr == nil {
			lastErr = errors.New("empty structured response from llm")
		}
		log.Printf("[SUMMARY WARN] recording %s: summarization attempt %d failed: %v", recordingID.String(), attempt, lastErr)
		if isPromptTooLongError(lastErr) {
			shrunkTarget := len(effectiveTranscript) * 60 / 100
			if shrunkTarget < 4000 {
				shrunkTarget = 4000
			}
			effectiveTranscript = deterministicCondenseChunk(effectiveTranscript, shrunkTarget)
		}
		if attempt < 3 {
			if waitErr := waitSummaryRetry(ctx, attempt); waitErr != nil {
				return nil, waitErr
			}
		}
	}
	return nil, lastErr
}

func renderSummaryPromptOrFallback(transcriptBody string, targetLang string, customAngle string, visualCueViews []templates.VisualCueView) string {
	userPrompt, err := templates.RenderSummaryUserPromptWithCues(transcriptBody, targetLang, customAngle, visualCueViews)
	if err != nil {
		return fmt.Sprintf("Please summarize the following meeting transcript:\n\n%s", transcriptBody)
	}
	return userPrompt
}

func waitSummaryRetry(ctx context.Context, attempt int) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(time.Duration(attempt) * time.Second):
		return nil
	}
}

func isPromptTooLongError(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "too long") ||
		strings.Contains(lower, "context_length") ||
		strings.Contains(lower, "maximum context") ||
		strings.Contains(lower, "request too large") ||
		strings.Contains(lower, "string_above_max_length")
}

func (s *Service) condenseTranscriptForSummary(ctx context.Context, recordingID uuid.UUID, transcriptBody string, targetLang string) string {
	if len(transcriptBody) <= maxDirectSummaryTranscriptChars {
		return transcriptBody
	}

	chunks := splitTranscriptIntoChunks(transcriptBody, summaryMapChunkMaxChars)
	if len(chunks) <= 1 {
		return deterministicCondenseChunk(transcriptBody, maxDirectSummaryTranscriptChars)
	}

	perChunkTarget := maxDirectSummaryTranscriptChars / len(chunks)
	if perChunkTarget < minCondensedChunkChars {
		perChunkTarget = minCondensedChunkChars
	}
	if perChunkTarget > maxCondensedChunkChars {
		perChunkTarget = maxCondensedChunkChars
	}

	numWorkers := len(chunks)
	if numWorkers > maxSummaryCondenseWorkers {
		numWorkers = maxSummaryCondenseWorkers
	}

	jobChan := make(chan summaryChunkJob, len(chunks))
	for i, c := range chunks {
		jobChan <- summaryChunkJob{index: i, text: c}
	}
	close(jobChan)

	results := make([]string, len(chunks))
	var (
		wg        sync.WaitGroup
		completed int64
		total     = int64(len(chunks))
	)

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobChan {
				results[job.index] = s.condenseSingleSummaryChunk(ctx, job.index, len(chunks), job.text, targetLang, perChunkTarget)
				done := atomic.AddInt64(&completed, 1)
				s.touchSummarizingHeartbeat(ctx, recordingID, done, total)
			}
		}()
	}
	wg.Wait()

	var sb strings.Builder
	for i, digest := range results {
		sb.WriteString(fmt.Sprintf("--- [Part %d/%d] ---\n%s\n\n", i+1, len(results), strings.TrimSpace(digest)))
	}
	combined := strings.TrimSpace(sb.String())
	if len(combined) > maxDirectSummaryTranscriptChars {
		return deterministicCondenseChunk(combined, maxDirectSummaryTranscriptChars)
	}
	return combined
}

func (s *Service) touchSummarizingHeartbeat(ctx context.Context, recordingID uuid.UUID, done int64, total int64) {
	if recordingID == uuid.Nil || total <= 0 {
		return
	}
	prog := 60 + int((float64(done)/float64(total))*20.0)
	s.publishCustomProgress(recordingID, models.RecordingStatusSummarizing, "SUMMARIZING", prog)
	_ = s.repo.UpdateRecordingStatus(ctx, recordingID, models.RecordingStatusSummarizing, nil, nil)
}

func (s *Service) condenseSingleSummaryChunk(
	ctx context.Context,
	chunkIdx int,
	totalChunks int,
	chunkText string,
	targetLang string,
	maxDigestChars int,
) string {
	if ctx.Err() != nil || s.llm == nil {
		return deterministicCondenseChunk(chunkText, maxDigestChars)
	}

	langHint := strings.TrimSpace(targetLang)
	if langHint == "" {
		langHint = "the primary language used by the speakers"
	}

	sysPrompt := "You are an executive meeting analyst. Condense the provided chronological meeting transcript slice into a dense, factual, timestamped digest. Preserve [start - end] timestamps, speaker names, discussion topics, key arguments, technical details, numbers/metrics, decisions made, root causes, contested points, action items (task, PIC, due date), and open points/issues. Do NOT omit concrete facts or names."
	usrPrompt := fmt.Sprintf(
		"Condense Part %d of %d of the meeting transcript in %s into a high-density chronological digest (target ~%d characters). Retain [start - end] Speaker: format for key statements, decisions, and action items:\n\n%s",
		chunkIdx+1,
		totalChunks,
		langHint,
		maxDigestChars,
		chunkText,
	)

	temp := 0.1
	res, err := s.llm.GenerateChatResponse(ctx, sysPrompt, []dtos.ChatMessageInput{
		{Role: "user", Content: usrPrompt},
	}, dtos.ChatOptions{Temperature: &temp})
	if err != nil || res == nil || strings.TrimSpace(res.Content) == "" {
		log.Printf("[SUMMARY WARN] map-reduce chunk %d/%d condensation fallback: %v", chunkIdx+1, totalChunks, err)
		return deterministicCondenseChunk(chunkText, maxDigestChars)
	}

	digest := strings.TrimSpace(res.Content)
	if len(digest) > maxDigestChars+500 {
		return deterministicCondenseChunk(digest, maxDigestChars)
	}
	return digest
}

func splitTranscriptIntoChunks(transcriptBody string, maxChunkChars int) []string {
	if maxChunkChars <= 0 || len(transcriptBody) <= maxChunkChars {
		return []string{transcriptBody}
	}

	lines := strings.Split(transcriptBody, "\n")
	var chunks []string
	var current strings.Builder

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if len(trimmed) > maxChunkChars {
			trimmed = trimmed[:maxChunkChars]
		}
		if current.Len() > 0 && current.Len()+len(trimmed)+1 > maxChunkChars {
			chunks = append(chunks, strings.TrimSpace(current.String()))
			current.Reset()
		}
		current.WriteString(trimmed)
		current.WriteByte('\n')
	}
	if current.Len() > 0 {
		chunks = append(chunks, strings.TrimSpace(current.String()))
	}
	return chunks
}

func deterministicCondenseChunk(text string, maxChars int) string {
	trimmed := strings.TrimSpace(text)
	if maxChars <= 0 || len(trimmed) <= maxChars {
		return trimmed
	}

	lines := strings.Split(trimmed, "\n")
	if len(lines) <= 1 {
		return trimmed[:maxChars]
	}

	// Sample evenly across chronological lines so start, middle, and end of the slice are preserved.
	var selected []string
	totalLen := 0
	avgLineLen := len(trimmed) / len(lines)
	if avgLineLen < 20 {
		avgLineLen = 20
	}
	targetLines := maxChars / avgLineLen
	if targetLines < 3 {
		targetLines = 3
	}
	step := len(lines) / targetLines
	if step < 1 {
		step = 1
	}

	for i := 0; i < len(lines); i += step {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if totalLen+len(line)+1 > maxChars {
			break
		}
		selected = append(selected, line)
		totalLen += len(line) + 1
	}

	lastLine := strings.TrimSpace(lines[len(lines)-1])
	if len(selected) > 0 && lastLine != "" && selected[len(selected)-1] != lastLine && totalLen+len(lastLine)+1 <= maxChars {
		selected = append(selected, lastLine)
	}

	if len(selected) == 0 {
		return trimmed[:maxChars]
	}
	return strings.Join(selected, "\n")
}

