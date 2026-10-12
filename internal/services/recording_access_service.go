package services

import (
	"context"

	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
	"code-base-golang/internal/pkg/ctxmeta"
	"code-base-golang/internal/pkg/strutil"
)

// authorizeRecordingAccess centralizes recording ownership verification. Access is granted when the
// caller is the authenticated owner, presents the matching ownership token, or satisfies the share
// policy. All token comparisons are constant-time to avoid leaking secrets via timing side channels.
func (s *Service) authorizeRecordingAccess(ctx context.Context, rec *models.Recording, ownershipToken string, policy dtos.RecordingAccessPolicy) bool {
	if rec == nil {
		return false
	}

	isAuth := ctxmeta.IsAuthenticated(ctx)
	userID, hasUserID := ctxmeta.GetAuthUserID(ctx)
	if isAuth && hasUserID && rec.UserID != nil && *rec.UserID == userID {
		return true
	}

	if ownershipToken != "" && strutil.ConstantTimeEqual(ownershipToken, rec.OwnershipToken) {
		return true
	}

	// Also grant access when the request carries an anonymous guest token matching the recording ownership anchor
	if authUser, ok := ctxmeta.GetAuthUser(ctx); ok && authUser.IsGuest && authUser.SessionID != "" {
		if strutil.ConstantTimeEqual(authUser.SessionID, rec.OwnershipToken) {
			return true
		}
	}

	if policy.AllowShareToken && ownershipToken != "" && rec.IsShareEnabled && rec.ShareToken != nil &&
		strutil.ConstantTimeEqual(ownershipToken, *rec.ShareToken) {
		return true
	}

	if policy.AllowShareOpen && rec.IsShareEnabled {
		return true
	}

	return false
}
