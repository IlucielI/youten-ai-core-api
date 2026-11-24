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
	"unicode/utf8"

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

	// Record AI token usage for summary regeneration
	s.RecordAIUsageAsync(dtos.AIUsageRecordInput{
		UserID:           rec.UserID,
		WorkspaceID:      rec.WorkspaceID,
		RecordingID:      &rec.ID,
		ServiceType:      constants.AIServiceTypeLLMSummary.String(),
		Provider:         s.cfg.LLMProvider,
		ModelName:        s.cfg.LLMModel,
		PromptTokens:     structuredRes.Usage.PromptTokens,
		CompletionTokens: structuredRes.Usage.CompletionTokens,
		TotalTokens:      structuredRes.Usage.TotalTokens,
	})

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
	maxDirectSummaryTranscriptChars = 20000
	summaryMapChunkMaxChars         = 14000
	summaryChunkOverlapRunes        = 600
	maxSummaryCondenseWorkers       = 4
	minCondensedChunkChars          = 1500
	maxCondensedChunkChars          = 8000
	maxCombinedDigestRunes          = 22000
)

type summaryChunkJob struct {
	index       int
	overlapText string
	text        string
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
		startTime := time.Now()
		structuredRes, lastErr = s.llm.GenerateStructured(ctx, systemPrompt, userPrompt, schema)
		if lastErr == nil && structuredRes != nil {
			if recordingID != uuid.Nil {
				s.RecordAIUsageAsync(dtos.AIUsageRecordInput{
					RecordingID:      &recordingID,
					ServiceType:      constants.AIServiceTypeLLMSummary.String(),
					Provider:         s.cfg.LLMProvider,
					ModelName:        s.cfg.LLMModel,
					PromptTokens:     structuredRes.Usage.PromptTokens,
					CompletionTokens: structuredRes.Usage.CompletionTokens,
					TotalTokens:      structuredRes.Usage.TotalTokens,
					LatencyMs:        int(time.Since(startTime).Milliseconds()),
				})
			}
			return structuredRes, nil
		}
		if lastErr == nil {
			lastErr = errors.New("empty structured response from llm")
		}
		log.Printf("[SUMMARY WARN] recording %s: summarization attempt %d failed: %v", recordingID.String(), attempt, lastErr)
		if isPromptTooLongError(lastErr) {
			transcriptRunes := utf8.RuneCountInString(effectiveTranscript)
			shrunkTarget := transcriptRunes * 70 / 100
			if shrunkTarget < 1200 && transcriptRunes >= 1200 {
				shrunkTarget = 1200
			}
			effectiveTranscript = smartDeterministicCondense(effectiveTranscript, shrunkTarget)
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
		strings.Contains(lower, "string_above_max_length") ||
		strings.Contains(lower, "max_tokens") ||
		strings.Contains(lower, "prompt_tokens") ||
		strings.Contains(lower, "exceeds") ||
		strings.Contains(lower, "413")
}

func (s *Service) condenseTranscriptForSummary(ctx context.Context, recordingID uuid.UUID, transcriptBody string, targetLang string) string {
	trimmed := strings.TrimSpace(transcriptBody)
	if utf8.RuneCountInString(trimmed) <= maxDirectSummaryTranscriptChars {
		return trimmed
	}

	chunks := splitTranscriptIntoChunksWithOverlap(trimmed, summaryMapChunkMaxChars, summaryChunkOverlapRunes)
	if len(chunks) <= 1 {
		return smartDeterministicCondense(trimmed, maxDirectSummaryTranscriptChars)
	}

	headerOverhead := len(chunks) * 50
	usableBudget := maxCombinedDigestRunes - headerOverhead
	perChunkTarget := usableBudget / len(chunks)
	if perChunkTarget < minCondensedChunkChars {
		perChunkTarget = minCondensedChunkChars
	}
	if perChunkTarget > maxCondensedChunkChars {
		perChunkTarget = maxCondensedChunkChars
	}

	start := time.Now()
	maxWorkers := s.cfg.MaxSummaryWorkers
	if maxWorkers <= 0 {
		maxWorkers = maxSummaryCondenseWorkers
	}
	numWorkers := len(chunks)
	if numWorkers > maxWorkers {
		numWorkers = maxWorkers
	}

	log.Printf("[SUMMARY MAP-REDUCE START] recording %s: condensing transcript (%d chars, %d chunks, %d workers, target_lang=%s)",
		recordingID.String(), len(trimmed), len(chunks), numWorkers, targetLang)

	jobChan := make(chan summaryChunkJob, len(chunks))
	for _, c := range chunks {
		jobChan <- c
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
				results[job.index] = s.condenseSingleSummaryChunk(ctx, job.index, len(chunks), job.overlapText, job.text, targetLang, perChunkTarget)
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
	log.Printf("[SUMMARY MAP-REDUCE DONE] recording %s: condensed into %d runes in %s",
		recordingID.String(), utf8.RuneCountInString(combined), time.Since(start).Round(time.Millisecond))
	if utf8.RuneCountInString(combined) > maxCombinedDigestRunes {
		return smartDeterministicCondense(combined, maxCombinedDigestRunes)
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
	overlapText string,
	chunkText string,
	targetLang string,
	maxDigestChars int,
) string {
	if ctx.Err() != nil || s.llm == nil {
		return smartDeterministicCondense(chunkText, maxDigestChars)
	}

	langHint := strings.TrimSpace(targetLang)
	if langHint == "" {
		langHint = "the primary language used by the speakers"
	}

	sysPrompt := "You are an executive meeting analyst and senior intelligence secretary. " +
		"Your objective is zero-loss factual condensation of this chronological meeting transcript slice into a high-density, timestamped digest. " +
		"Preserve: (1) All [start - end] timestamps and speaker names, " +
		"(2) All discussion topics and arguments, " +
		"(3) All key decisions, resolutions, and consensus points verbatim, " +
		"(4) All action items with task description, PIC/assignee, due date, and deliverable, " +
		"(5) All quantitative metrics, numbers, percentages, dates, system names, error codes, and technical details, " +
		"(6) All open points, pending questions, blockers, and risks. " +
		"Do NOT generalize, omit, or speculate. Do NOT drop concrete facts."

	var usrPrompt string
	if strings.TrimSpace(overlapText) != "" {
		usrPrompt = fmt.Sprintf(
			"[PREVIOUS DISCUSSION CONTEXT - FOR CONTINUITY REFERENCE ONLY]:\n%s\n\n"+
				"[ACTIVE TRANSCRIPT TO CONDENSE - Part %d of %d in %s (target ~%d characters)]:\n%s\n\n"+
				"Synthesize the active transcript into a high-density, timestamped chronological digest. Retain [start - end] Speaker: format for every significant statement, decision, and action item.",
			strings.TrimSpace(overlapText),
			chunkIdx+1,
			totalChunks,
			langHint,
			maxDigestChars,
			chunkText,
		)
	} else {
		usrPrompt = fmt.Sprintf(
			"Condense Part %d of %d of the meeting transcript in %s into a high-density chronological digest (target ~%d characters). Retain [start - end] Speaker: format for key statements, decisions, and action items:\n\n%s",
			chunkIdx+1,
			totalChunks,
			langHint,
			maxDigestChars,
			chunkText,
		)
	}

	temp := 0.1
	var res *dtos.ChatResponse
	var err error

	// 1 retry with fast backoff for resilience against transient proxy blips
	for attempt := 1; attempt <= 2; attempt++ {
		if ctx.Err() != nil {
			break
		}
		res, err = s.llm.GenerateChatResponse(ctx, sysPrompt, []dtos.ChatMessageInput{
			{Role: "user", Content: usrPrompt},
		}, dtos.ChatOptions{Temperature: &temp})
		if err == nil && res != nil && strings.TrimSpace(res.Content) != "" {
			break
		}
		if attempt < 2 {
			select {
			case <-ctx.Done():
				return smartDeterministicCondense(chunkText, maxDigestChars)
			case <-time.After(500 * time.Millisecond):
			}
		}
	}

	if err != nil || res == nil || strings.TrimSpace(res.Content) == "" {
		log.Printf("[SUMMARY WARN] map-reduce chunk %d/%d condensation fallback: %v", chunkIdx+1, totalChunks, err)
		return smartDeterministicCondense(chunkText, maxDigestChars)
	}

	digest := strings.TrimSpace(res.Content)
	// Allow headroom up to 1.5x target without destructive line dropping
	if utf8.RuneCountInString(digest) > (maxDigestChars*15)/10+500 {
		return smartDeterministicCondense(digest, maxDigestChars)
	}
	return digest
}

func splitTranscriptIntoChunksWithOverlap(transcriptBody string, maxChunkChars int, overlapChars int) []summaryChunkJob {
	trimmedBody := strings.TrimSpace(transcriptBody)
	if maxChunkChars <= 0 || utf8.RuneCountInString(trimmedBody) <= maxChunkChars {
		return []summaryChunkJob{{index: 0, text: trimmedBody}}
	}

	rawChunks := splitTranscriptIntoChunks(trimmedBody, maxChunkChars)
	if len(rawChunks) <= 1 {
		return []summaryChunkJob{{index: 0, text: trimmedBody}}
	}

	result := make([]summaryChunkJob, len(rawChunks))
	for i, c := range rawChunks {
		var overlap string
		if i > 0 && overlapChars > 0 {
			prevLines := strings.Split(rawChunks[i-1], "\n")
			var selectedPrev []string
			accum := 0
			// Take up to 3 trailing lines from previous chunk
			for j := len(prevLines) - 1; j >= 0; j-- {
				line := strings.TrimSpace(prevLines[j])
				if line == "" {
					continue
				}
				lineRunes := utf8.RuneCountInString(line)
				if accum > 0 && accum+lineRunes+1 > overlapChars {
					break
				}
				selectedPrev = append([]string{line}, selectedPrev...)
				accum += lineRunes + 1
				if len(selectedPrev) >= 3 {
					break
				}
			}
			overlap = strings.Join(selectedPrev, "\n")
		}
		result[i] = summaryChunkJob{
			index:       i,
			overlapText: overlap,
			text:        c,
		}
	}
	return result
}

func splitTranscriptIntoChunks(transcriptBody string, maxChunkChars int) []string {
	if maxChunkChars <= 0 || utf8.RuneCountInString(transcriptBody) <= maxChunkChars {
		return []string{transcriptBody}
	}

	lines := strings.Split(transcriptBody, "\n")
	var chunks []string
	var current strings.Builder
	currentRunes := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		for utf8.RuneCountInString(trimmed) > maxChunkChars {
			if current.Len() > 0 {
				chunks = append(chunks, strings.TrimSpace(current.String()))
				current.Reset()
				currentRunes = 0
			}
			runes := []rune(trimmed)
			chunks = append(chunks, string(runes[:maxChunkChars]))
			trimmed = strings.TrimSpace(string(runes[maxChunkChars:]))
		}
		if trimmed == "" {
			continue
		}

		trimmedRunes := utf8.RuneCountInString(trimmed)
		if currentRunes > 0 && currentRunes+trimmedRunes+1 > maxChunkChars {
			chunks = append(chunks, strings.TrimSpace(current.String()))
			current.Reset()
			currentRunes = 0
		}
		current.WriteString(trimmed)
		current.WriteByte('\n')
		currentRunes += trimmedRunes + 1
	}
	if current.Len() > 0 {
		chunks = append(chunks, strings.TrimSpace(current.String()))
	}
	return chunks
}

func truncateRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes])
}

