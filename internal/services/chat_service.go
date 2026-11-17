package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
	"code-base-golang/internal/pkg/ctxmeta"
	"code-base-golang/internal/pkg/jsonutil"
	"code-base-golang/internal/pkg/timeutil"
)

// InitiateRecordingChatStream validates access (owner, guest, or share token), retrieves relevant context chunks,
// restores conversational session history, enriches with meeting intelligence metadata, and initiates an LLM response stream.
func (s *Service) InitiateRecordingChatStream(
	ctx context.Context,
	recordingID uuid.UUID,
	ownershipToken string,
	req dtos.RecordingChatRequest,
) (*dtos.RecordingChatStreamResult, error) {
	rec, err := s.repo.FindRecordingByID(ctx, recordingID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrRecordingNotFound
		}
		return nil, fmt.Errorf("failed to lookup recording: %w", err)
	}

	// Access verification: owner, matching ownership token, active guest session, or valid share token
	policy := dtos.RecordingAccessPolicy{AllowShareToken: true}
	if !s.authorizeRecordingAccess(ctx, rec, ownershipToken, policy) {
		if req.ShareToken == "" || !s.authorizeRecordingAccess(ctx, rec, req.ShareToken, policy) {
			return nil, constants.ErrForbidden
		}
	}

	trimmedMsg := strings.TrimSpace(req.Message)

	// Resolve conversation session ID and restore session history
	var sessionID uuid.UUID
	var priorMessages []models.ChatMessage
	if req.SessionID != nil && *req.SessionID != uuid.Nil {
		sessionID = *req.SessionID
		// Prevent share visitors from appending to sessions owned by another user or recording owner
		existingMsgs, err := s.repo.ListChatMessagesBySession(ctx, rec.ID, sessionID)
		if err != nil {
			return nil, fmt.Errorf("failed to check session access: %w", err)
		}
		if !s.isSessionAccessibleByCaller(ctx, rec, ownershipToken, existingMsgs) {
			return nil, constants.ErrForbidden
		}
		priorMessages = existingMsgs
	} else {
		sessionID = uuid.New()
	}

	var userID *uuid.UUID
	if authUserID, ok := ctxmeta.GetAuthUserID(ctx); ok && authUserID != uuid.Nil {
		userID = &authUserID
	}

	// Persist user question turn with session_id
	userMsg := &models.ChatMessage{
		ID:                uuid.New(),
		RecordingID:       rec.ID,
		SessionID:         &sessionID,
		UserID:            userID,
		SenderRole:        constants.ChatRoleUser,
		Content:           trimmedMsg,
		Citations:         json.RawMessage("[]"),
		RetrievedChunkIDs: json.RawMessage("[]"),
		CreatedAt:         time.Now().UTC(),
	}
	if err := s.repo.SaveChatMessage(ctx, userMsg); err != nil {
		return nil, fmt.Errorf("failed to save user chat message: %w", err)
	}

	// Extract high-level meeting metadata from recording and active summary
	meta := extractRecordingMetadata(rec)
	if s.repo != nil {
		if activeSum, sumErr := s.repo.FindActiveSummaryByRecordingID(ctx, rec.ID); sumErr == nil && activeSum != nil {
			populateSummaryMetadata(meta, activeSum)
		}
	}
	if req.SummaryOverview != "" && meta.SummaryOverview == "" {
		meta.SummaryOverview = req.SummaryOverview
	}
	if len(req.ActionItems) > 0 && len(meta.ActionItems) == 0 {
		meta.ActionItems = req.ActionItems
	}

	// Vector semantic search for top-k relevant chunks
	var chunkData []dtos.TranscriptChunkData
	var retrievedChunkIDs []string

	if s.embedding != nil {
		embeddings, embErr := s.embedding.CreateEmbeddings(ctx, []string{trimmedMsg})
		if embErr == nil && len(embeddings) > 0 && len(embeddings[0]) > 0 {
			chunks, searchErr := s.repo.SearchSimilarTranscriptChunks(ctx, rec.ID, embeddings[0], 5)
			if searchErr == nil && len(chunks) > 0 {
				for _, c := range chunks {
					chunkData = append(chunkData, dtos.TranscriptChunkData{
						ChunkIndex: c.ChunkIndex,
						Content:    c.Content,
						StartTime:  c.StartTime,
						EndTime:    c.EndTime,
					})
					retrievedChunkIDs = append(retrievedChunkIDs, c.ID.String())
				}
			}
		}
	}

	// Prompt generation with RAG templates enriched with metadata
	systemPrompt, userPrompt := BuildRAGPrompt(trimmedMsg, chunkData, meta)

	var messages []dtos.ChatMessageInput
	if len(req.ConversationHistory) > 0 {
		messages = append(messages, req.ConversationHistory...)
	} else if len(priorMessages) > 0 {
		// Populate from persisted prior session turns (up to the last 8 messages)
		startIdx := 0
		if len(priorMessages) > 8 {
			startIdx = len(priorMessages) - 8
		}
		for _, pm := range priorMessages[startIdx:] {
			if strings.TrimSpace(pm.Content) == "" {
				continue
			}
			role := pm.SenderRole
			if role != constants.ChatRoleUser && role != constants.ChatRoleAssistant {
				role = constants.ChatRoleUser
			}
			messages = append(messages, dtos.ChatMessageInput{
				Role:    role,
				Content: pm.Content,
			})
		}
	}

	messages = append(messages, dtos.ChatMessageInput{
		Role:    constants.ChatRoleUser,
		Content: userPrompt,
	})

	temp := 0.3
	maxTokens := 2048
	topP := 0.95
	chatOpts := dtos.ChatOptions{
		Temperature: &temp,
		MaxTokens:   &maxTokens,
		TopP:        &topP,
	}

	streamCh, err := s.llm.StreamChatResponse(ctx, systemPrompt, messages, chatOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to stream chat completion: %w", err)
	}

	return &dtos.RecordingChatStreamResult{
		SessionID:         sessionID,
		StreamChannel:     streamCh,
		RetrievedChunkIDs: retrievedChunkIDs,
	}, nil
}

