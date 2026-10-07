package controllers

import (
	"encoding/json"
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

	"code-base-golang/internal/config"
	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
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
