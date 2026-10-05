package services

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/pkg/ctxmeta"
	"code-base-golang/internal/repositories"
	"code-base-golang/internal/templates"
)

// BuildWorkspaceRAGPrompt constructs system and user prompts for cross-meeting Q&A over multiple meeting recordings.
func BuildWorkspaceRAGPrompt(query string, matches []repositories.WorkspaceChunkMatch) (systemPrompt string, userPrompt string) {
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

	sys, user, err := templates.RenderWorkspaceRAGPrompts(query, views)
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
		m, searchErr := s.repo.SearchWorkspaceTranscriptChunks(ctx, authUser.UserID, embeddings[0], 5, 0.45)
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
			ChunkIndex:     m.ChunkIndex,
			Snippet:        m.Content,
			StartTime:      m.StartTime,
			EndTime:        m.EndTime,
		})
	}

	systemPrompt, userPrompt := BuildWorkspaceRAGPrompt(trimmedQ, matches)

	messages := make([]dtos.ChatMessageInput, 0, len(req.History)+1)
	if len(req.History) > 0 {
		messages = append(messages, req.History...)
	}
	messages = append(messages, dtos.ChatMessageInput{
		Role:    constants.ChatRoleUser,
		Content: userPrompt,
	})

	chatResp, err := s.llm.GenerateChatResponse(ctx, systemPrompt, messages, dtos.ChatOptions{})
	if err != nil {
		return nil, constants.ErrInternalServerError.Wrap(err)
	}

	return &dtos.WorkspaceAskResponse{
		Answer:  chatResp.Content,
		Sources: sources,
	}, nil
}
