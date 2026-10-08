package middlewares

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/pkg/ctxmeta"
)

// AuthValidator defines the required interface for authenticating incoming requests.
type AuthValidator interface {
	Authenticate(ctx context.Context, tokenStr string) (*ctxmeta.AuthUser, error)
}

// RequireAuth returns a Gin middleware that requires a valid JWT Bearer token.
// Returns 401 Unauthorized if the header is missing, malformed, or contains an invalid/expired token.
func RequireAuth(validator AuthValidator) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, dtos.BaseResponse{
				Status:    constants.ResponseStatusFail,
				Code:      constants.ResponseCodeUnauthorized,
				Message:   "missing authorization header",
				Timestamp: time.Now(),
			})
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, dtos.BaseResponse{
				Status:    constants.ResponseStatusFail,
				Code:      constants.ResponseCodeUnauthorized,
				Message:   "invalid authorization header format, expected 'Bearer <token>'",
				Timestamp: time.Now(),
			})
			return
		}

		tokenStr := strings.TrimSpace(parts[1])
		if tokenStr == "" || validator == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, dtos.BaseResponse{
				Status:    constants.ResponseStatusFail,
				Code:      constants.ResponseCodeUnauthorized,
				Message:   "invalid or expired token",
				Timestamp: time.Now(),
			})
			return
		}

		authUser, err := validator.Authenticate(c.Request.Context(), tokenStr)
		if err != nil || authUser == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, dtos.BaseResponse{
				Status:    constants.ResponseStatusFail,
				Code:      constants.ResponseCodeUnauthorized,
				Message:   "invalid or expired token",
				Timestamp: time.Now(),
			})
			return
		}

		// Inject authenticated user into standard context
		c.Request = c.Request.WithContext(ctxmeta.WithAuthUser(c.Request.Context(), *authUser))

		c.Next()
	}
}

// Auth is an alias for RequireAuth to maintain backward compatibility.
func Auth(validator AuthValidator) gin.HandlerFunc {
	return RequireAuth(validator)
}

// OptionalAuth returns a Gin middleware that extracts user context if a valid Bearer token
// is present, but falls back gracefully to an anonymous guest context when the header is absent.
// If an Authorization header is provided but invalid/expired, it returns 401 Unauthorized.
func OptionalAuth(validator AuthValidator) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
		if authHeader == "" {
			// Fallback to anonymous guest context
			c.Request = c.Request.WithContext(ctxmeta.WithGuestUser(c.Request.Context()))
			c.Next()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, dtos.BaseResponse{
				Status:    constants.ResponseStatusFail,
				Code:      constants.ResponseCodeUnauthorized,
				Message:   "invalid authorization header format, expected 'Bearer <token>'",
				Timestamp: time.Now(),
			})
			return
		}

		tokenStr := strings.TrimSpace(parts[1])
		if tokenStr == "" || validator == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, dtos.BaseResponse{
				Status:    constants.ResponseStatusFail,
				Code:      constants.ResponseCodeUnauthorized,
				Message:   "invalid or expired token",
				Timestamp: time.Now(),
			})
			return
		}

		authUser, err := validator.Authenticate(c.Request.Context(), tokenStr)
		if err != nil || authUser == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, dtos.BaseResponse{
				Status:    constants.ResponseStatusFail,
				Code:      constants.ResponseCodeUnauthorized,
				Message:   "invalid or expired token",
				Timestamp: time.Now(),
			})
			return
		}

		// Inject authenticated user into standard context
		c.Request = c.Request.WithContext(ctxmeta.WithAuthUser(c.Request.Context(), *authUser))

		c.Next()
	}
}

// RequireUserPermission returns a Gin middleware ensuring the authenticated user has the specified permission.
// Returns 403 Forbidden if permission is not granted, or 401 if unauthenticated.
func RequireUserPermission(requiredPerm string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authUser, ok := ctxmeta.GetAuthUser(c.Request.Context())
		if !ok || authUser.IsGuest || authUser.UserID == uuid.Nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, dtos.BaseResponse{
				Status:    constants.ResponseStatusFail,
				Code:      constants.ResponseCodeUnauthorized,
				Message:   "unauthorized: user authentication required",
				Timestamp: time.Now(),
			})
			return
		}

		if !ctxmeta.HasUserPermission(authUser, requiredPerm) {
			c.AbortWithStatusJSON(http.StatusForbidden, dtos.BaseResponse{
				Status:    constants.ResponseStatusFail,
				Code:      constants.ResponseCodeForbidden,
				Message:   "forbidden: insufficient permissions",
				Timestamp: time.Now(),
			})
			return
		}

		c.Next()
	}
}

