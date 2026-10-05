package services

import (
	"context"
	"math"

	"github.com/google/uuid"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/pkg/ctxmeta"
)

// GetWorkspaceSpeakers aggregates distinct speaker participation statistics across all recordings owned by the user.
func (s *Service) GetWorkspaceSpeakers(ctx context.Context) (*dtos.SpeakerDirectoryResponse, error) {
	authUser, ok := ctxmeta.GetAuthUser(ctx)
	if !ok || authUser.UserID == uuid.Nil {
		return nil, constants.ErrUnauthorized
	}

	stats, err := s.repo.AggregateWorkspaceSpeakers(ctx, authUser.UserID)
	if err != nil {
		return nil, constants.ErrInternalServerError.Wrap(err)
	}

	speakers := make([]dtos.SpeakerSummary, 0, len(stats))
	for _, stat := range stats {
		talkTime := math.Round(stat.TotalTalkTime*100) / 100
		speakers = append(speakers, dtos.SpeakerSummary{
			Name:          stat.Name,
			TotalMeetings: stat.TotalMeetings,
			TotalTalkTime: talkTime,
			LastActive:    stat.LastActive,
		})
	}

	return &dtos.SpeakerDirectoryResponse{
		Count:    len(speakers),
		Speakers: speakers,
	}, nil
}
