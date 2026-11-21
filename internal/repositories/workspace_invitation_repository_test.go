package repositories

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/models"
)

func TestRepositories_CreateInvitation(t *testing.T) {
	repo, mock, cleanup := setupMockFolderDB(t)
	defer cleanup()

	inv := &models.WorkspaceInvitation{
		BaseModel:       models.BaseModel{ID: uuid.New()},
		WorkspaceID:     uuid.New(),
		Email:           "newbie@example.com",
		InviteToken:     "test-token-123",
		Role:            constants.WorkspaceRoleMember,
		InvitedByUserID: uuid.New(),
		ExpiresAt:       time.Now().Add(7 * 24 * time.Hour),
	}

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "workspace_invitations"`)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(inv.ID))
	mock.ExpectCommit()

	err := repo.CreateInvitation(context.Background(), inv)
	if err != nil {
		t.Fatalf("unexpected error creating invitation: %v", err)
	}
}

func TestRepositories_DeleteInvitation(t *testing.T) {
	repo, mock, cleanup := setupMockFolderDB(t)
	defer cleanup()

	invID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "workspace_invitations" SET "deleted_at"=$1 WHERE id = $2 AND "workspace_invitations"."deleted_at" IS NULL`)).
		WithArgs(sqlmock.AnyArg(), invID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := repo.DeleteInvitation(context.Background(), invID)
	if err != nil {
		t.Fatalf("unexpected error deleting invitation: %v", err)
	}
}

func TestRepositories_AcceptInvitationInTransaction(t *testing.T) {
	repo, mock, cleanup := setupMockFolderDB(t)
	defer cleanup()

	invID := uuid.New()
	wsID := uuid.New()
	userID := uuid.New()

	member := &models.WorkspaceMember{
		BaseModel:   models.BaseModel{ID: uuid.New()},
		WorkspaceID: wsID,
		UserID:      userID,
		Role:        constants.WorkspaceRoleMember,
	}

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "workspace_invitations" SET`)).
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "workspace_members"`)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(member.ID))
	mock.ExpectCommit()

	err := repo.AcceptInvitationInTransaction(context.Background(), invID, member)
	if err != nil {
		t.Fatalf("unexpected error accepting invitation: %v", err)
	}
}
