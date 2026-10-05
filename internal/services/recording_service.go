package services

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
	"code-base-golang/internal/payload"
	"code-base-golang/internal/pkg/apperror"
	"code-base-golang/internal/pkg/ctxmeta"
	"code-base-golang/internal/pkg/ssrf"
	"code-base-golang/internal/repositories"
	"code-base-golang/internal/sse"
	"code-base-golang/internal/templates"
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
		template = constants.TemplateKeyGeneral
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

// GetRecordingDetail retrieves complete metadata, segments, active summary, chapters, highlights,
// and presigned playback URL for a recording, with ownership verification.
func (s *Service) GetRecordingDetail(ctx context.Context, id uuid.UUID, ownershipToken string) (*dtos.RecordingDetailResponse, error) {
	rec, err := s.repo.FindRecordingByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrRecordingNotFound
		}
		return nil, fmt.Errorf("failed to lookup recording: %w", err)
	}

	// Ownership verification:
	// 1. Authenticated user matching recording.user_id
	// 2. Ownership token matching recording.ownership_token
	// 3. Share token matching recording.share_token (if sharing enabled)
	hasAccess := false
	isAuth := ctxmeta.IsAuthenticated(ctx)
	userID, hasUserID := ctxmeta.GetAuthUserID(ctx)

	if isAuth && hasUserID && rec.UserID != nil && *rec.UserID == userID {
		hasAccess = true
	} else if ownershipToken != "" && ownershipToken == rec.OwnershipToken {
		hasAccess = true
	} else if ownershipToken != "" && rec.IsShareEnabled && rec.ShareToken != nil && ownershipToken == *rec.ShareToken {
		hasAccess = true
	}

	if !hasAccess {
		return nil, constants.ErrForbidden
	}

	// Generate pre-signed audio playback URL if stored in object storage
	var playbackURL *string
	if rec.AudioURL != nil && *rec.AudioURL != "" {
		if strings.HasPrefix(*rec.AudioURL, "http://") || strings.HasPrefix(*rec.AudioURL, "https://") {
			playbackURL = rec.AudioURL
		} else if s.storage != nil {
			presigned, err := s.storage.PresignGetObject(ctx, s.cfg.S3BucketName, *rec.AudioURL, 1*time.Hour)
			if err == nil {
				playbackURL = &presigned
			} else {
				playbackURL = rec.AudioURL
			}
		} else {
			playbackURL = rec.AudioURL
		}
	}

	// Fetch related child entities
	segments, err := s.repo.ListTranscriptSegmentsByRecordingID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch transcript segments: %w", err)
	}

	activeSummary, err := s.repo.FindActiveSummaryByRecordingID(ctx, id)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to fetch active summary: %w", err)
	}

	chapters, err := s.repo.ListChaptersByRecordingID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch chapters: %w", err)
	}

	highlights, err := s.repo.ListHighlightsByRecordingID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch highlights: %w", err)
	}

	segmentDTOs := make([]dtos.TranscriptSegmentDTO, len(segments))
	for i, seg := range segments {
		segmentDTOs[i] = dtos.TranscriptSegmentDTO{
			ID:            seg.ID.String(),
			SpeakerLabel:  seg.SpeakerLabel,
			SpeakerName:   seg.SpeakerName,
			StartTime:     seg.StartTime,
			EndTime:       seg.EndTime,
			Text:          seg.Text,
			WordsData:     seg.WordsData,
			SequenceOrder: seg.SequenceOrder,
		}
	}

	var summaryDTO *dtos.SummaryDTO
	if activeSummary != nil {
		summaryDTO = &dtos.SummaryDTO{
			ID:               activeSummary.ID.String(),
			TemplateCategory: activeSummary.TemplateCategory,
			CustomAngle:      activeSummary.CustomAngle,
			Version:          activeSummary.Version,
			IsActive:         activeSummary.IsActive,
			StructuredData:   activeSummary.StructuredData,
			MarkdownContent:  activeSummary.MarkdownContent,
			CreatedAt:        activeSummary.CreatedAt,
			UpdatedAt:        activeSummary.UpdatedAt,
		}
	}

	chapterDTOs := make([]dtos.ChapterDTO, len(chapters))
	for i, chap := range chapters {
		chapterDTOs[i] = dtos.ChapterDTO{
			ID:            chap.ID.String(),
			Title:         chap.Title,
			StartTime:     chap.StartTime,
			EndTime:       chap.EndTime,
			Summary:       chap.Summary,
			SequenceOrder: chap.SequenceOrder,
			CreatedAt:     chap.CreatedAt,
		}
	}

	highlightDTOs := make([]dtos.HighlightDTO, len(highlights))
	for i, hl := range highlights {
		highlightDTOs[i] = dtos.HighlightDTO{
			ID:        hl.ID.String(),
			StartTime: hl.StartTime,
			EndTime:   hl.EndTime,
			Title:     hl.Title,
			Note:      hl.Note,
			Source:    hl.Source,
			ClipURL:   hl.ClipURL,
			CreatedAt: hl.CreatedAt,
		}
	}

	var userIDStr *string
	if rec.UserID != nil {
		str := rec.UserID.String()
		userIDStr = &str
	}

	return &dtos.RecordingDetailResponse{
		ID:               rec.ID.String(),
		UserID:           userIDStr,
		Title:            rec.Title,
		OriginalFilename: rec.OriginalFilename,
		FileSizeBytes:    rec.FileSizeBytes,
		DurationSeconds:  rec.DurationSeconds,
		AudioURL:         playbackURL,
		PlaybackURL:      playbackURL,
		SourceType:       rec.SourceType,
		Status:           rec.Status,
		ErrorMessage:     rec.ErrorMessage,
		ErrorCode:        rec.ErrorCode,
		SelectedTemplate: rec.SelectedTemplate,
		DetectedLanguage: rec.DetectedLanguage,
		OutputLanguage:   rec.OutputLanguage,
		IsGuest:          rec.IsGuest,
		ConsentGiven:     rec.ConsentGiven,
		ConsentVersion:   rec.ConsentVersion,
		ExpiresAt:        rec.ExpiresAt,
		AnalyticsData:    rec.AnalyticsData,
		Segments:         segmentDTOs,
		ActiveSummary:    summaryDTO,
		Chapters:         chapterDTOs,
		Highlights:       highlightDTOs,
		CreatedAt:        rec.CreatedAt,
		UpdatedAt:        rec.UpdatedAt,
	}, nil
}

