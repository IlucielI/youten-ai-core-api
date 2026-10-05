package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
	"code-base-golang/internal/payload"
	"code-base-golang/internal/pkg/ctxmeta"
)

// GeneratePresignUpload generates a pre-signed S3 PUT URL for direct client-to-storage upload
// with quota validation and media extension checks.
func (s *Service) GeneratePresignUpload(ctx context.Context, req dtos.PresignUploadRequest) (*dtos.PresignUploadResponse, error) {
	safeFilename := filepath.Base(req.Filename)
	if safeFilename == "." || safeFilename == "/" || safeFilename == "" {
		return nil, constants.ErrBadRequest
	}

	// Pre-flight media extension check
	if !dtos.IsValidMediaMIME(req.ContentType, req.Filename) {
		return nil, constants.ErrUnsupportedMediaType
	}

	// Quota validation
	isAuth := ctxmeta.IsAuthenticated(ctx)
	userID, hasUserID := ctxmeta.GetAuthUserID(ctx)
	clientIP := ctxmeta.GetClientIP(ctx)

	if isAuth && hasUserID {
		user, err := s.repo.FindUserByID(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("failed to lookup user for quota check: %w", err)
		}
		if user.Status != constants.UserStatusActive {
			return nil, constants.ErrUserInactive
		}

		dailyQuota := constants.DefaultUserDailyQuota
		if user.DailyQuotaOverride != nil {
			dailyQuota = *user.DailyQuotaOverride
		}

		countToday, err := s.repo.CountUserRecordingsToday(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("failed to count user daily recordings: %w", err)
		}
		if countToday >= int64(dailyQuota) {
			return nil, constants.ErrDailyQuotaExceeded
		}
	} else {
		countToday, err := s.repo.CountGuestRecordingsToday(ctx, clientIP)
		if err != nil {
			return nil, fmt.Errorf("failed to count guest daily recordings: %w", err)
		}
		if countToday >= int64(constants.DefaultGuestDailyQuota) {
			return nil, constants.ErrGuestDailyQuotaExceeded
		}
	}

	// Generate unique object key: recordings/<uuid>/<original_filename>
	recID := uuid.New()
	objectKey := fmt.Sprintf("recordings/%s/%s", recID.String(), safeFilename)

	uploadURL, err := s.storage.PresignPutObject(ctx, s.cfg.S3BucketName, objectKey, 15*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("failed to generate presigned upload url: %w", err)
	}

	return &dtos.PresignUploadResponse{
		UploadURL: uploadURL,
		ObjectKey: objectKey,
		Filename:  safeFilename,
	}, nil
}

