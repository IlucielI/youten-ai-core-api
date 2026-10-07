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

// AdminAuthValidator defines the contract for authenticating administrative requests.
type AdminAuthValidator interface {
	AuthenticateAdmin(ctx context.Context, tokenStr string) (*ctxmeta.AdminAuthUser, error)
}

// RequireAdminAuth returns a Gin middleware that requires a valid Admin JWT Bearer token.
// Returns 401 Unauthorized if the header is missing, malformed, or contains an invalid/expired token.
func RequireAdminAuth(validator AdminAuthValidator) gin.HandlerFunc {
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
				Message:   "invalid or expired admin token",
				Timestamp: time.Now(),
			})
			return
		}

		adminUser, err := validator.AuthenticateAdmin(c.Request.Context(), tokenStr)
		if err != nil || adminUser == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, dtos.BaseResponse{
				Status:    constants.ResponseStatusFail,
				Code:      constants.ResponseCodeUnauthorized,
				Message:   "invalid or expired admin token",
				Timestamp: time.Now(),
			})
			return
		}

		// Inject authenticated admin into standard context
		c.Request = c.Request.WithContext(ctxmeta.WithAdminAuthUser(c.Request.Context(), *adminUser))
		c.Next()
	}
}

// RequireAdminPermission returns a Gin middleware ensuring the authenticated admin has the specified permission.
// Returns 403 Forbidden if permission is not granted, or 401 if unauthenticated.
func RequireAdminPermission(requiredPerm string) gin.HandlerFunc {
	return func(c *gin.Context) {
		adminUser, ok := ctxmeta.GetAdminAuthUser(c.Request.Context())
		if !ok || adminUser.AdminID == uuid.Nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, dtos.BaseResponse{
				Status:    constants.ResponseStatusFail,
				Code:      constants.ResponseCodeUnauthorized,
				Message:   "unauthorized: admin authentication required",
				Timestamp: time.Now(),
			})
			return
		}

		if !ctxmeta.HasAdminPermission(adminUser, requiredPerm) {
			c.AbortWithStatusJSON(http.StatusForbidden, dtos.BaseResponse{
				Status:    constants.ResponseStatusFail,
				Code:      constants.ResponseCodeForbidden,
				Message:   "forbidden: insufficient administrative permissions",
				Timestamp: time.Now(),
			})
			return
		}

		c.Next()
	}
}
