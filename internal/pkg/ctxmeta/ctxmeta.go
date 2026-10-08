package ctxmeta

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

type contextKey string

const (
	clientIPKey      contextKey = "client_ip"
	userAgentKey     contextKey = "user_agent"
	authUserKey      contextKey = "auth_user"
	adminAuthUserKey contextKey = "admin_auth_user"
	requestIDKey     contextKey = "request_id"
)

// AuthUser represents authenticated user identity extracted from JWT and session.
type AuthUser struct {
	UserID      uuid.UUID
	Email       string
	SessionID   string
	RoleCode    string
	Permissions []string
	DailyQuota  int
	IsGuest     bool
}

// WithAuthUser injects the authenticated user into the context.
func WithAuthUser(ctx context.Context, user AuthUser) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, authUserKey, user)
}

// WithGuestUser injects an anonymous guest identity into the context.
func WithGuestUser(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, authUserKey, AuthUser{
		IsGuest: true,
	})
}

// GetAuthUser retrieves the authenticated or guest user from the context.
func GetAuthUser(ctx context.Context) (AuthUser, bool) {
	if ctx == nil {
		return AuthUser{}, false
	}
	user, ok := ctx.Value(authUserKey).(AuthUser)
	return user, ok
}

// GetAuthUserID retrieves the authenticated non-guest user ID from context.
// Returns uuid.Nil and false if context has no user, is nil, or represents a guest.
func GetAuthUserID(ctx context.Context) (uuid.UUID, bool) {
	user, ok := GetAuthUser(ctx)
	if !ok || user.IsGuest || user.UserID == uuid.Nil {
		return uuid.Nil, false
	}
	return user.UserID, true
}

// IsAuthenticated checks whether the context contains an authenticated, non-guest user.
func IsAuthenticated(ctx context.Context) bool {
	user, ok := GetAuthUser(ctx)
	return ok && !user.IsGuest && user.UserID != uuid.Nil
}

// WithClientMeta injects client IP and User-Agent metadata into the context.
func WithClientMeta(ctx context.Context, clientIP, userAgent string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = context.WithValue(ctx, clientIPKey, clientIP)
	return context.WithValue(ctx, userAgentKey, userAgent)
}

// GetClientIP retrieves the client IP address from context.
func GetClientIP(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if ip, ok := ctx.Value(clientIPKey).(string); ok {
		return ip
	}
	return ""
}

// GetUserAgent retrieves the User-Agent header from context.
func GetUserAgent(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if ua, ok := ctx.Value(userAgentKey).(string); ok {
		return ua
	}
	return ""
}

// WithRequestID injects request_id into the context.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, requestIDKey, requestID)
}

// GetRequestID retrieves the request_id from context.
func GetRequestID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if reqID, ok := ctx.Value(requestIDKey).(string); ok {
		return reqID
	}
	return ""
}

// AdminAuthUser represents an authenticated staff administrator.
type AdminAuthUser struct {
	AdminID     uuid.UUID
	Username    string
	FullName    string
	RoleID      uuid.UUID
	RoleName    string
	Permissions []string
}

// WithAdminAuthUser injects the authenticated admin into the context.
func WithAdminAuthUser(ctx context.Context, admin AdminAuthUser) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, adminAuthUserKey, admin)
}

// GetAdminAuthUser retrieves the authenticated admin from context.
func GetAdminAuthUser(ctx context.Context) (AdminAuthUser, bool) {
	if ctx == nil {
		return AdminAuthUser{}, false
	}
	admin, ok := ctx.Value(adminAuthUserKey).(AdminAuthUser)
	return admin, ok
}

// GetAdminAuthUserID retrieves the authenticated admin ID from context.
func GetAdminAuthUserID(ctx context.Context) (uuid.UUID, bool) {
	admin, ok := GetAdminAuthUser(ctx)
	if !ok || admin.AdminID == uuid.Nil {
		return uuid.Nil, false
	}
	return admin.AdminID, true
}

// IsAdminAuthenticated checks whether the context contains an authenticated admin.
func IsAdminAuthenticated(ctx context.Context) bool {
	admin, ok := GetAdminAuthUser(ctx)
	return ok && admin.AdminID != uuid.Nil
}

// HasAdminPermission checks if the admin user possesses the required permission.
// Grants access if:
// 1. Admin has wildcard "*" (Super Admin).
// 2. Admin has exact matching permission (e.g., "users:read").
// 3. Admin has category wildcard matching prefix (e.g., "users:*" matches "users:read").
func HasAdminPermission(user AdminAuthUser, requiredPerm string) bool {
	requiredPerm = strings.TrimSpace(requiredPerm)
	if requiredPerm == "" {
		return true
	}
	parts := strings.Split(requiredPerm, ":")
	domainPrefix := ""
	if len(parts) > 1 {
		domainPrefix = parts[0] + ":*"
	}

	for _, p := range user.Permissions {
		p = strings.TrimSpace(p)
		if p == "*" {
			return true
		}
		if p == requiredPerm {
			return true
		}
		if domainPrefix != "" && p == domainPrefix {
			return true
		}
	}
	return false
}

// HasUserPermission checks if the authenticated user possesses the required permission.
// Grants access if:
// 1. User has wildcard "*" (Full Access).
// 2. User has exact matching permission (e.g., "export:pdf").
// 3. User has category wildcard matching prefix (e.g., "recordings:*" matches "recordings:share").
func HasUserPermission(user AuthUser, requiredPerm string) bool {
	if user.IsGuest {
		return false
	}
	requiredPerm = strings.TrimSpace(requiredPerm)
	if requiredPerm == "" {
		return true
	}
	parts := strings.Split(requiredPerm, ":")
	domainPrefix := ""
	if len(parts) > 1 {
		domainPrefix = parts[0] + ":*"
	}

	for _, p := range user.Permissions {
		p = strings.TrimSpace(p)
		if p == "*" {
			return true
		}
		if p == requiredPerm {
			return true
		}
		if domainPrefix != "" && p == domainPrefix {
			return true
		}
	}
	return false
}