// UploadRecording confirms media file ingestion after direct upload to S3, persists the recording record,
// and triggers the background pipeline extraction event.
func (s *Service) UploadRecording(
	ctx context.Context,
	req dtos.UploadRecordingRequest,
) (*dtos.RecordingUploadResponse, error) {
	safeFilename := filepath.Base(req.Filename)
	if safeFilename == "." || safeFilename == "/" || safeFilename == "" {
		return nil, constants.ErrBadRequest
	}

	isAuth := ctxmeta.IsAuthenticated(ctx)
	userID, hasUserID := ctxmeta.GetAuthUserID(ctx)
	clientIP := ctxmeta.GetClientIP(ctx)

	// Validate daily quota
	if isAuth && hasUserID {
		user, err := s.repo.FindUserByID(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("failed to lookup user for quota check: %w", err)
		}
		if user.Status != constants.UserStatusActive {
			return nil, constants.ErrUserInactive
		}

		dailyQuota := constants.DefaultUserDailyQuota
		if user.DailyQuotaOverride != nil {
			dailyQuota = *user.DailyQuotaOverride
		}

		countToday, err := s.repo.CountUserRecordingsToday(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("failed to count user daily recordings: %w", err)
		}
		if countToday >= int64(dailyQuota) {
			return nil, constants.ErrDailyQuotaExceeded
		}
	} else {
		countToday, err := s.repo.CountGuestRecordingsToday(ctx, clientIP)
		if err != nil {
			return nil, fmt.Errorf("failed to count guest daily recordings: %w", err)
		}
		if countToday >= int64(constants.DefaultGuestDailyQuota) {
			return nil, constants.ErrGuestDailyQuotaExceeded
		}
	}

	objectKey := strings.TrimSpace(req.ObjectKey)
	if objectKey == "" {
		objectKey = fmt.Sprintf("recordings/%s/%s", uuid.New().String(), safeFilename)
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = strings.TrimSuffix(safeFilename, filepath.Ext(safeFilename))
	}
	template := strings.TrimSpace(req.Template)
	if template == "" {
		template = "GENERAL"
	}
	language := strings.TrimSpace(req.Language)
	if language == "" {
		language = "id"
	}

	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("failed to generate ownership token: %w", err)
	}
	ownershipToken := hex.EncodeToString(tokenBytes)
	recordingID := uuid.New()
	now := time.Now().UTC()

	rec := models.Recording{
		BaseModel: models.BaseModel{
			ID:        recordingID,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Title:            title,
		OriginalFilename: safeFilename,
		AudioURL:         &objectKey,
		SourceType:       "UPLOAD",
		Status:           models.RecordingStatusPending,
		SelectedTemplate: template,
		OutputLanguage:   language,
		OwnershipToken:   ownershipToken,
		ConsentGiven:     true,
		ConsentVersion:   "1.0",
		ConsentAt:        &now,
	}

	if isAuth && hasUserID {
		rec.UserID = &userID
		rec.IsGuest = false
	} else {
		rec.IsGuest = true
		if clientIP != "" {
			rec.GuestIP = &clientIP
		}
		expiresAt := now.Add(24 * time.Hour)
		rec.ExpiresAt = &expiresAt
	}

	if err := s.repo.CreateRecording(ctx, &rec); err != nil {
		return nil, fmt.Errorf("failed to persist recording: %w", err)
	}

	// Publish background extraction event
	if s.publisher != nil {
		eventPayload := payload.RecordingPipelinePayload{
			RecordingID: rec.ID,
			SourcePath:  objectKey,
			Template:    rec.SelectedTemplate,
			Language:    rec.OutputLanguage,
			Stage:       models.RecordingStatusExtracting,
		}
		if err := s.publisher.Publish(ctx, constants.TopicRecordingUploaded, eventPayload); err != nil {
			return nil, fmt.Errorf("failed to publish recording event: %w", err)
		}
	}

	resp := &dtos.RecordingUploadResponse{
		ID:               rec.ID.String(),
		Title:            rec.Title,
		OriginalFilename: rec.OriginalFilename,
		FileSizeBytes:    rec.FileSizeBytes,
		Status:           rec.Status,
		SelectedTemplate: rec.SelectedTemplate,
		OutputLanguage:   rec.OutputLanguage,
		IsGuest:          rec.IsGuest,
		CreatedAt:        rec.CreatedAt,
	}
	if rec.IsGuest {
		resp.OwnershipToken = &rec.OwnershipToken
	}

	return resp, nil
}

// CleanupExpiredRecordings finds guest recordings that exceeded their retention TTL,
// deletes their media objects from storage, and hard-deletes the database rows (with cascade).
func (s *Service) CleanupExpiredRecordings(ctx context.Context, batchSize int) (int, error) {
	if batchSize <= 0 {
		batchSize = 100
	}
	expiredList, err := s.repo.FindExpiredRecordings(ctx, batchSize)
	if err != nil {
		return 0, fmt.Errorf("failed to query expired recordings: %w", err)
	}
	deletedCount := 0
	for _, rec := range expiredList {
		// 1. Delete media object from S3 storage
		if rec.AudioURL != nil && *rec.AudioURL != "" && s.storage != nil {
			if err := s.storage.Delete(ctx, s.cfg.S3BucketName, *rec.AudioURL); err != nil {
				fmt.Printf("[WARN] failed to delete storage object %s for expired recording %s: %v\n", *rec.AudioURL, rec.ID, err)
			}
		}

		// 2. Permanently delete recording row (cascades to transcripts, summaries, etc.)
		if err := s.repo.HardDeleteRecording(ctx, rec.ID); err != nil {
			fmt.Printf("[ERROR] failed to hard delete expired recording %s: %v\n", rec.ID, err)
			continue
		}
		deletedCount++
	}

	return deletedCount, nil
}
