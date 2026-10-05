package services

import (
	"context"
	"math"
	"strings"

	"github.com/google/uuid"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/pkg/ctxmeta"
)

// SearchWorkspaceSemantic performs cross-meeting semantic search across all recordings owned by the authenticated user.
func (s *Service) SearchWorkspaceSemantic(ctx context.Context, query dtos.SemanticSearchQuery) (*dtos.SemanticSearchResponse, error) {
	authUser, ok := ctxmeta.GetAuthUser(ctx)
	if !ok || authUser.UserID == uuid.Nil {
		return nil, constants.ErrUnauthorized
	}

	trimmedQuery := strings.TrimSpace(query.Q)

	threshold := query.Threshold
	if threshold < 0 {
		threshold = 0
	} else if threshold > 1 {
		threshold = 1
	}

	embeddings, err := s.embedding.CreateEmbeddings(ctx, []string{trimmedQuery})
	if err != nil {
		return nil, constants.ErrInternalServerError.Wrap(err)
	}
	if len(embeddings) == 0 || len(embeddings[0]) == 0 {
		return &dtos.SemanticSearchResponse{
			Query:   trimmedQuery,
			Count:   0,
			Results: []dtos.SearchResultItem{},
		}, nil
	}

	var maxDistance float64
	if threshold > 0 {
		maxDistance = math.Round((1.0-threshold)*10000) / 10000
	}

	matches, err := s.repo.SearchWorkspaceTranscriptChunks(ctx, authUser.UserID, embeddings[0], query.Limit, maxDistance)
	if err != nil {
		return nil, constants.ErrInternalServerError.Wrap(err)
	}

	results := make([]dtos.SearchResultItem, 0, len(matches))
	for _, m := range matches {
		score := 1.0 - m.Distance
		if score < 0 {
			score = 0
		} else if score > 1 {
			score = 1
		}
		// Round score to 4 decimal places for clean representation
		score = math.Round(score*10000) / 10000

		results = append(results, dtos.SearchResultItem{
			RecordingID:    m.RecordingID,
			RecordingTitle: m.RecordingTitle,
			ChunkIndex:     m.ChunkIndex,
			Snippet:        m.Content,
			StartTime:      m.StartTime,
			EndTime:        m.EndTime,
			Score:          score,
		})
	}

	return &dtos.SemanticSearchResponse{
		Query:   trimmedQuery,
		Count:   len(results),
		Results: results,
	}, nil
}
