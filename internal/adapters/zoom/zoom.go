package zoom

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

// Config encapsulates runtime settings for the Zoom voice bot adapter.
type Config struct {
	Enabled       bool
	WebhookSecret string
	ClientID      string
	ClientSecret  string
	AccountID     string
	Publisher     services.EventPublisher
}

type zoomSession struct {
	SessionID  string
	MeetingURL string
	Status     string
	StartedAt  time.Time
	EndedAt    *time.Time
}

// Adapter implements services.BotProvider for Zoom Meeting voice bot.
type Adapter struct {
	cfg      Config
	mu       sync.RWMutex
	sessions map[string]*zoomSession
}

// NewAdapter initializes a new Zoom bot provider adapter.
func NewAdapter(cfg Config) *Adapter {
	return &Adapter{
		cfg:      cfg,
		sessions: make(map[string]*zoomSession),
	}
}

// ProviderName returns the identifier for Zoom.
func (a *Adapter) ProviderName() string {
	return constants.BotProviderZoom
}

// IsEnabled indicates if Zoom bot integration is enabled.
func (a *Adapter) IsEnabled() bool {
	return a.cfg.Enabled
}

// ValidateMeetingURL validates and normalizes a Zoom meeting URL.
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
	isZoomHost := host == "zoom.us" || strings.HasSuffix(host, ".zoom.us") ||
		host == "zoomgov.com" || strings.HasSuffix(host, ".zoomgov.com")
	if !isZoomHost {
		return apperror.New(http.StatusBadRequest, "INVALID_MEETING_URL", "host must be a valid zoom.us or zoomgov.com domain")
	}

	path := strings.ToLower(u.Path)
	if !strings.HasPrefix(path, "/j/") && !strings.HasPrefix(path, "/my/") {
		return apperror.New(http.StatusBadRequest, "INVALID_MEETING_URL", "invalid Zoom meeting path, expected /j/<meeting_id> or /my/<vanity_name>")
	}

	// Ensure there is an identifier after /j/ or /my/
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
		return apperror.New(http.StatusBadRequest, "INVALID_MEETING_URL", "meeting ID or vanity name cannot be empty")
	}

	return nil
}

// Dispatch initiates joining a Zoom meeting.
func (a *Adapter) Dispatch(ctx context.Context, params services.BotDispatchParams) (string, error) {
	if !a.cfg.Enabled {
		return "", apperror.New(http.StatusServiceUnavailable, "PROVIDER_DISABLED", "zoom voice bot provider is disabled")
	}

	if err := ValidateMeetingURL(params.MeetingURL); err != nil {
		return "", err
	}

	externalID := fmt.Sprintf("zoom-%s", uuid.New().String())
	now := time.Now().UTC()

	a.mu.Lock()
	a.sessions[externalID] = &zoomSession{
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
			"client_id":           a.cfg.ClientID,
			"account_id":          a.cfg.AccountID,
			"dispatched_at":       now,
		}
		_ = a.cfg.Publisher.Publish(ctx, "bot.zoom.dispatch", eventPayload)
	}

	return externalID, nil
}

// GetStatus checks the live status of an active Zoom bot session.
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

// Stop orders the Zoom bot to leave the meeting.
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
