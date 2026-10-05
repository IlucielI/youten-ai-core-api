// recording_sharing_service.go — deletion, ownership claim, and public share toggle use-cases.
package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/pkg/ctxmeta"
)

// DeleteRecording handles soft-deleting a recording owned by the authenticated user.
func (s *Service) DeleteRecording(ctx context.Context, id uuid.UUID) error {
	isAuth := ctxmeta.IsAuthenticated(ctx)
	userID, hasUserID := ctxmeta.GetAuthUserID(ctx)
	if !isAuth || !hasUserID {
		return constants.ErrUnauthorized
	}

	rec, err := s.repo.FindRecordingByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return constants.ErrNotFound
		}
		return fmt.Errorf("failed to retrieve recording: %w", err)
	}

	if rec.UserID == nil || *rec.UserID != userID {
		return constants.ErrForbidden
	}

	if err := s.repo.DeleteRecording(ctx, id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return constants.ErrNotFound
		}
		return fmt.Errorf("failed to soft-delete recording: %w", err)
	}

	return nil
}

// ClaimRecording transfers ownership of a guest recording session to the authenticated user.
func (s *Service) ClaimRecording(ctx context.Context, id uuid.UUID, req dtos.ClaimRecordingRequest) error {
	isAuth := ctxmeta.IsAuthenticated(ctx)
	userID, hasUserID := ctxmeta.GetAuthUserID(ctx)
	if !isAuth || !hasUserID {
		return constants.ErrUnauthorized
	}

	token := strings.TrimSpace(req.OwnershipToken)
	if token == "" {
		return constants.ErrBadRequest
	}

	rec, err := s.repo.FindRecordingByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return constants.ErrNotFound
		}
		return fmt.Errorf("failed to retrieve recording: %w", err)
	}

	// Verify recording is an unclaimed guest recording
	if !rec.IsGuest || rec.UserID != nil {
		return constants.ErrConflict
	}

	// Verify ownership token
	if rec.OwnershipToken != token {
		return constants.ErrForbidden
	}

	if err := s.repo.ClaimRecordingToUser(ctx, id, token, userID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return constants.ErrNotFound
		}
		return fmt.Errorf("failed to claim recording: %w", err)
	}

	return nil
}

// ClaimBulkRecordings transfers ownership of multiple guest recording sessions to the authenticated user.
func (s *Service) ClaimBulkRecordings(ctx context.Context, req dtos.BulkClaimRequest) (*dtos.BulkClaimResponse, error) {
	isAuth := ctxmeta.IsAuthenticated(ctx)
	userID, hasUserID := ctxmeta.GetAuthUserID(ctx)
	if !isAuth || !hasUserID {
		return nil, constants.ErrUnauthorized
	}

	// Filter and deduplicate tokens
	tokenMap := make(map[string]bool)
	var tokens []string
	for _, t := range req.Tokens {
		trimmed := strings.TrimSpace(t)
		if trimmed != "" && !tokenMap[trimmed] {
			tokenMap[trimmed] = true
			tokens = append(tokens, trimmed)
		}
	}

	if len(tokens) == 0 {
		return nil, constants.ErrBadRequest
	}

	claimedIDs, err := s.repo.ClaimRecordingsByTokens(ctx, tokens, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to bulk claim recordings: %w", err)
	}

	idStrings := make([]string, 0, len(claimedIDs))
	for _, id := range claimedIDs {
		idStrings = append(idStrings, id.String())
	}

	return &dtos.BulkClaimResponse{
		ClaimedCount: len(idStrings),
		RecordingIDs: idStrings,
	}, nil
}

// ToggleRecordingShare updates the public sharing state of a recording owned by the authenticated user.
func (s *Service) ToggleRecordingShare(ctx context.Context, id uuid.UUID, req dtos.ShareToggleRequest) (*dtos.ShareToggleResponse, error) {
	isAuth := ctxmeta.IsAuthenticated(ctx)
	userID, hasUserID := ctxmeta.GetAuthUserID(ctx)
	if !isAuth || !hasUserID {
		return nil, constants.ErrUnauthorized
	}

	if req.IsShareEnabled == nil {
		return nil, constants.ErrBadRequest
	}

	rec, err := s.repo.FindRecordingByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrNotFound
		}
		return nil, fmt.Errorf("failed to retrieve recording: %w", err)
	}

	if rec.UserID == nil || *rec.UserID != userID {
		return nil, constants.ErrForbidden
	}

	if *req.IsShareEnabled {
		var shareToken string
		if rec.ShareToken != nil && *rec.ShareToken != "" {
			shareToken = *rec.ShareToken
		} else {
			tokenBytes := make([]byte, 24)
			if _, err := rand.Read(tokenBytes); err != nil {
				return nil, fmt.Errorf("failed to generate share token: %w", err)
			}
			shareToken = hex.EncodeToString(tokenBytes)
		}

		if err := s.repo.UpdateRecordingShareSettings(ctx, id, true, &shareToken); err != nil {
			return nil, fmt.Errorf("failed to update share settings: %w", err)
		}

		shareURL := fmt.Sprintf("/v1/recordings/shared/%s", shareToken)
		return &dtos.ShareToggleResponse{
			IsShareEnabled: true,
			ShareToken:     &shareToken,
			ShareURL:       &shareURL,
		}, nil
	}

	if err := s.repo.UpdateRecordingShareSettings(ctx, id, false, nil); err != nil {
		return nil, fmt.Errorf("failed to update share settings: %w", err)
	}

	return &dtos.ShareToggleResponse{
		IsShareEnabled: false,
		ShareToken:     nil,
		ShareURL:       nil,
	}, nil
}
