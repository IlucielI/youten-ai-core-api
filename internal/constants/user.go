package constants

const (
	// User status definitions
	UserStatusActive    = "active"
	UserStatusSuspended = "suspended"

	// Default daily quota allocations
	DefaultUserDailyQuota  = 5
	DefaultGuestDailyQuota = 1

	// Auth token types (database)
	AuthTokenTypeRefresh = "REFRESH"
	AuthTokenTypeReset   = "RESET_PASSWORD"

	// JWT token claim types
	JWTTokenTypeAccess  = "access"
	JWTTokenTypeRefresh = "refresh"
)
