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
)

// InitiateRecordingChatStream validates access (owner, guest, or share token), retrieves relevant context chunks,
// persists the user turn with session_id, and initiates an LLM response stream.
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

	// Resolve conversation session ID: use client-provided session ID, or generate a fresh unique UUID
	var sessionID uuid.UUID
	if req.SessionID != nil && *req.SessionID != uuid.Nil {
		sessionID = *req.SessionID
		// Prevent share visitors from appending to sessions owned by another user or recording owner
		existingMsgs, _ := s.repo.ListChatMessagesBySession(ctx, rec.ID, sessionID)
		if !s.isSessionAccessibleByCaller(ctx, rec, ownershipToken, existingMsgs) {
			return nil, constants.ErrForbidden
		}
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

	// Prompt generation with RAG templates (gracefully handles empty context)
	systemPrompt, userPrompt := BuildRAGPrompt(trimmedMsg, chunkData)

	var messages []dtos.ChatMessageInput
	if len(req.ConversationHistory) > 0 {
		messages = append(messages, req.ConversationHistory...)
	}
	messages = append(messages, dtos.ChatMessageInput{
		Role:    constants.ChatRoleUser,
		Content: userPrompt,
	})

	streamCh, err := s.llm.StreamChatResponse(ctx, systemPrompt, messages, dtos.ChatOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to stream chat completion: %w", err)
	}

	return &dtos.RecordingChatStreamResult{
		SessionID:         sessionID,
		StreamChannel:     streamCh,
		RetrievedChunkIDs: retrievedChunkIDs,
	}, nil
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

