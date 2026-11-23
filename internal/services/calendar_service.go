package services

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
	"code-base-golang/internal/pkg/apperror"
	"code-base-golang/internal/pkg/crypto"
	"code-base-golang/internal/pkg/ctxmeta"
	"code-base-golang/internal/repositories"
)

func (s *Service) getCalendarEncryptionKey() string {
	if s.cfg.CalendarTokenEncryptionKey != "" {
		return s.cfg.CalendarTokenEncryptionKey
	}
	if s.cfg.JWTSecret != "" {
		return s.cfg.JWTSecret
	}
	return "youten-ai-default-calendar-token-encryption-key-32b"
}

// GetGoogleCalendarAuthURL generates the Google OAuth authorization URL with CSRF state token.
func (s *Service) GetGoogleCalendarAuthURL(ctx context.Context, userID uuid.UUID) (*dtos.GoogleCalendarAuthURLResponse, error) {
	if s.googleCalendar == nil {
		return nil, apperror.New(http.StatusServiceUnavailable, constants.ErrCodeFeatureComingSoon, "google calendar integration is currently unavailable")
	}

	statePayload := fmt.Sprintf("%s:%d", userID.String(), time.Now().Unix())
	encryptedState, err := crypto.Encrypt(statePayload, s.getCalendarEncryptionKey())
	if err != nil {
		return nil, fmt.Errorf("failed to generate secure oauth state: %w", err)
	}

	authURL := s.googleCalendar.GetAuthURL(encryptedState)
	return &dtos.GoogleCalendarAuthURLResponse{
		AuthURL: authURL,
		State:   encryptedState,
	}, nil
}

// HandleGoogleCalendarCallback exchanges the authorization code for tokens, verifies state, and persists connection.
func (s *Service) HandleGoogleCalendarCallback(ctx context.Context, userID uuid.UUID, req dtos.GoogleCalendarCallbackRequest) (*dtos.GoogleCalendarConnectionResponse, error) {
	if s.googleCalendar == nil {
		return nil, apperror.New(http.StatusServiceUnavailable, constants.ErrCodeFeatureComingSoon, "google calendar integration is currently unavailable")
	}

	// Validate CSRF state token matches user ID
	decryptedState, err := crypto.Decrypt(req.State, s.getCalendarEncryptionKey())
	if err != nil || !strings.HasPrefix(decryptedState, userID.String()+":") {
		return nil, apperror.New(http.StatusBadRequest, constants.ResponseCodeBadRequest, "invalid or expired oauth state parameter")
	}

	tokens, userInfo, err := s.googleCalendar.ExchangeCode(ctx, req.Code)
	if err != nil {
		return nil, apperror.New(http.StatusBadRequest, constants.ResponseCodeBadRequest, fmt.Sprintf("google code exchange failed: %v", err))
	}

	if tokens.RefreshToken == "" {
		return nil, apperror.New(http.StatusBadRequest, constants.ResponseCodeBadRequest, "failed to obtain google refresh token, please re-authenticate and grant offline access")
	}

	encRefresh, err := crypto.Encrypt(tokens.RefreshToken, s.getCalendarEncryptionKey())
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt refresh token: %w", err)
	}

	encAccess, err := crypto.Encrypt(tokens.AccessToken, s.getCalendarEncryptionKey())
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt access token: %w", err)
	}

	now := time.Now().UTC()
	tokenExpiresAt := now.Add(tokens.ExpiresIn)

	conn, err := s.repo.GetGoogleCalendarConnectionByUserID(ctx, userID)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("failed to check existing calendar connection: %w", err)
		}
		// Create new connection
		newConn := &models.UserGoogleCalendarConnection{
			BaseModel: models.BaseModel{
				ID:        uuid.New(),
				CreatedAt: now,
				UpdatedAt: now,
			},
			UserID:                userID,
			GoogleUserID:          userInfo.ID,
			GoogleEmail:           userInfo.Email,
			EncryptedAccessToken:  encAccess,
			EncryptedRefreshToken: encRefresh,
			TokenExpiresAt:        tokenExpiresAt,
			IsAutoRecordEnabled:   true,
			LeadTimeMinutes:       constants.DefaultCalendarLeadTimeMinutes,
			Status:                constants.CalendarConnectionStatusActive,
		}
		if createErr := s.repo.CreateGoogleCalendarConnection(ctx, newConn); createErr != nil {
			return nil, fmt.Errorf("failed to persist google calendar connection: %w", createErr)
		}
		conn = newConn
	} else {
		// Update existing connection
		conn.GoogleUserID = userInfo.ID
		conn.GoogleEmail = userInfo.Email
		conn.EncryptedAccessToken = encAccess
		conn.EncryptedRefreshToken = encRefresh
		conn.TokenExpiresAt = tokenExpiresAt
		conn.Status = constants.CalendarConnectionStatusActive
		conn.UpdatedAt = now
		if updateErr := s.repo.UpdateGoogleCalendarConnection(ctx, conn); updateErr != nil {
			return nil, fmt.Errorf("failed to update google calendar connection: %w", updateErr)
		}
	}

	// Trigger initial asynchronous sync
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		_ = s.syncUserCalendarInternal(bgCtx, conn)
	}()

	return &dtos.GoogleCalendarConnectionResponse{
		Connected:           true,
		GoogleEmail:         conn.GoogleEmail,
		IsAutoRecordEnabled: conn.IsAutoRecordEnabled,
		LeadTimeMinutes:     conn.LeadTimeMinutes,
		LastSyncedAt:        conn.LastSyncedAt,
		Status:              conn.Status,
	}, nil
}

