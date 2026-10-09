package meetingbot

import (
	"context"
	"fmt"
	"net/http"

	"code-base-golang/internal/pkg/apperror"
	"code-base-golang/internal/services"
)

// StubAdapter provides a no-op placeholder for future bot providers (Google Meet, Teams, Zoom).
type StubAdapter struct {
	providerName string
}

// NewStubAdapter creates a stub bot provider returning FEATURE_COMING_SOON.
func NewStubAdapter(name string) *StubAdapter {
	return &StubAdapter{providerName: name}
}

// ProviderName returns the identifier for this stub provider.
func (s *StubAdapter) ProviderName() string {
	return s.providerName
}

// IsEnabled returns false for stub providers.
func (s *StubAdapter) IsEnabled() bool {
	return false
}

// Dispatch returns 501 Not Implemented.
func (s *StubAdapter) Dispatch(ctx context.Context, params services.BotDispatchParams) (string, error) {
	return "", apperror.New(http.StatusNotImplemented, "FEATURE_COMING_SOON", fmt.Sprintf("%s voice bot integration is coming soon", s.providerName))
}

// GetStatus returns 501 Not Implemented.
func (s *StubAdapter) GetStatus(ctx context.Context, externalSessionID string) (*services.BotProviderStatus, error) {
	return nil, apperror.New(http.StatusNotImplemented, "FEATURE_COMING_SOON", fmt.Sprintf("%s voice bot integration is coming soon", s.providerName))
}

// Stop returns 501 Not Implemented.
func (s *StubAdapter) Stop(ctx context.Context, externalSessionID string) error {
	return apperror.New(http.StatusNotImplemented, "FEATURE_COMING_SOON", fmt.Sprintf("%s voice bot integration is coming soon", s.providerName))
}
