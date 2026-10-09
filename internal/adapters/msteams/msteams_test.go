package msteams

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/services"
)

type mockPublisher struct {
	published []publishedEvent
}

type publishedEvent struct {
	topic   string
	payload interface{}
}

func (m *mockPublisher) Publish(ctx context.Context, topic string, payload interface{}) error {
	m.published = append(m.published, publishedEvent{topic: topic, payload: payload})
	return nil
}

func (m *mockPublisher) MustPublish(ctx context.Context, topic string, payload interface{}) {
	_ = m.Publish(ctx, topic, payload)
}

func (m *mockPublisher) Ping(ctx context.Context) error {
	return nil
}

func TestMSTeamsAdapter(t *testing.T) {
	ctx := context.Background()

	t.Run("provider metadata", func(t *testing.T) {
		adapter := NewAdapter(Config{Enabled: true})
		if adapter.ProviderName() != constants.BotProviderMSTeams {
			t.Errorf("expected provider name %s, got %s", constants.BotProviderMSTeams, adapter.ProviderName())
		}
		if !adapter.IsEnabled() {
			t.Error("expected adapter to be enabled")
		}
	})

	t.Run("validate meeting url", func(t *testing.T) {
		validURLs := []string{
			"https://teams.microsoft.com/l/meetup-join/19%3ameeting_xyz",
			"HTTPS://teams.microsoft.com/l/meetup-join/19%3ameeting_xyz",
			"http://teams.microsoft.com/l/meetup-join/abc",
			"teams.microsoft.com/l/meetup-join/test",
			"https://teams.live.com/meet/1234567890",
			"https://gov.teams.microsoft.com/l/meetup-join/gov123",
		}

		for _, u := range validURLs {
			if err := ValidateMeetingURL(u); err != nil {
				t.Errorf("expected URL %q to be valid, got error: %v", u, err)
			}
		}

		invalidURLs := []string{
			"",
			"   ",
			"https://meet.google.com/abc-defg-hij",
			"https://zoom.us/j/1234567890",
			"https://teams.microsoft.com/invalid-path",
			"https://other.com/l/meetup-join/abc",
			"https://teams.microsoft.com/",
		}

		for _, u := range invalidURLs {
			if err := ValidateMeetingURL(u); err == nil {
				t.Errorf("expected URL %q to be invalid, got nil error", u)
			}
		}
	})

	t.Run("dispatch when disabled", func(t *testing.T) {
		adapter := NewAdapter(Config{Enabled: false})
		_, err := adapter.Dispatch(ctx, services.BotDispatchParams{
			MeetingURL: "https://teams.microsoft.com/l/meetup-join/abc",
		})
		if err == nil {
			t.Fatal("expected error when adapter is disabled")
		}
		if !strings.Contains(err.Error(), "disabled") {
			t.Errorf("expected disabled error message, got: %v", err)
		}
	})

	t.Run("dispatch with invalid url", func(t *testing.T) {
		adapter := NewAdapter(Config{Enabled: true})
		_, err := adapter.Dispatch(ctx, services.BotDispatchParams{
			MeetingURL: "https://invalid.com",
		})
		if err == nil {
			t.Fatal("expected error for invalid meeting URL")
		}
	})

	t.Run("dispatch success and event publication", func(t *testing.T) {
		pub := &mockPublisher{}
		adapter := NewAdapter(Config{
			Enabled:      true,
			TenantID:     "tenant-1",
			ClientID:     "client-1",
			CallbackURL:  "https://example.com/callback",
			Publisher:    pub,
		})

		sessID := uuid.New()
		recID := uuid.New()
		url := "https://teams.microsoft.com/l/meetup-join/19%3ameeting_abc"

		extID, err := adapter.Dispatch(ctx, services.BotDispatchParams{
			SessionID:   sessID,
			RecordingID: recID,
			MeetingURL:  url,
		})
		if err != nil {
			t.Fatalf("unexpected dispatch error: %v", err)
		}

		if !strings.HasPrefix(extID, "teams-") {
			t.Errorf("expected prefix teams-, got: %s", extID)
		}

		if len(pub.published) != 1 {
			t.Fatalf("expected 1 published event, got %d", len(pub.published))
		}
		if pub.published[0].topic != "bot.ms_teams.dispatch" {
			t.Errorf("expected topic bot.ms_teams.dispatch, got: %s", pub.published[0].topic)
		}

		// Check status
		status, err := adapter.GetStatus(ctx, extID)
		if err != nil {
			t.Fatalf("unexpected get status error: %v", err)
		}
		if status.Status != constants.BotSessionStatusWaitingAdmit {
			t.Errorf("expected status %s, got %s", constants.BotSessionStatusWaitingAdmit, status.Status)
		}
		if status.StartedAt == nil {
			t.Error("expected StartedAt to be populated")
		}
	})

	t.Run("get status for nonexistent session", func(t *testing.T) {
		adapter := NewAdapter(Config{Enabled: true})
		status, err := adapter.GetStatus(ctx, "teams-nonexistent")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status.Status != constants.BotSessionStatusCompleted {
			t.Errorf("expected default completed status, got: %s", status.Status)
		}
	})

	t.Run("stop active session", func(t *testing.T) {
		adapter := NewAdapter(Config{Enabled: true})
		extID, err := adapter.Dispatch(ctx, services.BotDispatchParams{
			MeetingURL: "https://teams.microsoft.com/l/meetup-join/active",
		})
		if err != nil {
			t.Fatalf("unexpected dispatch error: %v", err)
		}

		if err := adapter.Stop(ctx, extID); err != nil {
			t.Fatalf("unexpected stop error: %v", err)
		}

		status, err := adapter.GetStatus(ctx, extID)
		if err != nil {
			t.Fatalf("unexpected get status error: %v", err)
		}
		if status.Status != constants.BotSessionStatusCompleted {
			t.Errorf("expected status %s, got %s", constants.BotSessionStatusCompleted, status.Status)
		}
		if status.EndedAt == nil {
			t.Error("expected EndedAt to be set after stop")
		}

		// Stopping again or non-existent does not error
		if err := adapter.Stop(ctx, "teams-other"); err != nil {
			t.Errorf("expected nil error on nonexistent stop, got: %v", err)
		}
	})

	t.Run("update session state", func(t *testing.T) {
		adapter := NewAdapter(Config{Enabled: true})
		extID, err := adapter.Dispatch(ctx, services.BotDispatchParams{
			MeetingURL: "https://teams.live.com/meet/test12345",
		})
		if err != nil {
			t.Fatalf("unexpected dispatch error: %v", err)
		}

		ok := adapter.UpdateSessionState(extID, constants.BotSessionStatusRecording)
		if !ok {
			t.Error("expected UpdateSessionState to succeed")
		}

		status, _ := adapter.GetStatus(ctx, extID)
		if status.Status != constants.BotSessionStatusRecording {
			t.Errorf("expected recording status, got %s", status.Status)
		}

		// Updating to completed sets ended at
		time.Sleep(2 * time.Millisecond)
		adapter.UpdateSessionState(extID, constants.BotSessionStatusCompleted)
		status, _ = adapter.GetStatus(ctx, extID)
		if status.Status != constants.BotSessionStatusCompleted {
			t.Errorf("expected completed status, got %s", status.Status)
		}
		if status.EndedAt == nil {
			t.Error("expected EndedAt to be populated on completed")
		}

		// Unknown session returns false
		if adapter.UpdateSessionState("teams-unknown", constants.BotSessionStatusFailed) {
			t.Error("expected UpdateSessionState to return false for unknown session")
		}
	})
}
