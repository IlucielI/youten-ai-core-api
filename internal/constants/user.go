package constants

const (
	// User status definitions
	UserStatusActive    = "active"
	UserStatusSuspended = "suspended"
	UserStatusPending   = "pending"

	// Default daily quota allocations
	DefaultUserDailyQuota  = 5
	DefaultGuestDailyQuota = 1

	// Default max file size allocations (in MB)
	DefaultMaxFileSizeMB      = 500
	DefaultGuestMaxFileSizeMB = 500
)

// AuthTokenType defines valid token types stored in the database auth_tokens table.
type AuthTokenType string

const (
	AuthTokenTypeRefresh           AuthTokenType = "refresh"
	AuthTokenTypeAdminRefresh      AuthTokenType = "admin_refresh"
	AuthTokenTypeReset             AuthTokenType = "reset_password"
	AuthTokenTypeEmailVerification AuthTokenType = "email_verification"
	AuthTokenTypeEmailVerificationCode AuthTokenType = "email_verification_code"
)

// JWTTokenType defines token types embedded in JWT claims.
type JWTTokenType string

const (
	JWTTokenTypeAccess       JWTTokenType = "access"
	JWTTokenTypeRefresh      JWTTokenType = "refresh"
	JWTTokenTypeAdminAccess  JWTTokenType = "admin_access"
	JWTTokenTypeAdminRefresh JWTTokenType = "admin_refresh"
	JWTTokenTypeAnonAccess   JWTTokenType = "anon_access"
)
