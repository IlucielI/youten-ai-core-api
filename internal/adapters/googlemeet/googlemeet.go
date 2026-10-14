package googlemeet

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/pkg/apperror"
	"code-base-golang/internal/queue"
	"code-base-golang/internal/services"
)

var (
	// Google Meet code regex: 3 letters - 4 letters - 3 letters (e.g. abc-defg-hij)
	googleMeetCodeRegex = regexp.MustCompile(`^[a-z]{3}-[a-z]{4}-[a-z]{3}$`)
)

// Config encapsulates runtime settings for the Google Meet voice bot adapter.
type Config struct {
	Enabled       bool
	WebhookSecret string
	Publisher     services.EventPublisher
}

type meetSession struct {
	SessionID  string
	MeetingURL string
	Status     string
	StartedAt  time.Time
	EndedAt    *time.Time
}

// Adapter implements services.BotProvider for Google Meet.
type Adapter struct {
	cfg      Config
	mu       sync.RWMutex
	sessions map[string]*meetSession
}

// NewAdapter initializes a new Google Meet bot provider adapter.
func NewAdapter(cfg Config) *Adapter {
	return &Adapter{
		cfg:      cfg,
		sessions: make(map[string]*meetSession),
	}
}

// ProviderName returns the identifier for Google Meet.
func (a *Adapter) ProviderName() string {
	return constants.BotProviderGoogleMeet
}

// IsEnabled indicates if Google Meet bot integration is enabled.
func (a *Adapter) IsEnabled() bool {
	return a.cfg.Enabled
}

// ValidateMeetingURL validates and normalizes a Google Meet URL.
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
	if host != "meet.google.com" {
		return apperror.New(http.StatusBadRequest, "INVALID_MEETING_URL", "host must be meet.google.com")
	}

	path := strings.ToLower(strings.Trim(u.Path, "/"))
	if !googleMeetCodeRegex.MatchString(path) {
		return apperror.New(http.StatusBadRequest, "INVALID_MEETING_URL", "invalid Google Meet meeting code format, expected abc-defg-hij")
	}

	return nil
}

// Dispatch initiates joining a Google Meet room.
func (a *Adapter) Dispatch(ctx context.Context, params dtos.BotDispatchParams) (string, error) {
	if !a.cfg.Enabled {
		return "", apperror.New(http.StatusServiceUnavailable, "PROVIDER_DISABLED", "google meet voice bot provider is disabled")
	}

	if err := ValidateMeetingURL(params.MeetingURL); err != nil {
		return "", err
	}

	externalID := fmt.Sprintf("meet-%s", uuid.New().String())
	now := time.Now().UTC()

	a.mu.Lock()
	a.sessions[externalID] = &meetSession{
		SessionID:  externalID,
		MeetingURL: params.MeetingURL,
		Status:     constants.BotSessionStatusWaitingAdmit,
		StartedAt:  now,
	}
	a.mu.Unlock()

	if a.cfg.Publisher != nil {
		eventPayload := queue.BotDispatchPayload{
			SessionID:         params.SessionID,
			RecordingID:       params.RecordingID,
			Provider:          constants.BotProviderGoogleMeet,
			MeetingURL:        params.MeetingURL,
			ExternalSessionID: externalID,
			DispatchedAt:      now,
			ScheduledAt:       params.ScheduledAt,
		}
		if err := a.cfg.Publisher.Publish(ctx, constants.TopicBotGoogleMeetDispatch, eventPayload); err != nil {
			log.Printf("[GOOGLE_MEET] Error publishing bot dispatch event for %s: %v", externalID, err)
			return "", fmt.Errorf("failed to publish bot dispatch event to queue: %w", err)
		}
		log.Printf("[GOOGLE_MEET] Successfully published bot dispatch event for %s to %s", externalID, constants.TopicBotGoogleMeetDispatch)
	}

	return externalID, nil
}

// GetStatus checks the live status of an active Google Meet bot session.
func (a *Adapter) GetStatus(ctx context.Context, externalSessionID string) (*dtos.BotProviderStatus, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	sess, exists := a.sessions[externalSessionID]
	if !exists {
		return &dtos.BotProviderStatus{
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

	return &dtos.BotProviderStatus{
		ExternalSessionID: sess.SessionID,
		Status:            sess.Status,
		StartedAt:         &startedAt,
		EndedAt:           endedAt,
	}, nil
}

// Stop orders the Google Meet bot to leave the meeting and end the session.
func (a *Adapter) Stop(ctx context.Context, externalSessionID string) error {
	a.mu.Lock()
	sess, exists := a.sessions[externalSessionID]
	now := time.Now().UTC()
	if exists {
		sess.Status = constants.BotSessionStatusCompleted
		sess.EndedAt = &now
	}
	a.mu.Unlock()

	if a.cfg.Publisher != nil {
		stopPayload := queue.BotStopPayload{
			ExternalSessionID: externalSessionID,
			Reason:            "user_requested_stop",
			StoppedAt:         now,
		}
		_ = a.cfg.Publisher.Publish(ctx, constants.TopicBotGoogleMeetStop, stopPayload)
	}

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
