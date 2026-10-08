package controllers

import (
	"context"
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
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"code-base-golang/internal/adapters/llm"
	"code-base-golang/internal/config"
	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
	"code-base-golang/internal/pkg/ctxmeta"
	"code-base-golang/internal/repositories"
	"code-base-golang/internal/services"
)

func setupAdminTestControllers(t *testing.T) (*Controllers, sqlmock.Sqlmock, func()) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}

	gormDB, err := gorm.Open(postgres.New(postgres.Config{
		Conn: sqlDB,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open gorm: %v", err)
	}

	repo := repositories.New(gormDB, nil)
	cfg := config.Config{
		AppName: "youten-admin-ctrl-test",
	}

	svc := services.New(cfg, repo, nil)
	ctrls := New(cfg, svc)

	cleanup := func() {
		sqlDB.Close()
	}

	return ctrls, mock, cleanup
}

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

func TestControllers_AdminListRoles(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("success returns 200 with role list", func(t *testing.T) {
		ctrls, mock, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		roleID := uuid.New()
		now := time.Now()

		mock.ExpectQuery(`SELECT \* FROM "admin_roles"`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name", "description", "permissions", "is_system", "created_at", "updated_at"}).
				AddRow(roleID, "Support", "Support Staff", []byte(`["users:read"]`), false, now, now))

		r := gin.New()
		r.GET("/v1/admin/roles", ctrls.AdminListRoles)

		req := httptest.NewRequest(http.MethodGet, "/v1/admin/roles", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("service failure returns 500", func(t *testing.T) {
		ctrls, mock, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		mock.ExpectQuery(`SELECT \* FROM "admin_roles"`).
			WillReturnError(gorm.ErrInvalidDB)

		r := gin.New()
		r.GET("/v1/admin/roles", ctrls.AdminListRoles)

		req := httptest.NewRequest(http.MethodGet, "/v1/admin/roles", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected status 500, got %d", w.Code)
		}
	})
}

func TestControllers_AdminCreateRole(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("success returns 201", func(t *testing.T) {
		ctrls, mock, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		adminID := uuid.New()

		mock.ExpectQuery(`SELECT \* FROM "admin_roles" WHERE name = \$1 ORDER BY "admin_roles"\."id" LIMIT \$2`).
			WithArgs("Support Agent", 1).
			WillReturnError(gorm.ErrRecordNotFound)

		mock.ExpectBegin()
		mock.ExpectQuery(`INSERT INTO "admin_roles"`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(uuid.New(), time.Now(), time.Now()))
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
		r.POST("/v1/admin/roles", ctrls.AdminCreateRole)

		body := `{"name": "Support Agent", "description": "Support role", "permissions": ["users:read"]}`
		req := httptest.NewRequest(http.MethodPost, "/v1/admin/roles", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("expected status 201, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		ctrls, _, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		r := gin.New()
		r.POST("/v1/admin/roles", ctrls.AdminCreateRole)

		body := `{"name": "", "permissions": []}`
		req := httptest.NewRequest(http.MethodPost, "/v1/admin/roles", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", w.Code)
		}
	})
}

func TestControllers_AdminUpdateRole(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("success returns 200", func(t *testing.T) {
		ctrls, mock, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		adminID := uuid.New()
		roleID := uuid.New()

		mock.ExpectQuery(`SELECT \* FROM "admin_roles" WHERE id = \$1 ORDER BY "admin_roles"\."id" LIMIT \$2`).
			WithArgs(roleID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name", "description", "permissions", "is_system", "created_at", "updated_at"}).
				AddRow(roleID, "Role A", "Old", []byte(`["users:read"]`), false, time.Now(), time.Now()))

		mock.ExpectQuery(`SELECT \* FROM "admin_roles" WHERE name = \$1 ORDER BY "admin_roles"\."id" LIMIT \$2`).
			WithArgs("Role B", 1).
			WillReturnError(gorm.ErrRecordNotFound)

		mock.ExpectBegin()
		mock.ExpectExec(`UPDATE "admin_roles" SET`).
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
		r.PUT("/v1/admin/roles/:id", ctrls.AdminUpdateRole)

		body := `{"name": "Role B", "description": "New", "permissions": ["users:read", "users:write"]}`
		req := httptest.NewRequest(http.MethodPut, "/v1/admin/roles/"+roleID.String(), strings.NewReader(body))
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
		r.PUT("/v1/admin/roles/:id", ctrls.AdminUpdateRole)

		req := httptest.NewRequest(http.MethodPut, "/v1/admin/roles/bad-uuid", strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", w.Code)
		}
	})
}

func TestControllers_AdminListTemplates(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("success returns 200 with template list", func(t *testing.T) {
		ctrls, mock, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		tmplID := uuid.New()
		now := time.Now()

		mock.ExpectQuery(`SELECT \* FROM "templates" ORDER BY category_key ASC`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "category_key", "name", "description", "prompt", "output_schema", "version", "is_active", "created_at", "updated_at"}).
				AddRow(tmplID, "MOM", "Minutes of Meeting", "Standard MOM", "Prompt", []byte(`{}`), 1, true, now, now))

		r := gin.New()
		r.GET("/v1/admin/templates", ctrls.AdminListTemplates)

		req := httptest.NewRequest(http.MethodGet, "/v1/admin/templates", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("service failure returns 500", func(t *testing.T) {
		ctrls, mock, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		mock.ExpectQuery(`SELECT \* FROM "templates" ORDER BY category_key ASC`).
			WillReturnError(gorm.ErrInvalidDB)

		r := gin.New()
		r.GET("/v1/admin/templates", ctrls.AdminListTemplates)

		req := httptest.NewRequest(http.MethodGet, "/v1/admin/templates", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected status 500, got %d", w.Code)
		}
	})
}

func TestControllers_AdminCreateTemplate(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("success returns 201", func(t *testing.T) {
		ctrls, mock, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		adminID := uuid.New()

		mock.ExpectQuery(`SELECT \* FROM "templates" WHERE category_key = \$1 ORDER BY "templates"\."id" LIMIT \$2`).
			WithArgs("TUTORIAL", 1).
			WillReturnError(gorm.ErrRecordNotFound)

		mock.ExpectBegin()
		mock.ExpectQuery(`INSERT INTO "templates"`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(uuid.New(), time.Now(), time.Now()))
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
		r.POST("/v1/admin/templates", ctrls.AdminCreateTemplate)

		body := `{"category_key": "TUTORIAL", "name": "Tutorial", "prompt": "Extract steps", "output_schema": {"type": "object", "properties": {"steps": {"type": "array"}}}}`
		req := httptest.NewRequest(http.MethodPost, "/v1/admin/templates", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("expected status 201, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		ctrls, _, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		r := gin.New()
		r.POST("/v1/admin/templates", ctrls.AdminCreateTemplate)

		body := `{"category_key": "", "name": ""}`
		req := httptest.NewRequest(http.MethodPost, "/v1/admin/templates", strings.NewReader(body))
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

		r := gin.New()
		r.POST("/v1/admin/templates", ctrls.AdminCreateTemplate)

		body := `{"category_key": "TUTORIAL", "name": "Tutorial", "prompt": "Extract steps", "output_schema": {"type": "object", "properties": {"steps": {"type": "array"}}}}`
		req := httptest.NewRequest(http.MethodPost, "/v1/admin/templates", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", w.Code)
		}
	})
}

func TestControllers_AdminUpdateTemplate(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("success returns 200", func(t *testing.T) {
		ctrls, mock, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		adminID := uuid.New()
		tmplID := uuid.New()
		now := time.Now()

		mock.ExpectQuery(`SELECT \* FROM "templates" WHERE id = \$1 ORDER BY "templates"\."id" LIMIT \$2`).
			WithArgs(tmplID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "category_key", "name", "description", "prompt", "output_schema", "version", "is_active", "created_at", "updated_at"}).
				AddRow(tmplID, "MOM", "Old Name", "Old Desc", "Old prompt", []byte(`{"type":"object","properties":{"title":{"type":"string"}}}`), 1, true, now, now))

		mock.ExpectBegin()
		mock.ExpectExec(`UPDATE "templates" SET`).
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
		r.PUT("/v1/admin/templates/:id", ctrls.AdminUpdateTemplate)

		body := `{"name": "New Name", "prompt": "New prompt", "output_schema": {"type": "object", "properties": {"title": {"type": "string"}}}}`
		req := httptest.NewRequest(http.MethodPut, "/v1/admin/templates/"+tmplID.String(), strings.NewReader(body))
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
		r.PUT("/v1/admin/templates/:id", ctrls.AdminUpdateTemplate)

		req := httptest.NewRequest(http.MethodPut, "/v1/admin/templates/not-a-uuid", strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", w.Code)
		}
	})
}

func TestControllers_AdminTestTemplate(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("success returns 200 with test results", func(t *testing.T) {
		ctrls, _, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		adminID := uuid.New()
		mockLLM := llm.NewMock()
		mockLLM.GenerateStructuredFunc = func(ctx context.Context, systemPrompt, userPrompt string, schema map[string]interface{}) (*dtos.StructuredResponse, error) {
			return &dtos.StructuredResponse{
				RawJSON: `{"title": "Sample Title"}`,
				Usage: dtos.LLMUsage{
					TotalTokens: 42,
				},
			}, nil
		}
		ctrls.Service().SetLLM(mockLLM)

		r := gin.New()
		r.Use(func(c *gin.Context) {
			ctx := ctxmeta.WithAdminAuthUser(c.Request.Context(), ctxmeta.AdminAuthUser{
				AdminID:  adminID,
				Username: "admin_tester",
			})
			c.Request = c.Request.WithContext(ctx)
			c.Next()
		})
		r.POST("/v1/admin/templates/test", ctrls.AdminTestTemplate)

		body := `{"prompt": "Test Prompt", "sample_transcript": "Transcript", "output_schema": {"type": "object", "properties": {"title": {"type": "string"}}}}`
		req := httptest.NewRequest(http.MethodPost, "/v1/admin/templates/test", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("missing admin context returns 401", func(t *testing.T) {
		ctrls, _, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		r := gin.New()
		r.POST("/v1/admin/templates/test", ctrls.AdminTestTemplate)

		body := `{"prompt": "Test", "sample_transcript": "Text", "output_schema": {"type": "object", "properties": {"a": {"type": "string"}}}}`
		req := httptest.NewRequest(http.MethodPost, "/v1/admin/templates/test", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", w.Code)
		}
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		ctrls, _, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		r := gin.New()
		r.POST("/v1/admin/templates/test", ctrls.AdminTestTemplate)

		body := `{"prompt": "", "sample_transcript": ""}`
		req := httptest.NewRequest(http.MethodPost, "/v1/admin/templates/test", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", w.Code)
		}
	})
}

func TestControllers_AdminGetDLQPipeline(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("success returns 200 with DLQ items", func(t *testing.T) {
		ctrls, mock, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		adminID := uuid.New()
		recID := uuid.New()
		now := time.Now()

		mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings" WHERE status = 'FAILED' AND "recordings"\."deleted_at" IS NULL`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

		mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings" WHERE \(status IN \('QUEUED', 'VALIDATING', 'EXTRACTING', 'TRANSCRIBING', 'SUMMARIZING', 'INDEXING'\) AND updated_at < \$1\) AND "recordings"\."deleted_at" IS NULL`).
			WithArgs(sqlmock.AnyArg()).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

		mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE \(\(status = 'FAILED'\) OR \(status IN \('QUEUED', 'VALIDATING', 'EXTRACTING', 'TRANSCRIBING', 'SUMMARIZING', 'INDEXING'\) AND updated_at < \$1\)\) AND "recordings"\."deleted_at" IS NULL ORDER BY updated_at DESC LIMIT \$2`).
			WithArgs(sqlmock.AnyArg(), 20).
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "title", "original_filename", "status", "source_type", "created_at", "updated_at",
			}).
				AddRow(recID, "Failed Meeting", "meeting.mp4", models.RecordingStatusFailed, "UPLOAD", now, now))

		r := gin.New()
		r.Use(func(c *gin.Context) {
			ctx := ctxmeta.WithAdminAuthUser(c.Request.Context(), ctxmeta.AdminAuthUser{
				AdminID:  adminID,
				Username: "ops_admin",
			})
			c.Request = c.Request.WithContext(ctx)
			c.Next()
		})
		r.GET("/v1/admin/pipeline/dlq", ctrls.AdminGetDLQPipeline)

		req := httptest.NewRequest(http.MethodGet, "/v1/admin/pipeline/dlq?page=1&limit=20", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d, body: %s", w.Code, w.Body.String())
		}

		var apiResp dtos.APIResponse[dtos.DLQMessagesResponse]
		if err := json.Unmarshal(w.Body.Bytes(), &apiResp); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}
		if apiResp.Data.Total != 1 {
			t.Fatalf("expected total 1, got %d", apiResp.Data.Total)
		}
	})

	t.Run("unauthorized returns 401", func(t *testing.T) {
		ctrls, _, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		r := gin.New()
		r.GET("/v1/admin/pipeline/dlq", ctrls.AdminGetDLQPipeline)

		req := httptest.NewRequest(http.MethodGet, "/v1/admin/pipeline/dlq", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", w.Code)
		}
	})
}

func TestControllers_AdminRetryDLQJob(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("success returns 200 with retry response", func(t *testing.T) {
		ctrls, mock, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		adminID := uuid.New()
		recID := uuid.New()
		now := time.Now()

		mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL`).
			WithArgs(recID, 1).
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "status", "title", "original_filename", "selected_template", "output_language", "audio_url", "created_at", "updated_at",
			}).
				AddRow(recID, models.RecordingStatusFailed, "Meeting", "meet.mp4", "GENERAL", "id", nil, now, now))

		mock.ExpectQuery(`SELECT \* FROM "transcript_segments" WHERE recording_id = \$1`).
			WithArgs(recID).
			WillReturnRows(sqlmock.NewRows([]string{"id"}))

		mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND is_active = TRUE`).
			WithArgs(recID, 1).
			WillReturnError(gorm.ErrRecordNotFound)

		mock.ExpectQuery(`SELECT .* FROM "transcript_chunks" WHERE recording_id = \$1 ORDER BY chunk_index ASC`).
			WithArgs(recID).
			WillReturnRows(sqlmock.NewRows([]string{"id"}))

		mock.ExpectBegin()
		mock.ExpectExec(`UPDATE "recordings" SET .* WHERE id = .*`).
			WithArgs(models.RecordingStatusQueued, sqlmock.AnyArg(), recID).
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
				Username: "ops_admin",
			})
			c.Request = c.Request.WithContext(ctx)
			c.Next()
		})
		r.POST("/v1/admin/pipeline/dlq/retry", ctrls.AdminRetryDLQJob)

		body := fmt.Sprintf(`{"recording_id": "%s"}`, recID.String())
		req := httptest.NewRequest(http.MethodPost, "/v1/admin/pipeline/dlq/retry", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("unauthorized returns 401", func(t *testing.T) {
		ctrls, _, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		r := gin.New()
		r.POST("/v1/admin/pipeline/dlq/retry", ctrls.AdminRetryDLQJob)

		body := fmt.Sprintf(`{"recording_id": "%s"}`, uuid.New().String())
		req := httptest.NewRequest(http.MethodPost, "/v1/admin/pipeline/dlq/retry", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", w.Code)
		}
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		ctrls, _, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		r := gin.New()
		r.Use(func(c *gin.Context) {
			ctx := ctxmeta.WithAdminAuthUser(c.Request.Context(), ctxmeta.AdminAuthUser{
				AdminID:  uuid.New(),
				Username: "ops_admin",
			})
			c.Request = c.Request.WithContext(ctx)
			c.Next()
		})
		r.POST("/v1/admin/pipeline/dlq/retry", ctrls.AdminRetryDLQJob)

		body := `{"recording_id": "00000000-0000-0000-0000-000000000000"}`
		req := httptest.NewRequest(http.MethodPost, "/v1/admin/pipeline/dlq/retry", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", w.Code)
		}
	})
}

func TestControllers_AdminSystemConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("AdminGetSystemConfig success returns 200", func(t *testing.T) {
		ctrls, _, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		r := gin.New()
		r.Use(func(c *gin.Context) {
			ctx := ctxmeta.WithAdminAuthUser(c.Request.Context(), ctxmeta.AdminAuthUser{
				AdminID:  uuid.New(),
				Username: "sysadmin",
			})
			c.Request = c.Request.WithContext(ctx)
			c.Next()
		})
		r.GET("/v1/admin/config", ctrls.AdminGetSystemConfig)

		req := httptest.NewRequest(http.MethodGet, "/v1/admin/config", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("AdminGetSystemConfig unauthorized without context", func(t *testing.T) {
		ctrls, _, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		r := gin.New()
		r.GET("/v1/admin/config", ctrls.AdminGetSystemConfig)

		req := httptest.NewRequest(http.MethodGet, "/v1/admin/config", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", w.Code)
		}
	})

	t.Run("AdminUpdateSystemConfig success returns 200 and logs audit", func(t *testing.T) {
		ctrls, mock, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		adminID := uuid.New()

		mock.ExpectBegin()
		mock.ExpectQuery(`INSERT INTO "admin_audit_logs"`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(uuid.New(), time.Now()))
		mock.ExpectCommit()

		r := gin.New()
		r.Use(func(c *gin.Context) {
			ctx := ctxmeta.WithAdminAuthUser(c.Request.Context(), ctxmeta.AdminAuthUser{
				AdminID:  adminID,
				Username: "sysadmin",
			})
			c.Request = c.Request.WithContext(ctx)
			c.Next()
		})
		r.PATCH("/v1/admin/config", ctrls.AdminUpdateSystemConfig)

		body := `{"maintenance_mode": true, "allow_guest_uploads": false}`
		req := httptest.NewRequest(http.MethodPatch, "/v1/admin/config", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("AdminUpdateSystemConfig validation error returns 400", func(t *testing.T) {
		ctrls, _, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		r := gin.New()
		r.Use(func(c *gin.Context) {
			ctx := ctxmeta.WithAdminAuthUser(c.Request.Context(), ctxmeta.AdminAuthUser{
				AdminID:  uuid.New(),
				Username: "sysadmin",
			})
			c.Request = c.Request.WithContext(ctx)
			c.Next()
		})
		r.PATCH("/v1/admin/config", ctrls.AdminUpdateSystemConfig)

		body := `{}`
		req := httptest.NewRequest(http.MethodPatch, "/v1/admin/config", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", w.Code)
		}
	})
}

func TestControllers_AdminListReports(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("success returns 200", func(t *testing.T) {
		ctrls, mock, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		repID := uuid.New()
		recID := uuid.New()
		now := time.Now()

		mock.ExpectQuery(`SELECT count\(\*\) FROM "reports"`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

		mock.ExpectQuery(`SELECT \* FROM "reports" ORDER BY created_at DESC LIMIT \$1`).
			WithArgs(10).
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "recording_id", "reporter_type", "reason", "status", "created_at", "updated_at",
			}).AddRow(
				repID, recID, "guest", "spam", "open", now, now,
			))

		mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE "recordings"\."id" = \$1 AND "recordings"\."deleted_at" IS NULL`).
			WithArgs(recID).
			WillReturnRows(sqlmock.NewRows([]string{"id", "title"}).AddRow(recID, "Target Recording"))

		r := gin.New()
		r.Use(func(c *gin.Context) {
			ctx := ctxmeta.WithAdminAuthUser(c.Request.Context(), ctxmeta.AdminAuthUser{
				AdminID:  uuid.New(),
				Username: "moderator",
			})
			c.Request = c.Request.WithContext(ctx)
			c.Next()
		})
		r.GET("/v1/admin/reports", ctrls.AdminListReports)

		req := httptest.NewRequest(http.MethodGet, "/v1/admin/reports?page=1&limit=10", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("unauthorized without admin context returns 401", func(t *testing.T) {
		ctrls, _, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		r := gin.New()
		r.GET("/v1/admin/reports", ctrls.AdminListReports)

		req := httptest.NewRequest(http.MethodGet, "/v1/admin/reports", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", w.Code)
		}
	})
}

func TestControllers_AdminResolveReport(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("success returns 200", func(t *testing.T) {
		ctrls, mock, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		repID := uuid.New()
		recID := uuid.New()
		adminID := uuid.New()
		now := time.Now()

		mock.ExpectQuery(`SELECT \* FROM "reports" WHERE id = \$1 ORDER BY "reports"\."id" LIMIT \$2`).
			WithArgs(repID, 1).
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "recording_id", "reporter_type", "reason", "status", "created_at", "updated_at",
			}).AddRow(repID, recID, "guest", "spam", "open", now, now))

		mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE "recordings"\."id" = \$1 AND "recordings"\."deleted_at" IS NULL`).
			WithArgs(recID).
			WillReturnRows(sqlmock.NewRows([]string{"id", "title"}).AddRow(recID, "Target"))

		mock.ExpectBegin()
		mock.ExpectExec(`UPDATE "reports" SET .* WHERE id = .*`).
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
				Username: "moderator",
			})
			c.Request = c.Request.WithContext(ctx)
			c.Next()
		})
		r.POST("/v1/admin/reports/:id/resolve", ctrls.AdminResolveReport)

		body := `{"action": "DISMISS", "resolution_note": "Spam dismiss"}`
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/v1/admin/reports/%s/resolve", repID.String()), strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		ctrls, _, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		repID := uuid.New()
		r := gin.New()
		r.Use(func(c *gin.Context) {
			ctx := ctxmeta.WithAdminAuthUser(c.Request.Context(), ctxmeta.AdminAuthUser{
				AdminID:  uuid.New(),
				Username: "moderator",
			})
			c.Request = c.Request.WithContext(ctx)
			c.Next()
		})
		r.POST("/v1/admin/reports/:id/resolve", ctrls.AdminResolveReport)

		body := `{"action": "INVALID_ACTION"}`
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/v1/admin/reports/%s/resolve", repID.String()), strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", w.Code)
		}
	})

	t.Run("invalid id param returns 400", func(t *testing.T) {
		ctrls, _, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		r := gin.New()
		r.Use(func(c *gin.Context) {
			ctx := ctxmeta.WithAdminAuthUser(c.Request.Context(), ctxmeta.AdminAuthUser{
				AdminID:  uuid.New(),
				Username: "moderator",
			})
			c.Request = c.Request.WithContext(ctx)
			c.Next()
		})
		r.POST("/v1/admin/reports/:id/resolve", ctrls.AdminResolveReport)

		body := `{"action": "DISMISS"}`
		req := httptest.NewRequest(http.MethodPost, "/v1/admin/reports/invalid-uuid/resolve", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", w.Code)
		}
	})

	t.Run("unauthorized without admin context returns 401", func(t *testing.T) {
		ctrls, _, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		repID := uuid.New()
		r := gin.New()
		r.POST("/v1/admin/reports/:id/resolve", ctrls.AdminResolveReport)

		body := `{"action": "DISMISS"}`
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/v1/admin/reports/%s/resolve", repID.String()), strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", w.Code)
		}
	})
}

func TestControllers_AdminListAuditLogs(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("success returns 200", func(t *testing.T) {
		ctrls, mock, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		logID := uuid.New()
		adminID := uuid.New()
		now := time.Now()

		mock.ExpectQuery(`SELECT count\(\*\) FROM "admin_audit_logs"`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

		mock.ExpectQuery(`SELECT \* FROM "admin_audit_logs" ORDER BY created_at DESC LIMIT \$1`).
			WithArgs(20).
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "admin_id", "action", "entity", "created_at",
			}).AddRow(
				logID, adminID, "config.update_flags", "system_config", now,
			))

		mock.ExpectQuery(`SELECT \* FROM "admin_users" WHERE "admin_users"\."id" = \$1 AND "admin_users"\."deleted_at" IS NULL`).
			WithArgs(adminID).
			WillReturnRows(sqlmock.NewRows([]string{"id", "username", "full_name"}).AddRow(adminID, "sysadmin", "System Administrator"))

		r := gin.New()
		r.Use(func(c *gin.Context) {
			ctx := ctxmeta.WithAdminAuthUser(c.Request.Context(), ctxmeta.AdminAuthUser{
				AdminID:  adminID,
				Username: "sysadmin",
			})
			c.Request = c.Request.WithContext(ctx)
			c.Next()
		})
		r.GET("/v1/admin/audit-logs", ctrls.AdminListAuditLogs)

		req := httptest.NewRequest(http.MethodGet, "/v1/admin/audit-logs?page=1&limit=20", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("unauthorized without admin context returns 401", func(t *testing.T) {
		ctrls, _, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		r := gin.New()
		r.GET("/v1/admin/audit-logs", ctrls.AdminListAuditLogs)

		req := httptest.NewRequest(http.MethodGet, "/v1/admin/audit-logs", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", w.Code)
		}
	})
}

func TestControllers_AdminStatsAndCosts(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("AdminGetOverviewStats success returns 200", func(t *testing.T) {
		ctrls, mock, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		mock.ExpectQuery(`SELECT count\(\*\) FROM "users" WHERE "users"\."deleted_at" IS NULL`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(10))
		mock.ExpectQuery(`SELECT count\(\*\) FROM "users" WHERE status = \$1 AND "users"\."deleted_at" IS NULL`).
			WithArgs("active").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(8))
		mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings" WHERE "recordings"\."deleted_at" IS NULL`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(20))
		mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings" WHERE status = \$1 AND "recordings"\."deleted_at" IS NULL`).
			WithArgs("COMPLETED").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(18))
		mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings" WHERE status = \$1 AND "recordings"\."deleted_at" IS NULL`).
			WithArgs("FAILED").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
		mock.ExpectQuery(`SELECT COALESCE\(SUM\(file_size_bytes\), 0\) as total_storage, COALESCE\(SUM\(duration_seconds\), 0\) as total_duration FROM "recordings" WHERE "recordings"\."deleted_at" IS NULL`).
			WillReturnRows(sqlmock.NewRows([]string{"total_storage", "total_duration"}).AddRow(2000000, 7200.0))

		r := gin.New()
		r.Use(func(c *gin.Context) {
			ctx := ctxmeta.WithAdminAuthUser(c.Request.Context(), ctxmeta.AdminAuthUser{
				AdminID:  uuid.New(),
				Username: "analyst",
			})
			c.Request = c.Request.WithContext(ctx)
			c.Next()
		})
		r.GET("/v1/admin/stats/overview", ctrls.AdminGetOverviewStats)

		req := httptest.NewRequest(http.MethodGet, "/v1/admin/stats/overview", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("AdminGetCostOversight success returns 200", func(t *testing.T) {
		ctrls, mock, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		mock.ExpectQuery(`SELECT count\(\*\) FROM "users" WHERE "users"\."deleted_at" IS NULL`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(10))
		mock.ExpectQuery(`SELECT count\(\*\) FROM "users" WHERE status = \$1 AND "users"\."deleted_at" IS NULL`).
			WithArgs("active").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(8))
		mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings" WHERE "recordings"\."deleted_at" IS NULL`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(20))
		mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings" WHERE status = \$1 AND "recordings"\."deleted_at" IS NULL`).
			WithArgs("COMPLETED").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(18))
		mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings" WHERE status = \$1 AND "recordings"\."deleted_at" IS NULL`).
			WithArgs("FAILED").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
		mock.ExpectQuery(`SELECT COALESCE\(SUM\(file_size_bytes\), 0\) as total_storage, COALESCE\(SUM\(duration_seconds\), 0\) as total_duration FROM "recordings" WHERE "recordings"\."deleted_at" IS NULL`).
			WillReturnRows(sqlmock.NewRows([]string{"total_storage", "total_duration"}).AddRow(2000000, 7200.0))

		r := gin.New()
		r.Use(func(c *gin.Context) {
			ctx := ctxmeta.WithAdminAuthUser(c.Request.Context(), ctxmeta.AdminAuthUser{
				AdminID:  uuid.New(),
				Username: "analyst",
			})
			c.Request = c.Request.WithContext(ctx)
			c.Next()
		})
		r.GET("/v1/admin/stats/costs", ctrls.AdminGetCostOversight)

		req := httptest.NewRequest(http.MethodGet, "/v1/admin/stats/costs", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("unauthorized without admin context returns 401", func(t *testing.T) {
		ctrls, _, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		r := gin.New()
		r.GET("/v1/admin/stats/overview", ctrls.AdminGetOverviewStats)

		req := httptest.NewRequest(http.MethodGet, "/v1/admin/stats/overview", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", w.Code)
		}
	})
}

func TestControllers_AdminListJobs(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("successfully lists jobs with admin auth context", func(t *testing.T) {
		ctrls, mock, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		adminID := uuid.New()
		adminUser := ctxmeta.AdminAuthUser{
			AdminID:  adminID,
			Username: "admin",
		}

		mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings"`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

		mock.ExpectQuery(`SELECT \* FROM "recordings"`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "title", "status", "source_type", "selected_template", "output_language", "is_guest"}).
				AddRow(uuid.New(), "Meeting Analysis", "COMPLETED", "UPLOAD", "GENERAL", "id", false))

		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Request = c.Request.WithContext(ctxmeta.WithAdminAuthUser(c.Request.Context(), adminUser))
			c.Next()
		})
		r.GET("/v1/admin/jobs", ctrls.AdminListJobs)

		req := httptest.NewRequest(http.MethodGet, "/v1/admin/jobs?page=1&limit=10", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("unauthorized without admin context returns 401", func(t *testing.T) {
		ctrls, _, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		r := gin.New()
		r.GET("/v1/admin/jobs", ctrls.AdminListJobs)

		req := httptest.NewRequest(http.MethodGet, "/v1/admin/jobs", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", w.Code)
		}
	})
}

func TestControllers_AdminLogin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("invalid request payload returns 400", func(t *testing.T) {
		ctrls, _, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		r := gin.New()
		r.POST("/v1/admin/login", ctrls.AdminLogin)

		req := httptest.NewRequest(http.MethodPost, "/v1/admin/login", strings.NewReader("invalid-json"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("validation error on short password returns 400", func(t *testing.T) {
		ctrls, _, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		r := gin.New()
		r.POST("/v1/admin/login", ctrls.AdminLogin)

		body := dtos.AdminLoginRequest{
			Username: "admin",
			Password: "123", // too short
		}
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/v1/admin/login", strings.NewReader(string(raw)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("admin not found returns 401", func(t *testing.T) {
		ctrls, mock, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		mock.ExpectQuery(`SELECT (.+) FROM "admin_users"`).
			WithArgs("nonexistent", 1).
			WillReturnError(gorm.ErrRecordNotFound)

		r := gin.New()
		r.POST("/v1/admin/login", ctrls.AdminLogin)

		body := dtos.AdminLoginRequest{
			Username: "nonexistent",
			Password: "Password123!",
		}
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/v1/admin/login", strings.NewReader(string(raw)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", w.Code)
		}
	})

	t.Run("successful login returns 200 with admin token and info", func(t *testing.T) {
		ctrls, mock, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		adminID := uuid.New()
		roleID := uuid.New()
		// $2a$12$n33YfdVl5YvERU.q3cxT/.06OTgvKT1R2PXSPUFXvx67tSp8HXrq2 is hash for Admin123!
		hash := "$2a$12$n33YfdVl5YvERU.q3cxT/.06OTgvKT1R2PXSPUFXvx67tSp8HXrq2"

		mock.ExpectQuery(`SELECT (.+) FROM "admin_users"`).
			WithArgs("admin", 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "username", "password_hash", "full_name", "role_id", "status"}).
				AddRow(adminID, "admin", hash, "Platform Administrator", roleID, "active"))

		mock.ExpectQuery(`SELECT (.+) FROM "admin_roles"`).
			WithArgs(roleID).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name", "permissions"}).
				AddRow(roleID, "Super Admin", []byte(`["*"]`)))

		mock.ExpectBegin()
		mock.ExpectExec(`UPDATE "admin_users"`).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		mock.ExpectBegin()
		mock.ExpectQuery(`INSERT INTO "admin_audit_logs"`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(uuid.New(), time.Now()))
		mock.ExpectCommit()

		r := gin.New()
		r.POST("/v1/admin/login", ctrls.AdminLogin)

		body := dtos.AdminLoginRequest{
			Username: "admin",
			Password: "Admin123!",
		}
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/v1/admin/login", strings.NewReader(string(raw)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
		}

		var resp struct {
			Status string                  `json:"status"`
			Data   dtos.AdminLoginResponse `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if resp.Data.Token == "" {
			t.Errorf("expected non-empty token")
		}
		if resp.Data.Admin.Username != "admin" {
			t.Errorf("expected username admin, got %s", resp.Data.Admin.Username)
		}
		if resp.Data.Admin.RoleName != "Super Admin" {
			t.Errorf("expected role name Super Admin, got %s", resp.Data.Admin.RoleName)
		}
	})
}














