// recording_comments_service.go — inline comment create/list/delete use-cases.
package services

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
	"code-base-golang/internal/pkg/ctxmeta"
)

// CreateInlineComment creates a timestamped inline comment or reply for a recording.
func (s *Service) CreateInlineComment(ctx context.Context, id uuid.UUID, ownershipToken string, req dtos.CreateCommentRequest) (*dtos.CommentResponse, error) {
	rec, err := s.repo.FindRecordingByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrRecordingNotFound
		}
		return nil, fmt.Errorf("failed to lookup recording: %w", err)
	}

	// Verify access: owner, ownership token, or publicly shared recording.
	isAuth := ctxmeta.IsAuthenticated(ctx)
	userID, hasUserID := ctxmeta.GetAuthUserID(ctx)
	if !s.authorizeRecordingAccess(ctx, rec, ownershipToken, recordingAccessPolicy{allowShareOpen: true}) {
		return nil, constants.ErrForbidden
	}

	// Resolve author name
	authorName := strings.TrimSpace(req.AuthorName)
	if authorName == "" {
		if isAuth {
			if u, uErr := s.repo.FindUserByID(ctx, userID); uErr == nil && u != nil && strings.TrimSpace(u.FullName) != "" {
				authorName = strings.TrimSpace(u.FullName)
			}
		}
	}
	if authorName == "" {
		authorName = "Anonymous"
	}

	// If replying to a parent comment, verify the parent comment exists and belongs to this recording
	if req.ParentID != nil {
		parent, err := s.repo.FindInlineCommentByID(ctx, *req.ParentID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, constants.ErrNotFound
			}
			return nil, fmt.Errorf("failed to find parent comment: %w", err)
		}
		if parent.RecordingID != rec.ID {
			return nil, constants.ErrBadRequest
		}
	}

	var commentUserID *uuid.UUID
	if isAuth && hasUserID {
		commentUserID = &userID
	}

	comment := models.InlineComment{
		ID:           uuid.New(),
		RecordingID:  rec.ID,
		UserID:       commentUserID,
		SegmentID:    req.SegmentID,
		TimestampSec: req.TimestampSec,
		SelectedText: req.SelectedText,
		AuthorName:   authorName,
		CommentText:  strings.TrimSpace(req.CommentText),
		ParentID:     req.ParentID,
	}

	if err := s.repo.CreateInlineComment(ctx, &comment); err != nil {
		return nil, fmt.Errorf("failed to create inline comment: %w", err)
	}

	resp := toCommentResponse(comment)
	return &resp, nil
}

// ListInlineComments retrieves all top-level inline comments with nested replies for a recording.
func (s *Service) ListInlineComments(ctx context.Context, id uuid.UUID, ownershipToken string) ([]dtos.CommentResponse, error) {
	rec, err := s.repo.FindRecordingByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrRecordingNotFound
		}
		return nil, fmt.Errorf("failed to lookup recording: %w", err)
	}

	// Verify access: owner, ownership token, or publicly shared recording.
	if !s.authorizeRecordingAccess(ctx, rec, ownershipToken, recordingAccessPolicy{allowShareOpen: true}) {
		return nil, constants.ErrForbidden
	}

	comments, err := s.repo.ListInlineCommentsByRecordingID(ctx, rec.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to list inline comments: %w", err)
	}

	res := make([]dtos.CommentResponse, 0, len(comments))
	for _, c := range comments {
		res = append(res, toCommentResponse(c))
	}

	return res, nil
}

// DeleteInlineComment removes an inline comment if requested by the recording owner or comment author.
func (s *Service) DeleteInlineComment(ctx context.Context, recordingID uuid.UUID, commentID uuid.UUID, ownershipToken string) error {
	rec, err := s.repo.FindRecordingByID(ctx, recordingID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return constants.ErrRecordingNotFound
		}
		return fmt.Errorf("failed to lookup recording: %w", err)
	}

	comment, err := s.repo.FindInlineCommentByID(ctx, commentID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return constants.ErrNotFound
		}
		return fmt.Errorf("failed to find comment: %w", err)
	}

	if comment.RecordingID != rec.ID {
		return constants.ErrBadRequest
	}

	// Verify permission: recording owner (or ownership token), or the comment author.
	isAuth := ctxmeta.IsAuthenticated(ctx)
	userID, hasUserID := ctxmeta.GetAuthUserID(ctx)

	canDelete := s.authorizeRecordingAccess(ctx, rec, ownershipToken, recordingAccessPolicy{})
	if !canDelete && isAuth && hasUserID && comment.UserID != nil && *comment.UserID == userID {
		canDelete = true
	}

	if !canDelete {
		return constants.ErrForbidden
	}

	if err := s.repo.DeleteInlineComment(ctx, comment.ID); err != nil {
		return fmt.Errorf("failed to delete inline comment: %w", err)
	}

	return nil
}