// ListRecordings retrieves paginated and filtered recordings owned by the authenticated user.
func (s *Service) ListRecordings(ctx context.Context, query dtos.RecordingFilterQuery) (*dtos.RecordingListResponse, error) {
	isAuth := ctxmeta.IsAuthenticated(ctx)
	userID, hasUserID := ctxmeta.GetAuthUserID(ctx)
	if !isAuth || !hasUserID {
		return nil, constants.ErrUnauthorized
	}

	query.SetDefaults()
	offset := (query.Page - 1) * query.Limit

	repoFilter := repositories.RecordingFilter{
		Search:    query.Search,
		Status:    query.Status,
		Template:  query.Template,
		SortBy:    query.SortBy,
		SortOrder: query.SortOrder,
		Limit:     query.Limit,
		Offset:    offset,
	}

	recordings, total, err := s.repo.ListRecordingsWithFilter(ctx, userID, repoFilter)
	if err != nil {
		return nil, fmt.Errorf("failed to list recordings: %w", err)
	}

	items := make([]dtos.RecordingListItem, len(recordings))
	for i, rec := range recordings {
		items[i] = dtos.RecordingListItem{
			ID:               rec.ID.String(),
			Title:            rec.Title,
			OriginalFilename: rec.OriginalFilename,
			FileSizeBytes:    rec.FileSizeBytes,
			DurationSeconds:  rec.DurationSeconds,
			SourceType:       rec.SourceType,
			Status:           rec.Status,
			SelectedTemplate: rec.SelectedTemplate,
			DetectedLanguage: rec.DetectedLanguage,
			OutputLanguage:   rec.OutputLanguage,
			CreatedAt:        rec.CreatedAt,
			UpdatedAt:        rec.UpdatedAt,
		}
	}

	totalPages := 0
	if query.Limit > 0 {
		totalPages = int((total + int64(query.Limit) - 1) / int64(query.Limit))
	}

	return &dtos.RecordingListResponse{
		Items: items,
		Pagination: dtos.PaginationMeta{
			CurrentPage: query.Page,
			PageSize:    query.Limit,
			TotalItems:  total,
			TotalPages:  totalPages,
		},
	}, nil
}

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

