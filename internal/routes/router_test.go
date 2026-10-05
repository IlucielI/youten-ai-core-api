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

	// Verify POST /v1/auth/logout route is registered and resolves to handler
	wLogout := httptest.NewRecorder()
	reqLogout, err := http.NewRequest(http.MethodPost, "/v1/auth/logout", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("failed to create logout request: %v", err)
	}
	reqLogout.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(wLogout, reqLogout)
	if wLogout.Code == http.StatusNotFound {
		t.Fatalf("expected /v1/auth/logout to be registered, but got 404 Not Found")
	}
	if wLogout.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for empty json payload, got %d", wLogout.Code)
	}

	// Verify POST /v1/auth/forgot-password route is registered and resolves to handler
	wForgot := httptest.NewRecorder()
	reqForgot, err := http.NewRequest(http.MethodPost, "/v1/auth/forgot-password", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("failed to create forgot-password request: %v", err)
	}
	reqForgot.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(wForgot, reqForgot)
	if wForgot.Code == http.StatusNotFound {
		t.Fatalf("expected /v1/auth/forgot-password to be registered, but got 404 Not Found")
	}
	if wForgot.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for empty json payload, got %d", wForgot.Code)
	}

	// Verify POST /v1/auth/reset-password route is registered and resolves to handler
	wReset := httptest.NewRecorder()
	reqReset, err := http.NewRequest(http.MethodPost, "/v1/auth/reset-password", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("failed to create reset-password request: %v", err)
	}
	reqReset.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(wReset, reqReset)
	if wReset.Code == http.StatusNotFound {
		t.Fatalf("expected /v1/auth/reset-password to be registered, but got 404 Not Found")
	}
	if wReset.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for empty json payload, got %d", wReset.Code)
	}
}

