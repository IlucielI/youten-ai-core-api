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
	"gorm.io/gorm"

	"code-base-golang/internal/dtos"
)

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

func TestControllers_AdminForgotPassword(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("invalid json returns 400", func(t *testing.T) {
		ctrls, _, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		r := gin.New()
		r.POST("/v1/admin/forgot-password", ctrls.AdminForgotPassword)

		req := httptest.NewRequest(http.MethodPost, "/v1/admin/forgot-password", strings.NewReader("bad-json"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("validation error on invalid email returns 400", func(t *testing.T) {
		ctrls, _, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		r := gin.New()
		r.POST("/v1/admin/forgot-password", ctrls.AdminForgotPassword)

		req := httptest.NewRequest(http.MethodPost, "/v1/admin/forgot-password", strings.NewReader(`{"email": "not-valid"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("non-existent email returns 200 without error", func(t *testing.T) {
		ctrls, mock, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		mock.ExpectQuery(`SELECT (.+) FROM "admin_users"`).
			WithArgs("unknown@youten.ai", 1).
			WillReturnError(gorm.ErrRecordNotFound)

		r := gin.New()
		r.POST("/v1/admin/forgot-password", ctrls.AdminForgotPassword)

		req := httptest.NewRequest(http.MethodPost, "/v1/admin/forgot-password", strings.NewReader(`{"email": "unknown@youten.ai"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("existing admin generates reset token and returns 200", func(t *testing.T) {
		ctrls, mock, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		adminID := uuid.New()
		roleID := uuid.New()

		mock.ExpectQuery(`SELECT (.+) FROM "admin_users"`).
			WithArgs("admin@youten.ai", 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "username", "email", "full_name", "role_id", "status"}).
				AddRow(adminID, "admin", "admin@youten.ai", "Platform Admin", roleID, "active"))

		mock.ExpectQuery(`SELECT (.+) FROM "admin_roles"`).
			WithArgs(roleID).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name", "permissions"}).
				AddRow(roleID, "Super Admin", []byte(`["*"]`)))

		mock.ExpectBegin()
		mock.ExpectExec(`UPDATE "admin_auth_tokens"`).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		mock.ExpectBegin()
		mock.ExpectQuery(`INSERT INTO "admin_auth_tokens"`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).
				AddRow(uuid.New(), time.Now(), time.Now()))
		mock.ExpectCommit()

		r := gin.New()
		r.POST("/v1/admin/forgot-password", ctrls.AdminForgotPassword)

		req := httptest.NewRequest(http.MethodPost, "/v1/admin/forgot-password", strings.NewReader(`{"email": "admin@youten.ai"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
		}
	})
}

func TestControllers_AdminResetPassword(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("invalid json returns 400", func(t *testing.T) {
		ctrls, _, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		r := gin.New()
		r.POST("/v1/admin/reset-password", ctrls.AdminResetPassword)

		req := httptest.NewRequest(http.MethodPost, "/v1/admin/reset-password", strings.NewReader("bad-json"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("validation error on mismatched confirm password returns 400", func(t *testing.T) {
		ctrls, _, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		r := gin.New()
		r.POST("/v1/admin/reset-password", ctrls.AdminResetPassword)

		body := `{"token": "tok123", "new_password": "NewPass123!", "confirm_password": "OtherPass456!"}`
		req := httptest.NewRequest(http.MethodPost, "/v1/admin/reset-password", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("invalid or expired token returns error", func(t *testing.T) {
		ctrls, mock, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		mock.ExpectQuery(`SELECT (.+) FROM "admin_auth_tokens"`).
			WillReturnError(gorm.ErrRecordNotFound)

		r := gin.New()
		r.POST("/v1/admin/reset-password", ctrls.AdminResetPassword)

		body := `{"token": "invalid-token", "new_password": "NewAdmin123!"}`
		req := httptest.NewRequest(http.MethodPost, "/v1/admin/reset-password", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized && w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 or 401, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("successful reset updates password and logs audit", func(t *testing.T) {
		ctrls, mock, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		adminID := uuid.New()
		tokenID := uuid.New()
		roleID := uuid.New()

		mock.ExpectQuery(`SELECT (.+) FROM "admin_auth_tokens"`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "admin_id", "type", "expires_at", "revoked_at"}).
				AddRow(tokenID, adminID, "reset_password", time.Now().Add(10*time.Minute), nil))

		mock.ExpectQuery(`SELECT (.+) FROM "admin_users"`).
			WithArgs(adminID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "username", "email", "full_name", "role_id", "status"}).
				AddRow(adminID, "admin", "admin@youten.ai", "Platform Admin", roleID, "active"))

		mock.ExpectQuery(`SELECT (.+) FROM "admin_roles"`).
			WithArgs(roleID).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name", "permissions"}).
				AddRow(roleID, "Super Admin", []byte(`["*"]`)))

		mock.ExpectBegin()
		mock.ExpectExec(`UPDATE "admin_users"`).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		mock.ExpectBegin()
		mock.ExpectExec(`UPDATE "admin_auth_tokens"`).
			WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), tokenID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		mock.ExpectBegin()
		mock.ExpectQuery(`INSERT INTO "admin_audit_logs"`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(uuid.New(), time.Now()))
		mock.ExpectCommit()

		r := gin.New()
		r.POST("/v1/admin/reset-password", ctrls.AdminResetPassword)

		body := `{"token": "valid-token", "new_password": "NewAdminPass123!", "confirm_password": "NewAdminPass123!"}`
		req := httptest.NewRequest(http.MethodPost, "/v1/admin/reset-password", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
		}
	})
}