func extractRecordingMetadata(rec *models.Recording) *dtos.RecordingContextMetadata {
	if rec == nil {
		return nil
	}

	meta := &dtos.RecordingContextMetadata{
		Title:             rec.Title,
		DurationSeconds:   rec.DurationSeconds,
		DurationFormatted: timeutil.FormatTimestamp(rec.DurationSeconds),
		SelectedTemplate:  rec.SelectedTemplate,
		OutputLanguage:    rec.OutputLanguage,
	}
	if rec.DetectedLanguage != nil {
		meta.DetectedLanguage = *rec.DetectedLanguage
	}

	if rec.AnalyticsData != nil {
		if spkRaw, ok := rec.AnalyticsData["speakers"]; ok {
			meta.Speakers = parseSpeakersFromAnalytics(spkRaw)
		}
	}

	return meta
}

func parseSpeakersFromAnalytics(raw interface{}) []string {
	if raw == nil {
		return nil
	}

	var speakers []string
	seen := make(map[string]bool)

	switch list := raw.(type) {
	case []dtos.SpeakerAnalytics:
		for _, spk := range list {
			trimmed := strings.TrimSpace(spk.Name)
			if trimmed != "" && !seen[trimmed] {
				seen[trimmed] = true
				speakers = append(speakers, trimmed)
			}
		}
	case []interface{}:
		for _, item := range list {
			if m, ok := item.(map[string]interface{}); ok {
				if name, ok := m["name"].(string); ok {
					trimmed := strings.TrimSpace(name)
					if trimmed != "" && !seen[trimmed] {
						seen[trimmed] = true
						speakers = append(speakers, trimmed)
					}
				}
			}
		}
	}

	return speakers
}

