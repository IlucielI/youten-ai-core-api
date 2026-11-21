package services_test

import (
	"context"
	"errors"
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

func TestService_CreateFolder(t *testing.T) {
	t.Run("unauthorized", func(t *testing.T) {
		svc, _, _, _ := setupRecordingTestService(t)
		_, err := svc.CreateFolder(context.Background(), uuid.New(), dtos.CreateFolderRequest{Name: "Sprint"})
		if err != constants.ErrUnauthorized {
			t.Fatalf("expected ErrUnauthorized, got %v", err)
		}
	})

	t.Run("access denied when not member", func(t *testing.T) {
		svc, mock, _, _ := setupRecordingTestService(t)
		userID := uuid.New()
		wsID := uuid.New()
		ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspace_members" WHERE (workspace_id = $1 AND user_id = $2) AND "workspace_members"."deleted_at" IS NULL`)).
			WithArgs(wsID, userID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id"}))

		_, err := svc.CreateFolder(ctx, wsID, dtos.CreateFolderRequest{Name: "Sprint"})
		if err != constants.ErrWorkspaceAccessDenied {
			t.Fatalf("expected ErrWorkspaceAccessDenied, got %v", err)
		}
	})

	t.Run("forbidden when viewer", func(t *testing.T) {
		svc, mock, _, _ := setupRecordingTestService(t)
		userID := uuid.New()
		wsID := uuid.New()
		ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspace_members" WHERE (workspace_id = $1 AND user_id = $2) AND "workspace_members"."deleted_at" IS NULL`)).
			WithArgs(wsID, userID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "role"}).AddRow(uuid.New(), constants.WorkspaceRoleViewer))

		_, err := svc.CreateFolder(ctx, wsID, dtos.CreateFolderRequest{Name: "Sprint"})
		if err != constants.ErrForbidden {
			t.Fatalf("expected ErrForbidden, got %v", err)
		}
	})

	t.Run("success creating root folder", func(t *testing.T) {
		svc, mock, _, _ := setupRecordingTestService(t)
		userID := uuid.New()
		wsID := uuid.New()
		folderID := uuid.New()
		ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspace_members" WHERE (workspace_id = $1 AND user_id = $2) AND "workspace_members"."deleted_at" IS NULL`)).
			WithArgs(wsID, userID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "role"}).AddRow(uuid.New(), constants.WorkspaceRoleMember))

		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "folders"`)).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(folderID, time.Now(), time.Now()))
		mock.ExpectCommit()

		resp, err := svc.CreateFolder(ctx, wsID, dtos.CreateFolderRequest{Name: "Sprint Planning", Color: "#4F46E5"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Name != "Sprint Planning" {
			t.Errorf("expected name 'Sprint Planning', got %s", resp.Name)
		}
	})
}

func TestService_ListWorkspaceFolders(t *testing.T) {
	t.Run("success hierarchical tree", func(t *testing.T) {
		svc, mock, _, _ := setupRecordingTestService(t)
		userID := uuid.New()
		wsID := uuid.New()
		ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

		fRootID := uuid.New()
		fChildID := uuid.New()

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspace_members" WHERE (workspace_id = $1 AND user_id = $2) AND "workspace_members"."deleted_at" IS NULL`)).
			WithArgs(wsID, userID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "role"}).AddRow(uuid.New(), constants.WorkspaceRoleViewer))

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "folders" WHERE workspace_id = $1 AND "folders"."deleted_at" IS NULL ORDER BY name ASC`)).
			WithArgs(wsID).
			WillReturnRows(sqlmock.NewRows([]string{"id", "workspace_id", "parent_id", "name", "color"}).
				AddRow(fRootID, wsID, nil, "Design", "#4F46E5").
				AddRow(fChildID, wsID, &fRootID, "Wireframes", "#10B981"))

		tree, err := svc.ListWorkspaceFolders(ctx, wsID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(tree) != 1 {
			t.Fatalf("expected 1 root folder, got %d", len(tree))
		}
		if len(tree[0].Children) != 1 {
			t.Fatalf("expected 1 child folder, got %d", len(tree[0].Children))
		}
		if tree[0].Children[0].Name != "Wireframes" {
			t.Errorf("expected child name 'Wireframes', got %s", tree[0].Children[0].Name)
		}
	})
}

func TestService_UpdateFolder(t *testing.T) {
	t.Run("circular reference rejected", func(t *testing.T) {
		svc, mock, _, _ := setupRecordingTestService(t)
		userID := uuid.New()
		wsID := uuid.New()
		folderID := uuid.New()
		ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspace_members" WHERE (workspace_id = $1 AND user_id = $2) AND "workspace_members"."deleted_at" IS NULL`)).
			WithArgs(wsID, userID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "role"}).AddRow(uuid.New(), constants.WorkspaceRoleAdmin))

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "folders" WHERE id = $1 AND "folders"."deleted_at" IS NULL ORDER BY "folders"."id" LIMIT $2`)).
			WithArgs(folderID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "workspace_id", "name"}).AddRow(folderID, wsID, "Design"))

		_, err := svc.UpdateFolder(ctx, wsID, folderID, dtos.UpdateFolderRequest{
			ParentID: &folderID,
		})
		if err != constants.ErrCircularFolderReference {
			t.Fatalf("expected ErrCircularFolderReference, got %v", err)
		}
	})
}

func TestService_DeleteFolder(t *testing.T) {
	t.Run("success deleting folder", func(t *testing.T) {
		svc, mock, _, _ := setupRecordingTestService(t)
		userID := uuid.New()
		wsID := uuid.New()
		folderID := uuid.New()
		ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspace_members" WHERE (workspace_id = $1 AND user_id = $2) AND "workspace_members"."deleted_at" IS NULL`)).
			WithArgs(wsID, userID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "role"}).AddRow(uuid.New(), constants.WorkspaceRoleAdmin))

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "folders" WHERE id = $1 AND "folders"."deleted_at" IS NULL ORDER BY "folders"."id" LIMIT $2`)).
			WithArgs(folderID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "workspace_id", "name"}).AddRow(folderID, wsID, "Old Folder"))

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "folders" SET "deleted_at"=$1 WHERE id = $2 AND "folders"."deleted_at" IS NULL`)).
			WithArgs(sqlmock.AnyArg(), folderID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := svc.DeleteFolder(ctx, wsID, folderID)
		if err != nil {
			t.Fatalf("unexpected error deleting folder: %v", err)
		}
	})
}

