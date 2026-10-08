package services

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"code-base-golang/internal/models"
	"code-base-golang/internal/pkg/ctxmeta"
)

func TestAuthorizeRecordingAccess(t *testing.T) {
	svc := &Service{}

	ownerID := uuid.New()
	otherUserID := uuid.New()
	recID := uuid.New()
	anonSessionID := uuid.New().String()
	shareToken := "valid-share-token-123"

	rec := &models.Recording{
		BaseModel: models.BaseModel{
			ID: recID,
		},
		UserID:         &ownerID,
		OwnershipToken: anonSessionID,
		IsGuest:        false,
		IsShareEnabled: false,
	}

	guestRec := &models.Recording{
		BaseModel: models.BaseModel{
			ID: uuid.New(),
		},
		UserID:         nil,
		OwnershipToken: anonSessionID,
		IsGuest:        true,
		IsShareEnabled: false,
	}

	sharedRec := &models.Recording{
		BaseModel: models.BaseModel{
			ID: uuid.New(),
		},
		UserID:         &ownerID,
		OwnershipToken: "some-ownership-token",
		IsShareEnabled: true,
		ShareToken:     &shareToken,
	}

	t.Run("nil recording returns false", func(t *testing.T) {
		if svc.authorizeRecordingAccess(context.Background(), nil, "", recordingAccessPolicy{}) {
			t.Fatal("expected false for nil recording")
		}
	})

	t.Run("authenticated owner user grants access", func(t *testing.T) {
		ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{
			UserID: ownerID,
		})
		if !svc.authorizeRecordingAccess(ctx, rec, "", recordingAccessPolicy{}) {
			t.Fatal("expected true for authenticated owner")
		}
	})

	t.Run("authenticated non-owner user without token is denied", func(t *testing.T) {
		ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{
			UserID: otherUserID,
		})
		if svc.authorizeRecordingAccess(ctx, rec, "", recordingAccessPolicy{}) {
			t.Fatal("expected false for authenticated non-owner")
		}
	})

	t.Run("direct ownershipToken match grants access", func(t *testing.T) {
		if !svc.authorizeRecordingAccess(context.Background(), guestRec, anonSessionID, recordingAccessPolicy{}) {
			t.Fatal("expected true for direct ownership token match")
		}
	})

	t.Run("guest with matching anon session in context grants access without explicit query token", func(t *testing.T) {
		ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{
			SessionID: anonSessionID,
			IsGuest:   true,
		})
		if !svc.authorizeRecordingAccess(ctx, guestRec, "", recordingAccessPolicy{}) {
			t.Fatal("expected true for guest with matching anon session anchor")
		}
	})

	t.Run("guest with mismatched anon session is denied", func(t *testing.T) {
		ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{
			SessionID: "different-session-uuid",
			IsGuest:   true,
		})
		if svc.authorizeRecordingAccess(ctx, guestRec, "", recordingAccessPolicy{}) {
			t.Fatal("expected false for guest with mismatched anon session")
		}
	})

	t.Run("share token matches when policy allows", func(t *testing.T) {
		if !svc.authorizeRecordingAccess(context.Background(), sharedRec, shareToken, recordingAccessPolicy{allowShareToken: true}) {
			t.Fatal("expected true when allowShareToken and token matches")
		}
		if svc.authorizeRecordingAccess(context.Background(), sharedRec, shareToken, recordingAccessPolicy{allowShareToken: false}) {
			t.Fatal("expected false when allowShareToken is false")
		}
	})

	t.Run("open share grants access when enabled", func(t *testing.T) {
		if !svc.authorizeRecordingAccess(context.Background(), sharedRec, "", recordingAccessPolicy{allowShareOpen: true}) {
			t.Fatal("expected true when allowShareOpen and sharing enabled")
		}
	})
}
