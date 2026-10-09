package meetingbot

import (
	"context"
	"testing"

	"code-base-golang/internal/services"
)

func TestStubAdapter(t *testing.T) {
	ctx := context.Background()
	stub := NewStubAdapter("google_meet")

	if stub.ProviderName() != "google_meet" {
		t.Errorf("expected provider google_meet, got %s", stub.ProviderName())
	}
	if stub.IsEnabled() {
		t.Errorf("expected stub to be disabled")
	}

	_, err := stub.Dispatch(ctx, services.BotDispatchParams{})
	if err == nil {
		t.Errorf("expected error from stub Dispatch")
	}

	_, err = stub.GetStatus(ctx, "session-1")
	if err == nil {
		t.Errorf("expected error from stub GetStatus")
	}

	err = stub.Stop(ctx, "session-1")
	if err == nil {
		t.Errorf("expected error from stub Stop")
	}
}
