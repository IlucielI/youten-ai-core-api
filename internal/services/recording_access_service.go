// recording_access_service.go — centralized ownership/share authorization for recording resources.
package services

import (
	"context"
	"crypto/subtle"

	"code-base-golang/internal/models"
	"code-base-golang/internal/pkg/ctxmeta"
)

// recordingAccessPolicy configures which non-owner principals may access a recording.
type recordingAccessPolicy struct {
	// allowShareToken grants access when the supplied token matches the recording's active
	// share token. Used by read/progress/retry flows that honor share links.
	allowShareToken bool
	// allowShareOpen grants access to anyone while sharing is enabled, regardless of token.
	// Used by collaborative comment flows on publicly shared recordings.
	allowShareOpen bool
}

// constantTimeEqual compares two secrets without leaking timing information.
func constantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// authorizeRecordingAccess centralizes recording ownership verification. Access is granted when the
// caller is the authenticated owner, presents the matching ownership token, or satisfies the share
// policy. All token comparisons are constant-time to avoid leaking secrets via timing side channels.
func (s *Service) authorizeRecordingAccess(ctx context.Context, rec *models.Recording, ownershipToken string, policy recordingAccessPolicy) bool {
	if rec == nil {
		return false
	}

	isAuth := ctxmeta.IsAuthenticated(ctx)
	userID, hasUserID := ctxmeta.GetAuthUserID(ctx)
	if isAuth && hasUserID && rec.UserID != nil && *rec.UserID == userID {
		return true
	}

	if ownershipToken != "" && constantTimeEqual(ownershipToken, rec.OwnershipToken) {
		return true
	}

	if policy.allowShareToken && ownershipToken != "" && rec.IsShareEnabled && rec.ShareToken != nil &&
		constantTimeEqual(ownershipToken, *rec.ShareToken) {
		return true
	}

	if policy.allowShareOpen && rec.IsShareEnabled {
		return true
	}

	return false
}
