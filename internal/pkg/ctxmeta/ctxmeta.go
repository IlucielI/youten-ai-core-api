package ctxmeta

import (
	"context"

	"github.com/google/uuid"
)

type contextKey string

const (
	clientIPKey  contextKey = "client_ip"
	userAgentKey contextKey = "user_agent"
	authUserKey  contextKey = "auth_user"
	requestIDKey contextKey = "request_id"
)

// AuthUser represents authenticated user identity extracted from JWT and session.
type AuthUser struct {
	UserID    uuid.UUID
	Email     string
	SessionID string
	IsGuest   bool
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

