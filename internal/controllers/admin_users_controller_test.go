package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/pkg/ctxmeta"
)

func TestControllers_AdminListUsers(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("success returns 200 with user list", func(t *testing.T) {
		ctrls, mock, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		userID := uuid.New()
		now := time.Now()

		mock.ExpectQuery(`SELECT count\(\*\) FROM "users"`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

		mock.ExpectQuery(`SELECT \* FROM "users"`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "email", "full_name", "status", "daily_quota_override", "email_verified", "created_at"}).
				AddRow(userID, "testuser@example.com", "Test User", constants.UserStatusActive, nil, true, now))

		mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings"`).
			WithArgs(userID, sqlmock.AnyArg()).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

		r := gin.New()
		r.GET("/v1/admin/users", ctrls.AdminListUsers)

		req := httptest.NewRequest(http.MethodGet, "/v1/admin/users?page=1&limit=10&status=active", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}

		var resp dtos.BaseResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if resp.Status != constants.ResponseStatusSuccess {
			t.Errorf("expected status success, got %s", resp.Status)
		}
	})

	t.Run("service failure returns mapped error response", func(t *testing.T) {
		ctrls, mock, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		mock.ExpectQuery(`SELECT count\(\*\) FROM "users"`).
			WillReturnError(gorm.ErrInvalidDB)

		r := gin.New()
		r.GET("/v1/admin/users", ctrls.AdminListUsers)

		req := httptest.NewRequest(http.MethodGet, "/v1/admin/users", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected status 500, got %d", w.Code)
		}
	})
}

