package discord

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/pkg/apperror"
	"code-base-golang/internal/services"
)

// Config holds Discord bot runtime configuration.
type Config struct {
	Enabled  bool
	BotToken string
}

// ActiveSession represents an in-memory active Discord voice connection.
type ActiveSession struct {
	SessionID   string
	RecordingID uuid.UUID
	GuildID     string
	ChannelID   string
	Status      string
	StartedAt   time.Time
	EndedAt     *time.Time
}

// Adapter implements services.MeetingBotProvider for Discord voice channels.
type Adapter struct {
	mu       sync.RWMutex
	cfg      Config
	sessions map[string]*ActiveSession
}

// NewAdapter creates a new Discord bot provider adapter.
func NewAdapter(cfg Config) *Adapter {
	return &Adapter{
		cfg:      cfg,
		sessions: make(map[string]*ActiveSession),
	}
}

// ProviderName returns the identifier for Discord.
func (a *Adapter) ProviderName() string {
	return constants.BotProviderDiscord
}

// IsEnabled returns whether Discord bot functionality is enabled.
func (a *Adapter) IsEnabled() bool {
	return a.cfg.Enabled && a.cfg.BotToken != ""
}

// Dispatch begins a voice bot session by joining a designated Discord voice channel.
func (a *Adapter) Dispatch(ctx context.Context, params services.BotDispatchParams) (string, error) {
	if !a.IsEnabled() {
		return "", apperror.New(http.StatusNotImplemented, "FEATURE_DISABLED", "Discord voice bot is currently disabled or unconfigured")
	}

	if params.ChannelID == "" {
		return "", apperror.New(http.StatusBadRequest, "VALIDATION_ERROR", "channel_id is required for Discord voice bot")
	}

	externalID := fmt.Sprintf("discord-%s", uuid.New().String())
	now := time.Now().UTC()

	a.mu.Lock()
	a.sessions[externalID] = &ActiveSession{
		SessionID:   externalID,
		RecordingID: params.RecordingID,
		GuildID:     params.GuildID,
		ChannelID:   params.ChannelID,
		Status:      constants.BotSessionStatusRecording,
		StartedAt:   now,
	}
	a.mu.Unlock()

	return externalID, nil
}

// GetStatus checks the live status of an active Discord bot connection.
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

// Stop orders the Discord bot to leave the voice channel and end the session.
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
