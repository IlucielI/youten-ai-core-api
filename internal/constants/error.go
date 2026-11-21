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

	// ErrInvalidRetryState is returned when a retry is requested for a recording that is not in the FAILED state.
	ErrInvalidRetryState = apperror.New(http.StatusConflict, "ERR_INVALID_RETRY_STATE", "recording can only be retried when status is FAILED")

	// ErrSummaryVersionLimit is returned when attempting to generate more summary versions than allowed (cap of 5).
	ErrSummaryVersionLimit = apperror.New(http.StatusConflict, "SUMMARY_VERSION_LIMIT", "summary version limit reached, maximum allowed versions is 5")

	// ErrSummaryNotFound is returned when a requested summary version is not found.
	ErrSummaryNotFound = apperror.New(http.StatusNotFound, "ERR_SUMMARY_NOT_FOUND", "summary version not found")

	// ErrTranscriptSegmentNotFound is returned when a requested transcript segment is not found.
	ErrTranscriptSegmentNotFound = apperror.New(http.StatusNotFound, "ERR_SEGMENT_NOT_FOUND", "transcript segment not found")

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
	ErrPayloadTooLarge = apperror.New(http.StatusRequestEntityTooLarge, ResponseCodePayloadTooLarge, "payload too large, maximum size is 100MB")

	// ErrUnsupportedMediaType is returned when uploaded file format is not supported.
	ErrUnsupportedMediaType = apperror.New(http.StatusUnsupportedMediaType, ResponseCodeUnsupportedMediaType, "unsupported media type")

	// ErrSSRFBlocked is returned when an imported URL resolves to a forbidden, private, or internal network address.
	ErrSSRFBlocked = apperror.New(http.StatusBadRequest, "ERR_SSRF_BLOCKED", "import URL resolves to a forbidden, private, or internal network address")

	// ErrInvalidImportURL is returned when an imported URL has an invalid scheme or format.
	ErrInvalidImportURL = apperror.New(http.StatusBadRequest, "ERR_INVALID_IMPORT_URL", "invalid or unsupported import URL scheme, only http and https are allowed")

	// ErrImportFetchFailed is returned when downloading media from the imported URL fails.
	ErrImportFetchFailed = apperror.New(http.StatusBadRequest, "ERR_IMPORT_FETCH_FAILED", "failed to download media from import URL")

	// ErrEmailNotVerified is returned when attempting to login before verifying email address.
	ErrEmailNotVerified = apperror.New(http.StatusForbidden, "ERR_EMAIL_NOT_VERIFIED", "email address has not been verified, please verify your email first")

	// ErrInvalidVerificationCode is returned when verification code or token is invalid.
	ErrInvalidVerificationCode = apperror.New(http.StatusBadRequest, "ERR_INVALID_VERIFICATION_CODE", "verification code or token is invalid or has expired")

	// ErrResendCooldown is returned when attempting to resend verification email before cooldown expires.
	ErrResendCooldown = apperror.New(http.StatusTooManyRequests, "ERR_RESEND_COOLDOWN", "please wait before requesting another verification email")

	// ErrResendDailyLimit is returned when user exceeds maximum daily resend attempts.
	ErrResendDailyLimit = apperror.New(http.StatusTooManyRequests, "ERR_RESEND_DAILY_LIMIT", "daily verification resend limit reached (maximum 3 times per 24 hours)")

	// ErrPipelineTimeout is returned when processing exceeds the maximum duration limit or pipeline stalls.
	ErrPipelineTimeout = apperror.New(http.StatusInternalServerError, "ERR_PIPELINE_TIMEOUT", "pipeline processing timed out or was abandoned")

	// ErrWorkspaceNotFound is returned when a requested workspace does not exist or has been deleted.
	ErrWorkspaceNotFound = apperror.New(http.StatusNotFound, "ERR_WORKSPACE_NOT_FOUND", "workspace not found")

	// ErrWorkspaceAccessDenied is returned when a user does not have membership or sufficient rights in a workspace.
	ErrWorkspaceAccessDenied = apperror.New(http.StatusForbidden, "ERR_WORKSPACE_ACCESS_DENIED", "access to this workspace is denied")

	// ErrCannotDeletePersonalWorkspace is returned when attempting to delete a default personal workspace.
	ErrCannotDeletePersonalWorkspace = apperror.New(http.StatusBadRequest, "ERR_CANNOT_DELETE_PERSONAL_WORKSPACE", "personal workspace cannot be deleted")

	// ErrCannotRemoveWorkspaceOwner is returned when attempting to delete or demote the owner of a workspace.
	ErrCannotRemoveWorkspaceOwner = apperror.New(http.StatusBadRequest, "ERR_CANNOT_REMOVE_WORKSPACE_OWNER", "workspace owner cannot be removed or demoted; transfer ownership first")

	// ErrWorkspaceSlugTaken is returned when a workspace slug is already in use.
	ErrWorkspaceSlugTaken = apperror.New(http.StatusConflict, "ERR_WORKSPACE_SLUG_TAKEN", "workspace slug is already in use")

	// ErrInvalidWorkspaceRole is returned when an invalid role is provided for a workspace member.
	ErrInvalidWorkspaceRole = apperror.New(http.StatusBadRequest, "ERR_INVALID_WORKSPACE_ROLE", "invalid workspace role specified")

	// ErrFolderNotFound is returned when a requested folder does not exist or has been deleted.
	ErrFolderNotFound = apperror.New(http.StatusNotFound, "ERR_FOLDER_NOT_FOUND", "folder not found")

	// ErrCircularFolderReference is returned when moving a folder into its own descendant.
	ErrCircularFolderReference = apperror.New(http.StatusBadRequest, "ERR_CIRCULAR_FOLDER_REFERENCE", "cannot move folder into itself or its child")

	// ErrInvitationNotFound is returned when an invitation is not found.
	ErrInvitationNotFound = apperror.New(http.StatusNotFound, "ERR_INVITATION_NOT_FOUND", "workspace invitation not found")

	// ErrInvitationExpired is returned when attempting to accept an expired invitation.
	ErrInvitationExpired = apperror.New(http.StatusBadRequest, "ERR_INVITATION_EXPIRED", "workspace invitation has expired")

	// ErrInvitationAlreadyAccepted is returned when an invitation has already been accepted.
	ErrInvitationAlreadyAccepted = apperror.New(http.StatusBadRequest, "ERR_INVITATION_ALREADY_ACCEPTED", "workspace invitation has already been accepted")

	// ErrAlreadyWorkspaceMember is returned when the invited email is already a member.
	ErrAlreadyWorkspaceMember = apperror.New(http.StatusConflict, "ERR_ALREADY_WORKSPACE_MEMBER", "user is already a member of this workspace")

	// ErrInvitationEmailMismatch is returned when the accepting user's email doesn't match the invitation.
	ErrInvitationEmailMismatch = apperror.New(http.StatusForbidden, "ERR_INVITATION_EMAIL_MISMATCH", "logged-in user email does not match invitation recipient")

	// ErrInternalServerError is returned when an unexpected system error occurs.
	ErrInternalServerError = apperror.New(http.StatusInternalServerError, ResponseCodeInternalError, "internal server error")
)
