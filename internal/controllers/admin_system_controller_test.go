package controllers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"code-base-golang/internal/pkg/ctxmeta"
)

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
