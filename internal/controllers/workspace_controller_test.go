package controllers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"code-base-golang/internal/pkg/ctxmeta"
)

func TestControllers_ListWorkspaces(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	userID := uuid.New()
	wsID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "/v1/workspaces", nil)
	req = req.WithContext(ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID}))
	c.Request = req

	wsRows := sqlmock.NewRows([]string{"id", "name", "slug", "type", "owner_user_id", "created_at", "updated_at", "deleted_at"}).
		AddRow(wsID, "My Team", "my-team", "team", userID, time.Now(), time.Now(), nil)
	memberRows := sqlmock.NewRows([]string{"id", "workspace_id", "user_id", "role", "created_at", "updated_at", "deleted_at"}).
		AddRow(uuid.New(), wsID, userID, "owner", time.Now(), time.Now(), nil)
	countRows := sqlmock.NewRows([]string{"count"}).AddRow(1)

	mock.ExpectQuery(`SELECT .+ FROM "workspaces" INNER JOIN workspace_members ON workspace_members\.workspace_id = workspaces\.id`).
		WithArgs(userID).
		WillReturnRows(wsRows)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspace_members" WHERE (workspace_id = $1 AND user_id = $2) AND "workspace_members"."deleted_at" IS NULL`)).
		WithArgs(wsID, userID, 1).
		WillReturnRows(memberRows)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "workspace_members" WHERE workspace_id = $1 AND "workspace_members"."deleted_at" IS NULL`)).
		WithArgs(wsID).
		WillReturnRows(countRows)

	ctrls.ListWorkspaces(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}
}

func TestControllers_CreateWorkspace(t *testing.T) {
	t.Run("invalid json", func(t *testing.T) {
		ctrls, _, _, cleanup := setupRecordingTestControllers(t)
		defer cleanup()

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/workspaces", strings.NewReader("bad-json"))

		ctrls.CreateWorkspace(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request, got %d", w.Code)
		}
	})

	t.Run("validation error", func(t *testing.T) {
		ctrls, _, _, cleanup := setupRecordingTestControllers(t)
		defer cleanup()

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/workspaces", strings.NewReader(`{"name": ""}`))
		c.Request.Header.Set("Content-Type", "application/json")

		ctrls.CreateWorkspace(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request, got %d", w.Code)
		}
	})

	t.Run("success", func(t *testing.T) {
		ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
		defer cleanup()

		userID := uuid.New()
		wsID := uuid.New()
		memberID := uuid.New()

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		req := httptest.NewRequest(http.MethodPost, "/v1/workspaces", strings.NewReader(`{"name": "Engineering"}`))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID}))
		c.Request = req

		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "workspaces"`)).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(wsID, time.Now(), time.Now()))
		mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "workspace_members"`)).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(memberID, time.Now(), time.Now()))
		mock.ExpectCommit()

		ctrls.CreateWorkspace(c)
		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestControllers_GetWorkspaceDetail(t *testing.T) {
	t.Run("invalid uuid", func(t *testing.T) {
		ctrls, _, _, cleanup := setupRecordingTestControllers(t)
		defer cleanup()

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "id", Value: "invalid-uuid"}}
		c.Request = httptest.NewRequest(http.MethodGet, "/v1/workspaces/invalid-uuid", nil)

		ctrls.GetWorkspaceDetail(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request, got %d", w.Code)
		}
	})

	t.Run("success", func(t *testing.T) {
		ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
		defer cleanup()

		userID := uuid.New()
		wsID := uuid.New()

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "id", Value: wsID.String()}}
		req := httptest.NewRequest(http.MethodGet, "/v1/workspaces/"+wsID.String(), nil)
		req = req.WithContext(ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID}))
		c.Request = req

		memberRows := sqlmock.NewRows([]string{"id", "workspace_id", "user_id", "role", "created_at", "updated_at", "deleted_at"}).
			AddRow(uuid.New(), wsID, userID, "owner", time.Now(), time.Now(), nil)
		wsRows := sqlmock.NewRows([]string{"id", "name", "slug", "type", "owner_user_id", "created_at", "updated_at", "deleted_at"}).
			AddRow(wsID, "My Team", "my-team", "team", userID, time.Now(), time.Now(), nil)
		countRows := sqlmock.NewRows([]string{"count"}).AddRow(1)

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspace_members" WHERE (workspace_id = $1 AND user_id = $2) AND "workspace_members"."deleted_at" IS NULL`)).
			WithArgs(wsID, userID, 1).
			WillReturnRows(memberRows)

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspaces" WHERE id = $1 AND "workspaces"."deleted_at" IS NULL`)).
			WithArgs(wsID, 1).
			WillReturnRows(wsRows)

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "workspace_members" WHERE workspace_id = $1 AND "workspace_members"."deleted_at" IS NULL`)).
			WithArgs(wsID).
			WillReturnRows(countRows)

		ctrls.GetWorkspaceDetail(c)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestControllers_DeleteWorkspace(t *testing.T) {
	t.Run("invalid uuid", func(t *testing.T) {
		ctrls, _, _, cleanup := setupRecordingTestControllers(t)
		defer cleanup()

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "id", Value: "invalid-uuid"}}
		c.Request = httptest.NewRequest(http.MethodDelete, "/v1/workspaces/invalid-uuid", nil)

		ctrls.DeleteWorkspace(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request, got %d", w.Code)
		}
	})

	t.Run("success", func(t *testing.T) {
		ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
		defer cleanup()

		userID := uuid.New()
		wsID := uuid.New()

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "id", Value: wsID.String()}}
		req := httptest.NewRequest(http.MethodDelete, "/v1/workspaces/"+wsID.String(), nil)
		req = req.WithContext(ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID}))
		c.Request = req

		wsRows := sqlmock.NewRows([]string{"id", "name", "slug", "type", "owner_user_id", "created_at", "updated_at", "deleted_at"}).
			AddRow(wsID, "My Team", "my-team", "team", userID, time.Now(), time.Now(), nil)

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspaces" WHERE id = $1 AND "workspaces"."deleted_at" IS NULL`)).
			WithArgs(wsID, 1).
			WillReturnRows(wsRows)

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "workspaces" SET "deleted_at"=$1 WHERE id = $2 AND "workspaces"."deleted_at" IS NULL`)).
			WithArgs(sqlmock.AnyArg(), wsID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		ctrls.DeleteWorkspace(c)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestControllers_WorkspaceMembers(t *testing.T) {
	t.Run("UpdateWorkspaceMemberRole - invalid uuid", func(t *testing.T) {
		ctrls, _, _, cleanup := setupRecordingTestControllers(t)
		defer cleanup()

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "id", Value: "invalid"}, {Key: "userId", Value: uuid.New().String()}}
		c.Request = httptest.NewRequest(http.MethodPatch, "/v1/workspaces/invalid/members/123", strings.NewReader(`{"role":"admin"}`))

		ctrls.UpdateWorkspaceMemberRole(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request, got %d", w.Code)
		}
	})

	t.Run("RemoveWorkspaceMember - success", func(t *testing.T) {
		ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
		defer cleanup()

		ownerID := uuid.New()
		targetID := uuid.New()
		wsID := uuid.New()

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "id", Value: wsID.String()}, {Key: "userId", Value: targetID.String()}}
		req := httptest.NewRequest(http.MethodDelete, "/v1/workspaces/"+wsID.String()+"/members/"+targetID.String(), nil)
		req = req.WithContext(ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: ownerID}))
		c.Request = req

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
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "workspace_members" SET "deleted_at"=$1 WHERE (workspace_id = $2 AND user_id = $3) AND "workspace_members"."deleted_at" IS NULL`)).
			WithArgs(sqlmock.AnyArg(), wsID, targetID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		ctrls.RemoveWorkspaceMember(c)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
		}
	})
}
