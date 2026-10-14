package services

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
	"code-base-golang/internal/pkg/apperror"
	"code-base-golang/internal/queue"
)

// DispatchMeetingBot initiates a voice bot session for the requested platform and associates it with a recording.
func (s *Service) DispatchMeetingBot(ctx context.Context, req dtos.DispatchBotRequest) (*dtos.DispatchBotResponse, error) {
	provider, exists := s.BotProvider(req.Provider)
	if !exists {
		return nil, apperror.New(http.StatusBadRequest, constants.ErrCodeInvalidProvider, fmt.Sprintf("unsupported bot provider %q", req.Provider))
	}

	if !provider.IsEnabled() {
		return nil, apperror.New(http.StatusNotImplemented, constants.ErrCodeFeatureComingSoon, fmt.Sprintf("%s voice bot is currently unavailable or coming soon", req.Provider))
	}

	// Verify user daily quota
	if err := s.checkDailyQuota(ctx); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	recordingID := uuid.New()
	sessionID := uuid.New()
	ownershipToken := uuid.New().String()

	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = fmt.Sprintf("%s Voice Meeting - %s", strings.ToUpper(req.Provider[:1])+req.Provider[1:], now.Format("02 Jan 2006 15:04"))
	}

	template := strings.TrimSpace(req.Template)
	if template == "" || !constants.IsValidTemplate(template) {
		template = constants.DefaultTemplateKey
	}

	language := strings.TrimSpace(req.Language)
	if language == "" {
		language = "id"
	}

	recording := &models.Recording{
		BaseModel: models.BaseModel{
			ID:        recordingID,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Title:            title,
		OriginalFilename: fmt.Sprintf("%s_session_%s.wav", req.Provider, sessionID.String()[:8]),
		SourceType:       constants.RecordingSourceTypeMeetingBot,
		BotProvider:      &req.Provider,
		Status:           models.RecordingStatusRecording,
		SelectedTemplate: template,
		OutputLanguage:   language,
		OwnershipToken:   ownershipToken,
		ConsentGiven:     true,
		ConsentVersion:   "1.0",
		ConsentAt:        &now,
	}

	applyRecordingOwnership(ctx, recording, now)

	if err := s.repo.CreateRecording(ctx, recording); err != nil {
		return nil, fmt.Errorf("failed to persist recording for bot session: %w", err)
	}

	botSession := &models.BotSession{
		BaseModel: models.BaseModel{
			ID:        sessionID,
			CreatedAt: now,
			UpdatedAt: now,
		},
		RecordingID: recordingID,
		Provider:    req.Provider,
		Status:      constants.BotSessionStatusDispatched,
	}
	if req.MeetingURL != "" {
		botSession.MeetingURL = &req.MeetingURL
	}
	if req.ChannelID != "" {
		botSession.ChannelID = &req.ChannelID
	}
	if req.GuildID != "" {
		botSession.GuildID = &req.GuildID
	}

	if err := s.repo.CreateBotSession(ctx, botSession); err != nil {
		_ = s.repo.DeleteRecording(ctx, recordingID)
		return nil, fmt.Errorf("failed to persist bot session: %w", err)
	}

	// Dispatch to concrete provider
	params := dtos.BotDispatchParams{
		SessionID:   sessionID,
		RecordingID: recordingID,
		Provider:    req.Provider,
		MeetingURL:  req.MeetingURL,
		GuildID:     req.GuildID,
		ChannelID:   req.ChannelID,
		ScheduledAt: req.ScheduledAt,
	}

	extSessionID, err := provider.Dispatch(ctx, params)
	if err != nil {
		errMsg := err.Error()
		_ = s.repo.UpdateBotSessionStatus(ctx, sessionID, constants.BotSessionStatusFailed, &errMsg, nil, &now)
		return nil, err
	}

	// Update session with external connection ID and mark status active
	if err := s.repo.UpdateBotSessionExternalID(ctx, sessionID, extSessionID); err != nil {
		return nil, err
	}
	if err := s.repo.UpdateBotSessionStatus(ctx, sessionID, constants.BotSessionStatusRecording, nil, &now, nil); err != nil {
		return nil, err
	}

	s.publishProgress(recordingID, models.RecordingStatusRecording, nil, nil)

	return &dtos.DispatchBotResponse{
		SessionID:      sessionID,
		RecordingID:    recordingID,
		OwnershipToken: ownershipToken,
		Provider:       req.Provider,
		Status:         constants.BotSessionStatusRecording,
		Message:        fmt.Sprintf("successfully dispatched %s voice bot", req.Provider),
	}, nil
}

