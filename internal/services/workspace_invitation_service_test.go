package services_test

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/pkg/ctxmeta"
	"code-base-golang/internal/services"
)

func TestService_InviteMember_Unauthorized(t *testing.T) {
	svc := &services.Service{}
	_, err := svc.InviteMember(context.Background(), uuid.New(), dtos.InviteMemberRequest{
		Email: "colleague@example.com",
		Role:  "member",
	})
	if err != constants.ErrUnauthorized {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestService_InviteMember_Forbidden(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)

	callerID := uuid.New()
	wsID := uuid.New()

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: callerID})

	// Caller is only a viewer
	rows := sqlmock.NewRows([]string{"id", "workspace_id", "user_id", "role"}).
		AddRow(uuid.New(), wsID, callerID, constants.WorkspaceRoleViewer)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspace_members"`)).
		WithArgs(wsID, callerID, 1).
		WillReturnRows(rows)

	_, err := svc.InviteMember(ctx, wsID, dtos.InviteMemberRequest{
		Email: "colleague@example.com",
		Role:  "member",
	})
	if err != constants.ErrForbidden {
		t.Fatalf("expected ErrForbidden for viewer, got %v", err)
	}
}

func TestService_GetInvitationDetail_NotFound(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspace_invitations"`)).
		WithArgs("invalid-token", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, err := svc.GetInvitationDetail(context.Background(), "invalid-token")
	if err != constants.ErrInvitationNotFound {
		t.Fatalf("expected ErrInvitationNotFound, got %v", err)
	}
}

func TestService_AcceptInvitation_EmailMismatch(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)

	userID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	// User query
	userRows := sqlmock.NewRows([]string{"id", "email"}).
		AddRow(userID, "logged_in@example.com")
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users"`)).
		WithArgs(userID, 1).
		WillReturnRows(userRows)

	// Invitation query
	invRows := sqlmock.NewRows([]string{"id", "email", "invite_token", "expires_at", "accepted_at"}).
		AddRow(uuid.New(), "invited@example.com", "token123", time.Now().Add(24*time.Hour), nil)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspace_invitations"`)).
		WithArgs("token123", 1).
		WillReturnRows(invRows)

	_, err := svc.AcceptInvitation(ctx, "token123")
	if err != constants.ErrInvitationEmailMismatch {
		t.Fatalf("expected ErrInvitationEmailMismatch, got %v", err)
	}
}
