package constants

import (
	"net/http"

	"code-base-golang/internal/pkg/apperror"
)

var (
	// ErrBadRequest is returned for generic or wrapped invalid client requests.
	ErrBadRequest = apperror.New(http.StatusBadRequest, ResponseCodeBadRequest, "bad request")

	// ErrUnauthorized is returned when an operation is attempted without valid authentication.
	ErrUnauthorized = apperror.New(http.StatusUnauthorized, ResponseCodeUnauthorized, "unauthorized access")

	// ErrForbidden is returned when the authenticated user does not have permission.
	ErrForbidden = apperror.New(http.StatusForbidden, ResponseCodeForbidden, "forbidden access")

	// ErrNotFound is returned when a requested resource is not found.
	ErrNotFound = apperror.New(http.StatusNotFound, ResponseCodeNotFound, "resource not found")

	// ErrConflict is returned when an action conflicts with current state.
	ErrConflict = apperror.New(http.StatusConflict, ResponseCodeConflict, "resource conflict")

	// ErrEmailAlreadyExists is returned when attempting to register with an email that is already registered.
	ErrEmailAlreadyExists = apperror.New(http.StatusConflict, "ERR_EMAIL_ALREADY_EXISTS", "email is already registered")

	// ErrInvalidToken is returned when a JWT token is expired, malformed, or has an invalid signature.
	ErrInvalidToken = apperror.New(http.StatusUnauthorized, ResponseCodeUnauthorized, "invalid or expired token")

	// ErrTooManyRequests is returned when a client exceeds rate limits.
	ErrTooManyRequests = apperror.New(http.StatusTooManyRequests, ResponseCodeTooManyRequests, "too many requests, please try again later")

	// ErrInternalServerError is returned when an unexpected system error occurs.
	ErrInternalServerError = apperror.New(http.StatusInternalServerError, ResponseCodeInternalError, "internal server error")
)
