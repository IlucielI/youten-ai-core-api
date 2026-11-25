package services

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/models"
	"code-base-golang/internal/pkg/ctxmeta"
)

func TestApplyRecordingOwnership(t *testing.T) {
	now := time.Now().UTC()

	t.Run("assigns user_id and workspace_id for authenticated user with workspace context", func(t *testing.T) {
		userID := uuid.New()
		wsID := uuid.New()
		ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})
		ctx = ctxmeta.WithWorkspaceID(ctx, wsID)

		var rec models.Recording
		applyRecordingOwnership(ctx, &rec, now)

		if rec.IsGuest {
			t.Fatal("expected IsGuest to be false")
		}
		if rec.UserID == nil || *rec.UserID != userID {
			t.Fatalf("expected UserID %s, got %v", userID, rec.UserID)
		}
		if rec.WorkspaceID == nil || *rec.WorkspaceID != wsID {
			t.Fatalf("expected WorkspaceID %s, got %v", wsID, rec.WorkspaceID)
		}
	})
}

func TestMaxBytesReader(t *testing.T) {
	t.Run("reading under limit succeeds", func(t *testing.T) {
		input := "hello world"
		reader := &maxBytesReader{
			r:     strings.NewReader(input),
			limit: 20,
		}

		buf, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(buf) != input {
			t.Errorf("expected %q, got %q", input, string(buf))
		}
		if reader.read != int64(len(input)) {
			t.Errorf("expected read count %d, got %d", len(input), reader.read)
		}
	})

	t.Run("reading exact limit succeeds", func(t *testing.T) {
		input := "12345"
		reader := &maxBytesReader{
			r:     strings.NewReader(input),
			limit: 5,
		}

		buf, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(buf) != input {
			t.Errorf("expected %q, got %q", input, string(buf))
		}
	})

	t.Run("reading exceeding limit returns ErrPayloadTooLarge", func(t *testing.T) {
		input := "1234567890"
		reader := &maxBytesReader{
			r:     strings.NewReader(input),
			limit: 5,
		}

		_, err := io.ReadAll(reader)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		expectedErr := constants.ErrPayloadTooLarge.WithMessage("media size from link exceeds maximum limit of 1GB")
		if !errors.Is(err, expectedErr) {
			t.Errorf("expected %v, got %v", expectedErr, err)
		}
	})
}
