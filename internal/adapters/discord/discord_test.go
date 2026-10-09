package discord

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/services"
)

func TestDiscordAdapter(t *testing.T) {
	ctx := context.Background()

	t.Run("disabled when config is false or token is empty", func(t *testing.T) {
		adapter := NewAdapter(Config{Enabled: false, BotToken: ""})
		if adapter.IsEnabled() {
			t.Errorf("expected adapter to be disabled")
		}
		if adapter.ProviderName() != constants.BotProviderDiscord {
			t.Errorf("expected provider name %s, got %s", constants.BotProviderDiscord, adapter.ProviderName())
		}

		_, err := adapter.Dispatch(ctx, services.BotDispatchParams{
			ChannelID: "123",
		})
		if err == nil {
			t.Errorf("expected error when dispatching on disabled adapter")
		}
	})

	t.Run("dispatch, status, and stop lifecycle", func(t *testing.T) {
		adapter := NewAdapter(Config{Enabled: true, BotToken: "valid-bot-token"})
		if !adapter.IsEnabled() {
			t.Fatalf("expected adapter to be enabled")
		}

		recID := uuid.New()
		extID, err := adapter.Dispatch(ctx, services.BotDispatchParams{
			RecordingID: recID,
			GuildID:     "guild-1",
			ChannelID:   "chan-1",
		})
		if err != nil {
			t.Fatalf("unexpected dispatch error: %v", err)
		}
		if extID == "" {
			t.Fatalf("expected non-empty external ID")
		}

		status, err := adapter.GetStatus(ctx, extID)
		if err != nil {
			t.Fatalf("unexpected get status error: %v", err)
		}
		if status.Status != constants.BotSessionStatusRecording {
			t.Errorf("expected status %s, got %s", constants.BotSessionStatusRecording, status.Status)
		}

		err = adapter.Stop(ctx, extID)
		if err != nil {
			t.Fatalf("unexpected stop error: %v", err)
		}

		statusAfterStop, err := adapter.GetStatus(ctx, extID)
		if err != nil {
			t.Fatalf("unexpected get status after stop error: %v", err)
		}
		if statusAfterStop.Status != constants.BotSessionStatusCompleted {
			t.Errorf("expected status %s, got %s", constants.BotSessionStatusCompleted, statusAfterStop.Status)
		}
	})

	t.Run("dispatch missing channel id returns validation error", func(t *testing.T) {
		adapter := NewAdapter(Config{Enabled: true, BotToken: "valid-bot-token"})
		_, err := adapter.Dispatch(ctx, services.BotDispatchParams{
			ChannelID: "",
		})
		if err == nil {
			t.Errorf("expected validation error when channel ID is empty")
		}
	})

	t.Run("concurrent status and stop access", func(t *testing.T) {
		adapter := NewAdapter(Config{Enabled: true, BotToken: "valid-bot-token"})
		recID := uuid.New()
		extID, err := adapter.Dispatch(ctx, services.BotDispatchParams{
			RecordingID: recID,
			GuildID:     "guild-1",
			ChannelID:   "chan-1",
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
				if err := adapter.Stop(ctx, extID); err != nil {
					t.Errorf("unexpected stop error: %v", err)
				}
			}
			done <- true
		}()
		<-done
		<-done
	})
}
