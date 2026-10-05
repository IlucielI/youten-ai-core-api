package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
	"code-base-golang/internal/payload"
	"code-base-golang/internal/pkg/ctxmeta"
	"code-base-golang/internal/pkg/ssrf"
	"code-base-golang/internal/repositories"
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