func TestControllers_AdminOverrideUserQuota(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("success returns 200 with updated quota", func(t *testing.T) {
		ctrls, mock, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		adminID := uuid.New()
		userID := uuid.New()
		newQuota := 12

		mock.ExpectQuery(`SELECT \* FROM "users" WHERE id = \$1 AND "users"\."deleted_at" IS NULL ORDER BY "users"\."id" LIMIT \$2`).
			WithArgs(userID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "email", "full_name", "status", "daily_quota_override"}).
				AddRow(userID, "testuser@example.com", "Test User", "active", nil))

		mock.ExpectBegin()
		mock.ExpectExec(`UPDATE "users" SET "daily_quota_override"=\$1,"updated_at"=\$2 WHERE id = \$3 AND "users"\."deleted_at" IS NULL`).
			WithArgs(newQuota, sqlmock.AnyArg(), userID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		mock.ExpectBegin()
		mock.ExpectQuery(`INSERT INTO "admin_audit_logs"`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(uuid.New(), time.Now()))
		mock.ExpectCommit()

		r := gin.New()
		r.Use(func(c *gin.Context) {
			ctx := ctxmeta.WithAdminAuthUser(c.Request.Context(), ctxmeta.AdminAuthUser{
				AdminID:  adminID,
				Username: "admin_tester",
			})
			c.Request = c.Request.WithContext(ctx)
			c.Next()
		})
		r.PATCH("/v1/admin/users/:id/quota", ctrls.AdminOverrideUserQuota)

		body := `{"daily_quota_override": 12}`
		req := httptest.NewRequest(http.MethodPatch, "/v1/admin/users/"+userID.String()+"/quota", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("invalid UUID returns 400", func(t *testing.T) {
		ctrls, _, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		r := gin.New()
		r.PATCH("/v1/admin/users/:id/quota", ctrls.AdminOverrideUserQuota)

		req := httptest.NewRequest(http.MethodPatch, "/v1/admin/users/invalid-uuid/quota", strings.NewReader(`{"daily_quota_override": 5}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", w.Code)
		}
	})

	t.Run("missing admin context returns 401", func(t *testing.T) {
		ctrls, _, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		userID := uuid.New()
		r := gin.New()
		r.PATCH("/v1/admin/users/:id/quota", ctrls.AdminOverrideUserQuota)

		req := httptest.NewRequest(http.MethodPatch, "/v1/admin/users/"+userID.String()+"/quota", strings.NewReader(`{"daily_quota_override": 5}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", w.Code)
		}
	})

	t.Run("invalid json returns 400", func(t *testing.T) {
		ctrls, _, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		adminID := uuid.New()
		userID := uuid.New()

		r := gin.New()
		r.Use(func(c *gin.Context) {
			ctx := ctxmeta.WithAdminAuthUser(c.Request.Context(), ctxmeta.AdminAuthUser{
				AdminID:  adminID,
				Username: "admin_tester",
			})
			c.Request = c.Request.WithContext(ctx)
			c.Next()
		})
		r.PATCH("/v1/admin/users/:id/quota", ctrls.AdminOverrideUserQuota)

		req := httptest.NewRequest(http.MethodPatch, "/v1/admin/users/"+userID.String()+"/quota", strings.NewReader(`invalid-json`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", w.Code)
		}
	})
}

func TestControllers_AdminOverrideUserMaxFileSize(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("success returns 200 with updated max file size", func(t *testing.T) {
		ctrls, mock, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		adminID := uuid.New()
		userID := uuid.New()
		newSize := 1024

		mock.ExpectQuery(`SELECT \* FROM "users" WHERE id = \$1 AND "users"\."deleted_at" IS NULL ORDER BY "users"\."id" LIMIT \$2`).
			WithArgs(userID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "email", "full_name", "status", "max_file_size_mb"}).
				AddRow(userID, "testuser@example.com", "Test User", "active", nil))

		mock.ExpectBegin()
		mock.ExpectExec(`UPDATE "users" SET "max_file_size_mb"=\$1,"updated_at"=\$2 WHERE id = \$3 AND "users"\."deleted_at" IS NULL`).
			WithArgs(newSize, sqlmock.AnyArg(), userID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		mock.ExpectBegin()
		mock.ExpectQuery(`INSERT INTO "admin_audit_logs"`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(uuid.New(), time.Now()))
		mock.ExpectCommit()

		r := gin.New()
		r.Use(func(c *gin.Context) {
			ctx := ctxmeta.WithAdminAuthUser(c.Request.Context(), ctxmeta.AdminAuthUser{
				AdminID:  adminID,
				Username: "admin_tester",
			})
			c.Request = c.Request.WithContext(ctx)
			c.Next()
		})
		r.PATCH("/v1/admin/users/:id/max-file-size", ctrls.AdminOverrideUserMaxFileSize)

		body := `{"max_file_size_mb": 1024}`
		req := httptest.NewRequest(http.MethodPatch, "/v1/admin/users/"+userID.String()+"/max-file-size", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("invalid UUID returns 400", func(t *testing.T) {
		ctrls, _, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		r := gin.New()
		r.PATCH("/v1/admin/users/:id/max-file-size", ctrls.AdminOverrideUserMaxFileSize)

		req := httptest.NewRequest(http.MethodPatch, "/v1/admin/users/invalid-uuid/max-file-size", strings.NewReader(`{"max_file_size_mb": 1024}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", w.Code)
		}
	})

	t.Run("missing admin context returns 401", func(t *testing.T) {
		ctrls, _, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		userID := uuid.New()
		r := gin.New()
		r.PATCH("/v1/admin/users/:id/max-file-size", ctrls.AdminOverrideUserMaxFileSize)

		req := httptest.NewRequest(http.MethodPatch, "/v1/admin/users/"+userID.String()+"/max-file-size", strings.NewReader(`{"max_file_size_mb": 1024}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", w.Code)
		}
	})

	t.Run("invalid json returns 400", func(t *testing.T) {
		ctrls, _, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		adminID := uuid.New()
		userID := uuid.New()

		r := gin.New()
		r.Use(func(c *gin.Context) {
			ctx := ctxmeta.WithAdminAuthUser(c.Request.Context(), ctxmeta.AdminAuthUser{
				AdminID:  adminID,
				Username: "admin_tester",
			})
			c.Request = c.Request.WithContext(ctx)
			c.Next()
		})
		r.PATCH("/v1/admin/users/:id/max-file-size", ctrls.AdminOverrideUserMaxFileSize)

		req := httptest.NewRequest(http.MethodPatch, "/v1/admin/users/"+userID.String()+"/max-file-size", strings.NewReader(`invalid-json`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", w.Code)
		}
	})
}

func TestControllers_AdminRevokeUserSessions(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("success returns 200", func(t *testing.T) {
		ctrls, mock, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		adminID := uuid.New()
		userID := uuid.New()

		mock.ExpectQuery(`SELECT \* FROM "users" WHERE id = \$1 AND "users"\."deleted_at" IS NULL ORDER BY "users"\."id" LIMIT \$2`).
			WithArgs(userID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "email", "full_name", "status"}).
				AddRow(userID, "testuser@example.com", "Test User", "active"))

		mock.ExpectBegin()
		mock.ExpectExec(`UPDATE "auth_tokens" SET "revoked_at"=\$1,"updated_at"=\$2 WHERE user_id = \$3 AND revoked_at IS NULL`).
			WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), userID).
			WillReturnResult(sqlmock.NewResult(1, 2))
		mock.ExpectCommit()

		mock.ExpectBegin()
		mock.ExpectQuery(`INSERT INTO "admin_audit_logs"`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(uuid.New(), time.Now()))
		mock.ExpectCommit()

		r := gin.New()
		r.Use(func(c *gin.Context) {
			ctx := ctxmeta.WithAdminAuthUser(c.Request.Context(), ctxmeta.AdminAuthUser{
				AdminID:  adminID,
				Username: "admin_tester",
			})
			c.Request = c.Request.WithContext(ctx)
			c.Next()
		})
		r.POST("/v1/admin/users/:id/revoke-sessions", ctrls.AdminRevokeUserSessions)

		req := httptest.NewRequest(http.MethodPost, "/v1/admin/users/"+userID.String()+"/revoke-sessions", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("invalid UUID returns 400", func(t *testing.T) {
		ctrls, _, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		r := gin.New()
		r.POST("/v1/admin/users/:id/revoke-sessions", ctrls.AdminRevokeUserSessions)

		req := httptest.NewRequest(http.MethodPost, "/v1/admin/users/invalid-uuid/revoke-sessions", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", w.Code)
		}
	})

	t.Run("missing admin context returns 401", func(t *testing.T) {
		ctrls, _, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		userID := uuid.New()
		r := gin.New()
		r.POST("/v1/admin/users/:id/revoke-sessions", ctrls.AdminRevokeUserSessions)

		req := httptest.NewRequest(http.MethodPost, "/v1/admin/users/"+userID.String()+"/revoke-sessions", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", w.Code)
		}
	})
}

func TestControllers_AdminAssignUserRole(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("success returns 200", func(t *testing.T) {
		ctrls, mock, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		adminID := uuid.New()
		userID := uuid.New()
		roleID := uuid.New()
		now := time.Now()

		// 1. FindUserByID
		mock.ExpectQuery(`SELECT \* FROM "users" WHERE id = \$1 AND "users"\."deleted_at" IS NULL ORDER BY "users"\."id" LIMIT \$2`).
			WithArgs(userID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "email", "full_name", "status"}).
				AddRow(userID, "user@example.com", "Test User", constants.UserStatusActive))

		// 2. FindUserRoleByID
		mock.ExpectQuery(`SELECT \* FROM "user_roles" WHERE id = \$1 ORDER BY "user_roles"\."id" LIMIT \$2`).
			WithArgs(roleID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name", "code", "daily_quota"}).
				AddRow(roleID, "Pro Member", "PRO", 25))

		// 3. UpdateUserRoleID
		mock.ExpectBegin()
		mock.ExpectExec(`UPDATE "users" SET "role_id"=\$1,"updated_at"=\$2 WHERE`).
			WithArgs(roleID, sqlmock.AnyArg(), userID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		// 4. FindUserByIDWithRole (preload)
		mock.ExpectQuery(`SELECT \* FROM "users" WHERE id = \$1 AND "users"\."deleted_at" IS NULL ORDER BY "users"\."id" LIMIT \$2`).
			WithArgs(userID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "email", "full_name", "status", "role_id", "created_at"}).
				AddRow(userID, "user@example.com", "Test User", constants.UserStatusActive, roleID, now))

		mock.ExpectQuery(`SELECT \* FROM "user_roles" WHERE "user_roles"\."id" = \$1`).
			WithArgs(roleID).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name", "code", "daily_quota", "permissions"}).
				AddRow(roleID, "Pro Member", "PRO", 25, []byte(`["recordings:*"]`)))

		// 5. AuditLog
		mock.ExpectBegin()
		mock.ExpectQuery(`INSERT INTO "admin_audit_logs"`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(uuid.New(), time.Now()))
		mock.ExpectCommit()

		r := gin.New()
		r.Use(func(c *gin.Context) {
			ctx := ctxmeta.WithAdminAuthUser(c.Request.Context(), ctxmeta.AdminAuthUser{
				AdminID:  adminID,
				Username: "admin_tester",
			})
			c.Request = c.Request.WithContext(ctx)
			c.Next()
		})
		r.PATCH("/v1/admin/users/:id/role", ctrls.AdminAssignUserRole)

		body := fmt.Sprintf(`{"role_id": "%s"}`, roleID)
		req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/v1/admin/users/%s/role", userID), strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled expectations: %v", err)
		}
	})
}