// GetSharedRecording retrieves a public read-only recording payload using an active share token.
func (s *Service) GetSharedRecording(ctx context.Context, shareToken string) (*dtos.SharedRecordingResponse, error) {
	token := strings.TrimSpace(shareToken)
	if token == "" {
		return nil, constants.ErrNotFound
	}

	rec, err := s.repo.FindRecordingByShareToken(ctx, token)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrNotFound
		}
		return nil, fmt.Errorf("failed to retrieve shared recording: %w", err)
	}

	// Generate pre-signed audio playback URL if stored in object storage
	var playbackURL *string
	if rec.AudioURL != nil && *rec.AudioURL != "" {
		if strings.HasPrefix(*rec.AudioURL, "http://") || strings.HasPrefix(*rec.AudioURL, "https://") {
			playbackURL = rec.AudioURL
		} else {
			presigned, err := s.storage.PresignGetObject(ctx, s.cfg.S3BucketName, *rec.AudioURL, 1*time.Hour)
			if err == nil {
				playbackURL = &presigned
			} else {
				playbackURL = rec.AudioURL
			}
		}
	}

	// Fetch related child entities
	segments, err := s.repo.ListTranscriptSegmentsByRecordingID(ctx, rec.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch transcript segments: %w", err)
	}

	activeSummary, err := s.repo.FindActiveSummaryByRecordingID(ctx, rec.ID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to fetch active summary: %w", err)
	}

	chapters, err := s.repo.ListChaptersByRecordingID(ctx, rec.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch chapters: %w", err)
	}

	highlights, err := s.repo.ListHighlightsByRecordingID(ctx, rec.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch highlights: %w", err)
	}

	segmentDTOs := make([]dtos.TranscriptSegmentDTO, len(segments))
	for i, seg := range segments {
		segmentDTOs[i] = dtos.TranscriptSegmentDTO{
			ID:            seg.ID.String(),
			SpeakerLabel:  seg.SpeakerLabel,
			SpeakerName:   seg.SpeakerName,
			StartTime:     seg.StartTime,
			EndTime:       seg.EndTime,
			Text:          seg.Text,
			WordsData:     seg.WordsData,
			SequenceOrder: seg.SequenceOrder,
		}
	}

	var summaryDTO *dtos.SummaryDTO
	if activeSummary != nil {
		summaryDTO = &dtos.SummaryDTO{
			ID:               activeSummary.ID.String(),
			TemplateCategory: activeSummary.TemplateCategory,
			CustomAngle:      activeSummary.CustomAngle,
			Version:          activeSummary.Version,
			IsActive:         activeSummary.IsActive,
			StructuredData:   activeSummary.StructuredData,
			MarkdownContent:  activeSummary.MarkdownContent,
			CreatedAt:        activeSummary.CreatedAt,
			UpdatedAt:        activeSummary.UpdatedAt,
		}
	}

	chapterDTOs := make([]dtos.ChapterDTO, len(chapters))
	for i, chap := range chapters {
		chapterDTOs[i] = dtos.ChapterDTO{
			ID:            chap.ID.String(),
			Title:         chap.Title,
			StartTime:     chap.StartTime,
			EndTime:       chap.EndTime,
			Summary:       chap.Summary,
			SequenceOrder: chap.SequenceOrder,
			CreatedAt:     chap.CreatedAt,
		}
	}

	highlightDTOs := make([]dtos.HighlightDTO, len(highlights))
	for i, hl := range highlights {
		highlightDTOs[i] = dtos.HighlightDTO{
			ID:        hl.ID.String(),
			StartTime: hl.StartTime,
			EndTime:   hl.EndTime,
			Title:     hl.Title,
			Note:      hl.Note,
			Source:    hl.Source,
			ClipURL:   hl.ClipURL,
			CreatedAt: hl.CreatedAt,
		}
	}

	return &dtos.SharedRecordingResponse{
		ID:               rec.ID.String(),
		Title:            rec.Title,
		DurationSeconds:  rec.DurationSeconds,
		AudioURL:         playbackURL,
		PlaybackURL:      playbackURL,
		SelectedTemplate: rec.SelectedTemplate,
		DetectedLanguage: rec.DetectedLanguage,
		OutputLanguage:   rec.OutputLanguage,
		Segments:         segmentDTOs,
		ActiveSummary:    summaryDTO,
		Chapters:         chapterDTOs,
		Highlights:       highlightDTOs,
		CreatedAt:        rec.CreatedAt,
	}, nil
}

