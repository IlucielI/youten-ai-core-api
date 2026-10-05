package controllers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"code-base-golang/internal/config"
	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/pkg/apperror"
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
