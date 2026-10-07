package middlewares

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/pkg/ctxmeta"
)

type mockAdminAuthValidator struct {
	authenticateAdminFn func(ctx context.Context, tokenStr string) (*ctxmeta.AdminAuthUser, error)
}

func (m *mockAdminAuthValidator) AuthenticateAdmin(ctx context.Context, tokenStr string) (*ctxmeta.AdminAuthUser, error) {
	if m.authenticateAdminFn != nil {
		return m.authenticateAdminFn(ctx, tokenStr)
	}
	return nil, errors.New("not implemented")
}

func setupAdminTestRouter(validator AdminAuthValidator, requiredPerm string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	handlers := []gin.HandlerFunc{RequireAdminAuth(validator)}
	if requiredPerm != "" {
		handlers = append(handlers, RequireAdminPermission(requiredPerm))
	}
	handlers = append(handlers, func(c *gin.Context) {
		admin, _ := ctxmeta.GetAdminAuthUser(c.Request.Context())
		c.JSON(http.StatusOK, gin.H{
			"message":  "admin action allowed",
			"username": admin.Username,
			"admin_id": admin.AdminID.String(),
		})
	})

	r.GET("/admin/test", handlers...)
	return r
}

func TestRequireAdminAuth(t *testing.T) {
	t.Run("missing authorization header returns 401", func(t *testing.T) {
		validator := &mockAdminAuthValidator{}
		r := setupAdminTestRouter(validator, "")

		req := httptest.NewRequest(http.MethodGet, "/admin/test", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", w.Code)
		}

		var resp dtos.BaseResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp.Code != constants.ResponseCodeUnauthorized {
			t.Errorf("expected code %s, got %s", constants.ResponseCodeUnauthorized, resp.Code)
		}
	})

	t.Run("malformed authorization header returns 401", func(t *testing.T) {
		validator := &mockAdminAuthValidator{}
		r := setupAdminTestRouter(validator, "")

		req := httptest.NewRequest(http.MethodGet, "/admin/test", nil)
		req.Header.Set("Authorization", "Basic some-token")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", w.Code)
		}
	})

	t.Run("empty bearer token returns 401", func(t *testing.T) {
		validator := &mockAdminAuthValidator{}
		r := setupAdminTestRouter(validator, "")

		req := httptest.NewRequest(http.MethodGet, "/admin/test", nil)
		req.Header.Set("Authorization", "Bearer    ")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", w.Code)
		}
	})

	t.Run("nil validator returns 401", func(t *testing.T) {
		r := setupAdminTestRouter(nil, "")

		req := httptest.NewRequest(http.MethodGet, "/admin/test", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", w.Code)
		}
	})

	t.Run("validator error returns 401", func(t *testing.T) {
		validator := &mockAdminAuthValidator{
			authenticateAdminFn: func(ctx context.Context, tokenStr string) (*ctxmeta.AdminAuthUser, error) {
				return nil, errors.New("token invalid")
			},
		}
		r := setupAdminTestRouter(validator, "")

		req := httptest.NewRequest(http.MethodGet, "/admin/test", nil)
		req.Header.Set("Authorization", "Bearer bad-token")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", w.Code)
		}
	})

	t.Run("valid admin token sets context and proceeds", func(t *testing.T) {
		expectedID := uuid.New()
		validator := &mockAdminAuthValidator{
			authenticateAdminFn: func(ctx context.Context, tokenStr string) (*ctxmeta.AdminAuthUser, error) {
				return &ctxmeta.AdminAuthUser{
					AdminID:     expectedID,
					Username:    "admin_alice",
					FullName:    "Alice Admin",
					RoleName:    "Operator",
					Permissions: []string{"users:read"},
				}, nil
			},
		}
		r := setupAdminTestRouter(validator, "")

		req := httptest.NewRequest(http.MethodGet, "/admin/test", nil)
		req.Header.Set("Authorization", "Bearer good-admin-token")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}

		var body map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if body["username"] != "admin_alice" {
			t.Errorf("expected username admin_alice, got %v", body["username"])
		}
		if body["admin_id"] != expectedID.String() {
			t.Errorf("expected admin_id %s, got %v", expectedID.String(), body["admin_id"])
		}
	})
}

func TestRequireAdminPermission(t *testing.T) {
	t.Run("missing admin in context returns 401", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		r := gin.New()
		r.GET("/admin/perm-check", RequireAdminPermission("users:read"), func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		req := httptest.NewRequest(http.MethodGet, "/admin/perm-check", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", w.Code)
		}
	})

	t.Run("insufficient permissions returns 403", func(t *testing.T) {
		validator := &mockAdminAuthValidator{
			authenticateAdminFn: func(ctx context.Context, tokenStr string) (*ctxmeta.AdminAuthUser, error) {
				return &ctxmeta.AdminAuthUser{
					AdminID:     uuid.New(),
					Username:    "viewer",
					Permissions: []string{"templates:read"},
				}, nil
			},
		}
		r := setupAdminTestRouter(validator, "users:write")

		req := httptest.NewRequest(http.MethodGet, "/admin/test", nil)
		req.Header.Set("Authorization", "Bearer token")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Fatalf("expected status 403, got %d", w.Code)
		}

		var resp dtos.BaseResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp.Code != constants.ResponseCodeForbidden {
			t.Errorf("expected code %s, got %s", constants.ResponseCodeForbidden, resp.Code)
		}
	})

	t.Run("exact matching permission succeeds", func(t *testing.T) {
		validator := &mockAdminAuthValidator{
			authenticateAdminFn: func(ctx context.Context, tokenStr string) (*ctxmeta.AdminAuthUser, error) {
				return &ctxmeta.AdminAuthUser{
					AdminID:     uuid.New(),
					Username:    "editor",
					Permissions: []string{"users:read", "users:write"},
				}, nil
			},
		}
		r := setupAdminTestRouter(validator, "users:write")

		req := httptest.NewRequest(http.MethodGet, "/admin/test", nil)
		req.Header.Set("Authorization", "Bearer token")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}
	})

	t.Run("wildcard permission succeeds", func(t *testing.T) {
		validator := &mockAdminAuthValidator{
			authenticateAdminFn: func(ctx context.Context, tokenStr string) (*ctxmeta.AdminAuthUser, error) {
				return &ctxmeta.AdminAuthUser{
					AdminID:     uuid.New(),
					Username:    "superadmin",
					Permissions: []string{"*"},
				}, nil
			},
		}
		r := setupAdminTestRouter(validator, "ops:write")

		req := httptest.NewRequest(http.MethodGet, "/admin/test", nil)
		req.Header.Set("Authorization", "Bearer token")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}
	})

	t.Run("domain prefix wildcard permission succeeds", func(t *testing.T) {
		validator := &mockAdminAuthValidator{
			authenticateAdminFn: func(ctx context.Context, tokenStr string) (*ctxmeta.AdminAuthUser, error) {
				return &ctxmeta.AdminAuthUser{
					AdminID:     uuid.New(),
					Username:    "user_manager",
					Permissions: []string{"users:*"},
				}, nil
			},
		}
		r := setupAdminTestRouter(validator, "users:delete")

		req := httptest.NewRequest(http.MethodGet, "/admin/test", nil)
		req.Header.Set("Authorization", "Bearer token")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}
	})
}