// GetGoogleCalendarStatus retrieves current integration details for the caller.
func (s *Service) GetGoogleCalendarStatus(ctx context.Context, userID uuid.UUID) (*dtos.GoogleCalendarConnectionResponse, error) {
	conn, err := s.repo.GetGoogleCalendarConnectionByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &dtos.GoogleCalendarConnectionResponse{
				Connected: false,
				Status:    constants.IntegrationStatusDisconnected,
			}, nil
		}
		return nil, fmt.Errorf("failed to lookup google calendar connection: %w", err)
	}

	return &dtos.GoogleCalendarConnectionResponse{
		Connected:           conn.Status == constants.CalendarConnectionStatusActive,
		GoogleEmail:         conn.GoogleEmail,
		IsAutoRecordEnabled: conn.IsAutoRecordEnabled,
		LeadTimeMinutes:     conn.LeadTimeMinutes,
		LastSyncedAt:        conn.LastSyncedAt,
		Status:              conn.Status,
	}, nil
}

// UpdateCalendarSettings modifies auto-recording settings for a connected user.
func (s *Service) UpdateCalendarSettings(ctx context.Context, userID uuid.UUID, req dtos.UpdateCalendarSettingsRequest) (*dtos.GoogleCalendarConnectionResponse, error) {
	conn, err := s.repo.GetGoogleCalendarConnectionByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperror.New(http.StatusNotFound, constants.ResponseCodeNotFound, "google calendar is not connected")
		}
		return nil, fmt.Errorf("failed to lookup calendar connection: %w", err)
	}

	if req.IsAutoRecordEnabled != nil {
		conn.IsAutoRecordEnabled = *req.IsAutoRecordEnabled
	}
	if req.LeadTimeMinutes != nil {
		conn.LeadTimeMinutes = *req.LeadTimeMinutes
	}
	conn.UpdatedAt = time.Now().UTC()

	if err := s.repo.UpdateGoogleCalendarConnection(ctx, conn); err != nil {
		return nil, fmt.Errorf("failed to update calendar settings: %w", err)
	}

	return &dtos.GoogleCalendarConnectionResponse{
		Connected:           conn.Status == constants.CalendarConnectionStatusActive,
		GoogleEmail:         conn.GoogleEmail,
		IsAutoRecordEnabled: conn.IsAutoRecordEnabled,
		LeadTimeMinutes:     conn.LeadTimeMinutes,
		LastSyncedAt:        conn.LastSyncedAt,
		Status:              conn.Status,
	}, nil
}

// DisconnectGoogleCalendar removes a user's Google Calendar integration.
func (s *Service) DisconnectGoogleCalendar(ctx context.Context, userID uuid.UUID) error {
	if err := s.repo.DeleteGoogleCalendarConnection(ctx, userID); err != nil {
		return fmt.Errorf("failed to disconnect google calendar: %w", err)
	}
	return nil
}

// SyncUserCalendar initiates an on-demand calendar synchronization for the user.
func (s *Service) SyncUserCalendar(ctx context.Context, userID uuid.UUID) (*dtos.GoogleCalendarConnectionResponse, error) {
	conn, err := s.repo.GetGoogleCalendarConnectionByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperror.New(http.StatusNotFound, constants.ResponseCodeNotFound, "google calendar is not connected")
		}
		return nil, fmt.Errorf("failed to lookup calendar connection: %w", err)
	}

	if err := s.syncUserCalendarInternal(ctx, conn); err != nil {
		return nil, fmt.Errorf("failed to sync user calendar: %w", err)
	}

	return &dtos.GoogleCalendarConnectionResponse{
		Connected:           conn.Status == constants.CalendarConnectionStatusActive,
		GoogleEmail:         conn.GoogleEmail,
		IsAutoRecordEnabled: conn.IsAutoRecordEnabled,
		LeadTimeMinutes:     conn.LeadTimeMinutes,
		LastSyncedAt:        conn.LastSyncedAt,
		Status:              conn.Status,
	}, nil
}

