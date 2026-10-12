package services

import (
	"context"

	"code-base-golang/internal/dtos"
)

// MeetingBotProvider is the consumer interface (port) for voice bot providers.
// Concrete implementations (Discord, Google Meet, Teams, Zoom) live in internal/adapters/.
type MeetingBotProvider interface {
	ProviderName() string
	IsEnabled() bool
	Dispatch(ctx context.Context, params dtos.BotDispatchParams) (externalSessionID string, err error)
	GetStatus(ctx context.Context, externalSessionID string) (*dtos.BotProviderStatus, error)
	Stop(ctx context.Context, externalSessionID string) error
}
