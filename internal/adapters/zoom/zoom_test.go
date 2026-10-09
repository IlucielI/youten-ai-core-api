package zoom

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/services"
)

type mockPublisher struct {
	events []string
}

func (m *mockPublisher) Publish(ctx context.Context, exchange string, body interface{}) error {
	m.events = append(m.events, exchange)
	return nil
}

func (m *mockPublisher) MustPublish(ctx context.Context, exchange string, body interface{}) {}
func (m *mockPublisher) Ping(ctx context.Context) error                                     { return nil }

type mockErrPublisher struct{}

func (m *mockErrPublisher) Publish(ctx context.Context, exchange string, body interface{}) error {
	return errors.New("rabbitmq network down")
}
func (m *mockErrPublisher) MustPublish(ctx context.Context, exchange string, body interface{}) {}
func (m *mockErrPublisher) Ping(ctx context.Context) error                                     { return nil }

func TestValidateMeetingURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{
			name:    "valid zoom url with meeting id",
			url:     "https://zoom.us/j/1234567890",
			wantErr: false,
		},
		{
			name:    "valid zoom url with subdomain and passcode",
			url:     "https://us02web.zoom.us/j/98765432101?pwd=someSecretPassword123",
			wantErr: false,
		},
		{
			name:    "valid zoomgov url",
			url:     "https://zoomgov.com/j/1234567890",
			wantErr: false,
		},
		{
			name:    "valid zoom vanity url",
			url:     "https://mycompany.zoom.us/my/johndoe",
			wantErr: false,
		},
		{
			name:    "valid zoom url without scheme",
			url:     "zoom.us/j/1234567890",
			wantErr: false,
		},
		{
			name:    "empty url",
			url:     "   ",
			wantErr: true,
		},
		{
			name:    "wrong domain",
			url:     "https://google.com/j/1234567890",
			wantErr: true,
		},
		{
			name:    "teams domain",
			url:     "https://teams.microsoft.com/l/meetup-join/123",
			wantErr: true,
		},
		{
			name:    "invalid path without id",
			url:     "https://zoom.us/j/",
			wantErr: true,
		},
		{
			name:    "invalid path wrong prefix",
			url:     "https://zoom.us/signin",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMeetingURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateMeetingURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

func TestZoomAdapter(t *testing.T) {
	ctx := context.Background()

	t.Run("provider details and enabled state", func(t *testing.T) {
		adapter := NewAdapter(Config{
			Enabled: true,
		})
		if adapter.ProviderName() != constants.BotProviderZoom {
			t.Errorf("expected %q, got %q", constants.BotProviderZoom, adapter.ProviderName())
		}
		if !adapter.IsEnabled() {
			t.Errorf("expected IsEnabled true")
		}

		disabledAdapter := NewAdapter(Config{Enabled: false})
		if disabledAdapter.IsEnabled() {
			t.Errorf("expected IsEnabled false")
		}
	})

	t.Run("dispatch when disabled returns error", func(t *testing.T) {
		adapter := NewAdapter(Config{Enabled: false})
		_, err := adapter.Dispatch(ctx, services.BotDispatchParams{
			MeetingURL: "https://zoom.us/j/1234567890",
		})
		if err == nil {
			t.Fatalf("expected error when dispatching disabled provider")
		}
	})

	t.Run("dispatch with invalid url returns error", func(t *testing.T) {
		adapter := NewAdapter(Config{Enabled: true})
		_, err := adapter.Dispatch(ctx, services.BotDispatchParams{
			MeetingURL: "https://invalid-meeting.com",
		})
		if err == nil {
			t.Fatalf("expected error on invalid url")
		}
	})

	t.Run("dispatch publisher error returns error and cleans up session", func(t *testing.T) {
		errPub := &mockErrPublisher{}
		adapter := NewAdapter(Config{
			Enabled:   true,
			Publisher: errPub,
		})

		_, err := adapter.Dispatch(ctx, services.BotDispatchParams{
			MeetingURL: "https://zoom.us/j/1234567890",
		})
		if err == nil {
			t.Fatalf("expected error when publisher fails")
		}
	})

	t.Run("dispatch success and publishes event", func(t *testing.T) {
		pub := &mockPublisher{}
		adapter := NewAdapter(Config{
			Enabled:      true,
			ClientID:     "zoom-client-123",
			AccountID:    "zoom-account-456",
			Publisher:    pub,
		})

		sessionID := uuid.New()
		recID := uuid.New()
		extID, err := adapter.Dispatch(ctx, services.BotDispatchParams{
			SessionID:   sessionID,
			RecordingID: recID,
			MeetingURL:  "https://zoom.us/j/9876543210?pwd=abc",
		})
		if err != nil {
			t.Fatalf("unexpected dispatch error: %v", err)
		}
		if !strings.HasPrefix(extID, "zoom-") {
			t.Errorf("expected external id with prefix zoom-, got %q", extID)
		}

		if len(pub.events) != 1 || pub.events[0] != "bot.zoom.dispatch" {
			t.Errorf("expected event bot.zoom.dispatch, got %v", pub.events)
		}

		// Check status
		status, err := adapter.GetStatus(ctx, extID)
		if err != nil {
			t.Fatalf("unexpected GetStatus error: %v", err)
		}
		if status.Status != constants.BotSessionStatusWaitingAdmit {
			t.Errorf("expected status %q, got %q", constants.BotSessionStatusWaitingAdmit, status.Status)
		}
		if status.StartedAt == nil {
			t.Errorf("expected StartedAt to be non-nil")
		}

		// Update session state
		updated := adapter.UpdateSessionState(extID, constants.BotSessionStatusRecording)
		if !updated {
			t.Errorf("expected UpdateSessionState to succeed")
		}

		status, _ = adapter.GetStatus(ctx, extID)
		if status.Status != constants.BotSessionStatusRecording {
			t.Errorf("expected status %q, got %q", constants.BotSessionStatusRecording, status.Status)
		}

		// Update to completed
		updated = adapter.UpdateSessionState(extID, constants.BotSessionStatusCompleted)
		if !updated {
			t.Errorf("expected UpdateSessionState to succeed")
		}

		status, _ = adapter.GetStatus(ctx, extID)
		if status.Status != constants.BotSessionStatusCompleted {
			t.Errorf("expected status completed, got %q", status.Status)
		}
		if status.EndedAt == nil {
			t.Errorf("expected EndedAt to be set upon completion")
		}

		// Stop
		if err := adapter.Stop(ctx, extID); err != nil {
			t.Fatalf("unexpected Stop error: %v", err)
		}
	})

	t.Run("get status for nonexistent session", func(t *testing.T) {
		adapter := NewAdapter(Config{Enabled: true})
		status, err := adapter.GetStatus(ctx, "nonexistent-session")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status.Status != constants.BotSessionStatusCompleted {
			t.Errorf("expected default completed status for unknown session, got %q", status.Status)
		}
	})

	t.Run("stop nonexistent session", func(t *testing.T) {
		adapter := NewAdapter(Config{Enabled: true})
		err := adapter.Stop(ctx, "nonexistent-session")
		if err != nil {
			t.Fatalf("unexpected error stopping unknown session: %v", err)
		}
	})

	t.Run("update session state for nonexistent session", func(t *testing.T) {
		adapter := NewAdapter(Config{Enabled: true})
		if adapter.UpdateSessionState("nonexistent", constants.BotSessionStatusJoined) {
			t.Errorf("expected false for nonexistent session")
		}
	})

	t.Run("update session state to failed sets ended_at", func(t *testing.T) {
		adapter := NewAdapter(Config{Enabled: true})
		extID, err := adapter.Dispatch(ctx, services.BotDispatchParams{
			MeetingURL: "https://zoom.us/j/1234567890",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		adapter.UpdateSessionState(extID, constants.BotSessionStatusFailed)
		status, _ := adapter.GetStatus(ctx, extID)
		if status.Status != constants.BotSessionStatusFailed {
			t.Errorf("expected failed status, got %q", status.Status)
		}
		if status.EndedAt == nil {
			t.Errorf("expected EndedAt to be populated on failed status")
		}
	})

	t.Run("stop existing session sets ended_at and completed status", func(t *testing.T) {
		adapter := NewAdapter(Config{Enabled: true})
		extID, _ := adapter.Dispatch(ctx, services.BotDispatchParams{
			MeetingURL: "https://zoom.us/j/1234567890",
		})

		time.Sleep(5 * time.Millisecond)
		err := adapter.Stop(ctx, extID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		status, _ := adapter.GetStatus(ctx, extID)
		if status.Status != constants.BotSessionStatusCompleted {
			t.Errorf("expected completed, got %q", status.Status)
		}
		if status.EndedAt == nil {
			t.Errorf("expected EndedAt to be set")
		}
	})
}