// ImportRecordingFromURL validates target link against SSRF defense policies, streams media into S3,
// records the new recording entry, and emits the background pipeline extraction event.
func (s *Service) ImportRecordingFromURL(
	ctx context.Context,
	req dtos.ImportURLRequest,
) (*dtos.RecordingUploadResponse, error) {
	// 1. Anti-SSRF Validation
	parsedURL, err := ssrf.ValidateURL(ctx, req.URL)
	if err != nil {
		if errors.Is(err, ssrf.ErrBlockedAddress) {
			return nil, constants.ErrSSRFBlocked
		}
		if errors.Is(err, ssrf.ErrInvalidScheme) {
			return nil, constants.ErrInvalidImportURL
		}
		return nil, constants.ErrBadRequest.WithMessage(err.Error())
	}

	// 2. Validate daily quota (identical policy to direct upload)
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

	// 3. Securely fetch media stream via SSRF-safe HTTP client
	fetchCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	fetcher := s.mediaFetcher
	if fetcher == nil {
		fetcher = ssrf.FetchMediaStream
	}

	resp, err := fetcher(fetchCtx, req.URL, 60*time.Second)
	if err != nil {
		if errors.Is(err, ssrf.ErrBlockedAddress) {
			return nil, constants.ErrSSRFBlocked
		}
		return nil, constants.ErrImportFetchFailed.WithMessage(err.Error())
	}
	defer resp.Body.Close()

	// 4. Resolve filename and content type
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	urlFilename := filepath.Base(parsedURL.Path)
	ext := strings.ToLower(filepath.Ext(urlFilename))

	var safeFilename string
	if ext != "" && dtos.SupportedExtensions[ext] {
		safeFilename = urlFilename
	} else {
		inferredExt := inferMediaExtension(contentType)
		if inferredExt == "" {
			return nil, constants.ErrUnsupportedMediaType
		}
		safeFilename = fmt.Sprintf("import_%s%s", uuid.New().String()[:8], inferredExt)
	}

	if !dtos.IsValidMediaMIME(contentType, safeFilename) {
		return nil, constants.ErrUnsupportedMediaType
	}

	// 5. Stream media directly into S3 storage
	recID := uuid.New()
	objectKey := fmt.Sprintf("recordings/%s/%s", recID.String(), safeFilename)

	if err := s.storage.Upload(fetchCtx, s.cfg.S3BucketName, objectKey, resp.Body, resp.ContentLength, contentType); err != nil {
		return nil, fmt.Errorf("failed to stream media to storage: %w", err)
	}

	// 6. Build and persist Recording record
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = strings.TrimSuffix(safeFilename, filepath.Ext(safeFilename))
	}
	template := strings.TrimSpace(req.Template)
	if template == "" {
		template = constants.TemplateKeyGeneral
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
	now := time.Now().UTC()

	var fileSizeBytes int64
	if resp.ContentLength > 0 {
		fileSizeBytes = resp.ContentLength
	}

	rec := models.Recording{
		BaseModel: models.BaseModel{
			ID:        recID,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Title:            title,
		OriginalFilename: safeFilename,
		FileSizeBytes:    fileSizeBytes,
		AudioURL:         &objectKey,
		SourceType:       "LINK",
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

	// 7. Publish background extraction event
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

	respDTO := &dtos.RecordingUploadResponse{
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
		respDTO.OwnershipToken = &rec.OwnershipToken
	}

	return respDTO, nil
}

// GetRecordingProgress verifies ownership access and returns the current ProgressEvent
// along with an active SSE subscription channel and unsubscribe function.
func (s *Service) GetRecordingProgress(ctx context.Context, id uuid.UUID, ownershipToken string) (*sse.ProgressEvent, <-chan sse.ProgressEvent, func(), error) {
	rec, err := s.repo.FindRecordingByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, nil, constants.ErrRecordingNotFound
		}
		return nil, nil, nil, fmt.Errorf("failed to lookup recording: %w", err)
	}

	// Ownership verification
	hasAccess := false
	isAuth := ctxmeta.IsAuthenticated(ctx)
	userID, hasUserID := ctxmeta.GetAuthUserID(ctx)

	if isAuth && hasUserID && rec.UserID != nil && *rec.UserID == userID {
		hasAccess = true
	} else if ownershipToken != "" && ownershipToken == rec.OwnershipToken {
		hasAccess = true
	} else if ownershipToken != "" && rec.IsShareEnabled && rec.ShareToken != nil && ownershipToken == *rec.ShareToken {
		hasAccess = true
	}

	if !hasAccess {
		return nil, nil, nil, constants.ErrForbidden
	}

	stage, prog := sse.MapStatusToProgress(rec.Status)
	initialEvent := &sse.ProgressEvent{
		RecordingID:  rec.ID.String(),
		Status:       rec.Status,
		Stage:        stage,
		Progress:     prog,
		ErrorCode:    rec.ErrorCode,
		ErrorMessage: rec.ErrorMessage,
		UpdatedAt:    rec.UpdatedAt,
	}

	if s.sseHub == nil {
		return initialEvent, nil, func() {}, nil
	}

	subCh, unsub := s.sseHub.Subscribe(id)
	return initialEvent, subCh, unsub, nil
}

