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

	// ErrInvalidCredentials is returned when login credentials do not match.
	ErrInvalidCredentials = apperror.New(http.StatusUnauthorized, ResponseCodeUnauthorized, "invalid email or password")

	// ErrForbidden is returned when the authenticated user does not have permission.
	ErrForbidden = apperror.New(http.StatusForbidden, ResponseCodeForbidden, "forbidden access")

	// ErrUserInactive is returned when an inactive or suspended user attempts an action.
	ErrUserInactive = apperror.New(http.StatusForbidden, ResponseCodeForbidden, "user account is suspended or inactive")

	// ErrNotFound is returned when a requested resource is not found.
	ErrNotFound = apperror.New(http.StatusNotFound, ResponseCodeNotFound, "resource not found")

	// ErrUserNotFound is returned when a specified user does not exist.
	ErrUserNotFound = apperror.New(http.StatusNotFound, "ERR_USER_NOT_FOUND", "user not found")

	// ErrRecordingNotFound is returned when a specified recording is not found or has been deleted.
	ErrRecordingNotFound = apperror.New(http.StatusNotFound, "ERR_RECORDING_NOT_FOUND", "recording not found")

	// ErrConflict is returned when an action conflicts with current state.
	ErrConflict = apperror.New(http.StatusConflict, ResponseCodeConflict, "resource conflict")

	// ErrConflictProcessing is returned when attempting to retry or mutate a recording that is currently actively processing.
	ErrConflictProcessing = apperror.New(http.StatusConflict, "CONFLICT_PROCESSING", "recording is currently being processed, so retry cannot be initiated")

	// ErrRecordingAlreadyCompleted is returned when attempting to retry a recording that has already completed.
	ErrRecordingAlreadyCompleted = apperror.New(http.StatusConflict, "ERR_ALREADY_COMPLETED", "recording has already completed")

	// ErrSummaryVersionLimit is returned when attempting to generate more summary versions than allowed (cap of 5).
	ErrSummaryVersionLimit = apperror.New(http.StatusConflict, "SUMMARY_VERSION_LIMIT", "summary version limit reached, maximum allowed versions is 5")

	// ErrSummaryNotFound is returned when a requested summary version is not found.
	ErrSummaryNotFound = apperror.New(http.StatusNotFound, "ERR_SUMMARY_NOT_FOUND", "summary version not found")

	// ErrEmailAlreadyExists is returned when attempting to register with an email that is already registered.
	ErrEmailAlreadyExists = apperror.New(http.StatusConflict, "ERR_EMAIL_ALREADY_EXISTS", "email is already registered")

	// ErrInvalidToken is returned when a JWT token is expired, malformed, or has an invalid signature.
	ErrInvalidToken = apperror.New(http.StatusUnauthorized, ResponseCodeUnauthorized, "invalid or expired token")

	// ErrTooManyRequests is returned when a client exceeds rate limits.
	ErrTooManyRequests = apperror.New(http.StatusTooManyRequests, ResponseCodeTooManyRequests, "too many requests, please try again later")

	// ErrDailyQuotaExceeded is returned when an authenticated user exceeds their daily recording quota.
	ErrDailyQuotaExceeded = apperror.New(http.StatusTooManyRequests, ResponseCodeTooManyRequests, "daily recording quota exceeded")

	// ErrGuestDailyQuotaExceeded is returned when a guest user exceeds their daily recording quota.
	ErrGuestDailyQuotaExceeded = apperror.New(http.StatusTooManyRequests, ResponseCodeTooManyRequests, "guest daily recording quota exceeded")

	// ErrPayloadTooLarge is returned when uploaded file exceeds maximum allowed size.
	ErrPayloadTooLarge = apperror.New(http.StatusRequestEntityTooLarge, ResponseCodePayloadTooLarge, "payload too large, maximum size is 500MB")

	// ErrUnsupportedMediaType is returned when uploaded file format is not supported.
	ErrUnsupportedMediaType = apperror.New(http.StatusUnsupportedMediaType, ResponseCodeUnsupportedMediaType, "unsupported media type")

	// ErrSSRFBlocked is returned when an imported URL resolves to a forbidden, private, or internal network address.
	ErrSSRFBlocked = apperror.New(http.StatusBadRequest, "ERR_SSRF_BLOCKED", "import URL resolves to a forbidden, private, or internal network address")

	// ErrInvalidImportURL is returned when an imported URL has an invalid scheme or format.
	ErrInvalidImportURL = apperror.New(http.StatusBadRequest, "ERR_INVALID_IMPORT_URL", "invalid or unsupported import URL scheme, only http and https are allowed")

	// ErrImportFetchFailed is returned when downloading media from the imported URL fails.
	ErrImportFetchFailed = apperror.New(http.StatusBadRequest, "ERR_IMPORT_FETCH_FAILED", "failed to download media from import URL")

	// ErrInternalServerError is returned when an unexpected system error occurs.
	ErrInternalServerError = apperror.New(http.StatusInternalServerError, ResponseCodeInternalError, "internal server error")
)
