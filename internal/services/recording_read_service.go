// recording_read_service.go — recording detail, listing, shared read, and progress use-cases.
package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/pkg/ctxmeta"
	"code-base-golang/internal/repositories"
	"code-base-golang/internal/sse"
)

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

	segmentDTOs := toTranscriptSegmentDTOs(segments)
	summaryDTO := toSummaryDTO(activeSummary)
	chapterDTOs := toChapterDTOs(chapters)
	highlightDTOs := toHighlightDTOs(highlights)

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

	segmentDTOs := toTranscriptSegmentDTOs(segments)
	summaryDTO := toSummaryDTO(activeSummary)
	chapterDTOs := toChapterDTOs(chapters)
	highlightDTOs := toHighlightDTOs(highlights)

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