// PollRecordingProgress fetches the latest progress state for a recording.
func (s *Service) PollRecordingProgress(ctx context.Context, id uuid.UUID) (*sse.ProgressEvent, error) {
	rec, err := s.repo.FindRecordingByID(ctx, id)
	if err != nil {
		return nil, err
	}

	stage, prog := sse.MapStatusToProgress(rec.Status)
	return &sse.ProgressEvent{
		RecordingID:  rec.ID.String(),
		Status:       rec.Status,
		Stage:        stage,
		Progress:     prog,
		ErrorCode:    rec.ErrorCode,
		ErrorMessage: rec.ErrorMessage,
		UpdatedAt:    rec.UpdatedAt,
	}, nil
}

// RetryRecordingPipeline verifies ownership, validates that the recording is in a retryable state (FAILED),
// prevents concurrency conflicts (409 CONFLICT_PROCESSING if active, 409 ERR_ALREADY_COMPLETED if completed),
// and performs Smart State Recovery by resuming from the failing worker stage without re-extracting completed assets.
func (s *Service) RetryRecordingPipeline(ctx context.Context, id uuid.UUID, ownershipToken string) (*dtos.RetryRecordingResponse, error) {
	rec, err := s.repo.FindRecordingByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrRecordingNotFound
		}
		return nil, fmt.Errorf("failed to lookup recording: %w", err)
	}

	// 1. Ownership verification
	hasAccess := false
	isAuth := ctxmeta.IsAuthenticated(ctx)
	userID, hasUserID := ctxmeta.GetAuthUserID(ctx)

	if isAuth && hasUserID && rec.UserID != nil && *rec.UserID == userID {
		hasAccess = true
	} else if ownershipToken != "" && subtle.ConstantTimeCompare([]byte(ownershipToken), []byte(rec.OwnershipToken)) == 1 {
		hasAccess = true
	} else if ownershipToken != "" && rec.IsShareEnabled && rec.ShareToken != nil && subtle.ConstantTimeCompare([]byte(ownershipToken), []byte(*rec.ShareToken)) == 1 {
		hasAccess = true
	}

	if !hasAccess {
		return nil, constants.ErrForbidden
	}

	// 2. Concurrency Conflict Protection
	switch strings.ToUpper(strings.TrimSpace(rec.Status)) {
	case models.RecordingStatusPending,
		models.RecordingStatusQueued,
		models.RecordingStatusValidating,
		models.RecordingStatusExtracting,
		models.RecordingStatusTranscribing,
		models.RecordingStatusSummarizing,
		models.RecordingStatusIndexing,
		"PROCESSING":
		return nil, constants.ErrConflictProcessing
	case models.RecordingStatusCompleted:
		return nil, constants.ErrRecordingAlreadyCompleted
	case models.RecordingStatusFailed:
		// Allowed for smart retry
	default:
		return nil, constants.ErrConflictProcessing
	}

	// 3. Smart State Recovery: delegate to pipeline resume directly using existing recording model
	p, err := s.ResumeRecordingPipeline(ctx, rec)
	if err != nil {
		return nil, fmt.Errorf("failed to execute pipeline retry: %w", err)
	}

	status := p.Status
	if status == "" {
		status = models.RecordingStatusQueued
	}

	return &dtos.RetryRecordingResponse{
		ID:        rec.ID.String(),
		Status:    status,
		Stage:     p.Stage,
		Message:   "pipeline retry initiated successfully",
		UpdatedAt: time.Now().UTC(),
	}, nil
}

func inferMediaExtension(contentType string) string {
	cleanMIME := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	switch cleanMIME {
	case "audio/mpeg", "audio/mp3":
		return ".mp3"
	case "audio/wav", "audio/x-wav", "audio/wave":
		return ".wav"
	case "audio/mp4", "video/mp4":
		return ".mp4"
	case "audio/m4a", "audio/x-m4a":
		return ".m4a"
	case "audio/webm", "video/webm":
		return ".webm"
	case "audio/ogg":
		return ".ogg"
	case "video/quicktime":
		return ".mov"
	default:
		return ""
	}
}

