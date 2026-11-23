package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
	"code-base-golang/internal/pkg/ctxmeta"
	"code-base-golang/internal/pkg/timeutil"
	"code-base-golang/internal/repositories"
	"code-base-golang/internal/templates"
)

func formatWorkspaceCatalogItems(catalog []repositories.WorkspaceCatalogRecord) []templates.WorkspaceMeetingCatalogItem {
	items := make([]templates.WorkspaceMeetingCatalogItem, 0, len(catalog))
	for _, c := range catalog {
		dateStr := c.CreatedAt.Format("02 Jan 2006")
		durStr := timeutil.FormatTimestamp(c.DurationSeconds)
		cleanOverview := strings.TrimSpace(c.Overview)
		runes := []rune(cleanOverview)
		if len(runes) > 300 {
			cleanOverview = string(runes[:300]) + "..."
		}
		items = append(items, templates.WorkspaceMeetingCatalogItem{
			RecordingID:        c.RecordingID.String(),
			Title:              c.Title,
			FolderName:         c.FolderName,
			DurationFormatted:  durStr,
			CreatedAtFormatted: dateStr,
			Overview:           cleanOverview,
		})
	}
	return items
}

// BuildWorkspaceRAGPrompt constructs system and user prompts for cross-meeting Q&A over multiple meeting recordings.
func BuildWorkspaceRAGPrompt(query string, matches []repositories.WorkspaceChunkMatch, catalog ...templates.WorkspaceMeetingCatalogItem) (systemPrompt string, userPrompt string) {
	views := make([]templates.WorkspaceChunkView, len(matches))
	for i, m := range matches {
		views[i] = templates.WorkspaceChunkView{
			ChunkIndex:     m.ChunkIndex,
			Content:        m.Content,
			RecordingTitle: m.RecordingTitle,
			StartTime:      m.StartTime,
			EndTime:        m.EndTime,
		}
	}

	sys, user, err := templates.RenderWorkspaceRAGPrompts(query, views, catalog...)
	if err != nil {
		return "You are a helpful, accurate AI assistant for workspace meeting memory. Strict Grounding Rules apply.", "Question: " + query
	}
	return sys, user
}

