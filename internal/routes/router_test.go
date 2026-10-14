package routes_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"code-base-golang/docs"
	"code-base-golang/internal/config"
	"code-base-golang/internal/constants"
	"code-base-golang/internal/controllers"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/pkg/ctxmeta"
	"code-base-golang/internal/routes"
	"code-base-golang/internal/services"
	"gopkg.in/yaml.v3"
)

func TestRouter_HealthCheck(t *testing.T) {
	cfg := config.Load()
	// Build a real service with no wired adapters so the health check reports
	// every dependency as disconnected without a backing infrastructure.
	svc := services.New(cfg, nil, nil)
	ctrls := controllers.New(cfg, svc)
	router := routes.NewRouter(cfg, ctrls, svc)

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
	router := routes.NewRouter(cfg, ctrls, services.New(cfg, nil, nil))

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
	router := routes.NewRouter(cfg, ctrls, services.New(cfg, nil, nil))

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
	mockVal := &mockScopedValidator{
		user: &ctxmeta.AuthUser{
			UserID:    uuid.New(),
			SessionID: uuid.New().String(),
			RoleCode:  "USER",
		},
	}
	router := routes.NewRouter(cfg, ctrls, mockVal)

	authProtectedEndpoints := []struct {
		name   string
		method string
		path   string
	}{
		{"login", http.MethodPost, "/v1/auth/login"},
		{"refresh", http.MethodPost, "/v1/auth/refresh"},
		{"logout", http.MethodPost, "/v1/auth/logout"},
		{"forgot-password", http.MethodPost, "/v1/auth/forgot-password"},
		{"reset-password", http.MethodPost, "/v1/auth/reset-password"},
	}

	for _, ep := range authProtectedEndpoints {
		t.Run(ep.name+" without token returns 401", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(ep.method, ep.path, strings.NewReader("{}"))
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")

			router.ServeHTTP(w, req)
			if w.Code == http.StatusNotFound {
				t.Fatalf("expected %s %s to be registered, got 404", ep.method, ep.path)
			}
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401 Unauthorized without bearer token for %s %s, got %d", ep.method, ep.path, w.Code)
			}
		})

		if ep.path != "/v1/auth/logout" {
			t.Run(ep.name+" with valid bearer resolves to handler", func(t *testing.T) {
				w := httptest.NewRecorder()
				req, err := http.NewRequest(ep.method, ep.path, strings.NewReader("{}"))
				if err != nil {
					t.Fatalf("failed to create request: %v", err)
				}
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer valid-token")

				router.ServeHTTP(w, req)
				if w.Code != http.StatusBadRequest {
					t.Fatalf("expected 400 Bad Request for empty json with bearer for %s %s, got %d", ep.method, ep.path, w.Code)
				}
			})
		}
	}
}