// UpdateTranscriptSpeakers verifies recording ownership and batch-updates speaker names
// across matching transcript segments.
func (s *Service) UpdateTranscriptSpeakers(ctx context.Context, id uuid.UUID, ownershipToken string, req dtos.UpdateSpeakersRequest) (*dtos.UpdateSpeakersResponse, error) {
	rec, err := s.repo.FindRecordingByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrRecordingNotFound
		}
		return nil, fmt.Errorf("failed to lookup recording: %w", err)
	}

	// Ownership verification: only recording owner (authenticated user or guest with ownership token)
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

	cleanedSpeakers := make(map[string]string, len(req.Speakers))
	for label, name := range req.Speakers {
		cleanedSpeakers[strings.TrimSpace(label)] = strings.TrimSpace(name)
	}

	updatedCount, err := s.repo.UpdateTranscriptSpeakerNames(ctx, rec.ID, cleanedSpeakers)
	if err != nil {
		return nil, fmt.Errorf("failed to update transcript speaker names: %w", err)
	}

	return &dtos.UpdateSpeakersResponse{
		UpdatedCount: int(updatedCount),
		Speakers:     cleanedSpeakers,
	}, nil
}

// RegenerateSummary generates a new structured summary version for a recording with optional template category and custom angle.
func (s *Service) RegenerateSummary(ctx context.Context, id uuid.UUID, ownershipToken string, req dtos.RegenerateSummaryRequest) (*dtos.SummaryVersionResponse, error) {
	rec, err := s.repo.FindRecordingByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrRecordingNotFound
		}
		return nil, fmt.Errorf("failed to lookup recording: %w", err)
	}

	// Verify ownership
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

	// Conflict protection: check if currently actively processing
	switch strings.ToUpper(strings.TrimSpace(rec.Status)) {
	case models.RecordingStatusPending,
		models.RecordingStatusQueued,
		models.RecordingStatusValidating,
		models.RecordingStatusExtracting,
		models.RecordingStatusTranscribing,
		models.RecordingStatusSummarizing,
		models.RecordingStatusIndexing,
		"PROCESSING":
		return nil, constants.ErrConflictProcessing
	}

	// Enforce version cap: If total summary versions >= 5, return 409 SUMMARY_VERSION_LIMIT
	count, err := s.repo.CountSummaryVersions(ctx, rec.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to count summary versions: %w", err)
	}
	if count >= 5 {
		return nil, constants.ErrSummaryVersionLimit
	}

	// Retrieve transcript segments
	segments, err := s.repo.ListTranscriptSegmentsByRecordingID(ctx, rec.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to list transcript segments: %w", err)
	}
	if len(segments) == 0 {
		return nil, apperror.New(http.StatusBadRequest, "NO_TRANSCRIPT", "cannot regenerate summary without transcript")
	}

	// Build transcript text
	var sb strings.Builder
	for _, seg := range segments {
		startStr := templates.FormatTimestamp(seg.StartTime)
		endStr := templates.FormatTimestamp(seg.EndTime)
		sb.WriteString(fmt.Sprintf("[%s - %s] %s: %s\n", startStr, endStr, seg.SpeakerName, seg.Text))
	}
	transcriptBody := sb.String()

	// Determine template
	templateKey := strings.TrimSpace(req.TemplateCategory)
	if templateKey == "" {
		templateKey = rec.SelectedTemplate
	}
	if templateKey == "" {
		templateKey = constants.TemplateKeyGeneral
	}

	template, err := s.repo.FindTemplateByCategoryKey(ctx, templateKey)
	if err != nil || template == nil {
		template, _ = s.repo.FindTemplateByCategoryKey(ctx, constants.TemplateKeyGeneral)
	}

	systemPrompt, err := templates.DefaultSummarySystemPrompt()
	if err != nil {
		systemPrompt = "You are a professional executive meeting secretary and structured summarizer."
	}
	if template != nil && template.Prompt != "" {
		systemPrompt = template.Prompt
	}

	targetLang := rec.OutputLanguage
	if targetLang == "" && rec.DetectedLanguage != nil {
		targetLang = *rec.DetectedLanguage
	}

	var customAngle string
	if req.CustomAngle != nil {
		customAngle = strings.TrimSpace(*req.CustomAngle)
	}

	userPrompt, err := templates.RenderSummaryUserPrompt(transcriptBody, targetLang, customAngle)
	if err != nil {
		userPrompt = fmt.Sprintf("Please summarize the following meeting transcript:\n\n%s", transcriptBody)
	}

	var schema map[string]interface{}
	if template != nil && len(template.OutputSchema) > 0 {
		schema = map[string]interface{}(template.OutputSchema)
	}

	structuredRes, err := s.llm.GenerateStructured(ctx, systemPrompt, userPrompt, schema)
	if err != nil {
		return nil, fmt.Errorf("llm generation failed: %w", err)
	}

	var structMap map[string]interface{}
	if err := json.Unmarshal([]byte(structuredRes.RawJSON), &structMap); err != nil {
		return nil, fmt.Errorf("invalid structured response JSON: %w", err)
	}

	markdownContent := ""
	if md, ok := structMap["markdown_content"].(string); ok && md != "" {
		markdownContent = md
	} else if md, ok := structMap["executive_summary"].(string); ok && md != "" {
		markdownContent = md
	} else {
		markdownContent = structuredRes.RawJSON
	}

	var customAnglePtr *string
	if req.CustomAngle != nil && strings.TrimSpace(*req.CustomAngle) != "" {
		trimmed := strings.TrimSpace(*req.CustomAngle)
		customAnglePtr = &trimmed
	}

	newSummary := models.Summary{
		ID:               uuid.New(),
		RecordingID:      rec.ID,
		TemplateCategory: templateKey,
		CustomAngle:      customAnglePtr,
		StructuredData:   models.JSONMap(structMap),
		MarkdownContent:  markdownContent,
	}

	if err := s.repo.SaveNewSummaryVersion(ctx, &newSummary); err != nil {
		return nil, fmt.Errorf("failed to save summary version: %w", err)
	}

	return &dtos.SummaryVersionResponse{
		ID:               newSummary.ID.String(),
		Version:          newSummary.Version,
		TemplateCategory: newSummary.TemplateCategory,
		CustomAngle:      newSummary.CustomAngle,
		StructuredData:   newSummary.StructuredData,
		MarkdownContent:  newSummary.MarkdownContent,
		IsActive:         newSummary.IsActive,
		CreatedAt:        newSummary.CreatedAt,
	}, nil
}