func TestService_MoveRecording(t *testing.T) {
	t.Run("success moving recording", func(t *testing.T) {
		svc, mock, _, _ := setupRecordingTestService(t)
		userID := uuid.New()
		wsID := uuid.New()
		recID := uuid.New()
		folderID := uuid.New()
		ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

		// FindRecordingByID
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "recordings" WHERE id = $1 AND "recordings"."deleted_at" IS NULL ORDER BY "recordings"."id" LIMIT $2`)).
			WithArgs(recID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "workspace_id"}).AddRow(recID, &userID, &wsID))

		// FindFolderByID
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "folders" WHERE id = $1 AND "folders"."deleted_at" IS NULL ORDER BY "folders"."id" LIMIT $2`)).
			WithArgs(folderID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "workspace_id"}).AddRow(folderID, wsID))

		// MoveRecording update
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "recordings" SET "folder_id"=$1,"updated_at"=$2 WHERE id = $3 AND "recordings"."deleted_at" IS NULL`)).
			WithArgs(folderID, sqlmock.AnyArg(), recID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := svc.MoveRecording(ctx, recID, dtos.MoveRecordingRequest{FolderID: &folderID})
		if err != nil {
			t.Fatalf("unexpected error moving recording: %v", err)
		}
	})

	t.Run("success cross-workspace move as creator", func(t *testing.T) {
		svc, mock, _, _ := setupRecordingTestService(t)
		userID := uuid.New()
		sourceWsID := uuid.New()
		targetWsID := uuid.New()
		recID := uuid.New()
		targetFolderID := uuid.New()
		ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

		// FindRecordingByID returns COMPLETED status and owned by userID
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "recordings" WHERE id = $1 AND "recordings"."deleted_at" IS NULL ORDER BY "recordings"."id" LIMIT $2`)).
			WithArgs(recID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "workspace_id", "status"}).
				AddRow(recID, &userID, &sourceWsID, models.RecordingStatusCompleted))

		// FindWorkspaceMember on target workspace (member exists)
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspace_members" WHERE (workspace_id = $1 AND user_id = $2) AND "workspace_members"."deleted_at" IS NULL ORDER BY "workspace_members"."id" LIMIT $3`)).
			WithArgs(targetWsID, userID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "workspace_id", "user_id", "role"}).
				AddRow(uuid.New(), targetWsID, userID, constants.WorkspaceRoleMember))

		// FindFolderByID on target folder
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "folders" WHERE id = $1 AND "folders"."deleted_at" IS NULL ORDER BY "folders"."id" LIMIT $2`)).
			WithArgs(targetFolderID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "workspace_id"}).AddRow(targetFolderID, targetWsID))

		// MoveRecordingToWorkspace update
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "recordings" SET "folder_id"=$1,"is_share_enabled"=$2,"workspace_id"=$3,"updated_at"=$4 WHERE id = $5 AND "recordings"."deleted_at" IS NULL`)).
			WithArgs(targetFolderID, false, targetWsID, sqlmock.AnyArg(), recID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := svc.MoveRecording(ctx, recID, dtos.MoveRecordingRequest{
			WorkspaceID: &targetWsID,
			FolderID:    &targetFolderID,
		})
		if err != nil {
			t.Fatalf("unexpected error in cross-workspace move: %v", err)
		}
	})

	t.Run("fail cross-workspace move when recording is processing", func(t *testing.T) {
		svc, mock, _, _ := setupRecordingTestService(t)
		userID := uuid.New()
		sourceWsID := uuid.New()
		targetWsID := uuid.New()
		recID := uuid.New()
		ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

		// FindRecordingByID returns TRANSCRIBING
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "recordings" WHERE id = $1 AND "recordings"."deleted_at" IS NULL ORDER BY "recordings"."id" LIMIT $2`)).
			WithArgs(recID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "workspace_id", "status"}).
				AddRow(recID, &userID, &sourceWsID, models.RecordingStatusTranscribing))

		err := svc.MoveRecording(ctx, recID, dtos.MoveRecordingRequest{
			WorkspaceID: &targetWsID,
		})
		if !errors.Is(err, constants.ErrRecordingProcessing) {
			t.Fatalf("expected ErrRecordingProcessing, got %v", err)
		}
	})

	t.Run("fail cross-workspace move when not creator and not source owner", func(t *testing.T) {
		svc, mock, _, _ := setupRecordingTestService(t)
		otherUser := uuid.New()
		callerID := uuid.New()
		sourceWsID := uuid.New()
		targetWsID := uuid.New()
		recID := uuid.New()
		ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: callerID})

		// FindRecordingByID owned by otherUser
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "recordings" WHERE id = $1 AND "recordings"."deleted_at" IS NULL ORDER BY "recordings"."id" LIMIT $2`)).
			WithArgs(recID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "workspace_id", "status"}).
				AddRow(recID, &otherUser, &sourceWsID, models.RecordingStatusCompleted))

		// FindWorkspaceMember on source workspace returns role 'member' (not owner)
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspace_members" WHERE (workspace_id = $1 AND user_id = $2) AND "workspace_members"."deleted_at" IS NULL ORDER BY "workspace_members"."id" LIMIT $3`)).
			WithArgs(sourceWsID, callerID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "workspace_id", "user_id", "role"}).
				AddRow(uuid.New(), sourceWsID, callerID, constants.WorkspaceRoleMember))

		err := svc.MoveRecording(ctx, recID, dtos.MoveRecordingRequest{
			WorkspaceID: &targetWsID,
		})
		if !errors.Is(err, constants.ErrForbidden) {
			t.Fatalf("expected ErrForbidden, got %v", err)
		}
	})
}
