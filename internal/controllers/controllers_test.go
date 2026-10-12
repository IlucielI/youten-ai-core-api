package controllers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"code-base-golang/internal/config"
	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/pkg/apperror"
	"code-base-golang/internal/pkg/ctxmeta"
)

func TestControllers_wrapError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctrls := New(config.Config{}, nil)

	t.Run("nil error does nothing", func(t *testing.T) {
		w := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(w)
		ctrls.wrapError(ctx, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("expected default 200 on nil error, got %d", w.Code)
		}
	})

	t.Run("AppError renders exact HTTP status and code", func(t *testing.T) {
		w := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(w)
		appErr := apperror.New(http.StatusConflict, "CONFLICT", "resource conflict")
		ctrls.wrapError(ctx, appErr)

		if w.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d", w.Code)
		}
		var resp dtos.BaseResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp.Status != constants.ResponseStatusFail || resp.Code != "CONFLICT" || resp.Message != "resource conflict" {
			t.Fatalf("unexpected response payload: %+v", resp)
		}
	})

	t.Run("constants errors from constants/error.go", func(t *testing.T) {
		cases := []struct {
			name       string
			err        error
			wantStatus int
			wantCode   string
		}{
			{"unauthorized", constants.ErrUnauthorized, http.StatusUnauthorized, constants.ResponseCodeUnauthorized},
			{"forbidden", constants.ErrForbidden, http.StatusForbidden, constants.ResponseCodeForbidden},
			{"not found", constants.ErrNotFound, http.StatusNotFound, constants.ResponseCodeNotFound},
			{"conflict", constants.ErrConflict, http.StatusConflict, constants.ResponseCodeConflict},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				w := httptest.NewRecorder()
				ctx, _ := gin.CreateTestContext(w)
				ctrls.wrapError(ctx, tc.err)

				if w.Code != tc.wantStatus {
					t.Fatalf("expected status %d, got %d", tc.wantStatus, w.Code)
				}
				var resp dtos.BaseResponse
				if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				if resp.Code != tc.wantCode {
					t.Fatalf("expected code %s, got %s", tc.wantCode, resp.Code)
				}
			})
		}
	})

	t.Run("wrapped constant error preserves code and uses custom error message", func(t *testing.T) {
		w := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(w)
		ctrls.wrapError(ctx, constants.ErrBadRequest.Wrap(errors.New("custom validation failure")))

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", w.Code)
		}
		var resp dtos.BaseResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp.Code != constants.ResponseCodeBadRequest || resp.Message != "custom validation failure" {
			t.Fatalf("unexpected response: %+v", resp)
		}
	})

	t.Run("raw unclassified error falls back to ErrInternalServerError", func(t *testing.T) {
		w := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(w)
		ctrls.wrapError(ctx, errors.New("unhandled database failure"))

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected status 500, got %d", w.Code)
		}
		var resp dtos.BaseResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}
		if resp.Code != constants.ResponseCodeInternalError || resp.Message != "internal server error" {
			t.Fatalf("expected code %s and safe message 'internal server error', got code %s message %s",
				constants.ResponseCodeInternalError, resp.Code, resp.Message)
		}
	})
}

func TestControllers_respondHelpers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctrls := New(config.Config{}, nil)

	t.Run("respondOK formats standard 200 envelope", func(t *testing.T) {
		w := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(w)
		data := map[string]string{"foo": "bar"}

		ctrls.respondOK(ctx, "success message", data)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		var resp dtos.APIResponse[map[string]string]
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp.Status != constants.ResponseStatusSuccess || resp.Code != constants.ResponseCodeSuccess {
			t.Errorf("unexpected status/code: %+v", resp)
		}
		if resp.Message != "success message" {
			t.Errorf("expected message 'success message', got %q", resp.Message)
		}
		if resp.Data["foo"] != "bar" {
			t.Errorf("expected data foo=bar, got %v", resp.Data)
		}
	})

	t.Run("respondCreated formats standard 201 envelope", func(t *testing.T) {
		w := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(w)
		data := map[string]int{"id": 42}

		ctrls.respondCreated(ctx, "created successfully", data)

		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d", w.Code)
		}
		var resp dtos.APIResponse[map[string]int]
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}
		if resp.Data["id"] != 42 {
			t.Errorf("expected id 42, got %v", resp.Data)
		}
	})

	t.Run("respondEmpty formats standard 200 base envelope", func(t *testing.T) {
		w := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(w)

		ctrls.respondEmpty(ctx, "deleted successfully")

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		var resp dtos.BaseResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}
		if resp.Message != "deleted successfully" {
			t.Errorf("expected message 'deleted successfully', got %q", resp.Message)
		}
	})
}

func TestControllers_getAdminActionMeta(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctrls := New(config.Config{}, nil)

	t.Run("returns unauthorized error when context lacks admin auth", func(t *testing.T) {
		w := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(w)
		req := httptest.NewRequest(http.MethodGet, "/v1/admin/users", nil)
		ctx.Request = req

		meta, ok := ctrls.getAdminActionMeta(ctx)
		if ok {
			t.Fatal("expected ok to be false")
		}
		if meta.AdminID != uuid.Nil {
			t.Fatalf("expected nil admin ID, got %v", meta.AdminID)
		}
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", w.Code)
		}
	})

	t.Run("returns valid AdminActionMeta when admin auth exists", func(t *testing.T) {
		w := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(w)
		adminID := uuid.New()
		req := httptest.NewRequest(http.MethodGet, "/v1/admin/users", nil)
		req.Header.Set("User-Agent", "TestAgent/1.0")
		req.RemoteAddr = "192.168.1.1:12345"

		cCtx := ctxmeta.WithAdminAuthUser(req.Context(), ctxmeta.AdminAuthUser{
			AdminID:  adminID,
			Username: "admin_tester",
		})
		cCtx = ctxmeta.WithClientMeta(cCtx, "192.168.1.1", "TestAgent/1.0")
		ctx.Request = req.WithContext(cCtx)

		meta, ok := ctrls.getAdminActionMeta(ctx)
		if !ok {
			t.Fatal("expected ok to be true")
		}
		if meta.AdminID != adminID {
			t.Fatalf("expected admin ID %v, got %v", adminID, meta.AdminID)
		}
		if meta.IPAddress != "192.168.1.1" {
			t.Errorf("expected IP 192.168.1.1, got %s", meta.IPAddress)
		}
		if meta.UserAgent != "TestAgent/1.0" {
			t.Errorf("expected User-Agent TestAgent/1.0, got %s", meta.UserAgent)
		}
	})
}

func TestControllers_getClientActionMeta(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctrls := New(config.Config{}, nil)

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/login", nil)
	cCtx := ctxmeta.WithClientMeta(req.Context(), "10.0.0.1", "AdminClient/2.0")
	ctx.Request = req.WithContext(cCtx)

	meta := ctrls.getClientActionMeta(ctx)
	if meta.IPAddress != "10.0.0.1" {
		t.Errorf("expected IP 10.0.0.1, got %s", meta.IPAddress)
	}
	if meta.UserAgent != "AdminClient/2.0" {
		t.Errorf("expected User-Agent AdminClient/2.0, got %s", meta.UserAgent)
	}
}