// GetBotSessionStatus queries the current live status of an active or past bot session.
func (s *Service) GetBotSessionStatus(ctx context.Context, sessionID uuid.UUID) (*dtos.BotSessionStatusResponse, error) {
	session, err := s.repo.FindBotSessionByID(ctx, sessionID)
	if err != nil {
		return nil, constants.ErrNotFound
	}

	if err := s.syncBotSessionLiveStatus(ctx, session); err != nil {
		return nil, err
	}

	return &dtos.BotSessionStatusResponse{
		SessionID:    session.ID,
		RecordingID:  session.RecordingID,
		Provider:     session.Provider,
		Status:       session.Status,
		MeetingURL:   session.MeetingURL,
		ChannelID:    session.ChannelID,
		GuildID:      session.GuildID,
		StartedAt:    session.StartedAt,
		EndedAt:      session.EndedAt,
		ErrorMessage: session.ErrorMessage,
	}, nil
}

// syncBotSessionLiveStatus queries the live provider status for active recording sessions and persists any state changes.
func (s *Service) syncBotSessionLiveStatus(ctx context.Context, session *models.BotSession) error {
	if session.ExternalSessionID == nil || session.Status != constants.BotSessionStatusRecording {
		return nil
	}

	provider, exists := s.BotProvider(session.Provider)
	if !exists || !provider.IsEnabled() {
		return nil
	}

	liveStatus, err := provider.GetStatus(ctx, *session.ExternalSessionID)
	if err != nil || liveStatus == nil || liveStatus.Status == session.Status {
		return nil
	}

	session.Status = liveStatus.Status
	session.StartedAt = liveStatus.StartedAt
	session.EndedAt = liveStatus.EndedAt

	return s.repo.UpdateBotSessionStatus(ctx, session.ID, liveStatus.Status, nil, liveStatus.StartedAt, liveStatus.EndedAt)
}

// StopBotSession commands an active voice bot to leave the meeting/channel and conclude recording.
func (s *Service) StopBotSession(ctx context.Context, sessionID uuid.UUID) (*dtos.StopBotResponse, error) {
	session, err := s.repo.FindBotSessionByID(ctx, sessionID)
	if err != nil {
		return nil, constants.ErrNotFound
	}

	now := time.Now().UTC()
	if provider, exists := s.BotProvider(session.Provider); exists && session.ExternalSessionID != nil {
		if err := provider.Stop(ctx, *session.ExternalSessionID); err != nil {
			return nil, err
		}
	}

	if err := s.repo.UpdateBotSessionStatus(ctx, sessionID, constants.BotSessionStatusCompleted, nil, nil, &now); err != nil {
		return nil, err
	}

	// Transition recording status to pending/extracting
	rec, err := s.repo.FindRecordingByID(ctx, session.RecordingID)
	if err == nil && rec != nil {
		rec.Status = models.RecordingStatusPending
		if err := s.repo.DB().WithContext(ctx).Model(rec).Update("status", models.RecordingStatusPending).Error; err != nil {
			return nil, err
		}
	}

	return &dtos.StopBotResponse{
		SessionID:   session.ID,
		RecordingID: session.RecordingID,
		Status:      constants.BotSessionStatusCompleted,
		Message:     fmt.Sprintf("%s voice bot session completed", session.Provider),
	}, nil
}