// ListSummaryVersions retrieves all summary versions for a recording ordered by version ASC.
func (s *Service) ListSummaryVersions(ctx context.Context, id uuid.UUID, ownershipToken string) ([]dtos.SummaryVersionResponse, error) {
	rec, err := s.repo.FindRecordingByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrRecordingNotFound
		}
		return nil, fmt.Errorf("failed to lookup recording: %w", err)
	}

	// Verify ownership
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

	summaries, err := s.repo.ListSummaryVersions(ctx, rec.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to list summary versions: %w", err)
	}

	result := make([]dtos.SummaryVersionResponse, 0, len(summaries))
	for _, sum := range summaries {
		result = append(result, dtos.SummaryVersionResponse{
			ID:               sum.ID.String(),
			Version:          sum.Version,
			TemplateCategory: sum.TemplateCategory,
			CustomAngle:      sum.CustomAngle,
			StructuredData:   sum.StructuredData,
			MarkdownContent:  sum.MarkdownContent,
			IsActive:         sum.IsActive,
			CreatedAt:        sum.CreatedAt,
		})
	}

	return result, nil
}

// ActivateSummaryVersion activates a specific historical summary version for a recording.
func (s *Service) ActivateSummaryVersion(ctx context.Context, id uuid.UUID, versionID string, ownershipToken string) (*dtos.SummaryVersionResponse, error) {
	rec, err := s.repo.FindRecordingByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrRecordingNotFound
		}
		return nil, fmt.Errorf("failed to lookup recording: %w", err)
	}

	// Verify ownership
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

	activated, err := s.repo.ActivateSummaryVersion(ctx, rec.ID, versionID)
	if err != nil {
		return nil, err
	}

	return &dtos.SummaryVersionResponse{
		ID:               activated.ID.String(),
		Version:          activated.Version,
		TemplateCategory: activated.TemplateCategory,
		CustomAngle:      activated.CustomAngle,
		StructuredData:   activated.StructuredData,
		MarkdownContent:  activated.MarkdownContent,
		IsActive:         activated.IsActive,
		CreatedAt:        activated.CreatedAt,
	}, nil
}

