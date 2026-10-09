package services

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// BotDispatchParams contains provider-level parameters for dispatching a bot.
type BotDispatchParams struct {
	SessionID   uuid.UUID
	RecordingID uuid.UUID
	Provider    string
	MeetingURL  string
	GuildID     string
	ChannelID   string
	ScheduledAt *time.Time
}

// BotProviderStatus contains real-time status reported by a bot provider.
type BotProviderStatus struct {
	ExternalSessionID string
	Status            string
	ErrorMessage      string
	StartedAt         *time.Time
	EndedAt           *time.Time
}

// MeetingBotProvider is the consumer interface (port) for voice bot providers.
// Concrete implementations (Discord, Google Meet, Teams, Zoom) live in internal/adapters/.
type MeetingBotProvider interface {
	ProviderName() string
	IsEnabled() bool
	Dispatch(ctx context.Context, params BotDispatchParams) (externalSessionID string, err error)
	GetStatus(ctx context.Context, externalSessionID string) (*BotProviderStatus, error)
	Stop(ctx context.Context, externalSessionID string) error
}
