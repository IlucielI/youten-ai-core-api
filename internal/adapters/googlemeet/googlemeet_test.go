package googlemeet

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/services"
)

type mockPublisher struct {
	publishedTopic string
	publishedBody  interface{}
	publishCalls   int
}

func (m *mockPublisher) Ping(ctx context.Context) error {
	return nil
}

func (m *mockPublisher) Publish(ctx context.Context, topic string, payload any) error {
	m.publishedTopic = topic
	m.publishedBody = payload
	m.publishCalls++
	return nil
}

func (m *mockPublisher) MustPublish(ctx context.Context, topic string, payload any) {
	_ = m.Publish(ctx, topic, payload)
}

func TestGoogleMeetAdapter(t *testing.T) {
	ctx := context.Background()

	t.Run("provider name and is enabled", func(t *testing.T) {
		disabledAdapter := NewAdapter(Config{Enabled: false})
		if disabledAdapter.ProviderName() != constants.BotProviderGoogleMeet {
			t.Errorf("expected provider name %s, got %s", constants.BotProviderGoogleMeet, disabledAdapter.ProviderName())
		}
		if disabledAdapter.IsEnabled() {
			t.Errorf("expected IsEnabled false, got true")
		}

		enabledAdapter := NewAdapter(Config{Enabled: true})
		if !enabledAdapter.IsEnabled() {
			t.Errorf("expected IsEnabled true, got false")
		}
	})

	t.Run("validate meeting url", func(t *testing.T) {
		validURLs := []string{
			"https://meet.google.com/abc-defg-hij",
			"HTTPS://meet.google.com/abc-defg-hij",
			"http://meet.google.com/xyz-uvwx-rst",
			"meet.google.com/aaa-bbbb-ccc",
			"https://meet.google.com/ABC-DEFG-HIJ",
		}

		for _, u := range validURLs {
			if err := ValidateMeetingURL(u); err != nil {
				t.Errorf("expected URL %q to be valid, got error: %v", u, err)
			}
		}

		invalidURLs := []string{
			"",
			"   ",
			"https://zoom.us/j/1234567890",
			"https://meet.google.com/invalid-code",
			"https://meet.google.com/abc-def-ghi",
			"https://other.com/abc-defg-hij",
			"https://meet.google.com/",
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
			MeetingURL: "https://meet.google.com/abc-defg-hij",
		})
		if err == nil {
			t.Errorf("expected error when adapter is disabled")
		}
	})

	t.Run("dispatch invalid url", func(t *testing.T) {
		adapter := NewAdapter(Config{Enabled: true})
		_, err := adapter.Dispatch(ctx, services.BotDispatchParams{
			MeetingURL: "https://zoom.us/j/12345",
		})
		if err == nil {
			t.Errorf("expected error on invalid meeting url")
		}
	})

	t.Run("dispatch success with publisher", func(t *testing.T) {
		pub := &mockPublisher{}
		adapter := NewAdapter(Config{Enabled: true, Publisher: pub})
		recID := uuid.New()

		extID, err := adapter.Dispatch(ctx, services.BotDispatchParams{
			RecordingID: recID,
			MeetingURL:  "https://meet.google.com/abc-defg-hij",
		})
		if err != nil {
			t.Fatalf("unexpected dispatch error: %v", err)
		}

		if !strings.HasPrefix(extID, "meet-") {
			t.Errorf("expected prefix meet-, got %s", extID)
		}

		if pub.publishCalls != 1 {
			t.Errorf("expected 1 publish call, got %d", pub.publishCalls)
		}
		if pub.publishedTopic != "bot.google_meet.dispatch" {
			t.Errorf("expected topic bot.google_meet.dispatch, got %s", pub.publishedTopic)
		}

		// Verify GetStatus
		status, err := adapter.GetStatus(ctx, extID)
		if err != nil {
			t.Fatalf("unexpected get status error: %v", err)
		}
		if status == nil || status.Status != constants.BotSessionStatusWaitingAdmit {
			t.Errorf("expected status %s, got %v", constants.BotSessionStatusWaitingAdmit, status)
		}
		if status.StartedAt == nil {
			t.Errorf("expected non-nil StartedAt")
		}

		// UpdateSessionState
		updated := adapter.UpdateSessionState(extID, constants.BotSessionStatusRecording)
		if !updated {
			t.Errorf("expected update to succeed")
		}

		status, err = adapter.GetStatus(ctx, extID)
		if err != nil {
			t.Fatalf("unexpected get status error: %v", err)
		}
		if status.Status != constants.BotSessionStatusRecording {
			t.Errorf("expected status %s, got %s", constants.BotSessionStatusRecording, status.Status)
		}

		// Stop session
		if err := adapter.Stop(ctx, extID); err != nil {
			t.Fatalf("unexpected stop error: %v", err)
		}

		status, err = adapter.GetStatus(ctx, extID)
		if err != nil {
			t.Fatalf("unexpected get status error: %v", err)
		}
		if status.Status != constants.BotSessionStatusCompleted {
			t.Errorf("expected status %s, got %s", constants.BotSessionStatusCompleted, status.Status)
		}
		if status.EndedAt == nil {
			t.Errorf("expected non-nil EndedAt after stop")
		}
	})

	t.Run("non-existent session status and stop", func(t *testing.T) {
		adapter := NewAdapter(Config{Enabled: true})

		status, err := adapter.GetStatus(ctx, "non-existent")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status.Status != constants.BotSessionStatusCompleted {
			t.Errorf("expected status completed for unknown session, got %s", status.Status)
		}

		if err := adapter.Stop(ctx, "non-existent"); err != nil {
			t.Errorf("unexpected error stopping unknown session: %v", err)
		}

		if adapter.UpdateSessionState("non-existent", constants.BotSessionStatusRecording) {
			t.Errorf("expected UpdateSessionState on non-existent session to return false")
		}
	})

	t.Run("concurrent status, update, and stop access", func(t *testing.T) {
		adapter := NewAdapter(Config{Enabled: true})
		extID, err := adapter.Dispatch(ctx, services.BotDispatchParams{
			RecordingID: uuid.New(),
			MeetingURL:  "https://meet.google.com/abc-defg-hij",
		})
		if err != nil {
			t.Fatalf("unexpected dispatch error: %v", err)
		}

		done := make(chan bool)
		go func() {
			for i := 0; i < 50; i++ {
				status, err := adapter.GetStatus(ctx, extID)
				if err != nil {
					t.Errorf("unexpected get status error: %v", err)
				}
				if status == nil {
					t.Errorf("expected non-nil status")
				}
			}
			done <- true
		}()
		go func() {
			for i := 0; i < 50; i++ {
				adapter.UpdateSessionState(extID, constants.BotSessionStatusRecording)
			}
			done <- true
		}()
		go func() {
			for i := 0; i < 50; i++ {
				if err := adapter.Stop(ctx, extID); err != nil {
					t.Errorf("unexpected stop error: %v", err)
				}
			}
			done <- true
		}()
		<-done
		<-done
		<-done
	})
}
