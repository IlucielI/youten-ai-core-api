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
	"code-base-golang/internal/models"
	"code-base-golang/internal/pkg/ctxmeta"
)

func TestService_CreateWorkspace(t *testing.T) {
	t.Run("unauthorized when no auth user", func(t *testing.T) {
		svc, _, _, _ := setupRecordingTestService(t)
		_, err := svc.CreateWorkspace(context.Background(), dtos.CreateWorkspaceRequest{Name: "Team"})
		if err != constants.ErrUnauthorized {
			t.Fatalf("expected ErrUnauthorized, got %v", err)
		}
	})

	t.Run("success creating workspace", func(t *testing.T) {
		svc, mock, _, _ := setupRecordingTestService(t)
		userID := uuid.New()
		ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})
		wsID := uuid.New()
		memberID := uuid.New()

		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "workspaces"`)).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(wsID, time.Now(), time.Now()))
		mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "workspace_members"`)).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(memberID, time.Now(), time.Now()))
		mock.ExpectCommit()

		res, err := svc.CreateWorkspace(ctx, dtos.CreateWorkspaceRequest{Name: "Engineering"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Name != "Engineering" {
			t.Errorf("expected name Engineering, got %s", res.Name)
		}
		if res.Role != string(constants.WorkspaceRoleOwner) {
			t.Errorf("expected role owner, got %s", res.Role)
		}
	})
}