// GetCapabilities aggregates runtime availability across media ingestion and meeting bot platforms.
func (s *Service) GetCapabilities(ctx context.Context) *dtos.CapabilitiesResponse {
	meetingBots := make(map[string]string)
	supportedProviders := []string{
		constants.BotProviderDiscord,
		constants.BotProviderGoogleMeet,
		constants.BotProviderMSTeams,
		constants.BotProviderZoom,
	}

	for _, name := range supportedProviders {
		p, exists := s.BotProvider(name)
		if exists && p.IsEnabled() {
			meetingBots[name] = constants.CapabilityStatusAvailable
		} else {
			meetingBots[name] = constants.CapabilityStatusComingSoon
		}
	}

	return &dtos.CapabilitiesResponse{
		MeetingBot:  meetingBots,
		AudioUpload: true,
		LinkImport:  true,
	}
}

// botWebhookPayload encapsulates normalized callback attributes received across heterogeneous meeting bot engines.
type botWebhookPayload struct {
	ExternalSessionID string
	Event             string
	AudioURL          *string
	DurationSeconds   *int
	ErrorMessage      *string
}

// mapBotWebhookEvent maps external provider lifecycle events into canonical bot session statuses.
func mapBotWebhookEvent(event string) (status string, isStarted bool, isEnded bool, ok bool) {
	switch event {
	case constants.BotWebhookEventWaitingAdmit:
		return constants.BotSessionStatusWaitingAdmit, false, false, true
	case constants.BotWebhookEventJoined, constants.BotWebhookEventCallConnected:
		return constants.BotSessionStatusJoined, true, false, true
	case constants.BotWebhookEventRecording, constants.BotWebhookEventRecordingStarted:
		return constants.BotSessionStatusRecording, true, false, true
	case constants.BotWebhookEventCompleted:
		return constants.BotSessionStatusCompleted, false, true, true
	case constants.BotWebhookEventFailed:
		return constants.BotSessionStatusFailed, false, true, true
	default:
		return "", false, false, false
	}
}

// processBotWebhook handles shared state transitions and recording updates triggered by webhook callbacks.
func (s *Service) processBotWebhook(ctx context.Context, payload botWebhookPayload) error {
	session, err := s.repo.FindBotSessionByExternalID(ctx, payload.ExternalSessionID)
	if err != nil {
		return apperror.New(http.StatusNotFound, constants.ErrCodeSessionNotFound, "bot session not found for external id")
	}

	mappedStatus, isStarted, isEnded, ok := mapBotWebhookEvent(payload.Event)
	if !ok {
		return apperror.New(http.StatusBadRequest, constants.ErrCodeInvalidEvent, "unrecognized webhook event")
	}

	// Idempotency: Ignore duplicate completed events to prevent re-triggering transcription pipeline
	if session.Status == constants.BotSessionStatusCompleted && mappedStatus == constants.BotSessionStatusCompleted {
		return nil
	}

	now := time.Now().UTC()
	var startedAt, endedAt *time.Time
	var errMsg *string

	if isStarted && (session.StartedAt == nil || mappedStatus == constants.BotSessionStatusJoined) {
		startedAt = &now
	}
	if isEnded {
		endedAt = &now
		if mappedStatus == constants.BotSessionStatusFailed {
			errMsg = payload.ErrorMessage
		}
	}

	if err := s.repo.UpdateBotSessionStatus(ctx, session.ID, mappedStatus, errMsg, startedAt, endedAt); err != nil {
		return err
	}

	if mappedStatus == constants.BotSessionStatusCompleted && payload.AudioURL != nil && *payload.AudioURL != "" {
		duration := float64(0)
		if payload.DurationSeconds != nil {
			duration = float64(*payload.DurationSeconds)
		}
		if err := s.repo.UpdateRecordingAudioURL(ctx, session.RecordingID, *payload.AudioURL, duration); err != nil {
			return err
		}
		if err := s.repo.UpdateRecordingStatus(ctx, session.RecordingID, models.RecordingStatusTranscribing, nil, nil); err != nil {
			return err
		}

		s.publishProgress(session.RecordingID, models.RecordingStatusTranscribing, nil, nil)

		template := constants.DefaultTemplateKey
		language := "id"
		if rec, err := s.repo.FindRecordingByID(ctx, session.RecordingID); err == nil && rec != nil {
			if rec.SelectedTemplate != "" {
				template = rec.SelectedTemplate
			}
			if rec.OutputLanguage != "" {
				language = rec.OutputLanguage
			}
		}

		nextPayload := queue.RecordingPipelinePayload{
			RecordingID: session.RecordingID,
			AudioPath:   *payload.AudioURL,
			Template:    template,
			Language:    language,
			Stage:       models.RecordingStatusTranscribing,
		}
		return s.publishEvent(ctx, constants.TopicRecordingTranscribe, nextPayload)
	}

	if mappedStatus == constants.BotSessionStatusFailed {
		errCode := constants.ErrCodeBotSessionFailed
		s.publishProgress(session.RecordingID, models.RecordingStatusFailed, &errCode, payload.ErrorMessage)
		return s.repo.UpdateRecordingStatus(ctx, session.RecordingID, models.RecordingStatusFailed, &errCode, payload.ErrorMessage)
	}

	return nil
}

