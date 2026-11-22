package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
	"code-base-golang/internal/pkg/ctxmeta"
)

func TestControllers_CreateFolder(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("invalid workspace id param", func(t *testing.T) {
		ctrls, _, _, cleanup := setupRecordingTestControllers(t)
		defer cleanup()

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "id", Value: "invalid-uuid"}}
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/workspaces/invalid-uuid/folders", nil)

		ctrls.CreateFolder(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request, got %d", w.Code)
		}
	})

	t.Run("success creating folder", func(t *testing.T) {
		ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
		defer cleanup()

		userID := uuid.New()
		wsID := uuid.New()
		folderID := uuid.New()

		body, _ := json.Marshal(dtos.CreateFolderRequest{
			Name:  "Sprint Planning",
			Color: "#4F46E5",
		})

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "id", Value: wsID.String()}}
		req := httptest.NewRequest(http.MethodPost, "/v1/workspaces/"+wsID.String()+"/folders", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		c.Request = req.WithContext(ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID}))

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspace_members" WHERE (workspace_id = $1 AND user_id = $2) AND "workspace_members"."deleted_at" IS NULL`)).
			WithArgs(wsID, userID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "role"}).AddRow(uuid.New(), constants.WorkspaceRoleAdmin))

		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "folders"`)).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(folderID, time.Now(), time.Now()))
		mock.ExpectCommit()

		ctrls.CreateFolder(c)
		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestControllers_ListWorkspaceFolders(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("success listing folders", func(t *testing.T) {
		ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
		defer cleanup()

		userID := uuid.New()
		wsID := uuid.New()
		fID := uuid.New()

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "id", Value: wsID.String()}}
		req := httptest.NewRequest(http.MethodGet, "/v1/workspaces/"+wsID.String()+"/folders", nil)
		c.Request = req.WithContext(ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID}))

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspace_members" WHERE (workspace_id = $1 AND user_id = $2) AND "workspace_members"."deleted_at" IS NULL`)).
			WithArgs(wsID, userID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "role"}).AddRow(uuid.New(), constants.WorkspaceRoleViewer))

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "folders" WHERE workspace_id = $1 AND "folders"."deleted_at" IS NULL ORDER BY name ASC`)).
			WithArgs(wsID).
			WillReturnRows(sqlmock.NewRows([]string{"id", "workspace_id", "name", "color"}).
				AddRow(fID, wsID, "Design", "#4F46E5"))

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT folder_id, count(*) as count FROM "recordings" WHERE (workspace_id = $1 AND folder_id IS NOT NULL) AND "recordings"."deleted_at" IS NULL GROUP BY "folder_id"`)).
			WithArgs(wsID).
			WillReturnRows(sqlmock.NewRows([]string{"folder_id", "count"}).AddRow(fID, 2))

		ctrls.ListWorkspaceFolders(c)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestControllers_MoveRecording(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("success moving recording", func(t *testing.T) {
		ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
		defer cleanup()

		userID := uuid.New()
		recID := uuid.New()
		folderID := uuid.New()
		wsID := uuid.New()

		body, _ := json.Marshal(dtos.MoveRecordingRequest{
			FolderID: &folderID,
		})

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "id", Value: recID.String()}}
		req := httptest.NewRequest(http.MethodPatch, "/v1/recordings/"+recID.String()+"/move", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		c.Request = req.WithContext(ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID}))

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "recordings" WHERE id = $1 AND "recordings"."deleted_at" IS NULL ORDER BY "recordings"."id" LIMIT $2`)).
			WithArgs(recID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "workspace_id"}).AddRow(recID, &userID, &wsID))

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "folders" WHERE id = $1 AND "folders"."deleted_at" IS NULL ORDER BY "folders"."id" LIMIT $2`)).
			WithArgs(folderID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "workspace_id"}).AddRow(folderID, wsID))

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "recordings" SET "folder_id"=$1,"updated_at"=$2 WHERE id = $3 AND "recordings"."deleted_at" IS NULL`)).
			WithArgs(folderID, sqlmock.AnyArg(), recID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		ctrls.MoveRecording(c)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("success cross-workspace move", func(t *testing.T) {
		ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
		defer cleanup()

		userID := uuid.New()
		recID := uuid.New()
		sourceWsID := uuid.New()
		targetWsID := uuid.New()

		body, _ := json.Marshal(dtos.MoveRecordingRequest{
			WorkspaceID: &targetWsID,
		})

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "id", Value: recID.String()}}
		req := httptest.NewRequest(http.MethodPatch, "/v1/recordings/"+recID.String()+"/move", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		c.Request = req.WithContext(ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID}))

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "recordings" WHERE id = $1 AND "recordings"."deleted_at" IS NULL ORDER BY "recordings"."id" LIMIT $2`)).
			WithArgs(recID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "workspace_id", "status"}).
				AddRow(recID, &userID, &sourceWsID, models.RecordingStatusCompleted))

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspace_members" WHERE (workspace_id = $1 AND user_id = $2) AND "workspace_members"."deleted_at" IS NULL ORDER BY "workspace_members"."id" LIMIT $3`)).
			WithArgs(targetWsID, userID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "workspace_id", "user_id", "role"}).
				AddRow(uuid.New(), targetWsID, userID, constants.WorkspaceRoleMember))

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "recordings" SET "folder_id"=$1,"is_share_enabled"=$2,"workspace_id"=$3,"updated_at"=$4 WHERE id = $5 AND "recordings"."deleted_at" IS NULL`)).
			WithArgs(nil, false, targetWsID, sqlmock.AnyArg(), recID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		ctrls.MoveRecording(c)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
		}
	})
}