func populateSummaryMetadata(meta *dtos.RecordingContextMetadata, summary *models.Summary) {
	if meta == nil || summary == nil {
		return
	}

	formatted := formatSummaryContent(summary)
	if formatted.Overview != "" {
		meta.SummaryOverview = formatted.Overview
	}
	if len(formatted.ActionItems) > 0 {
		meta.ActionItems = formatted.ActionItems
	}
	for _, sec := range formatted.Sections {
		if strings.TrimSpace(sec.Heading) != "" {
			meta.KeyTopics = append(meta.KeyTopics, sec.Heading)
		}
	}
}

// SaveAssistantChatMessage parses citations, maps the model entity, and persists the assistant response turn for the session.
func (s *Service) SaveAssistantChatMessage(
	ctx context.Context,
	recordingID uuid.UUID,
	sessionID uuid.UUID,
	content string,
	chunkIDs []string,
) (*models.ChatMessage, []string, error) {
	citations := ExtractCitations(content)
	if citations == nil {
		citations = []string{}
	}

	var userID *uuid.UUID
	if authUserID, ok := ctxmeta.GetAuthUserID(ctx); ok && authUserID != uuid.Nil {
		userID = &authUserID
	}

	var sessionIDPtr *uuid.UUID
	if sessionID != uuid.Nil {
		sessionIDPtr = &sessionID
	}

	asstMsg := &models.ChatMessage{
		ID:                uuid.New(),
		RecordingID:       recordingID,
		SessionID:         sessionIDPtr,
		UserID:            userID,
		SenderRole:        constants.ChatRoleAssistant,
		Content:           content,
		Citations:         jsonutil.RawStringSlice(citations),
		RetrievedChunkIDs: jsonutil.RawStringSlice(chunkIDs),
		CreatedAt:         time.Now().UTC(),
	}

	if err := s.repo.SaveChatMessage(ctx, asstMsg); err != nil {
		return nil, nil, fmt.Errorf("failed to persist assistant chat message: %w", err)
	}
	return asstMsg, citations, nil
}

// ListRecordingChatHistory retrieves chat messages for a specific conversation session on a recording.
func (s *Service) ListRecordingChatHistory(
	ctx context.Context,
	recordingID uuid.UUID,
	sessionID uuid.UUID,
	ownershipToken string,
	shareToken string,
) ([]models.ChatMessage, error) {
	rec, err := s.repo.FindRecordingByID(ctx, recordingID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrRecordingNotFound
		}
		return nil, fmt.Errorf("failed to lookup recording: %w", err)
	}

	policy := dtos.RecordingAccessPolicy{AllowShareToken: true}
	if !s.authorizeRecordingAccess(ctx, rec, ownershipToken, policy) {
		if shareToken == "" || !s.authorizeRecordingAccess(ctx, rec, shareToken, policy) {
			return nil, constants.ErrForbidden
		}
	}

	messages, err := s.repo.ListChatMessagesBySession(ctx, recordingID, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to list chat messages: %w", err)
	}
	if !s.isSessionAccessibleByCaller(ctx, rec, ownershipToken, messages) {
		return nil, constants.ErrForbidden
	}
	if messages == nil {
		messages = []models.ChatMessage{}
	}
	return messages, nil
}

// isSessionAccessibleByCaller enforces strict cross-user and owner-share boundary isolation on chat sessions.
func (s *Service) isSessionAccessibleByCaller(ctx context.Context, rec *models.Recording, ownershipToken string, messages []models.ChatMessage) bool {
	if len(messages) == 0 {
		return true
	}

	// Owner (authenticated user or matching guest ownership token) has full access
	if s.authorizeRecordingAccess(ctx, rec, ownershipToken, dtos.RecordingAccessPolicy{AllowShareToken: false}) {
		return true
	}

	callerUserID, hasCallerUser := ctxmeta.GetAuthUserID(ctx)
	for _, msg := range messages {
		if msg.UserID == nil {
			continue
		}
		// Prevent share visitors from accessing sessions created by the recording owner
		if rec.UserID != nil && *msg.UserID == *rec.UserID {
			return false
		}
		// Prevent cross-user session access among logged-in share viewers
		if !hasCallerUser || *msg.UserID != callerUserID {
			return false
		}
	}

	return true
}