func TestService_ListUserWorkspaces(t *testing.T) {
	t.Run("unauthorized", func(t *testing.T) {
		svc, _, _, _ := setupRecordingTestService(t)
		_, err := svc.ListUserWorkspaces(context.Background())
		if err != constants.ErrUnauthorized {
			t.Fatalf("expected ErrUnauthorized, got %v", err)
		}
	})

	t.Run("success", func(t *testing.T) {
		svc, mock, _, _ := setupRecordingTestService(t)
		userID := uuid.New()
		wsID := uuid.New()
		ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

		wsRows := sqlmock.NewRows([]string{"id", "name", "slug", "type", "owner_user_id", "created_at", "updated_at", "deleted_at"}).
			AddRow(wsID, "My Team", "my-team", "team", userID, time.Now(), time.Now(), nil)
		memberRows := sqlmock.NewRows([]string{"id", "workspace_id", "user_id", "role", "created_at", "updated_at", "deleted_at"}).
			AddRow(uuid.New(), wsID, userID, "owner", time.Now(), time.Now(), nil)
		countRows := sqlmock.NewRows([]string{"count"}).AddRow(3)

		mock.ExpectQuery(`SELECT .+ FROM "workspaces" INNER JOIN workspace_members ON workspace_members\.workspace_id = workspaces\.id`).
			WithArgs(userID).
			WillReturnRows(wsRows)

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspace_members" WHERE (workspace_id = $1 AND user_id = $2) AND "workspace_members"."deleted_at" IS NULL`)).
			WithArgs(wsID, userID, 1).
			WillReturnRows(memberRows)

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "workspace_members" WHERE workspace_id = $1 AND "workspace_members"."deleted_at" IS NULL`)).
			WithArgs(wsID).
			WillReturnRows(countRows)

		list, err := svc.ListUserWorkspaces(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(list) != 1 {
			t.Fatalf("expected 1 workspace, got %d", len(list))
		}
		if list[0].MembersCount != 3 {
			t.Errorf("expected 3 members, got %d", list[0].MembersCount)
		}
	})
}

func TestService_GetWorkspaceDetail(t *testing.T) {
	t.Run("access denied when not a member", func(t *testing.T) {
		svc, mock, _, _ := setupRecordingTestService(t)
		userID := uuid.New()
		wsID := uuid.New()
		ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspace_members" WHERE (workspace_id = $1 AND user_id = $2) AND "workspace_members"."deleted_at" IS NULL`)).
			WithArgs(wsID, userID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id"}))

		_, err := svc.GetWorkspaceDetail(ctx, wsID)
		if err != constants.ErrWorkspaceAccessDenied {
			t.Fatalf("expected ErrWorkspaceAccessDenied, got %v", err)
		}
	})

	t.Run("success", func(t *testing.T) {
		svc, mock, _, _ := setupRecordingTestService(t)
		userID := uuid.New()
		wsID := uuid.New()
		ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

		memberRows := sqlmock.NewRows([]string{"id", "workspace_id", "user_id", "role", "created_at", "updated_at", "deleted_at"}).
			AddRow(uuid.New(), wsID, userID, "admin", time.Now(), time.Now(), nil)
		wsRows := sqlmock.NewRows([]string{"id", "name", "slug", "type", "owner_user_id", "created_at", "updated_at", "deleted_at"}).
			AddRow(wsID, "My Team", "my-team", "team", userID, time.Now(), time.Now(), nil)
		countRows := sqlmock.NewRows([]string{"count"}).AddRow(2)

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspace_members" WHERE (workspace_id = $1 AND user_id = $2) AND "workspace_members"."deleted_at" IS NULL`)).
			WithArgs(wsID, userID, 1).
			WillReturnRows(memberRows)

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspaces" WHERE id = $1 AND "workspaces"."deleted_at" IS NULL`)).
			WithArgs(wsID, 1).
			WillReturnRows(wsRows)

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "workspace_members" WHERE workspace_id = $1 AND "workspace_members"."deleted_at" IS NULL`)).
			WithArgs(wsID).
			WillReturnRows(countRows)

		res, err := svc.GetWorkspaceDetail(ctx, wsID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Role != string(constants.WorkspaceRoleAdmin) {
			t.Errorf("expected role admin, got %s", res.Role)
		}
	})
}

func TestService_DeleteWorkspace(t *testing.T) {
	t.Run("cannot delete personal workspace", func(t *testing.T) {
		svc, mock, _, _ := setupRecordingTestService(t)
		userID := uuid.New()
		wsID := uuid.New()
		ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

		wsRows := sqlmock.NewRows([]string{"id", "name", "slug", "type", "owner_user_id", "created_at", "updated_at", "deleted_at"}).
			AddRow(wsID, "Personal", "personal-123", "personal", userID, time.Now(), time.Now(), nil)

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspaces" WHERE id = $1 AND "workspaces"."deleted_at" IS NULL`)).
			WithArgs(wsID, 1).
			WillReturnRows(wsRows)

		err := svc.DeleteWorkspace(ctx, wsID)
		if err != constants.ErrCannotDeletePersonalWorkspace {
			t.Fatalf("expected ErrCannotDeletePersonalWorkspace, got %v", err)
		}
	})

	t.Run("forbidden when not owner", func(t *testing.T) {
		svc, mock, _, _ := setupRecordingTestService(t)
		userID := uuid.New()
		otherID := uuid.New()
		wsID := uuid.New()
		ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

		wsRows := sqlmock.NewRows([]string{"id", "name", "slug", "type", "owner_user_id", "created_at", "updated_at", "deleted_at"}).
			AddRow(wsID, "Team", "team-123", "team", otherID, time.Now(), time.Now(), nil)

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspaces" WHERE id = $1 AND "workspaces"."deleted_at" IS NULL`)).
			WithArgs(wsID, 1).
			WillReturnRows(wsRows)

		err := svc.DeleteWorkspace(ctx, wsID)
		if err != constants.ErrForbidden {
			t.Fatalf("expected ErrForbidden, got %v", err)
		}
	})

	t.Run("success", func(t *testing.T) {
		svc, mock, _, _ := setupRecordingTestService(t)
		userID := uuid.New()
		wsID := uuid.New()
		ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

		wsRows := sqlmock.NewRows([]string{"id", "name", "slug", "type", "owner_user_id", "created_at", "updated_at", "deleted_at"}).
			AddRow(wsID, "Team", "team-123", "team", userID, time.Now(), time.Now(), nil)

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspaces" WHERE id = $1 AND "workspaces"."deleted_at" IS NULL`)).
			WithArgs(wsID, 1).
			WillReturnRows(wsRows)

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "workspaces" SET "deleted_at"=$1 WHERE id = $2 AND "workspaces"."deleted_at" IS NULL`)).
			WithArgs(sqlmock.AnyArg(), wsID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := svc.DeleteWorkspace(ctx, wsID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestService_WorkspaceMembers(t *testing.T) {
	t.Run("UpdateWorkspaceMemberRole - demote owner forbidden", func(t *testing.T) {
		svc, mock, _, _ := setupRecordingTestService(t)
		ownerID := uuid.New()
		targetID := uuid.New()
		wsID := uuid.New()
		ctxOwner := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: ownerID})

		callerRows := sqlmock.NewRows([]string{"id", "workspace_id", "user_id", "role"}).
			AddRow(uuid.New(), wsID, ownerID, "owner")
		targetRows := sqlmock.NewRows([]string{"id", "workspace_id", "user_id", "role"}).
			AddRow(uuid.New(), wsID, targetID, "owner")

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspace_members" WHERE (workspace_id = $1 AND user_id = $2) AND "workspace_members"."deleted_at" IS NULL`)).
			WithArgs(wsID, ownerID, 1).
			WillReturnRows(callerRows)

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspace_members" WHERE (workspace_id = $1 AND user_id = $2) AND "workspace_members"."deleted_at" IS NULL`)).
			WithArgs(wsID, targetID, 1).
			WillReturnRows(targetRows)

		err := svc.UpdateWorkspaceMemberRole(ctxOwner, wsID, targetID, dtos.UpdateWorkspaceMemberRoleRequest{Role: "member"})
		if err != constants.ErrCannotRemoveWorkspaceOwner {
			t.Fatalf("expected ErrCannotRemoveWorkspaceOwner, got %v", err)
		}
	})

	t.Run("UpdateWorkspaceMemberRole - success", func(t *testing.T) {
		svc, mock, _, _ := setupRecordingTestService(t)
		ownerID := uuid.New()
		targetID := uuid.New()
		wsID := uuid.New()
		ctxOwner := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: ownerID})

		callerRows := sqlmock.NewRows([]string{"id", "workspace_id", "user_id", "role"}).
			AddRow(uuid.New(), wsID, ownerID, "owner")
		targetRows := sqlmock.NewRows([]string{"id", "workspace_id", "user_id", "role"}).
			AddRow(uuid.New(), wsID, targetID, "member")

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspace_members" WHERE (workspace_id = $1 AND user_id = $2) AND "workspace_members"."deleted_at" IS NULL`)).
			WithArgs(wsID, ownerID, 1).
			WillReturnRows(callerRows)

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspace_members" WHERE (workspace_id = $1 AND user_id = $2) AND "workspace_members"."deleted_at" IS NULL`)).
			WithArgs(wsID, targetID, 1).
			WillReturnRows(targetRows)

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "workspace_members" SET "role"=$1,"updated_at"=$2 WHERE (workspace_id = $3 AND user_id = $4) AND "workspace_members"."deleted_at" IS NULL`)).
			WithArgs(string(constants.WorkspaceRoleAdmin), sqlmock.AnyArg(), wsID, targetID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := svc.UpdateWorkspaceMemberRole(ctxOwner, wsID, targetID, dtos.UpdateWorkspaceMemberRoleRequest{Role: "admin"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("RemoveWorkspaceMember - owner cannot leave workspace", func(t *testing.T) {
		svc, mock, _, _ := setupRecordingTestService(t)
		ownerID := uuid.New()
		wsID := uuid.New()
		ctxOwner := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: ownerID})

		callerRows := sqlmock.NewRows([]string{"id", "workspace_id", "user_id", "role"}).
			AddRow(uuid.New(), wsID, ownerID, "owner")

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspace_members" WHERE (workspace_id = $1 AND user_id = $2) AND "workspace_members"."deleted_at" IS NULL`)).
			WithArgs(wsID, ownerID, 1).
			WillReturnRows(callerRows)

		err := svc.RemoveWorkspaceMember(ctxOwner, wsID, ownerID)
		if err != constants.ErrCannotRemoveWorkspaceOwner {
			t.Fatalf("expected ErrCannotRemoveWorkspaceOwner, got %v", err)
		}
	})
}

