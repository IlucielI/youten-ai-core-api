package msteams

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/pkg/apperror"
	"code-base-golang/internal/services"
)

// Config encapsulates runtime settings for the Microsoft Teams voice bot adapter.
type Config struct {
	Enabled       bool
	TenantID      string
	ClientID      string
	ClientSecret  string
	WebhookSecret string
	CallbackURL   string
	Publisher     services.EventPublisher
}

type teamsSession struct {
	SessionID  string
	MeetingURL string
	Status     string
	StartedAt  time.Time
	EndedAt    *time.Time
}

// Adapter implements services.BotProvider for Microsoft Teams / Azure Calling Bot.
type Adapter struct {
	cfg      Config
	mu       sync.RWMutex
	sessions map[string]*teamsSession
}

// NewAdapter initializes a new Microsoft Teams bot provider adapter.
func NewAdapter(cfg Config) *Adapter {
	return &Adapter{
		cfg:      cfg,
		sessions: make(map[string]*teamsSession),
	}
}

// ProviderName returns the identifier for Microsoft Teams.
func (a *Adapter) ProviderName() string {
	return constants.BotProviderMSTeams
}

// IsEnabled indicates if Microsoft Teams bot integration is enabled.
func (a *Adapter) IsEnabled() bool {
	return a.cfg.Enabled
}

// ValidateMeetingURL validates and normalizes a Microsoft Teams meeting URL.
func ValidateMeetingURL(rawURL string) error {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return apperror.New(http.StatusBadRequest, "INVALID_MEETING_URL", "meeting URL cannot be empty")
	}

	parsedURL := trimmed
	lowerURL := strings.ToLower(parsedURL)
	if !strings.HasPrefix(lowerURL, "http://") && !strings.HasPrefix(lowerURL, "https://") {
		parsedURL = "https://" + parsedURL
	}

	u, err := url.Parse(parsedURL)
	if err != nil {
		return apperror.New(http.StatusBadRequest, "INVALID_MEETING_URL", "invalid meeting URL format")
	}

	host := strings.ToLower(u.Host)
	if host != "teams.microsoft.com" && host != "teams.live.com" && !strings.HasSuffix(host, ".teams.microsoft.com") {
		return apperror.New(http.StatusBadRequest, "INVALID_MEETING_URL", "host must be teams.microsoft.com or teams.live.com")
	}

	path := strings.ToLower(u.Path)
	if !strings.HasPrefix(path, "/l/meetup-join") && !strings.HasPrefix(path, "/meet") {
		return apperror.New(http.StatusBadRequest, "INVALID_MEETING_URL", "invalid Microsoft Teams meeting path, expected /l/meetup-join/... or /meet/...")
	}

	return nil
}

// Dispatch initiates joining a Microsoft Teams meeting.
func (a *Adapter) Dispatch(ctx context.Context, params services.BotDispatchParams) (string, error) {
	if !a.cfg.Enabled {
		return "", apperror.New(http.StatusServiceUnavailable, "PROVIDER_DISABLED", "microsoft teams voice bot provider is disabled")
	}

	if err := ValidateMeetingURL(params.MeetingURL); err != nil {
		return "", err
	}

	externalID := fmt.Sprintf("teams-%s", uuid.New().String())
	now := time.Now().UTC()

	a.mu.Lock()
	a.sessions[externalID] = &teamsSession{
		SessionID:  externalID,
		MeetingURL: params.MeetingURL,
		Status:     constants.BotSessionStatusWaitingAdmit,
		StartedAt:  now,
	}
	a.mu.Unlock()

	if a.cfg.Publisher != nil {
		eventPayload := map[string]interface{}{
			"session_id":          params.SessionID,
			"external_session_id": externalID,
			"recording_id":        params.RecordingID,
			"meeting_url":         params.MeetingURL,
			"tenant_id":           a.cfg.TenantID,
			"client_id":           a.cfg.ClientID,
			"callback_url":        a.cfg.CallbackURL,
			"dispatched_at":       now,
		}
		_ = a.cfg.Publisher.Publish(ctx, "bot.ms_teams.dispatch", eventPayload)
	}

	return externalID, nil
}

// GetStatus checks the live status of an active Microsoft Teams bot session.
func (a *Adapter) GetStatus(ctx context.Context, externalSessionID string) (*services.BotProviderStatus, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	sess, exists := a.sessions[externalSessionID]
	if !exists {
		return &services.BotProviderStatus{
			ExternalSessionID: externalSessionID,
			Status:            constants.BotSessionStatusCompleted,
		}, nil
	}

	startedAt := sess.StartedAt
	var endedAt *time.Time
	if sess.EndedAt != nil {
		ended := *sess.EndedAt
		endedAt = &ended
	}

	return &services.BotProviderStatus{
		ExternalSessionID: sess.SessionID,
		Status:            sess.Status,
		StartedAt:         &startedAt,
		EndedAt:           endedAt,
	}, nil
}

// Stop orders the Microsoft Teams bot to leave the meeting and end the call.
func (a *Adapter) Stop(ctx context.Context, externalSessionID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	sess, exists := a.sessions[externalSessionID]
	if !exists {
		return nil
	}

	now := time.Now().UTC()
	sess.Status = constants.BotSessionStatusCompleted
	sess.EndedAt = &now

	return nil
}

// UpdateSessionState updates session status from worker webhook callbacks.
func (a *Adapter) UpdateSessionState(externalSessionID string, status string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	sess, exists := a.sessions[externalSessionID]
	if !exists {
		return false
	}

	sess.Status = status
	if status == constants.BotSessionStatusCompleted || status == constants.BotSessionStatusFailed {
		if sess.EndedAt == nil {
			now := time.Now().UTC()
			sess.EndedAt = &now
		}
	}
	return true
}
