package controllers_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"code-base-golang/internal/config"
	"code-base-golang/internal/controllers"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestDocsController_OpenAPISpec(t *testing.T) {
	ctrls := controllers.New(config.Config{}, nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/openapi.yaml", nil)

	ctrls.OpenAPISpec(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if !strings.Contains(contentType, "application/x-yaml") {
		t.Errorf("expected Content-Type application/x-yaml, got %q", contentType)
	}

	body := w.Body.String()
	if !strings.Contains(body, "openapi: 3.0.3") {
		t.Errorf("expected body to contain 'openapi: 3.0.3', got: %s", body[:min(len(body), 100)])
	}
	if !strings.Contains(body, "url: /") {
		t.Errorf("expected body to contain relative server url 'url: /'")
	}
	if !strings.Contains(body, "description: Primary API Server") {
		t.Errorf("expected body to contain 'description: Primary API Server'")
	}
	if !strings.Contains(body, "/v1/health") {
		t.Errorf("expected body to contain '/v1/health'")
	}
}

func TestDocsController_APIDocs(t *testing.T) {
	ctrls := controllers.New(config.Config{}, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/docs", nil)

	ctrls.APIDocs(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		t.Errorf("expected Content-Type text/html, got %q", contentType)
	}

	body := w.Body.String()
	if !strings.Contains(body, "@scalar/api-reference") {
		t.Errorf("expected body to contain @scalar/api-reference")
	}
	if !strings.Contains(body, "data-url=\"/openapi.yaml\"") {
		t.Errorf("expected body to contain data-url=\"/openapi.yaml\"")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
