package apperror

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
)

// AppError represents an application-level domain error carrying HTTP status, response codes, and caller location.
type AppError struct {
	HTTPStatus int
	Code       string
	Message    string
	RawErr     error
	Caller     string
}

// captureCaller returns the short file and line number of the caller (skip frames).
func captureCaller(skip int) string {
	_, file, line, ok := runtime.Caller(skip)
	if !ok {
		return ""
	}
	dir := filepath.Base(filepath.Dir(file))
	base := filepath.Base(file)
	return fmt.Sprintf("%s/%s:%d", dir, base, line)
}

// Error satisfies the Go standard error interface.
func (e *AppError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	if e.RawErr != nil {
		return e.RawErr.Error()
	}
	return ""
}

// Unwrap returns the underlying raw error for errors.Is / errors.As chaining.
func (e *AppError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.RawErr
}

// Is compares equality with another error or AppError.
func (e *AppError) Is(target error) bool {
	if e == nil || target == nil {
		return e == nil && target == nil
	}
	var targetAppErr *AppError
	if errors.As(target, &targetAppErr) {
		return e.HTTPStatus == targetAppErr.HTTPStatus &&
			e.Code == targetAppErr.Code &&
			e.Message == targetAppErr.Message
	}
	return false
}

// CallerInfo returns the recorded caller location (file:line) or an empty string.
func (e *AppError) CallerInfo() string {
	if e == nil {
		return ""
	}
	return e.Caller
}

// New constructs a new AppError.
func New(httpStatus int, code string, message string, rawErr ...error) *AppError {
	var err error
	if len(rawErr) > 0 {
		err = rawErr[0]
	}
	return &AppError{
		HTTPStatus: httpStatus,
		Code:       code,
		Message:    message,
		RawErr:     err,
		Caller:     captureCaller(2),
	}
}

// Wrap returns a copy of AppError wrapping rawErr with caller context.
// If rawErr is already an *AppError, it is returned unchanged (preserving existing caller).
// For client errors (HTTPStatus < 500), rawErr.Error() is used as Message to expose validation details.
// For server errors (HTTPStatus >= 500), the template's safe Message is preserved to prevent internal data leaks.
func (e *AppError) Wrap(rawErr error) *AppError {
	if e == nil {
		return nil
	}
	if rawErr == nil {
		return e
	}
	var existing *AppError
	if errors.As(rawErr, &existing) {
		return existing
	}

	msg := e.Message
	if e.HTTPStatus < 500 {
		msg = rawErr.Error()
	}

	return &AppError{
		HTTPStatus: e.HTTPStatus,
		Code:       e.Code,
		Message:    msg,
		RawErr:     rawErr,
		Caller:     captureCaller(2),
	}
}

// WithMessage returns a copy of AppError with an updated message.
func (e *AppError) WithMessage(msg string) *AppError {
	if e == nil {
		return nil
	}
	caller := e.Caller
	if caller == "" {
		caller = captureCaller(2)
	}
	return &AppError{
		HTTPStatus: e.HTTPStatus,
		Code:       e.Code,
		Message:    msg,
		RawErr:     e.RawErr,
		Caller:     caller,
	}
}

// WithCaller returns a copy of AppError with an explicit caller location override.
func (e *AppError) WithCaller(caller string) *AppError {
	if e == nil {
		return nil
	}
	return &AppError{
		HTTPStatus: e.HTTPStatus,
		Code:       e.Code,
		Message:    e.Message,
		RawErr:     e.RawErr,
		Caller:     caller,
	}
}
