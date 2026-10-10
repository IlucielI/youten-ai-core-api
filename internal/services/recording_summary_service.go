// recording_summary_service.go — transcript speaker edits and summary version use-cases.
package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
	"code-base-golang/internal/payload"
	"code-base-golang/internal/pkg/apperror"
	"code-base-golang/internal/pkg/jsonutil"
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
	if !s.authorizeRecordingAccess(ctx, rec, ownershipToken, recordingAccessPolicy{}) {
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

// UpdateTranscriptSegment updates the text of a single transcript segment and dispatches async chunk re-indexing.
func (s *Service) UpdateTranscriptSegment(ctx context.Context, id uuid.UUID, segmentID uuid.UUID, ownershipToken string, req dtos.UpdateTranscriptSegmentRequest) (*dtos.TranscriptSegmentDTO, error) {
	rec, err := s.repo.FindRecordingByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrRecordingNotFound
		}
		return nil, fmt.Errorf("failed to lookup recording: %w", err)
	}

	// Ownership verification (owner or ownership token).
	if !s.authorizeRecordingAccess(ctx, rec, ownershipToken, recordingAccessPolicy{}) {
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

	// Trigger async re-indexing so RAG chunk embeddings stay in sync
	_ = s.publishEvent(ctx, constants.TopicRecordingIndex, payload.RecordingPipelinePayload{
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
	if !s.authorizeRecordingAccess(ctx, rec, ownershipToken, recordingAccessPolicy{}) {
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
	templateKey := strings.TrimSpace(req.TemplateCategory)
	if templateKey == "" {
		templateKey = rec.SelectedTemplate
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

	userPrompt, err := templates.RenderSummaryUserPrompt(transcriptBody, targetLang, customAngle)
	if err != nil {
		userPrompt = fmt.Sprintf("Please summarize the following meeting transcript:\n\n%s", transcriptBody)
	}

	var schema map[string]interface{}
	if template != nil && len(template.OutputSchema) > 0 {
		schema = map[string]interface{}(template.OutputSchema)
	} else {
		schema = templates.DefaultSchemaForTemplate(templateKey)
	}

	structuredRes, err := s.llm.GenerateStructured(ctx, systemPrompt, userPrompt, schema)
	if err != nil {
		return nil, fmt.Errorf("llm generation failed: %w", err)
	}

	cleanedJSON := jsonutil.CleanMarkdownJSON(structuredRes.RawJSON)
	var structMap map[string]interface{}
	if err := json.Unmarshal([]byte(cleanedJSON), &structMap); err != nil {
		return nil, fmt.Errorf("invalid structured response JSON: %w", err)
	}

	markdownContent := ""
	if md, ok := structMap["markdown_content"].(string); ok && md != "" {
		markdownContent = md
	} else if md, ok := structMap["executive_summary"].(string); ok && md != "" {
		markdownContent = md
	} else if md, ok := structMap["executive_overview"].(string); ok && md != "" {
		markdownContent = md
	} else {
		markdownContent = cleanedJSON
	}

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
	if !s.authorizeRecordingAccess(ctx, rec, ownershipToken, recordingAccessPolicy{}) {
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
	if !s.authorizeRecordingAccess(ctx, rec, ownershipToken, recordingAccessPolicy{}) {
		return nil, constants.ErrForbidden
	}

	activated, err := s.repo.ActivateSummaryVersion(ctx, rec.ID, versionID)
	if err != nil {
		return nil, err
	}

	resp := toSummaryVersionResponse(*activated)
	return &resp, nil
}