// syncUserCalendarInternal refreshes tokens if needed, fetches events, and upserts scheduled meetings.
func (s *Service) syncUserCalendarInternal(ctx context.Context, conn *models.UserGoogleCalendarConnection) error {
	if s.googleCalendar == nil {
		return errors.New("google calendar client is nil")
	}

	now := time.Now().UTC()

	// Decrypt refresh token
	refreshToken, err := crypto.Decrypt(conn.EncryptedRefreshToken, s.getCalendarEncryptionKey())
	if err != nil {
		return fmt.Errorf("failed to decrypt refresh token: %w", err)
	}

	accessToken := ""
	// Refresh token if within 5 minutes of expiration
	if conn.TokenExpiresAt.Before(now.Add(5 * time.Minute)) {
		refreshed, refreshErr := s.googleCalendar.RefreshToken(ctx, refreshToken)
		if refreshErr != nil {
			conn.Status = constants.CalendarConnectionStatusExpired
			_ = s.repo.UpdateGoogleCalendarConnection(ctx, conn)
			return fmt.Errorf("failed to refresh google access token: %w", refreshErr)
		}
		accessToken = refreshed.AccessToken
		conn.TokenExpiresAt = now.Add(refreshed.ExpiresIn)
		encAccess, encErr := crypto.Encrypt(accessToken, s.getCalendarEncryptionKey())
		if encErr == nil {
			conn.EncryptedAccessToken = encAccess
		}
	} else {
		decryptedAccess, decErr := crypto.Decrypt(conn.EncryptedAccessToken, s.getCalendarEncryptionKey())
		if decErr != nil || decryptedAccess == "" {
			refreshed, refreshErr := s.googleCalendar.RefreshToken(ctx, refreshToken)
			if refreshErr != nil {
				return fmt.Errorf("failed to refresh token on empty access token: %w", refreshErr)
			}
			accessToken = refreshed.AccessToken
			conn.TokenExpiresAt = now.Add(refreshed.ExpiresIn)
		} else {
			accessToken = decryptedAccess
		}
	}

	syncTokenVal := ""
	if conn.SyncToken != nil {
		syncTokenVal = *conn.SyncToken
	}

	timeMin := now.Add(-24 * time.Hour)
	syncResult, err := s.googleCalendar.FetchEvents(ctx, accessToken, syncTokenVal, timeMin)
	if err != nil {
		return fmt.Errorf("failed to fetch events from google api: %w", err)
	}
	if syncResult == nil {
		return nil
	}

	// Filter & build scheduled events
	var eventsToUpsert []models.ScheduledCalendarEvent
	leadDuration := time.Duration(conn.LeadTimeMinutes) * time.Minute

	for _, item := range syncResult.Events {
		// Only consider meetings with identifiable video platforms
		if item.MeetingURL == "" || item.MeetingPlatform == constants.MeetingPlatformNone {
			continue
		}

		// Only schedule if user confirmed/accepted
		if item.UserResponseStatus != constants.CalendarResponseStatusAccepted {
			continue
		}

		scheduledDispatchAt := item.StartTime.Add(-leadDuration)

		// Auto-record is enabled if global setting is true and meeting is in the future
		autoRecord := conn.IsAutoRecordEnabled && item.EndTime.After(now)

		var descPtr *string
		if item.Description != "" {
			descPtr = &item.Description
		}
		var orgPtr *string
		if item.OrganizerEmail != "" {
			orgPtr = &item.OrganizerEmail
		}

		eventsToUpsert = append(eventsToUpsert, models.ScheduledCalendarEvent{
			BaseModel: models.BaseModel{
				ID:        uuid.New(),
				CreatedAt: now,
				UpdatedAt: now,
			},
			UserID:              conn.UserID,
			GoogleEventID:       item.ID,
			Title:               item.Title,
			Description:         descPtr,
			StartTime:           item.StartTime,
			EndTime:             item.EndTime,
			ScheduledDispatchAt: scheduledDispatchAt,
			MeetingURL:          item.MeetingURL,
			MeetingPlatform:     item.MeetingPlatform,
			OrganizerEmail:      orgPtr,
			UserResponseStatus:  item.UserResponseStatus,
			AutoRecordEnabled:   autoRecord,
			DispatchStatus:      constants.CalendarDispatchStatusPending,
		})
	}

	if len(eventsToUpsert) > 0 {
		if err := s.repo.UpsertScheduledCalendarEvents(ctx, eventsToUpsert); err != nil {
			return fmt.Errorf("failed to upsert calendar events: %w", err)
		}
	}

	conn.SyncToken = &syncResult.NextSyncToken
	conn.LastSyncedAt = &now
	conn.UpdatedAt = now
	return s.repo.UpdateGoogleCalendarConnection(ctx, conn)
}

