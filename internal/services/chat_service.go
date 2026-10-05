package services

import (
	"context"
	"crypto/subtle"
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
)

// RecordingChatStreamResult carries the stream channel and completion callback.
type RecordingChatStreamResult struct {
	StreamChannel     <-chan dtos.StreamChunk
	RetrievedChunkIDs []string
	SaveAssistantMsg  func(ctx context.Context, fullContent string) (*models.ChatMessage, []string, error)
}

// InitiateRecordingChatStream validates ownership, retrieves relevant context chunks via vector search,
// persists the user turn, and initiates an LLM response stream.
func (s *Service) InitiateRecordingChatStream(
	ctx context.Context,
	recordingID uuid.UUID,
	ownershipToken string,
	req dtos.RecordingChatRequest,
) (*RecordingChatStreamResult, error) {
	rec, err := s.repo.FindRecordingByID(ctx, recordingID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrRecordingNotFound
		}
		return nil, fmt.Errorf("failed to lookup recording: %w", err)
	}

	// Owner verification: only the owner (authenticated or guest with ownership token) may chat
	hasAccess := false
	isAuth := ctxmeta.IsAuthenticated(ctx)
	userID, hasUserID := ctxmeta.GetAuthUserID(ctx)

	if isAuth && hasUserID && rec.UserID != nil && *rec.UserID == userID {
		hasAccess = true
	} else if ownershipToken != "" && subtle.ConstantTimeCompare([]byte(ownershipToken), []byte(rec.OwnershipToken)) == 1 {
		hasAccess = true
	}

	if !hasAccess {
		return nil, constants.ErrForbidden
	}

	trimmedMsg := strings.TrimSpace(req.Message)

	// Persist user question turn
	userMsg := &models.ChatMessage{
		ID:                uuid.New(),
		RecordingID:       rec.ID,
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

	saveAssistantMsg := func(saveCtx context.Context, fullContent string) (*models.ChatMessage, []string, error) {
		return s.saveAssistantChatMessage(saveCtx, rec.ID, fullContent, retrievedChunkIDs)
	}

	return &RecordingChatStreamResult{
		StreamChannel:     streamCh,
		RetrievedChunkIDs: retrievedChunkIDs,
		SaveAssistantMsg:  saveAssistantMsg,
	}, nil
}

// saveAssistantChatMessage parses citations, maps the model entity, and persists the assistant response turn.
func (s *Service) saveAssistantChatMessage(
	ctx context.Context,
	recordingID uuid.UUID,
	content string,
	chunkIDs []string,
) (*models.ChatMessage, []string, error) {
	citations := ExtractCitations(content)
	if citations == nil {
		citations = []string{}
	}

	asstMsg := &models.ChatMessage{
		ID:                uuid.New(),
		RecordingID:       recordingID,
		SenderRole:        constants.ChatRoleAssistant,
		Content:           content,
		Citations:         rawStringSliceJSON(citations),
		RetrievedChunkIDs: rawStringSliceJSON(chunkIDs),
		CreatedAt:         time.Now().UTC(),
	}

	if err := s.repo.SaveChatMessage(ctx, asstMsg); err != nil {
		return nil, nil, fmt.Errorf("failed to persist assistant chat message: %w", err)
	}
	return asstMsg, citations, nil
}

// rawStringSliceJSON encodes a slice of strings to json.RawMessage, safely defaulting to "[]" without allocation on empty slices.
func rawStringSliceJSON(items []string) json.RawMessage {
	if len(items) == 0 {
		return json.RawMessage("[]")
	}
	b, err := json.Marshal(items)
	if err != nil {
		return json.RawMessage("[]")
	}
	return json.RawMessage(b)
}
