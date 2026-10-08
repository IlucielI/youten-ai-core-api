package middlewares

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/pkg/ctxmeta"
)

type mockAuthValidator struct {
	authenticateFn func(ctx context.Context, tokenStr string) (*ctxmeta.AuthUser, error)
}

func (m *mockAuthValidator) Authenticate(ctx context.Context, tokenStr string) (*ctxmeta.AuthUser, error) {
	if m.authenticateFn != nil {
		return m.authenticateFn(ctx, tokenStr)
	}
	return nil, constants.ErrInvalidToken
}

func TestAuthMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	userID := uuid.New()
	validUser := &ctxmeta.AuthUser{
		UserID:    userID,
		Email:     "user@example.com",
		SessionID: "sess-123",
	}

	// 1. Missing Authorization header
	{
		validator := &mockAuthValidator{}
		w := httptest.NewRecorder()
		c, r := gin.CreateTestContext(w)
		r.Use(Auth(validator))
		r.GET("/protected", func(ctx *gin.Context) {
			ctx.Status(http.StatusOK)
		})
		c.Request = httptest.NewRequest(http.MethodGet, "/protected", nil)
		r.ServeHTTP(w, c.Request)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 on missing auth header, got %d", w.Code)
		}
	}

	// 2. Invalid Authorization header format
	{
		validator := &mockAuthValidator{}
		w := httptest.NewRecorder()
		c, r := gin.CreateTestContext(w)
		r.Use(Auth(validator))
		r.GET("/protected", func(ctx *gin.Context) {
			ctx.Status(http.StatusOK)
		})
		c.Request = httptest.NewRequest(http.MethodGet, "/protected", nil)
		c.Request.Header.Set("Authorization", "Basic 123456")
		r.ServeHTTP(w, c.Request)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 on invalid format, got %d", w.Code)
		}
	}

	// 3. Nil validator or empty token
	{
		w := httptest.NewRecorder()
		c, r := gin.CreateTestContext(w)
		r.Use(Auth(nil))
		r.GET("/protected", func(ctx *gin.Context) {
			ctx.Status(http.StatusOK)
		})
		c.Request = httptest.NewRequest(http.MethodGet, "/protected", nil)
		c.Request.Header.Set("Authorization", "Bearer valid-token")
		r.ServeHTTP(w, c.Request)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 on nil validator, got %d", w.Code)
		}
	}

	// 4. Validator returns error
	{
		validator := &mockAuthValidator{
			authenticateFn: func(ctx context.Context, tokenStr string) (*ctxmeta.AuthUser, error) {
				return nil, errors.New("invalid or expired token")
			},
		}
		w := httptest.NewRecorder()
		c, r := gin.CreateTestContext(w)
		r.Use(Auth(validator))
		r.GET("/protected", func(ctx *gin.Context) {
			ctx.Status(http.StatusOK)
		})
		c.Request = httptest.NewRequest(http.MethodGet, "/protected", nil)
		c.Request.Header.Set("Authorization", "Bearer bad-token")
		r.ServeHTTP(w, c.Request)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 on validation error, got %d", w.Code)
		}
	}

	// 5. Success authentication
	{
		validator := &mockAuthValidator{
			authenticateFn: func(ctx context.Context, tokenStr string) (*ctxmeta.AuthUser, error) {
				if tokenStr == "good-token" {
					return validUser, nil
				}
				return nil, constants.ErrInvalidToken
			},
		}
		var capturedUser ctxmeta.AuthUser
		w := httptest.NewRecorder()
		c, r := gin.CreateTestContext(w)
		r.Use(Auth(validator))
		r.GET("/protected", func(ctx *gin.Context) {
			u, _ := ctxmeta.GetAuthUser(ctx.Request.Context())
			capturedUser = u
			ctx.Status(http.StatusOK)
		})
		c.Request = httptest.NewRequest(http.MethodGet, "/protected", nil)
		c.Request.Header.Set("Authorization", "Bearer good-token")
		r.ServeHTTP(w, c.Request)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 on valid token, got %d", w.Code)
		}
		if capturedUser.UserID != userID || capturedUser.Email != "user@example.com" {
			t.Fatalf("unexpected captured user: %+v", capturedUser)
		}
	}

	// 6. RequireAuth explicit usage
	{
		validator := &mockAuthValidator{
			authenticateFn: func(ctx context.Context, tokenStr string) (*ctxmeta.AuthUser, error) {
				return validUser, nil
			},
		}
		w := httptest.NewRecorder()
		c, r := gin.CreateTestContext(w)
		r.Use(RequireAuth(validator))
		r.GET("/protected-req", func(ctx *gin.Context) {
			ctx.Status(http.StatusOK)
		})
		c.Request = httptest.NewRequest(http.MethodGet, "/protected-req", nil)
		c.Request.Header.Set("Authorization", "Bearer valid-req-token")
		r.ServeHTTP(w, c.Request)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 with RequireAuth on valid token, got %d", w.Code)
		}
	}
}

func TestOptionalAuthMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	userID := uuid.New()
	validUser := &ctxmeta.AuthUser{
		UserID:    userID,
		Email:     "optional@example.com",
		SessionID: "sess-optional",
		IsGuest:   false,
	}

	// 1. Missing Authorization header -> falls back to guest context (200 OK)
	{
		validator := &mockAuthValidator{}
		w := httptest.NewRecorder()
		c, r := gin.CreateTestContext(w)
		r.Use(OptionalAuth(validator))
		var isGuest bool
		var isAuth bool
		r.GET("/optional", func(ctx *gin.Context) {
			user, ok := ctxmeta.GetAuthUser(ctx.Request.Context())
			isGuest = ok && user.IsGuest
			isAuth = ctxmeta.IsAuthenticated(ctx.Request.Context())
			ctx.Status(http.StatusOK)
		})
		c.Request = httptest.NewRequest(http.MethodGet, "/optional", nil)
		r.ServeHTTP(w, c.Request)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 on missing auth header with OptionalAuth, got %d", w.Code)
		}
		if !isGuest {
			t.Fatal("expected isGuest to be true for missing header")
		}
		if isAuth {
			t.Fatal("expected isAuth to be false for missing header")
		}
	}

	// 2. Empty Authorization header -> falls back to guest context (200 OK)
	{
		validator := &mockAuthValidator{}
		w := httptest.NewRecorder()
		c, r := gin.CreateTestContext(w)
		r.Use(OptionalAuth(validator))
		var isGuest bool
		r.GET("/optional", func(ctx *gin.Context) {
			user, ok := ctxmeta.GetAuthUser(ctx.Request.Context())
			isGuest = ok && user.IsGuest
			ctx.Status(http.StatusOK)
		})
		c.Request = httptest.NewRequest(http.MethodGet, "/optional", nil)
		c.Request.Header.Set("Authorization", "   ")
		r.ServeHTTP(w, c.Request)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 on whitespace auth header with OptionalAuth, got %d", w.Code)
		}
		if !isGuest {
			t.Fatal("expected isGuest to be true for whitespace header")
		}
	}

	// 3. Valid Bearer token -> sets authenticated context (200 OK)
	{
		validator := &mockAuthValidator{
			authenticateFn: func(ctx context.Context, tokenStr string) (*ctxmeta.AuthUser, error) {
				if tokenStr == "optional-token" {
					return validUser, nil
				}
				return nil, constants.ErrInvalidToken
			},
		}
		var capturedUser ctxmeta.AuthUser
		var isAuth bool
		w := httptest.NewRecorder()
		c, r := gin.CreateTestContext(w)
		r.Use(OptionalAuth(validator))
		r.GET("/optional", func(ctx *gin.Context) {
			u, _ := ctxmeta.GetAuthUser(ctx.Request.Context())
			capturedUser = u
			isAuth = ctxmeta.IsAuthenticated(ctx.Request.Context())
			ctx.Status(http.StatusOK)
		})
		c.Request = httptest.NewRequest(http.MethodGet, "/optional", nil)
		c.Request.Header.Set("Authorization", "Bearer optional-token")
		r.ServeHTTP(w, c.Request)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 on valid token, got %d", w.Code)
		}
		if !isAuth || capturedUser.IsGuest || capturedUser.UserID != userID {
			t.Fatalf("unexpected captured user: %+v", capturedUser)
		}
	}

	// 4. Invalid header format when provided (e.g. Basic) -> 401 Unauthorized
	{
		validator := &mockAuthValidator{}
		w := httptest.NewRecorder()
		c, r := gin.CreateTestContext(w)
		r.Use(OptionalAuth(validator))
		r.GET("/optional", func(ctx *gin.Context) {
			ctx.Status(http.StatusOK)
		})
		c.Request = httptest.NewRequest(http.MethodGet, "/optional", nil)
		c.Request.Header.Set("Authorization", "Basic xyz123")
		r.ServeHTTP(w, c.Request)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 on malformed Authorization header with OptionalAuth, got %d", w.Code)
		}
	}

	// 5. Empty token after Bearer -> 401 Unauthorized
	{
		validator := &mockAuthValidator{}
		w := httptest.NewRecorder()
		c, r := gin.CreateTestContext(w)
		r.Use(OptionalAuth(validator))
		r.GET("/optional", func(ctx *gin.Context) {
			ctx.Status(http.StatusOK)
		})
		c.Request = httptest.NewRequest(http.MethodGet, "/optional", nil)
		c.Request.Header.Set("Authorization", "Bearer   ")
		r.ServeHTTP(w, c.Request)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 on empty Bearer token with OptionalAuth, got %d", w.Code)
		}
	}

	// 6. Expired or invalid token when provided -> 401 Unauthorized
	{
		validator := &mockAuthValidator{
			authenticateFn: func(ctx context.Context, tokenStr string) (*ctxmeta.AuthUser, error) {
				return nil, constants.ErrInvalidToken
			},
		}
		w := httptest.NewRecorder()
		c, r := gin.CreateTestContext(w)
		r.Use(OptionalAuth(validator))
		r.GET("/optional", func(ctx *gin.Context) {
			ctx.Status(http.StatusOK)
		})
		c.Request = httptest.NewRequest(http.MethodGet, "/optional", nil)
		c.Request.Header.Set("Authorization", "Bearer expired-token")
		r.ServeHTTP(w, c.Request)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 on invalid/expired token with OptionalAuth, got %d", w.Code)
		}
	}

	// 7. Nil validator with provided token -> 401 Unauthorized
	{
		w := httptest.NewRecorder()
		c, r := gin.CreateTestContext(w)
		r.Use(OptionalAuth(nil))
		r.GET("/optional", func(ctx *gin.Context) {
			ctx.Status(http.StatusOK)
		})
		c.Request = httptest.NewRequest(http.MethodGet, "/optional", nil)
		c.Request.Header.Set("Authorization", "Bearer some-token")
		r.ServeHTTP(w, c.Request)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 on nil validator with OptionalAuth, got %d", w.Code)
		}
	}
}

