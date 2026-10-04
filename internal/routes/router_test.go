package routes_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"code-base-golang/internal/config"
	"code-base-golang/internal/constants"
	"code-base-golang/internal/controllers"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/routes"
)

func TestRouter_HealthCheck(t *testing.T) {
	cfg := config.Load()
	ctrls := controllers.New(cfg, nil)
	router := routes.NewRouter(cfg, ctrls)

	w := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/v1/health", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp dtos.APIResponse[dtos.HealthData]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Status != constants.ResponseStatusSuccess {
		t.Errorf("expected status 'success', got %v", resp.Status)
	}
	if resp.Code != constants.ResponseCodeSuccess {
		t.Errorf("expected code %q, got %q", constants.ResponseCodeSuccess, resp.Code)
	}
	if resp.Message != constants.ResponseMessageSuccess {
		t.Errorf("expected message %q, got %q", constants.ResponseMessageSuccess, resp.Message)
	}
	if resp.Data.Version != cfg.Version {
		t.Errorf("expected version %q, got %q", cfg.Version, resp.Data.Version)
	}
	if resp.Data.GitHash != cfg.GitHash {
		t.Errorf("expected git_hash %q, got %q", cfg.GitHash, resp.Data.GitHash)
	}
	if resp.Data.Uptime == "" {
		t.Error("expected non-empty uptime in response")
	}
	if resp.Data.Services.Database != constants.IntegrationStatusDisconnected {
		t.Errorf("expected database status %q when db is nil, got %q", constants.IntegrationStatusDisconnected, resp.Data.Services.Database)
	}
	if resp.Data.Services.Redis != constants.IntegrationStatusDisconnected {
		t.Errorf("expected redis status %q when rdb is nil, got %q", constants.IntegrationStatusDisconnected, resp.Data.Services.Redis)
	}
	if resp.Data.Services.S3 != constants.IntegrationStatusDisconnected {
		t.Errorf("expected s3 status %q when storage is nil, got %q", constants.IntegrationStatusDisconnected, resp.Data.Services.S3)
	}
}

func TestRouter_PanicRecovery(t *testing.T) {
	cfg := config.Load()
	ctrls := controllers.New(cfg, nil)
	router := routes.NewRouter(cfg, ctrls)

	// Register an endpoint that panics
	router.GET("/v1/panic-test", func(c *gin.Context) {
		panic("database connection string contains password123")
	})

	w := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/v1/panic-test", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", w.Code)
	}

	reqID := w.Header().Get("X-Request-ID")
	if reqID == "" {
		t.Error("expected X-Request-ID header in response")
	}

	var resp dtos.BaseResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal JSON response: %v, body: %s", err, w.Body.String())
	}

	if resp.Status != constants.ResponseStatusError {
		t.Errorf("expected status 'error', got %q", resp.Status)
	}
	if resp.Code != constants.ResponseCodeInternalError {
		t.Errorf("expected code %q, got %q", constants.ResponseCodeInternalError, resp.Code)
	}
	if resp.Message != "An internal server error occurred" {
		t.Errorf("expected message 'An internal server error occurred', got %q", resp.Message)
	}
	if resp.Timestamp.IsZero() {
		t.Errorf("expected non-zero timestamp")
	}

	bodyStr := w.Body.String()
	if strings.Contains(bodyStr, "password123") {
		t.Errorf("security violation: panic message leaked to response body: %s", bodyStr)
	}
}

func TestRouter_DocsEndpoints(t *testing.T) {
	cfg := config.Load()
	ctrls := controllers.New(cfg, nil)
	router := routes.NewRouter(cfg, ctrls)

	tests := []struct {
		path        string
		contentType string
		mustContain string
	}{
		{"/openapi.yaml", "application/x-yaml", "openapi: 3.0.3"},
		{"/docs", "text/html", "@scalar/api-reference"},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, tc.path, nil)
			if err != nil {
				t.Fatalf("failed to create request for %s: %v", tc.path, err)
			}

			router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("expected status 200 for %s, got %d", tc.path, w.Code)
			}

			ct := w.Header().Get("Content-Type")
			if !strings.Contains(ct, tc.contentType) {
				t.Errorf("expected Content-Type %s, got %s", tc.contentType, ct)
			}

			if !strings.Contains(w.Body.String(), tc.mustContain) {
				t.Errorf("expected body to contain %q", tc.mustContain)
			}
		})
	}
}

func TestRouter_AuthEndpoints(t *testing.T) {
	cfg := config.Load()
	ctrls := controllers.New(cfg, nil)
	router := routes.NewRouter(cfg, ctrls)

	// Verify POST /v1/auth/login route is registered and resolves to handler (returns 400 for empty body, not 404)
	w := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodPost, "/v1/auth/login", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(w, req)
	if w.Code == http.StatusNotFound {
		t.Fatalf("expected /v1/auth/login to be registered, but got 404 Not Found")
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for empty json payload, got %d", w.Code)
	}

	// Verify POST /v1/auth/refresh route is registered and resolves to handler
	wRefresh := httptest.NewRecorder()
	reqRefresh, err := http.NewRequest(http.MethodPost, "/v1/auth/refresh", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("failed to create refresh request: %v", err)
	}
	reqRefresh.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(wRefresh, reqRefresh)
	if wRefresh.Code == http.StatusNotFound {
		t.Fatalf("expected /v1/auth/refresh to be registered, but got 404 Not Found")
	}
	if wRefresh.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for empty json payload, got %d", wRefresh.Code)
	}
}

func TestRouter_RoutesRegistration(t *testing.T) {
	cfg := config.Load()
	ctrls := controllers.New(cfg, nil)
	router := routes.NewRouter(cfg, ctrls)

	expectedRoutes := map[string]string{
		"/v1/health":        "GET",
		"/openapi.yaml":     "GET",
		"/docs":             "GET",
		"/v1/auth/register": "POST",
		"/v1/auth/login":    "POST",
		"/v1/auth/refresh":  "POST",
	}

	registered := make(map[string]string)
	for _, route := range router.Routes() {
		registered[route.Path] = route.Method
	}

	for path, method := range expectedRoutes {
		if gotMethod, exists := registered[path]; !exists || gotMethod != method {
			t.Errorf("expected route %s %s to be registered in Gin, found method %s (exists=%v)", method, path, gotMethod, exists)
		}
	}
}
