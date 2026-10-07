package middlewares

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
)

// MaintenanceChecker is a function returning true when maintenance mode is active.
type MaintenanceChecker func(ctx context.Context) bool

// MaintenanceMode blocks non-admin public requests with 503 Service Unavailable when enabled.
func MaintenanceMode(isMaintenance MaintenanceChecker) gin.HandlerFunc {
	return func(c *gin.Context) {
		if isMaintenance == nil {
			c.Next()
			return
		}

		path := c.Request.URL.Path
		// Whitelist admin routes, healthcheck, docs, and OpenAPI specification
		if strings.HasPrefix(path, "/v1/admin") ||
			strings.HasPrefix(path, "/health") ||
			strings.HasPrefix(path, "/docs") ||
			strings.HasPrefix(path, "/openapi.yaml") {
			c.Next()
			return
		}

		if isMaintenance(c.Request.Context()) {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, dtos.BaseResponse{
				Status:    constants.ResponseStatusError,
				Code:      "SERVICE_UNAVAILABLE",
				Message:   "system is currently undergoing scheduled maintenance, please try again later",
				Timestamp: time.Now(),
			})
			return
		}

		c.Next()
	}
}