func TestRequireUserPermission(t *testing.T) {
	gin.SetMode(gin.TestMode)

	userID := uuid.New()

	// 1. Unauthenticated / No user in context -> 401
	{
		w := httptest.NewRecorder()
		_, r := gin.CreateTestContext(w)
		r.Use(RequireUserPermission("export:pdf"))
		r.GET("/export", func(c *gin.Context) { c.Status(http.StatusOK) })
		req := httptest.NewRequest(http.MethodGet, "/export", nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 on unauthenticated, got %d", w.Code)
		}
	}

	// 2. Guest user in context -> 401
	{
		w := httptest.NewRecorder()
		_, r := gin.CreateTestContext(w)
		r.Use(func(c *gin.Context) {
			c.Request = c.Request.WithContext(ctxmeta.WithGuestUser(c.Request.Context()))
			c.Next()
		})
		r.Use(RequireUserPermission("export:pdf"))
		r.GET("/export", func(c *gin.Context) { c.Status(http.StatusOK) })
		req := httptest.NewRequest(http.MethodGet, "/export", nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 for guest user, got %d", w.Code)
		}
	}

	// 3. Authenticated user without required permission -> 403
	{
		w := httptest.NewRecorder()
		_, r := gin.CreateTestContext(w)
		r.Use(func(c *gin.Context) {
			user := ctxmeta.AuthUser{
				UserID:      userID,
				Email:       "free@example.com",
				RoleCode:    "FREE",
				Permissions: []string{"recordings:create", "recordings:read"},
			}
			c.Request = c.Request.WithContext(ctxmeta.WithAuthUser(c.Request.Context(), user))
			c.Next()
		})
		r.Use(RequireUserPermission("export:pdf"))
		r.GET("/export", func(c *gin.Context) { c.Status(http.StatusOK) })
		req := httptest.NewRequest(http.MethodGet, "/export", nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403 on missing permission, got %d", w.Code)
		}
	}

	// 4. Authenticated user with exact matching permission -> 200
	{
		w := httptest.NewRecorder()
		_, r := gin.CreateTestContext(w)
		r.Use(func(c *gin.Context) {
			user := ctxmeta.AuthUser{
				UserID:      userID,
				Email:       "pro@example.com",
				RoleCode:    "PRO",
				Permissions: []string{"recordings:create", "export:pdf"},
			}
			c.Request = c.Request.WithContext(ctxmeta.WithAuthUser(c.Request.Context(), user))
			c.Next()
		})
		r.Use(RequireUserPermission("export:pdf"))
		r.GET("/export", func(c *gin.Context) { c.Status(http.StatusOK) })
		req := httptest.NewRequest(http.MethodGet, "/export", nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 on exact permission match, got %d", w.Code)
		}
	}

	// 5. Authenticated user with wildcard category permission -> 200
	{
		w := httptest.NewRecorder()
		_, r := gin.CreateTestContext(w)
		r.Use(func(c *gin.Context) {
			user := ctxmeta.AuthUser{
				UserID:      userID,
				Email:       "pro@example.com",
				RoleCode:    "PRO",
				Permissions: []string{"recordings:*"},
			}
			c.Request = c.Request.WithContext(ctxmeta.WithAuthUser(c.Request.Context(), user))
			c.Next()
		})
		r.Use(RequireUserPermission("recordings:share"))
		r.GET("/recordings/share", func(c *gin.Context) { c.Status(http.StatusOK) })
		req := httptest.NewRequest(http.MethodGet, "/recordings/share", nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 on category wildcard match, got %d", w.Code)
		}
	}
}

