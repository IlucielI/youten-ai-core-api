package middlewares

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestMaintenanceMode(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("passes when maintenance is false", func(t *testing.T) {
		r := gin.New()
		r.Use(MaintenanceMode(func(ctx context.Context) bool {
			return false
		}))
		r.GET("/v1/recordings", func(c *gin.Context) {
			c.String(http.StatusOK, "ok")
		})

		req := httptest.NewRequest(http.MethodGet, "/v1/recordings", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}
	})

	t.Run("returns 503 for public endpoint when maintenance is true", func(t *testing.T) {
		r := gin.New()
		r.Use(MaintenanceMode(func(ctx context.Context) bool {
			return true
		}))
		r.GET("/v1/recordings", func(c *gin.Context) {
			c.String(http.StatusOK, "ok")
		})

		req := httptest.NewRequest(http.MethodGet, "/v1/recordings", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected status 503, got %d", w.Code)
		}
	})

	t.Run("whitelists admin endpoints during maintenance", func(t *testing.T) {
		r := gin.New()
		r.Use(MaintenanceMode(func(ctx context.Context) bool {
			return true
		}))
		r.GET("/v1/admin/config", func(c *gin.Context) {
			c.String(http.StatusOK, "admin_ok")
		})

		req := httptest.NewRequest(http.MethodGet, "/v1/admin/config", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200 for admin route, got %d", w.Code)
		}
	})

	t.Run("whitelists health endpoint during maintenance", func(t *testing.T) {
		r := gin.New()
		r.Use(MaintenanceMode(func(ctx context.Context) bool {
			return true
		}))
		r.GET("/health", func(c *gin.Context) {
			c.String(http.StatusOK, "health_ok")
		})

		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200 for health route, got %d", w.Code)
		}
	})
}
