package services

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
	"code-base-golang/internal/pkg/ctxmeta"
)

// SubmitFeedback handles submission of user feedback with idempotency and user identity propagation.
func (s *Service) SubmitFeedback(ctx context.Context, req dtos.FeedbackRequest) (*dtos.FeedbackResponse, error) {
	req.SetDefaults()

	// 1. Idempotency Check: if idempotency key is provided, check if already recorded
	if req.IdempotencyKey != nil && *req.IdempotencyKey != "" {
		existing, err := s.repo.FindFeedbackByIdempotencyKey(ctx, *req.IdempotencyKey)
		if err != nil {
			return nil, constants.ErrInternalServerError.Wrap(err)
		}
		if existing != nil {
			// Return cached / existing response without re-inserting
			return mapFeedbackToResponse(existing), nil
		}
	}

	// 2. Extract client metadata
	clientIP := ctxmeta.GetClientIP(ctx)
	userAgent := ctxmeta.GetUserAgent(ctx)

	var ptrClientIP *string
	if strings.TrimSpace(clientIP) != "" {
		trimmed := strings.TrimSpace(clientIP)
		ptrClientIP = &trimmed
	}

	var ptrUserAgent *string
	if strings.TrimSpace(userAgent) != "" {
		trimmed := strings.TrimSpace(userAgent)
		ptrUserAgent = &trimmed
	}

	// 3. User association if authenticated
	var userID *uuid.UUID
	userName := req.UserName
	userEmail := req.UserEmail

	if authUser, ok := ctxmeta.GetAuthUser(ctx); ok && !authUser.IsGuest && authUser.UserID != uuid.Nil {
		uid := authUser.UserID
		userID = &uid

		if userEmail == nil || *userEmail == "" {
			if authUser.Email != "" {
				userEmail = &authUser.Email
			}
		}
	}

	// 4. Construct entity
	metadata := make(models.JSONMap)
	for k, v := range req.Metadata {
		metadata[k] = v
	}

	feedback := &models.Feedback{
		UserID:         userID,
		UserName:       userName,
		UserEmail:      userEmail,
		Title:          req.Title,
		Category:       req.Category,
		Rating:         req.Rating,
		Message:        req.Message,
		Status:         constants.FeedbackStatusNew,
		ClientIP:       ptrClientIP,
		UserAgent:      ptrUserAgent,
		IdempotencyKey: req.IdempotencyKey,
		Metadata:       metadata,
	}

	// 5. Persist to database
	saved, err := s.repo.CreateFeedback(ctx, feedback)
	if err != nil {
		// Handle potential race condition on unique index uidx_feedbacks_idempotency_key
		if req.IdempotencyKey != nil && *req.IdempotencyKey != "" {
			if existing, findErr := s.repo.FindFeedbackByIdempotencyKey(ctx, *req.IdempotencyKey); findErr == nil && existing != nil {
				return mapFeedbackToResponse(existing), nil
			}
		}
		return nil, constants.ErrInternalServerError.Wrap(err)
	}

	return mapFeedbackToResponse(saved), nil
}

func mapFeedbackToResponse(f *models.Feedback) *dtos.FeedbackResponse {
	var metadata map[string]interface{}
	if len(f.Metadata) > 0 {
		metadata = f.Metadata
	}

	return &dtos.FeedbackResponse{
		ID:             f.ID,
		Title:          f.Title,
		Category:       f.Category,
		Rating:         f.Rating,
		Message:        f.Message,
		Status:         f.Status,
		UserName:       f.UserName,
		UserEmail:      f.UserEmail,
		IdempotencyKey: f.IdempotencyKey,
		Metadata:       metadata,
		CreatedAt:      f.CreatedAt,
	}
}