func TestService_EnsureUserPersonalWorkspace(t *testing.T) {
	t.Run("returns existing when already present", func(t *testing.T) {
		svc, mock, _, _ := setupRecordingTestService(t)
		userID := uuid.New()
		user := &models.User{
			BaseModel: models.BaseModel{ID: userID},
			FullName:  "Bayu Anugerah",
		}
		wsID := uuid.New()
		wsRows := sqlmock.NewRows([]string{"id", "name", "slug", "type", "owner_user_id"}).
			AddRow(wsID, "Bayu Anugerah's Workspace", "personal-123", "personal", userID)

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspaces" WHERE (owner_user_id = $1 AND type = $2) AND "workspaces"."deleted_at" IS NULL`)).
			WithArgs(userID, string(constants.WorkspaceTypePersonal), 1).
			WillReturnRows(wsRows)

		ws, err := svc.EnsureUserPersonalWorkspace(context.Background(), user)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ws.ID != wsID {
			t.Errorf("expected existing ID %s, got %s", wsID, ws.ID)
		}
	})

	t.Run("creates new workspace when record not found", func(t *testing.T) {
		svc, mock, _, _ := setupRecordingTestService(t)
		userID := uuid.New()
		user := &models.User{
			BaseModel: models.BaseModel{ID: userID},
			FullName:  "Bayu Anugerah",
		}
		newWSID := uuid.New()
		memberID := uuid.New()

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspaces" WHERE (owner_user_id = $1 AND type = $2) AND "workspaces"."deleted_at" IS NULL`)).
			WithArgs(userID, string(constants.WorkspaceTypePersonal), 1).
			WillReturnRows(sqlmock.NewRows([]string{"id"}))

		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "workspaces"`)).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(newWSID, time.Now(), time.Now()))
		mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "workspace_members"`)).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(memberID, time.Now(), time.Now()))
		mock.ExpectCommit()

		ws, err := svc.EnsureUserPersonalWorkspace(context.Background(), user)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ws.Name != "Bayu Anugerah's Workspace" {
			t.Errorf("expected workspace name Bayu Anugerah's Workspace, got %s", ws.Name)
		}
	})

	t.Run("propagates database error when query fails", func(t *testing.T) {
		svc, mock, _, _ := setupRecordingTestService(t)
		userID := uuid.New()
		user := &models.User{
			BaseModel: models.BaseModel{ID: userID},
			FullName:  "Bayu Anugerah",
		}

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspaces" WHERE (owner_user_id = $1 AND type = $2) AND "workspaces"."deleted_at" IS NULL`)).
			WithArgs(userID, string(constants.WorkspaceTypePersonal), 1).
			WillReturnError(assertError("connection refused"))

		_, err := svc.EnsureUserPersonalWorkspace(context.Background(), user)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

type assertError string

func (e assertError) Error() string { return string(e) }