// HandleGoogleMeetWebhook processes asynchronous status updates and completion signals from Google Meet workers.
func (s *Service) HandleGoogleMeetWebhook(ctx context.Context, req dtos.GoogleMeetWebhookRequest, secretHeader string) error {
	if s.cfg.GoogleMeetBotWebhookSecret != "" && secretHeader != s.cfg.GoogleMeetBotWebhookSecret {
		return apperror.New(http.StatusUnauthorized, constants.ErrCodeInvalidWebhookSecret, "invalid webhook secret")
	}

	return s.processBotWebhook(ctx, botWebhookPayload{
		ExternalSessionID: req.ExternalSessionID,
		Event:             req.Event,
		AudioURL:          req.AudioURL,
		DurationSeconds:   req.DurationSeconds,
		ErrorMessage:      req.ErrorMessage,
	})
}

// HandleMSTeamsWebhook processes asynchronous status updates and completion signals from Microsoft Teams / Azure Calling bot workers.
func (s *Service) HandleMSTeamsWebhook(ctx context.Context, req dtos.MSTeamsWebhookRequest, secretHeader string) error {
	if s.cfg.MSTeamsBotWebhookSecret != "" && secretHeader != s.cfg.MSTeamsBotWebhookSecret {
		return apperror.New(http.StatusUnauthorized, constants.ErrCodeInvalidWebhookSecret, "invalid webhook secret")
	}

	return s.processBotWebhook(ctx, botWebhookPayload{
		ExternalSessionID: req.ExternalSessionID,
		Event:             req.Event,
		AudioURL:          req.AudioURL,
		DurationSeconds:   req.DurationSeconds,
		ErrorMessage:      req.ErrorMessage,
	})
}

// HandleZoomWebhook processes asynchronous status updates and completion signals from Zoom bot workers.
func (s *Service) HandleZoomWebhook(ctx context.Context, req dtos.ZoomWebhookRequest, secretHeader string) error {
	if s.cfg.ZoomBotWebhookSecret != "" && secretHeader != s.cfg.ZoomBotWebhookSecret {
		return apperror.New(http.StatusUnauthorized, constants.ErrCodeInvalidWebhookSecret, "invalid webhook secret")
	}

	return s.processBotWebhook(ctx, botWebhookPayload{
		ExternalSessionID: req.ExternalSessionID,
		Event:             req.Event,
		AudioURL:          req.AudioURL,
		DurationSeconds:   req.DurationSeconds,
		ErrorMessage:      req.ErrorMessage,
	})
}