func TestRouter_RoutesRegistration(t *testing.T) {
	cfg := config.Load()
	ctrls := controllers.New(cfg, nil)
	router := routes.NewRouter(cfg, ctrls, services.New(cfg, nil, nil))

	expectedRoutePairs := []struct {
		method string
		path   string
	}{
		{"GET", "/v1/health"},
		{"GET", "/openapi.yaml"},
		{"GET", "/docs"},
		{"POST", "/v1/auth/register"},
		{"POST", "/v1/auth/login"},
		{"POST", "/v1/auth/refresh"},
		{"POST", "/v1/auth/logout"},
		{"POST", "/v1/auth/forgot-password"},
		{"POST", "/v1/auth/reset-password"},
		{"GET", "/v1/auth/me"},
		{"PUT", "/v1/auth/me"},
		{"PUT", "/v1/auth/change-password"},
		{"POST", "/v1/recordings/upload"},
		{"POST", "/v1/recordings/import-url"},
		{"GET", "/v1/recordings/:id"},
		{"GET", "/v1/recordings"},
		{"DELETE", "/v1/recordings/:id"},
		{"POST", "/v1/recordings/:id/claim"},
		{"POST", "/v1/recordings/claim"},
		{"PATCH", "/v1/recordings/:id/share"},
		{"GET", "/v1/recordings/shared/:token"},
		{"GET", "/v1/recordings/:id/progress"},
		{"POST", "/v1/recordings/:id/retry"},
		{"GET", "/v1/recordings/:id/chat"},
		{"POST", "/v1/recordings/:id/chat"},
		{"PUT", "/v1/recordings/:id/speakers"},
		{"POST", "/v1/recordings/:id/regenerate"},
		{"POST", "/v1/waitlist/bot"},
		{"POST", "/v1/recordings/meeting-bot"},
		{"GET", "/v1/recordings/meeting-bot/:id/status"},
		{"POST", "/v1/recordings/meeting-bot/:id/stop"},
		{"GET", "/v1/capabilities"},
	}

	registeredPairs := make(map[string]bool)
	for _, route := range router.Routes() {
		registeredPairs[route.Method+" "+route.Path] = true
	}

	for _, r := range expectedRoutePairs {
		key := r.method + " " + r.path
		if !registeredPairs[key] {
			t.Errorf("expected route %s to be registered in Gin", key)
		}
	}

	// Verify /v1/auth/me (GET and PUT) and /v1/auth/change-password (PUT) are protected by auth middleware (returns 401 Unauthorized without header)
	wMeGet := httptest.NewRecorder()
	reqMeGet, err := http.NewRequest(http.MethodGet, "/v1/auth/me", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	router.ServeHTTP(wMeGet, reqMeGet)
	if wMeGet.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for GET /v1/auth/me without token, got %d", wMeGet.Code)
	}

	wMePut := httptest.NewRecorder()
	reqMePut, err := http.NewRequest(http.MethodPut, "/v1/auth/me", strings.NewReader(`{"full_name":"New Name"}`))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	reqMePut.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(wMePut, reqMePut)
	if wMePut.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for PUT /v1/auth/me without token, got %d", wMePut.Code)
	}

	wChangePwd := httptest.NewRecorder()
	reqChangePwd, err := http.NewRequest(http.MethodPut, "/v1/auth/change-password", strings.NewReader(`{"old_password":"old","new_password":"new"}`))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	reqChangePwd.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(wChangePwd, reqChangePwd)
	if wChangePwd.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for PUT /v1/auth/change-password without token, got %d", wChangePwd.Code)
	}

	// Verify /v1/recordings/upload is configured with scope: recordings:create (returns 401 Unauthorized without header)
	wUpload := httptest.NewRecorder()
	reqUpload, err := http.NewRequest(http.MethodPost, "/v1/recordings/upload", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	router.ServeHTTP(wUpload, reqUpload)
	if wUpload.Code != http.StatusUnauthorized {
		t.Fatalf("expected /v1/recordings/upload to return 401 Unauthorized without header (scope: recordings:create), got %d", wUpload.Code)
	}

	// Verify /v1/recordings/import-url is configured with scope: recordings:create (returns 401 Unauthorized without header)
	wImport := httptest.NewRecorder()
	reqImport, err := http.NewRequest(http.MethodPost, "/v1/recordings/import-url", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	router.ServeHTTP(wImport, reqImport)
	if wImport.Code != http.StatusUnauthorized {
		t.Fatalf("expected /v1/recordings/import-url to return 401 Unauthorized without header (scope: recordings:create), got %d", wImport.Code)
	}

	// Verify /v1/recordings/:id is configured with auth: true (returns 401 Unauthorized without header)
	wDetail := httptest.NewRecorder()
	reqDetail, err := http.NewRequest(http.MethodGet, "/v1/recordings/00000000-0000-0000-0000-000000000001", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	router.ServeHTTP(wDetail, reqDetail)
	if wDetail.Code != http.StatusUnauthorized {
		t.Fatalf("expected /v1/recordings/:id to return 401 Unauthorized without header, got %d", wDetail.Code)
	}

	// Verify /v1/recordings is configured with auth: true (returns 401 Unauthorized without header)
	wList := httptest.NewRecorder()
	reqList, err := http.NewRequest(http.MethodGet, "/v1/recordings", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	router.ServeHTTP(wList, reqList)
	if wList.Code != http.StatusUnauthorized {
		t.Fatalf("expected /v1/recordings to return 401 Unauthorized without header (auth: true), got %d", wList.Code)
	}

	// Verify DELETE /v1/recordings/:id is configured with auth: true (returns 401 Unauthorized without header)
	wDelete := httptest.NewRecorder()
	reqDelete, err := http.NewRequest(http.MethodDelete, "/v1/recordings/00000000-0000-0000-0000-000000000001", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	router.ServeHTTP(wDelete, reqDelete)
	if wDelete.Code != http.StatusUnauthorized {
		t.Fatalf("expected DELETE /v1/recordings/:id to return 401 Unauthorized without header (auth: true), got %d", wDelete.Code)
	}

	// Verify POST /v1/recordings/:id/claim is configured with auth: true (returns 401 Unauthorized without header)
	wClaim := httptest.NewRecorder()
	reqClaim, err := http.NewRequest(http.MethodPost, "/v1/recordings/00000000-0000-0000-0000-000000000001/claim", strings.NewReader(`{"ownership_token":"tok"}`))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	reqClaim.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(wClaim, reqClaim)
	if wClaim.Code != http.StatusUnauthorized {
		t.Fatalf("expected POST /v1/recordings/:id/claim to return 401 Unauthorized without header (auth: true), got %d", wClaim.Code)
	}

	// Verify POST /v1/recordings/claim is configured with auth: true (returns 401 Unauthorized without header)
	wBulkClaim := httptest.NewRecorder()
	reqBulkClaim, err := http.NewRequest(http.MethodPost, "/v1/recordings/claim", strings.NewReader(`{"tokens":["tok1","tok2"]}`))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	reqBulkClaim.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(wBulkClaim, reqBulkClaim)
	if wBulkClaim.Code != http.StatusUnauthorized {
		t.Fatalf("expected POST /v1/recordings/claim to return 401 Unauthorized without header (auth: true), got %d", wBulkClaim.Code)
	}

	// Verify PATCH /v1/recordings/:id/share is configured with auth: true (returns 401 Unauthorized without header)
	wShare := httptest.NewRecorder()
	reqShare, err := http.NewRequest(http.MethodPatch, "/v1/recordings/00000000-0000-0000-0000-000000000001/share", strings.NewReader(`{"is_share_enabled":true}`))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	reqShare.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(wShare, reqShare)
	if wShare.Code != http.StatusUnauthorized {
		t.Fatalf("expected PATCH /v1/recordings/:id/share to return 401 Unauthorized without header (auth: true), got %d", wShare.Code)
	}

	// Verify GET /v1/recordings/shared/:token is protected by auth (returns 401 Unauthorized without header)
	wShared := httptest.NewRecorder()
	reqShared, err := http.NewRequest(http.MethodGet, "/v1/recordings/shared/token-123", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	router.ServeHTTP(wShared, reqShared)
	if wShared.Code != http.StatusUnauthorized {
		t.Fatalf("expected GET /v1/recordings/shared/:token to return 401 Unauthorized without header, got %d", wShared.Code)
	}

	// Verify GET /v1/recordings/:id/progress is configured with auth: true (returns 401 Unauthorized without header)
	wProgress := httptest.NewRecorder()
	reqProgress, err := http.NewRequest(http.MethodGet, "/v1/recordings/00000000-0000-0000-0000-000000000001/progress", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	router.ServeHTTP(wProgress, reqProgress)
	if wProgress.Code != http.StatusUnauthorized {
		t.Fatalf("expected GET /v1/recordings/:id/progress to return 401 Unauthorized without header (auth: true), got %d", wProgress.Code)
	}

	// Verify POST /v1/recordings/:id/retry is configured with auth: true (returns 401 Unauthorized without header)
	wRetry := httptest.NewRecorder()
	reqRetry, err := http.NewRequest(http.MethodPost, "/v1/recordings/00000000-0000-0000-0000-000000000001/retry", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	router.ServeHTTP(wRetry, reqRetry)
	if wRetry.Code != http.StatusUnauthorized {
		t.Fatalf("expected POST /v1/recordings/:id/retry to return 401 Unauthorized without header (auth: true), got %d", wRetry.Code)
	}

	// Verify POST /v1/recordings/:id/chat is configured with auth: true (returns 401 Unauthorized without header)
	wChat := httptest.NewRecorder()
	reqChat, err := http.NewRequest(http.MethodPost, "/v1/recordings/00000000-0000-0000-0000-000000000001/chat", strings.NewReader(`{"message":"test"}`))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	reqChat.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(wChat, reqChat)
	if wChat.Code != http.StatusUnauthorized {
		t.Fatalf("expected POST /v1/recordings/:id/chat to return 401 Unauthorized without header (auth: true), got %d", wChat.Code)
	}

	// Verify PUT /v1/recordings/:id/speakers is configured with auth: true (returns 401 Unauthorized without header)
	wSpeakers := httptest.NewRecorder()
	reqSpeakers, err := http.NewRequest(http.MethodPut, "/v1/recordings/00000000-0000-0000-0000-000000000001/speakers", strings.NewReader(`{"speakers":{"SPEAKER_00":"Alice"}}`))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	reqSpeakers.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(wSpeakers, reqSpeakers)
	if wSpeakers.Code != http.StatusUnauthorized {
		t.Fatalf("expected PUT /v1/recordings/:id/speakers to return 401 Unauthorized without header (auth: true), got %d", wSpeakers.Code)
	}

	// Verify POST /v1/recordings/:id/regenerate is configured with auth: true (returns 401 Unauthorized without header)
	wRegen := httptest.NewRecorder()
	reqRegen, err := http.NewRequest(http.MethodPost, "/v1/recordings/00000000-0000-0000-0000-000000000001/regenerate", strings.NewReader(`{"template_category":"MOM"}`))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	reqRegen.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(wRegen, reqRegen)
	if wRegen.Code != http.StatusUnauthorized {
		t.Fatalf("expected POST /v1/recordings/:id/regenerate to return 401 Unauthorized without header (auth: true), got %d", wRegen.Code)
	}

	// Verify GET /v1/recordings/:id/summaries is configured with auth: true (returns 401 Unauthorized without header)
	wSummaries := httptest.NewRecorder()
	reqSummaries, err := http.NewRequest(http.MethodGet, "/v1/recordings/00000000-0000-0000-0000-000000000001/summaries", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	router.ServeHTTP(wSummaries, reqSummaries)
	if wSummaries.Code != http.StatusUnauthorized {
		t.Fatalf("expected GET /v1/recordings/:id/summaries to return 401 Unauthorized without header (auth: true), got %d", wSummaries.Code)
	}

	// Verify PATCH /v1/recordings/:id/summaries/:versionId/activate is configured with auth: true (returns 401 Unauthorized without header)
	wActivate := httptest.NewRecorder()
	reqActivate, err := http.NewRequest(http.MethodPatch, "/v1/recordings/00000000-0000-0000-0000-000000000001/summaries/1/activate", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	router.ServeHTTP(wActivate, reqActivate)
	if wActivate.Code != http.StatusUnauthorized {
		t.Fatalf("expected PATCH /v1/recordings/:id/summaries/:versionId/activate to return 401 Unauthorized without header (auth: true), got %d", wActivate.Code)
	}

	// Verify POST /v1/recordings/:id/comments is configured with auth: true (returns 401 Unauthorized without header)
	wComments := httptest.NewRecorder()
	reqComments, err := http.NewRequest(http.MethodPost, "/v1/recordings/00000000-0000-0000-0000-000000000001/comments", strings.NewReader(`{"comment_text":"test"}`))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	reqComments.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(wComments, reqComments)
	if wComments.Code != http.StatusUnauthorized {
		t.Fatalf("expected POST /v1/recordings/:id/comments to return 401 Unauthorized without header (auth: true), got %d", wComments.Code)
	}

	// Verify GET /v1/recordings/:id/comments is configured with auth: true (returns 401 Unauthorized without header)
	wListComments := httptest.NewRecorder()
	reqListComments, err := http.NewRequest(http.MethodGet, "/v1/recordings/00000000-0000-0000-0000-000000000001/comments", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	router.ServeHTTP(wListComments, reqListComments)
	if wListComments.Code != http.StatusUnauthorized {
		t.Fatalf("expected GET /v1/recordings/:id/comments to return 401 Unauthorized without header (auth: true), got %d", wListComments.Code)
	}

	foundDeleteCommentRoute := false
	for _, route := range router.Routes() {
		if route.Method == http.MethodDelete && route.Path == "/v1/recordings/:id/comments/:commentId" {
			foundDeleteCommentRoute = true
			break
		}
	}
	if !foundDeleteCommentRoute {
		t.Fatalf("expected DELETE /v1/recordings/:id/comments/:commentId route to be registered")
	}

	// Verify DELETE /v1/recordings/:id/comments/:commentId is configured with auth: true (returns 401 Unauthorized without header)
	wDelComment := httptest.NewRecorder()
	reqDelComment, err := http.NewRequest(http.MethodDelete, "/v1/recordings/00000000-0000-0000-0000-000000000001/comments/00000000-0000-0000-0000-000000000002", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	router.ServeHTTP(wDelComment, reqDelComment)
	if wDelComment.Code != http.StatusUnauthorized {
		t.Fatalf("expected DELETE /v1/recordings/:id/comments/:commentId to return 401 Unauthorized without header (auth: true), got %d", wDelComment.Code)
	}

	foundExportRoute := false
	for _, route := range router.Routes() {
		if route.Method == http.MethodGet && route.Path == "/v1/recordings/:id/export" {
			foundExportRoute = true
			break
		}
	}
	if !foundExportRoute {
		t.Fatalf("expected GET /v1/recordings/:id/export route to be registered")
	}

	// Verify GET /v1/recordings/:id/export is configured with auth: true (returns 401 Unauthorized without header)
	wExport := httptest.NewRecorder()
	reqExport, err := http.NewRequest(http.MethodGet, "/v1/recordings/00000000-0000-0000-0000-000000000001/export?format=markdown", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	router.ServeHTTP(wExport, reqExport)
	if wExport.Code != http.StatusUnauthorized {
		t.Fatalf("expected GET /v1/recordings/:id/export to return 401 Unauthorized without header (auth: true), got %d", wExport.Code)
	}

	foundSearchRoute := false
	for _, route := range router.Routes() {
		if route.Method == http.MethodGet && route.Path == "/v1/recordings/search" {
			foundSearchRoute = true
			break
		}
	}
	if !foundSearchRoute {
		t.Fatalf("expected GET /v1/recordings/search route to be registered")
	}

	// Verify GET /v1/recordings/search is protected by auth (returns 401 Unauthorized without header)
	wSearch := httptest.NewRecorder()
	reqSearch, err := http.NewRequest(http.MethodGet, "/v1/recordings/search?q=test", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	router.ServeHTTP(wSearch, reqSearch)
	if wSearch.Code != http.StatusUnauthorized {
		t.Fatalf("expected GET /v1/recordings/search to return 401 Unauthorized without auth header, got %d", wSearch.Code)
	}

	foundAskRoute := false
	for _, route := range router.Routes() {
		if route.Method == http.MethodPost && route.Path == "/v1/recordings/ask" {
			foundAskRoute = true
			break
		}
	}
	if !foundAskRoute {
		t.Fatalf("expected POST /v1/recordings/ask route to be registered")
	}

	// Verify POST /v1/recordings/ask is protected by auth (returns 401 Unauthorized without header)
	wAsk := httptest.NewRecorder()
	reqAsk, err := http.NewRequest(http.MethodPost, "/v1/recordings/ask", strings.NewReader(`{"question":"test"}`))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	reqAsk.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(wAsk, reqAsk)
	if wAsk.Code != http.StatusUnauthorized {
		t.Fatalf("expected POST /v1/recordings/ask to return 401 Unauthorized without auth header, got %d", wAsk.Code)
	}

	foundSpeakersRoute := false
	for _, route := range router.Routes() {
		if route.Method == http.MethodGet && route.Path == "/v1/speakers" {
			foundSpeakersRoute = true
			break
		}
	}
	if !foundSpeakersRoute {
		t.Fatalf("expected GET /v1/speakers route to be registered")
	}

	// Verify GET /v1/speakers is protected by auth (returns 401 Unauthorized without header)
	wSpkDir := httptest.NewRecorder()
	reqSpkDir, reqSpkErr := http.NewRequest(http.MethodGet, "/v1/speakers", nil)
	if reqSpkErr != nil {
		t.Fatalf("failed to create request: %v", reqSpkErr)
	}
	router.ServeHTTP(wSpkDir, reqSpkDir)
	if wSpkDir.Code != http.StatusUnauthorized {
		t.Fatalf("expected GET /v1/speakers to return 401 Unauthorized without auth header, got %d", wSpkDir.Code)
	}

	// Verify POST /v1/waitlist/bot is protected by auth: true (returns 401 Unauthorized without auth header)
	wWaitlist := httptest.NewRecorder()
	reqWaitlist, err := http.NewRequest(http.MethodPost, "/v1/waitlist/bot", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	reqWaitlist.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(wWaitlist, reqWaitlist)
	if wWaitlist.Code != http.StatusUnauthorized {
		t.Fatalf("expected POST /v1/waitlist/bot to return 401 Unauthorized without header, got %d", wWaitlist.Code)
	}

	// Verify POST /v1/admin/refresh is registered and protected by auth: true
	foundAdminRefreshRoute := false
	for _, route := range router.Routes() {
		if route.Method == http.MethodPost && route.Path == "/v1/admin/refresh" {
			foundAdminRefreshRoute = true
			break
		}
	}
	if !foundAdminRefreshRoute {
		t.Fatalf("expected POST /v1/admin/refresh route to be registered")
	}

	wAdminRefresh := httptest.NewRecorder()
	reqAdminRefresh, err := http.NewRequest(http.MethodPost, "/v1/admin/refresh", strings.NewReader(`{"refresh_token":"abc"}`))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	reqAdminRefresh.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(wAdminRefresh, reqAdminRefresh)
	if wAdminRefresh.Code != http.StatusUnauthorized {
		t.Fatalf("expected POST /v1/admin/refresh to return 401 Unauthorized without auth header, got %d", wAdminRefresh.Code)
	}
}

func TestRouter_OpenAPISpecCompleteness(t *testing.T) {
	var spec struct {
		OpenAPI string                            `yaml:"openapi"`
		Info    map[string]interface{}            `yaml:"info"`
		Paths   map[string]map[string]interface{} `yaml:"paths"`
		Components struct {
			Schemas map[string]interface{} `yaml:"schemas"`
		} `yaml:"components"`
	}

	if err := yaml.Unmarshal(docs.OpenAPISpec, &spec); err != nil {
		t.Fatalf("failed to parse openapi.yaml: %v", err)
	}

	if spec.OpenAPI != "3.0.3" {
		t.Errorf("expected openapi: 3.0.3, got %s", spec.OpenAPI)
	}
	if len(spec.Paths) == 0 {
		t.Fatalf("expected non-empty paths in openapi.yaml")
	}
	if len(spec.Components.Schemas) == 0 {
		t.Fatalf("expected non-empty components.schemas in openapi.yaml")
	}

	cfg := config.Load()
	ctrls := controllers.New(cfg, nil)
	router := routes.NewRouter(cfg, ctrls, services.New(cfg, nil, nil))

	for _, route := range router.Routes() {
		if route.Path == "/openapi.yaml" || route.Path == "/docs" {
			continue
		}

		// Normalize Gin param syntax (:param) to OpenAPI path parameter syntax ({param})
		openApiPath := route.Path
		openApiPath = strings.ReplaceAll(openApiPath, ":id", "{id}")
		openApiPath = strings.ReplaceAll(openApiPath, ":token", "{token}")
		openApiPath = strings.ReplaceAll(openApiPath, ":segmentId", "{segmentId}")
		openApiPath = strings.ReplaceAll(openApiPath, ":versionId", "{versionId}")
		openApiPath = strings.ReplaceAll(openApiPath, ":commentId", "{commentId}")

		methods, exists := spec.Paths[openApiPath]
		if !exists {
			t.Errorf("route %s %s is registered in routes.yaml but missing from openapi.yaml (expected path %s)",
				route.Method, route.Path, openApiPath)
			continue
		}

		httpMethod := strings.ToLower(route.Method)
		if _, ok := methods[httpMethod]; !ok {
			t.Errorf("route %s %s is missing HTTP method %s in openapi.yaml path %s",
				route.Method, route.Path, httpMethod, openApiPath)
		}
	}

	// Reverse check: verify that every operation in openapi.yaml maps back to a registered Gin route
	registeredRoutes := make(map[string]map[string]bool)
	for _, route := range router.Routes() {
		openApiPath := route.Path
		openApiPath = strings.ReplaceAll(openApiPath, ":id", "{id}")
		openApiPath = strings.ReplaceAll(openApiPath, ":token", "{token}")
		openApiPath = strings.ReplaceAll(openApiPath, ":segmentId", "{segmentId}")
		openApiPath = strings.ReplaceAll(openApiPath, ":versionId", "{versionId}")
		openApiPath = strings.ReplaceAll(openApiPath, ":commentId", "{commentId}")

		if registeredRoutes[openApiPath] == nil {
			registeredRoutes[openApiPath] = make(map[string]bool)
		}
		registeredRoutes[openApiPath][strings.ToLower(route.Method)] = true
	}

	for path, methods := range spec.Paths {
		for method := range methods {
			if !registeredRoutes[path][method] {
				t.Errorf("openapi.yaml documents operation %s %s which is not registered in routes.yaml",
					strings.ToUpper(method), path)
			}
		}
	}
}

type mockScopedValidator struct {
	user *ctxmeta.AuthUser
	err  error
}

func (m *mockScopedValidator) Authenticate(ctx context.Context, tokenStr string) (*ctxmeta.AuthUser, error) {
	return m.user, m.err
}

func (m *mockScopedValidator) AuthenticateAdmin(ctx context.Context, tokenStr string) (*ctxmeta.AdminAuthUser, error) {
	return nil, nil
}

func TestRouter_ScopedRoutes(t *testing.T) {
	cfg := config.Load()
	ctrls := controllers.New(cfg, nil)

	t.Run("upload missing token returns 401", func(t *testing.T) {
		validator := &mockScopedValidator{}
		r := routes.NewRouter(cfg, ctrls, validator)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/v1/recordings/upload", nil)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 without token, got %d", w.Code)
		}
	})

	t.Run("upload with token lacking scope returns 403", func(t *testing.T) {
		validator := &mockScopedValidator{
			user: &ctxmeta.AuthUser{
				UserID:      uuid.New(),
				SessionID:   uuid.New().String(),
				RoleCode:    "ANON",
				Permissions: []string{"waitlist:join"},
				IsGuest:     true,
			},
		}
		r := routes.NewRouter(cfg, ctrls, validator)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/v1/recordings/upload", nil)
		req.Header.Set("Authorization", "Bearer token-without-scope")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403 when token lacks recordings:create scope, got %d", w.Code)
		}
	})

	t.Run("upload with guest token having recordings:create scope passes to controller", func(t *testing.T) {
		validator := &mockScopedValidator{
			user: &ctxmeta.AuthUser{
				UserID:      uuid.New(),
				SessionID:   uuid.New().String(),
				RoleCode:    "ANON",
				Permissions: []string{"recordings:create", "recordings:read"},
				IsGuest:     true,
			},
		}
		r := routes.NewRouter(cfg, ctrls, validator)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/v1/recordings/upload", strings.NewReader(`{}`))
		req.Header.Set("Authorization", "Bearer valid-scoped-token")
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		// Passes auth and scope middleware, reaches controller handler which fails on empty payload with 400
		if w.Code == http.StatusUnauthorized || w.Code == http.StatusForbidden {
			t.Fatalf("expected request to pass scope middleware, but got %d", w.Code)
		}
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 from controller validation, got %d", w.Code)
		}
	})

	t.Run("upload with user token having recordings:create scope passes to controller", func(t *testing.T) {
		validator := &mockScopedValidator{
			user: &ctxmeta.AuthUser{
				UserID:      uuid.New(),
				Email:       "user@example.com",
				RoleCode:    "FREE",
				Permissions: []string{"recordings:create", "recordings:read"},
				IsGuest:     false,
			},
		}
		r := routes.NewRouter(cfg, ctrls, validator)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/v1/recordings/upload", strings.NewReader(`{}`))
		req.Header.Set("Authorization", "Bearer valid-user-token")
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		// Passes auth and scope middleware, reaches controller handler which fails on empty payload with 400
		if w.Code == http.StatusUnauthorized || w.Code == http.StatusForbidden {
			t.Fatalf("expected request to pass scope middleware, but got %d", w.Code)
		}
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 from controller validation, got %d", w.Code)
		}
	})
}

func TestRouter_UserPermissionRoutes(t *testing.T) {
	cfg := config.Load()
	ctrls := controllers.New(cfg, nil)

	t.Run("search without workspace:search permission returns 403", func(t *testing.T) {
		validator := &mockScopedValidator{
			user: &ctxmeta.AuthUser{
				UserID:      uuid.New(),
				Email:       "free@example.com",
				RoleCode:    "FREE",
				Permissions: []string{"recordings:create", "recordings:read", "profile:manage"},
				IsGuest:     false,
			},
		}
		r := routes.NewRouter(cfg, ctrls, validator)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/v1/recordings/search?q=test", nil)
		req.Header.Set("Authorization", "Bearer free-user-token")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden for search without workspace:search, got %d", w.Code)
		}
	})

	t.Run("search with workspace:search permission passes middleware", func(t *testing.T) {
		validator := &mockScopedValidator{
			user: &ctxmeta.AuthUser{
				UserID:      uuid.New(),
				Email:       "pro@example.com",
				RoleCode:    "PRO",
				Permissions: []string{"workspace:search", "workspace:memory"},
				IsGuest:     false,
			},
		}
		r := routes.NewRouter(cfg, ctrls, validator)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/v1/recordings/search?q=test", nil)
		req.Header.Set("Authorization", "Bearer pro-user-token")
		r.ServeHTTP(w, req)

		if w.Code == http.StatusUnauthorized || w.Code == http.StatusForbidden {
			t.Fatalf("expected user with workspace:search to pass middleware, got %d", w.Code)
		}
	})

	t.Run("ask memory without workspace:memory permission returns 403", func(t *testing.T) {
		validator := &mockScopedValidator{
			user: &ctxmeta.AuthUser{
				UserID:      uuid.New(),
				Email:       "free@example.com",
				RoleCode:    "FREE",
				Permissions: []string{"recordings:create", "recordings:read"},
				IsGuest:     false,
			},
		}
		r := routes.NewRouter(cfg, ctrls, validator)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/v1/recordings/ask", strings.NewReader(`{"question":"what?"}`))
		req.Header.Set("Authorization", "Bearer free-user-token")
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden for ask without workspace:memory, got %d", w.Code)
		}
	})

	t.Run("guest user attempting user permission route returns 401", func(t *testing.T) {
		validator := &mockScopedValidator{
			user: &ctxmeta.AuthUser{
				UserID:      uuid.New(),
				SessionID:   uuid.New().String(),
				RoleCode:    "ANON",
				Permissions: []string{"workspace:search"},
				IsGuest:     true,
			},
		}
		r := routes.NewRouter(cfg, ctrls, validator)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/v1/recordings/search?q=test", nil)
		req.Header.Set("Authorization", "Bearer guest-token")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 Unauthorized for guest on user permission route, got %d", w.Code)
		}
	})
}



