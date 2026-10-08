package constants

const (
	// User status definitions
	UserStatusActive    = "active"
	UserStatusSuspended = "suspended"

	// Default daily quota allocations
	DefaultUserDailyQuota  = 5
	DefaultGuestDailyQuota = 1
)

// AuthTokenType defines valid token types stored in the database auth_tokens table.
type AuthTokenType string

const (
	AuthTokenTypeRefresh AuthTokenType = "refresh"
	AuthTokenTypeReset   AuthTokenType = "reset_password"
)

// JWTTokenType defines token types embedded in JWT claims.
type JWTTokenType string

const (
	JWTTokenTypeAccess      JWTTokenType = "access"
	JWTTokenTypeRefresh     JWTTokenType = "refresh"
	JWTTokenTypeAdminAccess JWTTokenType = "admin_access"
	JWTTokenTypeAnonAccess  JWTTokenType = "anon_access"
)
