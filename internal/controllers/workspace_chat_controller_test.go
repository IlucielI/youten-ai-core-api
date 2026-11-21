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

	"code-base-golang/internal/dtos"
	"code-base-golang/internal/pkg/ctxmeta"
)

func TestControllers_ListWorkspaceChatSessions(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("invalid workspace id", func(t *testing.T) {
		ctrls, _, _, cleanup := setupRecordingTestControllers(t)
		defer cleanup()

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "id", Value: "invalid-uuid"}}
		c.Request = httptest.NewRequest(http.MethodGet, "/v1/workspaces/invalid-uuid/chat/sessions", nil)

		ctrls.ListWorkspaceChatSessions(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request, got %d", w.Code)
		}
	})

	t.Run("success listing sessions", func(t *testing.T) {
		ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
		defer cleanup()

		userID := uuid.New()
		wsID := uuid.New()
		sessID := uuid.New()

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "id", Value: wsID.String()}}
		req := httptest.NewRequest(http.MethodGet, "/v1/workspaces/"+wsID.String()+"/chat/sessions", nil)
		c.Request = req.WithContext(ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID}))

		// Member check
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspace_members" WHERE (workspace_id = $1 AND user_id = $2) AND "workspace_members"."deleted_at" IS NULL`)).
			WithArgs(wsID, userID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"workspace_id", "user_id", "role"}).
				AddRow(wsID, userID, "member"))

		// Find sessions
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspace_chat_sessions" WHERE (workspace_id = $1 AND user_id = $2) AND "workspace_chat_sessions"."deleted_at" IS NULL ORDER BY updated_at DESC`)).
			WithArgs(wsID, userID).
			WillReturnRows(sqlmock.NewRows([]string{"id", "workspace_id", "user_id", "title", "updated_at"}).
				AddRow(sessID, wsID, userID, "Planning Chat", time.Now()))

		// Count messages
		mock.ExpectQuery(`SELECT session_id, count\(\*\) as count FROM "workspace_chat_messages"`).
			WithArgs(sessID).
			WillReturnRows(sqlmock.NewRows([]string{"session_id", "count"}).AddRow(sessID, 2))

		ctrls.ListWorkspaceChatSessions(c)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestControllers_CreateWorkspaceChatSession(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("success creating session", func(t *testing.T) {
		ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
		defer cleanup()

		userID := uuid.New()
		wsID := uuid.New()
		sessID := uuid.New()

		body, _ := json.Marshal(dtos.CreateWorkspaceChatSessionRequest{
			Title: "Sprint 42 Chat",
		})

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "id", Value: wsID.String()}}
		req := httptest.NewRequest(http.MethodPost, "/v1/workspaces/"+wsID.String()+"/chat/sessions", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		c.Request = req.WithContext(ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID}))

		// Member check
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspace_members" WHERE (workspace_id = $1 AND user_id = $2) AND "workspace_members"."deleted_at" IS NULL`)).
			WithArgs(wsID, userID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"workspace_id", "user_id", "role"}).
				AddRow(wsID, userID, "member"))

		// Insert session
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "workspace_chat_sessions"`)).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).
				AddRow(sessID, time.Now(), time.Now()))
		mock.ExpectCommit()

		ctrls.CreateWorkspaceChatSession(c)
		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestControllers_DeleteWorkspaceChatSession(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("success deleting session", func(t *testing.T) {
		ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
		defer cleanup()

		userID := uuid.New()
		wsID := uuid.New()
		sessID := uuid.New()

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{
			{Key: "id", Value: wsID.String()},
			{Key: "sessionId", Value: sessID.String()},
		}
		req := httptest.NewRequest(http.MethodDelete, "/v1/workspaces/"+wsID.String()+"/chat/sessions/"+sessID.String(), nil)
		c.Request = req.WithContext(ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID}))

		// Find session
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspace_chat_sessions" WHERE id = $1 AND "workspace_chat_sessions"."deleted_at" IS NULL`)).
			WithArgs(sessID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "workspace_id", "user_id", "title"}).
				AddRow(sessID, wsID, userID, "Sprint 42 Chat"))

		// Delete session
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "workspace_chat_sessions" SET "deleted_at"=$1 WHERE id = $2 AND "workspace_chat_sessions"."deleted_at" IS NULL`)).
			WithArgs(sqlmock.AnyArg(), sessID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		ctrls.DeleteWorkspaceChatSession(c)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
		}
	})
}