func TestRouter_RoutesRegistration(t *testing.T) {
	cfg := config.Load()
	ctrls := controllers.New(cfg, nil)
	router := routes.NewRouter(cfg, ctrls)

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
		{"POST", "/v1/recordings/:id/chat"},
		{"PUT", "/v1/recordings/:id/speakers"},
		{"POST", "/v1/recordings/:id/regenerate"},
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

	// Verify /v1/recordings/upload is configured with optional_auth (does NOT return 401 Unauthorized without header)
	wUpload := httptest.NewRecorder()
	reqUpload, err := http.NewRequest(http.MethodPost, "/v1/recordings/upload", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	router.ServeHTTP(wUpload, reqUpload)
	if wUpload.Code == http.StatusUnauthorized {
		t.Fatalf("expected /v1/recordings/upload not to return 401 Unauthorized without header (optional_auth), got %d", wUpload.Code)
	}

	// Verify /v1/recordings/import-url is configured with optional_auth (does NOT return 401 Unauthorized without header)
	wImport := httptest.NewRecorder()
	reqImport, err := http.NewRequest(http.MethodPost, "/v1/recordings/import-url", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	router.ServeHTTP(wImport, reqImport)
	if wImport.Code == http.StatusUnauthorized {
		t.Fatalf("expected /v1/recordings/import-url not to return 401 Unauthorized without header (optional_auth), got %d", wImport.Code)
	}

	// Verify /v1/recordings/:id is configured with optional_auth (does NOT return 401 Unauthorized without header)
	wDetail := httptest.NewRecorder()
	reqDetail, err := http.NewRequest(http.MethodGet, "/v1/recordings/00000000-0000-0000-0000-000000000001", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	router.ServeHTTP(wDetail, reqDetail)
	if wDetail.Code == http.StatusUnauthorized {
		t.Fatalf("expected /v1/recordings/:id not to return 401 Unauthorized without header (optional_auth), got %d", wDetail.Code)
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

	// Verify GET /v1/recordings/shared/:token is a public route (does NOT return 401 Unauthorized without header)
	wShared := httptest.NewRecorder()
	reqShared, err := http.NewRequest(http.MethodGet, "/v1/recordings/shared/token-123", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	router.ServeHTTP(wShared, reqShared)
	if wShared.Code == http.StatusUnauthorized {
		t.Fatalf("expected GET /v1/recordings/shared/:token not to return 401 Unauthorized (public route), got %d", wShared.Code)
	}

	// Verify GET /v1/recordings/:id/progress is configured with optional_auth (does NOT return 401 Unauthorized without header)
	wProgress := httptest.NewRecorder()
	reqProgress, err := http.NewRequest(http.MethodGet, "/v1/recordings/00000000-0000-0000-0000-000000000001/progress", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	router.ServeHTTP(wProgress, reqProgress)
	if wProgress.Code == http.StatusUnauthorized {
		t.Fatalf("expected GET /v1/recordings/:id/progress not to return 401 Unauthorized without header (optional_auth), got %d", wProgress.Code)
	}

	// Verify POST /v1/recordings/:id/retry is configured with optional_auth (does NOT return 401 Unauthorized without header)
	wRetry := httptest.NewRecorder()
	reqRetry, err := http.NewRequest(http.MethodPost, "/v1/recordings/00000000-0000-0000-0000-000000000001/retry", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	router.ServeHTTP(wRetry, reqRetry)
	if wRetry.Code == http.StatusUnauthorized {
		t.Fatalf("expected POST /v1/recordings/:id/retry not to return 401 Unauthorized without header (optional_auth), got %d", wRetry.Code)
	}

	// Verify POST /v1/recordings/:id/chat is configured with optional_auth (does NOT return 401 Unauthorized without header)
	wChat := httptest.NewRecorder()
	reqChat, err := http.NewRequest(http.MethodPost, "/v1/recordings/00000000-0000-0000-0000-000000000001/chat", strings.NewReader(`{"message":"test"}`))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	reqChat.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(wChat, reqChat)
	if wChat.Code == http.StatusUnauthorized {
		t.Fatalf("expected POST /v1/recordings/:id/chat not to return 401 Unauthorized without header (optional_auth), got %d", wChat.Code)
	}

	// Verify PUT /v1/recordings/:id/speakers is configured with optional_auth (does NOT return 401 Unauthorized without header)
	wSpeakers := httptest.NewRecorder()
	reqSpeakers, err := http.NewRequest(http.MethodPut, "/v1/recordings/00000000-0000-0000-0000-000000000001/speakers", strings.NewReader(`{"speakers":{"SPEAKER_00":"Alice"}}`))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	reqSpeakers.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(wSpeakers, reqSpeakers)
	if wSpeakers.Code == http.StatusUnauthorized {
		t.Fatalf("expected PUT /v1/recordings/:id/speakers not to return 401 Unauthorized without header (optional_auth), got %d", wSpeakers.Code)
	}

	// Verify POST /v1/recordings/:id/regenerate is configured with optional_auth (does NOT return 401 Unauthorized without header)
	wRegen := httptest.NewRecorder()
	reqRegen, err := http.NewRequest(http.MethodPost, "/v1/recordings/00000000-0000-0000-0000-000000000001/regenerate", strings.NewReader(`{"template_category":"MOM"}`))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	reqRegen.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(wRegen, reqRegen)
	if wRegen.Code == http.StatusUnauthorized {
		t.Fatalf("expected POST /v1/recordings/:id/regenerate not to return 401 Unauthorized without header (optional_auth), got %d", wRegen.Code)
	}

	// Verify GET /v1/recordings/:id/summaries is configured with optional_auth (does NOT return 401 Unauthorized without header)
	wSummaries := httptest.NewRecorder()
	reqSummaries, err := http.NewRequest(http.MethodGet, "/v1/recordings/00000000-0000-0000-0000-000000000001/summaries", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	router.ServeHTTP(wSummaries, reqSummaries)
	if wSummaries.Code == http.StatusUnauthorized {
		t.Fatalf("expected GET /v1/recordings/:id/summaries not to return 401 Unauthorized without header (optional_auth), got %d", wSummaries.Code)
	}

	// Verify PATCH /v1/recordings/:id/summaries/:versionId/activate is configured with optional_auth (does NOT return 401 Unauthorized without header)
	wActivate := httptest.NewRecorder()
	reqActivate, err := http.NewRequest(http.MethodPatch, "/v1/recordings/00000000-0000-0000-0000-000000000001/summaries/1/activate", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	router.ServeHTTP(wActivate, reqActivate)
	if wActivate.Code == http.StatusUnauthorized {
		t.Fatalf("expected PATCH /v1/recordings/:id/summaries/:versionId/activate not to return 401 Unauthorized without header (optional_auth), got %d", wActivate.Code)
	}

	// Verify POST /v1/recordings/:id/comments is configured with optional_auth (does NOT return 401 Unauthorized without header)
	wComments := httptest.NewRecorder()
	reqComments, err := http.NewRequest(http.MethodPost, "/v1/recordings/00000000-0000-0000-0000-000000000001/comments", strings.NewReader(`{"comment_text":"test"}`))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	reqComments.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(wComments, reqComments)
	if wComments.Code == http.StatusUnauthorized {
		t.Fatalf("expected POST /v1/recordings/:id/comments not to return 401 Unauthorized without header (optional_auth), got %d", wComments.Code)
	}

	// Verify GET /v1/recordings/:id/comments is configured with optional_auth (does NOT return 401 Unauthorized without header)
	wListComments := httptest.NewRecorder()
	reqListComments, err := http.NewRequest(http.MethodGet, "/v1/recordings/00000000-0000-0000-0000-000000000001/comments", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	router.ServeHTTP(wListComments, reqListComments)
	if wListComments.Code == http.StatusUnauthorized {
		t.Fatalf("expected GET /v1/recordings/:id/comments not to return 401 Unauthorized without header (optional_auth), got %d", wListComments.Code)
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

	// Verify DELETE /v1/recordings/:id/comments/:commentId is configured with optional_auth (does NOT return 401 Unauthorized without header)
	wDelComment := httptest.NewRecorder()
	reqDelComment, err := http.NewRequest(http.MethodDelete, "/v1/recordings/00000000-0000-0000-0000-000000000001/comments/00000000-0000-0000-0000-000000000002", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	router.ServeHTTP(wDelComment, reqDelComment)
	if wDelComment.Code == http.StatusUnauthorized {
		t.Fatalf("expected DELETE /v1/recordings/:id/comments/:commentId not to return 401 Unauthorized without header (optional_auth), got %d", wDelComment.Code)
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

	// Verify GET /v1/recordings/:id/export is configured with optional_auth (does NOT return 401 Unauthorized without header)
	wExport := httptest.NewRecorder()
	reqExport, err := http.NewRequest(http.MethodGet, "/v1/recordings/00000000-0000-0000-0000-000000000001/export?format=markdown", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	router.ServeHTTP(wExport, reqExport)
	if wExport.Code == http.StatusUnauthorized {
		t.Fatalf("expected GET /v1/recordings/:id/export not to return 401 Unauthorized without header (optional_auth), got %d", wExport.Code)
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
}





