package controllers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/pkg/ctxmeta"
)

func TestControllers_AdminListRoles(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("success returns 200 with role list and pagination", func(t *testing.T) {
		ctrls, mock, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		roleID := uuid.New()
		now := time.Now()

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "admin_roles"`)).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "admin_roles" ORDER BY is_system DESC, name ASC LIMIT $1`)).
			WithArgs(10).
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

	t.Run("success returns 200 with query parameters", func(t *testing.T) {
		ctrls, mock, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		roleID := uuid.New()
		now := time.Now()

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "admin_roles" WHERE name ILIKE $1 OR description ILIKE $2`)).
			WithArgs("%support%", "%support%").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "admin_roles" WHERE name ILIKE $1 OR description ILIKE $2 ORDER BY is_system DESC, name ASC LIMIT $3 OFFSET $4`)).
			WithArgs("%support%", "%support%", 5, 5).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name", "description", "permissions", "is_system", "created_at", "updated_at"}).
				AddRow(roleID, "Support", "Support Staff", []byte(`["users:read"]`), false, now, now))

		r := gin.New()
		r.GET("/v1/admin/roles", ctrls.AdminListRoles)

		req := httptest.NewRequest(http.MethodGet, "/v1/admin/roles?search=support&page=2&limit=5", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("service failure returns 500", func(t *testing.T) {
		ctrls, mock, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "admin_roles"`)).
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

func TestControllers_AdminUserRoles(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("AdminListUserRoles returns 200 with pagination", func(t *testing.T) {
		ctrls, mock, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		roleID := uuid.New()
		now := time.Now()
		rows := sqlmock.NewRows([]string{"id", "name", "code", "description", "permissions", "daily_quota", "is_default", "created_at", "updated_at"}).
			AddRow(roleID, "Free Member", "FREE", "Free tier", []byte(`["recordings:create"]`), 5, true, now, now)

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "user_roles"`)).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "user_roles" ORDER BY is_default DESC, daily_quota ASC, created_at ASC LIMIT $1`)).
			WithArgs(10).
			WillReturnRows(rows)

		r := gin.New()
		r.GET("/v1/admin/user-roles", ctrls.AdminListUserRoles)

		req := httptest.NewRequest(http.MethodGet, "/v1/admin/user-roles", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled expectations: %v", err)
		}
	})

	t.Run("AdminListUserRoles returns 200 with search query parameters", func(t *testing.T) {
		ctrls, mock, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		roleID := uuid.New()
		now := time.Now()
		rows := sqlmock.NewRows([]string{"id", "name", "code", "description", "permissions", "daily_quota", "is_default", "created_at", "updated_at"}).
			AddRow(roleID, "Free Member", "FREE", "Free tier", []byte(`["recordings:create"]`), 5, true, now, now)

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "user_roles" WHERE name ILIKE $1 OR code ILIKE $2 OR description ILIKE $3`)).
			WithArgs("%free%", "%free%", "%free%").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "user_roles" WHERE name ILIKE $1 OR code ILIKE $2 OR description ILIKE $3 ORDER BY is_default DESC, daily_quota ASC, created_at ASC LIMIT $4 OFFSET $5`)).
			WithArgs("%free%", "%free%", "%free%", 5, 5).
			WillReturnRows(rows)

		r := gin.New()
		r.GET("/v1/admin/user-roles", ctrls.AdminListUserRoles)

		req := httptest.NewRequest(http.MethodGet, "/v1/admin/user-roles?search=free&page=2&limit=5", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled expectations: %v", err)
		}
	})

	t.Run("AdminCreateUserRole returns 201", func(t *testing.T) {
		ctrls, mock, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		adminID := uuid.New()
		roleID := uuid.New()

		mock.ExpectQuery(`SELECT \* FROM "user_roles" WHERE LOWER\(name\) = LOWER\(\$1\) ORDER BY "user_roles"\."id" LIMIT \$2`).
			WithArgs("Gold Member", 1).
			WillReturnError(gorm.ErrRecordNotFound)

		mock.ExpectQuery(`SELECT \* FROM "user_roles" WHERE UPPER\(code\) = UPPER\(\$1\) ORDER BY "user_roles"\."id" LIMIT \$2`).
			WithArgs("GOLD", 1).
			WillReturnError(gorm.ErrRecordNotFound)

		mock.ExpectBegin()
		mock.ExpectQuery(`INSERT INTO "user_roles"`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(roleID, time.Now(), time.Now()))
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
		r.POST("/v1/admin/user-roles", ctrls.AdminCreateUserRole)

		body := `{"name": "Gold Member", "code": "GOLD", "daily_quota": 50, "permissions": ["recordings:*"]}`
		req := httptest.NewRequest(http.MethodPost, "/v1/admin/user-roles", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d. Body: %s", w.Code, w.Body.String())
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled expectations: %v", err)
		}
	})

	t.Run("AdminUpdateUserRole returns 200", func(t *testing.T) {
		ctrls, mock, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		adminID := uuid.New()
		roleID := uuid.New()

		mock.ExpectQuery(`SELECT \* FROM "user_roles" WHERE id = \$1 ORDER BY "user_roles"\."id" LIMIT \$2`).
			WithArgs(roleID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name", "code", "daily_quota", "permissions"}).
				AddRow(roleID, "Gold Member", "GOLD", 50, []byte(`["recordings:*"]`)))

		mock.ExpectBegin()
		mock.ExpectExec(`UPDATE "user_roles" SET`).
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
		r.PUT("/v1/admin/user-roles/:id", ctrls.AdminUpdateUserRole)

		body := `{"daily_quota": 60}`
		req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/v1/admin/user-roles/%s", roleID), strings.NewReader(body))
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