// CreateInlineComment creates a timestamped inline comment or reply for a recording.
func (s *Service) CreateInlineComment(ctx context.Context, id uuid.UUID, ownershipToken string, req dtos.CreateCommentRequest) (*dtos.CommentResponse, error) {
	rec, err := s.repo.FindRecordingByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrRecordingNotFound
		}
		return nil, fmt.Errorf("failed to lookup recording: %w", err)
	}

	// Verify access: owner, guest with token, or publicly shared recording
	hasAccess := false
	isAuth := ctxmeta.IsAuthenticated(ctx)
	userID, hasUserID := ctxmeta.GetAuthUserID(ctx)

	if isAuth && hasUserID && rec.UserID != nil && *rec.UserID == userID {
		hasAccess = true
	} else if ownershipToken != "" && subtle.ConstantTimeCompare([]byte(ownershipToken), []byte(rec.OwnershipToken)) == 1 {
		hasAccess = true
	} else if rec.IsShareEnabled {
		hasAccess = true
	}

	if !hasAccess {
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

	var segIDStr *string
	if comment.SegmentID != nil {
		s := comment.SegmentID.String()
		segIDStr = &s
	}

	var parentIDStr *string
	if comment.ParentID != nil {
		p := comment.ParentID.String()
		parentIDStr = &p
	}

	var userIDStr *string
	if comment.UserID != nil {
		u := comment.UserID.String()
		userIDStr = &u
	}

	return &dtos.CommentResponse{
		ID:           comment.ID.String(),
		RecordingID:  comment.RecordingID.String(),
		UserID:       userIDStr,
		SegmentID:    segIDStr,
		TimestampSec: comment.TimestampSec,
		SelectedText: comment.SelectedText,
		AuthorName:   comment.AuthorName,
		CommentText:  comment.CommentText,
		ParentID:     parentIDStr,
		CreatedAt:    comment.CreatedAt,
		UpdatedAt:    comment.UpdatedAt,
	}, nil
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

	// Verify access: owner, guest with token, or publicly shared recording
	hasAccess := false
	isAuth := ctxmeta.IsAuthenticated(ctx)
	userID, hasUserID := ctxmeta.GetAuthUserID(ctx)

	if isAuth && hasUserID && rec.UserID != nil && *rec.UserID == userID {
		hasAccess = true
	} else if ownershipToken != "" && subtle.ConstantTimeCompare([]byte(ownershipToken), []byte(rec.OwnershipToken)) == 1 {
		hasAccess = true
	} else if rec.IsShareEnabled {
		hasAccess = true
	}

	if !hasAccess {
		return nil, constants.ErrForbidden
	}

	comments, err := s.repo.ListInlineCommentsByRecordingID(ctx, rec.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to list inline comments: %w", err)
	}

	res := make([]dtos.CommentResponse, 0, len(comments))
	for _, c := range comments {
		var segIDStr *string
		if c.SegmentID != nil {
			str := c.SegmentID.String()
			segIDStr = &str
		}

		var parentIDStr *string
		if c.ParentID != nil {
			str := c.ParentID.String()
			parentIDStr = &str
		}

		var userIDStr *string
		if c.UserID != nil {
			str := c.UserID.String()
			userIDStr = &str
		}

		replies := make([]dtos.CommentResponse, 0, len(c.Replies))
		for _, r := range c.Replies {
			var rSegIDStr *string
			if r.SegmentID != nil {
				str := r.SegmentID.String()
				rSegIDStr = &str
			}

			var rParentIDStr *string
			if r.ParentID != nil {
				str := r.ParentID.String()
				rParentIDStr = &str
			}

			var rUserIDStr *string
			if r.UserID != nil {
				str := r.UserID.String()
				rUserIDStr = &str
			}

			replies = append(replies, dtos.CommentResponse{
				ID:           r.ID.String(),
				RecordingID:  r.RecordingID.String(),
				UserID:       rUserIDStr,
				SegmentID:    rSegIDStr,
				TimestampSec: r.TimestampSec,
				SelectedText: r.SelectedText,
				AuthorName:   r.AuthorName,
				CommentText:  r.CommentText,
				ParentID:     rParentIDStr,
				CreatedAt:    r.CreatedAt,
				UpdatedAt:    r.UpdatedAt,
			})
		}

		res = append(res, dtos.CommentResponse{
			ID:           c.ID.String(),
			RecordingID:  c.RecordingID.String(),
			UserID:       userIDStr,
			SegmentID:    segIDStr,
			TimestampSec: c.TimestampSec,
			SelectedText: c.SelectedText,
			AuthorName:   c.AuthorName,
			CommentText:  c.CommentText,
			ParentID:     parentIDStr,
			Replies:      replies,
			CreatedAt:    c.CreatedAt,
			UpdatedAt:    c.UpdatedAt,
		})
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

	// Verify permission: comment author or recording owner
	isAuth := ctxmeta.IsAuthenticated(ctx)
	userID, hasUserID := ctxmeta.GetAuthUserID(ctx)

	canDelete := false
	if isAuth && hasUserID && rec.UserID != nil && *rec.UserID == userID {
		canDelete = true
	} else if ownershipToken != "" && subtle.ConstantTimeCompare([]byte(ownershipToken), []byte(rec.OwnershipToken)) == 1 {
		canDelete = true
	} else if isAuth && hasUserID && comment.UserID != nil && *comment.UserID == userID {
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
