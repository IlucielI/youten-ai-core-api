package apperror

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestAppError_Error(t *testing.T) {
	// Without raw error
	err1 := New(http.StatusBadRequest, "BAD_REQUEST", "invalid title")
	if err1.Error() != "invalid title" {
		t.Fatalf("expected 'invalid title', got '%s'", err1.Error())
	}

	// With raw error
	raw := errors.New("underlying db crash")
	err2 := New(http.StatusInternalServerError, "INTERNAL_ERROR", "database error", raw)
	if err2.Error() != "database error" {
		t.Fatalf("unexpected error string: %s", err2.Error())
	}
	if !errors.Is(err2, raw) {
		t.Fatal("expected errors.Is to match raw error through Unwrap")
	}

	// With empty message but raw error
	err3 := New(http.StatusInternalServerError, "INTERNAL_ERROR", "", raw)
	if err3.Error() != "underlying db crash" {
		t.Fatalf("expected raw error message, got: %s", err3.Error())
	}

	// Nil error receiver
	var nilErr *AppError
	if nilErr.Error() != "" {
		t.Fatalf("expected empty string for nil error, got '%s'", nilErr.Error())
	}
	if nilErr.Unwrap() != nil {
		t.Fatal("expected nil unwrap for nil error")
	}
}

func TestAppError_Is(t *testing.T) {
	err1 := New(http.StatusNotFound, "NOT_FOUND", "task not found")
	err2 := New(http.StatusNotFound, "NOT_FOUND", "task not found")
	err3 := New(http.StatusNotFound, "NOT_FOUND", "user not found")
	err4 := New(http.StatusBadRequest, "BAD_REQUEST", "task not found")

	if !err1.Is(err2) {
		t.Fatal("expected err1.Is(err2) to be true")
	}
	if err1.Is(err3) {
		t.Fatal("expected err1.Is(err3) to be false (different message)")
	}
	if err1.Is(err4) {
		t.Fatal("expected err1.Is(err4) to be false (different status/code)")
	}
	if err1.Is(errors.New("standard error")) {
		t.Fatal("expected err1.Is to return false for standard error")
	}
}

func TestAppError_Wrap(t *testing.T) {
	template := New(http.StatusBadRequest, "BAD_REQUEST", "bad request")

	// Nil receiver
	var nilErr *AppError
	if nilErr.Wrap(errors.New("err")) != nil {
		t.Fatal("expected nil on nil receiver")
	}

	// Nil raw error
	if template.Wrap(nil) != template {
		t.Fatal("expected original template on nil rawErr")
	}

	// Raw error wrapping
	raw := errors.New("field title is required")
	wrapped := template.Wrap(raw)
	if wrapped.HTTPStatus != http.StatusBadRequest || wrapped.Code != "BAD_REQUEST" || wrapped.Message != "field title is required" {
		t.Fatalf("unexpected wrapped error: %+v", wrapped)
	}
	if !errors.Is(wrapped, raw) {
		t.Fatal("expected errors.Is to match wrapped raw error")
	}

	// Already an AppError
	existing := New(http.StatusNotFound, "NOT_FOUND", "not found")
	wrappedExisting := template.Wrap(existing)
	if wrappedExisting != existing {
		t.Fatal("expected Wrap to return existing AppError without double-wrapping")
	}

	// 5xx error preserves safe message
	serverTemplate := New(http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	dbErr := errors.New("pq: password authentication failed")
	wrappedServer := serverTemplate.Wrap(dbErr)
	if wrappedServer.Message != "internal server error" {
		t.Fatalf("expected 500 error to preserve safe message, got: %s", wrappedServer.Message)
	}
	if !errors.Is(wrappedServer, dbErr) {
		t.Fatal("expected 500 error to still wrap dbErr as RawErr")
	}
}

func TestAppError_WithMessage(t *testing.T) {
	template := New(http.StatusBadRequest, "BAD_REQUEST", "bad request")

	var nilErr *AppError
	if nilErr.WithMessage("msg") != nil {
		t.Fatal("expected nil on nil receiver")
	}

	updated := template.WithMessage("new custom message")
	if updated.Message != "new custom message" || updated.HTTPStatus != http.StatusBadRequest {
		t.Fatalf("unexpected updated error: %+v", updated)
	}
}

func TestAppError_Caller(t *testing.T) {
	err := New(http.StatusInternalServerError, "INTERNAL_ERROR", "db failed")
	if !strings.Contains(err.CallerInfo(), "apperror_test.go:") {
		t.Fatalf("expected caller info to contain apperror_test.go, got: %s", err.CallerInfo())
	}

	raw := errors.New("underlying io error")
	wrapped := err.Wrap(raw)
	if !strings.Contains(wrapped.CallerInfo(), "apperror_test.go:") {
		t.Fatalf("expected wrapped caller info to contain apperror_test.go, got: %s", wrapped.CallerInfo())
	}

	override := wrapped.WithCaller("custom/file.go:99")
	if override.CallerInfo() != "custom/file.go:99" {
		t.Fatalf("expected override caller, got: %s", override.CallerInfo())
	}

	var nilErr *AppError
	if nilErr.CallerInfo() != "" {
		t.Fatalf("expected empty string for nil caller info, got: %s", nilErr.CallerInfo())
	}
	if nilErr.WithCaller("xyz") != nil {
		t.Fatal("expected nil on WithCaller for nil receiver")
	}
}