// AskWorkspaceMemory performs cross-meeting RAG synthesis over all meetings owned by the user.
func (s *Service) AskWorkspaceMemory(ctx context.Context, req dtos.WorkspaceAskRequest) (*dtos.WorkspaceAskResponse, error) {
	authUser, ok := ctxmeta.GetAuthUser(ctx)
	if !ok || authUser.UserID == uuid.Nil {
		return nil, constants.ErrUnauthorized
	}

	trimmedQ := strings.TrimSpace(req.Question)

	embeddings, err := s.embedding.CreateEmbeddings(ctx, []string{trimmedQ})
	if err != nil {
		return nil, constants.ErrInternalServerError.Wrap(err)
	}

	var matches []repositories.WorkspaceChunkMatch
	if len(embeddings) > 0 && len(embeddings[0]) > 0 {
		var (
			m         []repositories.WorkspaceChunkMatch
			searchErr error
		)
		if activeWsID, ok := ctxmeta.GetWorkspaceID(ctx); ok && activeWsID != uuid.Nil {
			if _, err := s.repo.FindWorkspaceMember(ctx, activeWsID, authUser.UserID); err == nil {
				m, searchErr = s.repo.SearchWorkspaceTranscriptChunksByWorkspace(ctx, activeWsID, embeddings[0], constants.DefaultWorkspaceChatTopK, constants.DefaultWorkspaceChatMaxDistance)
			} else {
				m, searchErr = s.repo.SearchWorkspaceTranscriptChunks(ctx, authUser.UserID, embeddings[0], constants.DefaultWorkspaceChatTopK, constants.DefaultWorkspaceChatMaxDistance)
			}
		} else {
			m, searchErr = s.repo.SearchWorkspaceTranscriptChunks(ctx, authUser.UserID, embeddings[0], constants.DefaultWorkspaceChatTopK, constants.DefaultWorkspaceChatMaxDistance)
		}
		if searchErr != nil {
			return nil, constants.ErrInternalServerError.Wrap(searchErr)
		}
		matches = m
	}

	sources := make([]dtos.MeetingSourceCitation, 0, len(matches))
	for _, m := range matches {
		sources = append(sources, dtos.MeetingSourceCitation{
			RecordingID:    m.RecordingID,
			RecordingTitle: m.RecordingTitle,
			FolderID:       m.FolderID,
			FolderName:     m.FolderName,
			ChunkIndex:     m.ChunkIndex,
			Snippet:        m.Content,
			StartTime:      m.StartTime,
			EndTime:        m.EndTime,
		})
	}

	var catalogItems []templates.WorkspaceMeetingCatalogItem
	if activeWsID, ok := ctxmeta.GetWorkspaceID(ctx); ok && activeWsID != uuid.Nil {
		if rawCatalog, err := s.repo.GetWorkspaceMeetingCatalog(ctx, activeWsID, nil, constants.DefaultWorkspaceMeetingCatalogLimit); err == nil {
			catalogItems = formatWorkspaceCatalogItems(rawCatalog)
		}
	}

	systemPrompt, userPrompt := BuildWorkspaceRAGPrompt(trimmedQ, matches, catalogItems...)

	messages := make([]dtos.ChatMessageInput, 0, len(req.History)+1)
	if len(req.History) > 0 {
		messages = append(messages, req.History...)
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

	chatResp, err := s.llm.GenerateChatResponse(ctx, systemPrompt, messages, chatOpts)
	if err != nil {
		return nil, constants.ErrInternalServerError.Wrap(err)
	}

	return &dtos.WorkspaceAskResponse{
		Answer:  chatResp.Content,
		Sources: sources,
	}, nil
}

// CreateWorkspaceChatSession initializes a new multi-meeting chat session for a user.
func (s *Service) CreateWorkspaceChatSession(ctx context.Context, workspaceID uuid.UUID, req dtos.CreateWorkspaceChatSessionRequest) (*dtos.WorkspaceChatSessionResponse, error) {
	userID, ok := ctxmeta.GetAuthUserID(ctx)
	if !ok {
		return nil, constants.ErrUnauthorized
	}

	if _, err := s.repo.FindWorkspaceMember(ctx, workspaceID, userID); err != nil {
		return nil, constants.ErrWorkspaceAccessDenied
	}

	var folderName string
	if req.FolderID != nil && *req.FolderID != uuid.Nil {
		folder, err := s.repo.FindFolderByID(ctx, *req.FolderID)
		if err != nil || folder.WorkspaceID != workspaceID {
			return nil, constants.ErrFolderNotFound
		}
		folderName = folder.Name
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = "New Chat"
	}

	var targetFolderID *uuid.UUID
	if req.FolderID != nil && *req.FolderID != uuid.Nil {
		targetFolderID = req.FolderID
	}

	session := &models.WorkspaceChatSession{
		WorkspaceID: workspaceID,
		FolderID:    targetFolderID,
		UserID:      userID,
		Title:       title,
	}

	if err := s.repo.CreateWorkspaceChatSession(ctx, session); err != nil {
		return nil, constants.ErrInternalServerError.Wrap(err)
	}

	return &dtos.WorkspaceChatSessionResponse{
		ID:           session.ID,
		WorkspaceID:  session.WorkspaceID,
		FolderID:     session.FolderID,
		FolderName:   folderName,
		UserID:       session.UserID,
		Title:        session.Title,
		CreatedAt:    session.CreatedAt,
		UpdatedAt:    session.UpdatedAt,
		MessageCount: 0,
	}, nil
}

// ListWorkspaceChatSessions lists all past conversation sessions for a user in the workspace.
func (s *Service) ListWorkspaceChatSessions(ctx context.Context, workspaceID uuid.UUID, folderID *uuid.UUID) ([]dtos.WorkspaceChatSessionResponse, error) {
	userID, ok := ctxmeta.GetAuthUserID(ctx)
	if !ok {
		return nil, constants.ErrUnauthorized
	}

	if _, err := s.repo.FindWorkspaceMember(ctx, workspaceID, userID); err != nil {
		return nil, constants.ErrWorkspaceAccessDenied
	}

	sessions, err := s.repo.FindWorkspaceChatSessions(ctx, workspaceID, userID, folderID)
	if err != nil {
		return nil, constants.ErrInternalServerError.Wrap(err)
	}

	sessionIDs := make([]uuid.UUID, len(sessions))
	for i, sess := range sessions {
		sessionIDs[i] = sess.ID
	}
	counts, err := s.repo.CountWorkspaceChatMessagesBySessions(ctx, sessionIDs)
	if err != nil {
		return nil, constants.ErrInternalServerError.Wrap(err)
	}

	res := make([]dtos.WorkspaceChatSessionResponse, 0, len(sessions))
	for _, sess := range sessions {
		fName := ""
		if sess.Folder != nil {
			fName = sess.Folder.Name
		}
		res = append(res, dtos.WorkspaceChatSessionResponse{
			ID:           sess.ID,
			WorkspaceID:  sess.WorkspaceID,
			FolderID:     sess.FolderID,
			FolderName:   fName,
			UserID:       sess.UserID,
			Title:        sess.Title,
			CreatedAt:    sess.CreatedAt,
			UpdatedAt:    sess.UpdatedAt,
			MessageCount: int(counts[sess.ID]),
		})
	}
	return res, nil
}

// GetWorkspaceChatSessionHistory retrieves conversation messages for a specific session.
func (s *Service) GetWorkspaceChatSessionHistory(ctx context.Context, workspaceID, sessionID uuid.UUID) ([]dtos.WorkspaceChatMessageResponse, error) {
	userID, ok := ctxmeta.GetAuthUserID(ctx)
	if !ok {
		return nil, constants.ErrUnauthorized
	}

	sess, err := s.repo.FindWorkspaceChatSessionByID(ctx, sessionID)
	if err != nil || sess.WorkspaceID != workspaceID {
		return nil, constants.ErrSessionNotFound
	}

	if sess.UserID != userID {
		// Verify caller is at least a member of the workspace
		if _, err := s.repo.FindWorkspaceMember(ctx, workspaceID, userID); err != nil {
			return nil, constants.ErrWorkspaceAccessDenied
		}
	}

	msgs, err := s.repo.FindWorkspaceChatMessagesBySession(ctx, sessionID)
	if err != nil {
		return nil, constants.ErrInternalServerError.Wrap(err)
	}

	res := make([]dtos.WorkspaceChatMessageResponse, 0, len(msgs))
	for _, m := range msgs {
		var citations []dtos.MeetingSourceCitation
		if len(m.Citations) > 0 && string(m.Citations) != "[]" {
			if err := json.Unmarshal(m.Citations, &citations); err != nil {
				return nil, constants.ErrInternalServerError.Wrap(err)
			}
		}
		res = append(res, dtos.WorkspaceChatMessageResponse{
			ID:        m.ID,
			SessionID: m.SessionID,
			Role:      m.SenderRole,
			Content:   m.Content,
			Citations: citations,
			CreatedAt: m.CreatedAt,
		})
	}
	return res, nil
}

// DeleteWorkspaceChatSession deletes a session owned by the caller.
func (s *Service) DeleteWorkspaceChatSession(ctx context.Context, workspaceID, sessionID uuid.UUID) error {
	userID, ok := ctxmeta.GetAuthUserID(ctx)
	if !ok {
		return constants.ErrUnauthorized
	}

	sess, err := s.repo.FindWorkspaceChatSessionByID(ctx, sessionID)
	if err != nil || sess.WorkspaceID != workspaceID {
		return constants.ErrSessionNotFound
	}

	if sess.UserID != userID {
		// If not owner of session, check if workspace owner or admin
		member, err := s.repo.FindWorkspaceMember(ctx, workspaceID, userID)
		if err != nil || (member.Role != constants.WorkspaceRoleOwner && member.Role != constants.WorkspaceRoleAdmin) {
			return constants.ErrWorkspaceAccessDenied
		}
	}

	return s.repo.DeleteWorkspaceChatSession(ctx, sessionID)
}

// InitiateWorkspaceChatStream prepares RAG retrieval and starts LLM typewriter streaming.
func (s *Service) InitiateWorkspaceChatStream(
	ctx context.Context,
	workspaceID uuid.UUID,
	req dtos.StreamWorkspaceChatRequest,
) (*dtos.WorkspaceChatStreamResult, error) {
	userID, ok := ctxmeta.GetAuthUserID(ctx)
	if !ok {
		return nil, constants.ErrUnauthorized
	}

	if _, err := s.repo.FindWorkspaceMember(ctx, workspaceID, userID); err != nil {
		return nil, constants.ErrWorkspaceAccessDenied
	}

	var session *models.WorkspaceChatSession
	if req.SessionID != nil && *req.SessionID != uuid.Nil {
		sess, err := s.repo.FindWorkspaceChatSessionByID(ctx, *req.SessionID)
		if err != nil || sess.WorkspaceID != workspaceID {
			return nil, constants.ErrSessionNotFound
		}
		session = sess
	} else {
		// Auto-provision session with title based on first query
		title := strings.TrimSpace(req.Question)
		runes := []rune(title)
		if len(runes) > 40 {
			title = string(runes[:40]) + "..."
		}
		if title == "" {
			title = "New Chat"
		}
		var targetFolderID *uuid.UUID
		if req.FolderID != nil && *req.FolderID != uuid.Nil {
			targetFolderID = req.FolderID
		}
		session = &models.WorkspaceChatSession{
			WorkspaceID: workspaceID,
			FolderID:    targetFolderID,
			UserID:      userID,
			Title:       title,
		}
		if err := s.repo.CreateWorkspaceChatSession(ctx, session); err != nil {
			return nil, constants.ErrInternalServerError.Wrap(err)
		}
	}

	// 1. Persist User Message
	userMsg := &models.WorkspaceChatMessage{
		SessionID:  session.ID,
		SenderRole: constants.ChatRoleUser,
		Content:    req.Question,
	}
	if err := s.repo.CreateWorkspaceChatMessage(ctx, userMsg); err != nil {
		return nil, constants.ErrInternalServerError.Wrap(err)
	}

	// 2. Compute Embeddings & Perform Vector RAG Retrieval
	trimmedQ := strings.TrimSpace(req.Question)
	embeddings, err := s.embedding.CreateEmbeddings(ctx, []string{trimmedQ})
	if err != nil {
		return nil, constants.ErrInternalServerError.Wrap(err)
	}

	targetFolderID := req.FolderID
	if targetFolderID == nil && session.FolderID != nil {
		targetFolderID = session.FolderID
	}

	var matches []repositories.WorkspaceChunkMatch
	if len(embeddings) > 0 && len(embeddings[0]) > 0 {
		var (
			m         []repositories.WorkspaceChunkMatch
			searchErr error
		)

		if targetFolderID != nil && *targetFolderID != uuid.Nil {
			// Recursive folder retrieval
			m, searchErr = s.repo.SearchWorkspaceTranscriptChunksByFolderRecursive(ctx, workspaceID, *targetFolderID, embeddings[0], constants.DefaultWorkspaceChatTopK, constants.DefaultWorkspaceChatMaxDistance)
		} else {
			// Entire workspace retrieval
			m, searchErr = s.repo.SearchWorkspaceTranscriptChunksByWorkspace(ctx, workspaceID, embeddings[0], constants.DefaultWorkspaceChatTopK, constants.DefaultWorkspaceChatMaxDistance)
		}
		if searchErr != nil {
			return nil, constants.ErrInternalServerError.Wrap(searchErr)
		}
		matches = m
	}

	sources := make([]dtos.MeetingSourceCitation, 0, len(matches))
	retrievedChunkIDs := make([]uuid.UUID, 0, len(matches))
	for _, m := range matches {
		sources = append(sources, dtos.MeetingSourceCitation{
			RecordingID:    m.RecordingID,
			RecordingTitle: m.RecordingTitle,
			FolderID:       m.FolderID,
			FolderName:     m.FolderName,
			ChunkIndex:     m.ChunkIndex,
			Snippet:        m.Content,
			StartTime:      m.StartTime,
			EndTime:        m.EndTime,
		})
		retrievedChunkIDs = append(retrievedChunkIDs, m.ID)
	}

	var catalogItems []templates.WorkspaceMeetingCatalogItem
	if rawCatalog, err := s.repo.GetWorkspaceMeetingCatalog(ctx, workspaceID, targetFolderID, constants.DefaultWorkspaceMeetingCatalogLimit); err == nil {
		catalogItems = formatWorkspaceCatalogItems(rawCatalog)
	}

	systemPrompt, userPrompt := BuildWorkspaceRAGPrompt(trimmedQ, matches, catalogItems...)

	// Fetch past session history for multi-turn dialogue context
	pastMessages, _ := s.repo.FindWorkspaceChatMessagesBySession(ctx, session.ID)
	messages := make([]dtos.ChatMessageInput, 0, len(pastMessages)+1)
	for _, pm := range pastMessages {
		if pm.ID == userMsg.ID {
			continue // skip the just-created message as it's added below with userPrompt
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
		return nil, fmt.Errorf("failed to stream workspace chat: %w", err)
	}

	return &dtos.WorkspaceChatStreamResult{
		SessionID:         session.ID,
		StreamChannel:     streamCh,
		Sources:           sources,
		RetrievedChunkIDs: retrievedChunkIDs,
	}, nil
}

// SaveAssistantWorkspaceChatMessage persists the completed assistant response with citations.
func (s *Service) SaveAssistantWorkspaceChatMessage(
	ctx context.Context,
	workspaceID uuid.UUID,
	sessionID uuid.UUID,
	content string,
	citations []dtos.MeetingSourceCitation,
	retrievedChunkIDs []uuid.UUID,
) (*models.WorkspaceChatMessage, error) {
	sess, err := s.repo.FindWorkspaceChatSessionByID(ctx, sessionID)
	if err != nil || sess.WorkspaceID != workspaceID {
		return nil, constants.ErrSessionNotFound
	}

	citationsBytes, _ := json.Marshal(citations)
	chunksBytes, _ := json.Marshal(retrievedChunkIDs)

	msg := &models.WorkspaceChatMessage{
		SessionID:         sessionID,
		SenderRole:        constants.ChatRoleAssistant,
		Content:           content,
		Citations:         json.RawMessage(citationsBytes),
		RetrievedChunkIDs: json.RawMessage(chunksBytes),
	}

	if err := s.repo.CreateWorkspaceChatMessage(ctx, msg); err != nil {
		return nil, err
	}

	// Update session updated_at timestamp
	_ = s.repo.UpdateWorkspaceChatSessionTitle(ctx, sessionID, "")

	return msg, nil
}