func deterministicCondenseChunk(text string, maxChars int) string {
	return smartDeterministicCondense(text, maxChars)
}

func isPriorityLine(line string) bool {
	lower := strings.ToLower(line)
	if strings.HasPrefix(line, "--- [") {
		return true
	}
	priorityKeywords := []string{
		"decision", "keputusan", "sepakat", "setuju", "disepakati",
		"action item", "action:", "action :", "tindakan", "tugas", "pic:", "pic :", "pic ", "deadline", "target", "due date",
		"kesimpulan", "blocker", "masalah", "open point", "open issue",
		"risk", "risiko", "catatan", "rekomendasi", "rfc", "adr",
	}
	for _, kw := range priorityKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

func smartDeterministicCondense(text string, maxChars int) string {
	trimmed := strings.TrimSpace(text)
	if maxChars <= 0 || utf8.RuneCountInString(trimmed) <= maxChars {
		return trimmed
	}

	lines := strings.Split(trimmed, "\n")
	if len(lines) <= 1 {
		return truncateRunes(trimmed, maxChars)
	}

	type indexedLine struct {
		index    int
		text     string
		runes    int
		priority bool
	}

	var allLines []indexedLine
	var priorityLines []indexedLine
	var dialogueLines []indexedLine

	for idx, line := range lines {
		clean := strings.TrimSpace(line)
		if clean == "" {
			continue
		}
		rCount := utf8.RuneCountInString(clean)
		item := indexedLine{
			index:    idx,
			text:     clean,
			runes:    rCount,
			priority: isPriorityLine(clean),
		}
		allLines = append(allLines, item)
		if item.priority {
			priorityLines = append(priorityLines, item)
		} else {
			dialogueLines = append(dialogueLines, item)
		}
	}

	if len(allLines) == 0 {
		return truncateRunes(trimmed, maxChars)
	}

	priorityRunes := 0
	for _, pl := range priorityLines {
		priorityRunes += pl.runes + 1
	}

	selectedMap := make(map[int]string)
	budgetRemaining := maxChars

	if priorityRunes <= maxChars {
		for _, pl := range priorityLines {
			selectedMap[pl.index] = pl.text
		}
		budgetRemaining -= priorityRunes

		if len(dialogueLines) > 0 && budgetRemaining > 20 {
			dialogueBudget := budgetRemaining
			avgLen := 0
			for _, dl := range dialogueLines {
				avgLen += dl.runes
			}
			avgLen /= len(dialogueLines)
			if avgLen < 20 {
				avgLen = 20
			}
			targetCount := dialogueBudget / (avgLen + 1)
			if targetCount > 0 {
				step := 1
				if len(dialogueLines) > targetCount {
					step = len(dialogueLines) / targetCount
					if step < 1 {
						step = 1
					}
				}
				usedDialogue := 0
				for i := 0; i < len(dialogueLines); i += step {
					dl := dialogueLines[i]
					if usedDialogue+dl.runes+1 > dialogueBudget {
						break
					}
					selectedMap[dl.index] = dl.text
					usedDialogue += dl.runes + 1
				}
			}
		}
	} else {
		used := 0
		for _, pl := range priorityLines {
			if used+pl.runes+1 > maxChars {
				break
			}
			selectedMap[pl.index] = pl.text
			used += pl.runes + 1
		}
	}

	if len(selectedMap) == 0 {
		return truncateRunes(trimmed, maxChars)
	}

	var resultLines []string
	for _, item := range allLines {
		if txt, exists := selectedMap[item.index]; exists {
			resultLines = append(resultLines, txt)
		}
	}

	assembled := strings.Join(resultLines, "\n")
	if utf8.RuneCountInString(assembled) > maxChars {
		return truncateRunes(assembled, maxChars)
	}
	return assembled
}