// ListCalendarEvents returns paginated calendar events for the user.
func (s *Service) ListCalendarEvents(ctx context.Context, userID uuid.UUID, query dtos.ListCalendarEventsQuery) ([]dtos.CalendarEventDTO, int64, error) {
	filter := repositories.CalendarEventFilter{
		StartDate:      query.StartDate,
		EndDate:        query.EndDate,
		Platform:       query.Platform,
		DispatchStatus: query.DispatchStatus,
	}

	offset := (query.Page - 1) * query.Limit
	events, total, err := s.repo.ListScheduledCalendarEvents(ctx, userID, filter, query.Limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list calendar events: %w", err)
	}

	dtosList := make([]dtos.CalendarEventDTO, 0, len(events))
	for _, e := range events {
		dtosList = append(dtosList, dtos.CalendarEventDTO{
			ID:                  e.ID,
			GoogleEventID:       e.GoogleEventID,
			Title:               e.Title,
			Description:         e.Description,
			StartTime:           e.StartTime,
			EndTime:             e.EndTime,
			ScheduledDispatchAt: e.ScheduledDispatchAt,
			MeetingURL:          e.MeetingURL,
			MeetingPlatform:     e.MeetingPlatform,
			OrganizerEmail:      e.OrganizerEmail,
			UserResponseStatus:  e.UserResponseStatus,
			AutoRecordEnabled:   e.AutoRecordEnabled,
			DispatchStatus:      e.DispatchStatus,
			BotSessionID:        e.BotSessionID,
			RecordingID:         e.RecordingID,
			ErrorMessage:        e.ErrorMessage,
			CreatedAt:           e.CreatedAt,
		})
	}

	return dtosList, total, nil
}

// ToggleEventAutoRecord toggles auto-recording for a single meeting event.
func (s *Service) ToggleEventAutoRecord(ctx context.Context, eventID uuid.UUID, userID uuid.UUID, req dtos.ToggleEventAutoRecordRequest) error {
	if err := s.repo.ToggleEventAutoRecord(ctx, eventID, userID, req.AutoRecordEnabled); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperror.New(http.StatusNotFound, constants.ResponseCodeNotFound, "calendar event not found")
		}
		return fmt.Errorf("failed to toggle event auto record: %w", err)
	}
	return nil
}

// ProcessDueCalendarDispatches queries events due for dispatch, claims them, and dispatches the meeting bot.
func (s *Service) ProcessDueCalendarDispatches(ctx context.Context) error {
	now := time.Now().UTC()
	events, err := s.repo.GetPendingDispatches(ctx, now, 20)
	if err != nil {
		return fmt.Errorf("failed to get pending dispatches: %w", err)
	}

	for _, evt := range events {
		claimed, claimErr := s.repo.ClaimEventForDispatch(ctx, evt.ID)
		if claimErr != nil || !claimed {
			continue
		}

		// Check user daily quota under the user's authenticated context
		userCtx := ctxmeta.WithAuthUser(ctx, ctxmeta.AuthUser{UserID: evt.UserID})

		if quotaErr := s.checkDailyQuota(userCtx); quotaErr != nil {
			errMsg := "daily recording quota exceeded"
			_ = s.repo.MarkEventSkippedOrFailed(ctx, evt.ID, constants.CalendarDispatchStatusSkippedQuota, &errMsg)
			continue
		}

		// Dispatch meeting bot
		dispatchReq := dtos.DispatchBotRequest{
			Provider:   evt.MeetingPlatform,
			MeetingURL: evt.MeetingURL,
			Title:      evt.Title,
		}

		botResp, dispatchErr := s.DispatchMeetingBot(userCtx, dispatchReq)
		if dispatchErr != nil {
			errStr := dispatchErr.Error()
			_ = s.repo.MarkEventSkippedOrFailed(ctx, evt.ID, constants.CalendarDispatchStatusFailed, &errStr)
			continue
		}

		_ = s.repo.MarkEventDispatched(ctx, evt.ID, botResp.SessionID, botResp.RecordingID)
	}

	return nil
}

// SyncAllActiveCalendars iterates through all active connections in batches and syncs their schedules.
func (s *Service) SyncAllActiveCalendars(ctx context.Context) error {
	limit := 50
	offset := 0

	for {
		conns, err := s.repo.ListActiveGoogleCalendarConnections(ctx, limit, offset)
		if err != nil {
			return fmt.Errorf("failed to list active calendar connections: %w", err)
		}
		if len(conns) == 0 {
			break
		}

		for _, conn := range conns {
			_ = s.syncUserCalendarInternal(ctx, &conn)
		}

		if len(conns) < limit {
			break
		}
		offset += limit
	}

	return nil
}
