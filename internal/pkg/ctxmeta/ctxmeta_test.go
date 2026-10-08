package ctxmeta

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestContextMeta(t *testing.T) {
	ctx := context.Background()

	// Initial empty
	if ip := GetClientIP(ctx); ip != "" {
		t.Fatalf("expected empty IP, got %s", ip)
	}
	if ua := GetUserAgent(ctx); ua != "" {
		t.Fatalf("expected empty UA, got %s", ua)
	}

	// Injected
	ctx = WithClientMeta(ctx, "192.168.1.100", "Mozilla/5.0 UnitTester")
	if ip := GetClientIP(ctx); ip != "192.168.1.100" {
		t.Fatalf("expected 192.168.1.100, got %s", ip)
	}
	if ua := GetUserAgent(ctx); ua != "Mozilla/5.0 UnitTester" {
		t.Fatalf("expected Mozilla/5.0 UnitTester, got %s", ua)
	}

	// Nil context checks
	if ip := GetClientIP(nil); ip != "" {
		t.Fatalf("expected empty on nil ctx, got %s", ip)
	}
	if ua := GetUserAgent(nil); ua != "" {
		t.Fatalf("expected empty on nil ctx, got %s", ua)
	}
}

func TestAuthUser(t *testing.T) {
	ctx := context.Background()

	// Initial empty
	if _, ok := GetAuthUser(ctx); ok {
		t.Fatal("expected no auth user on empty ctx")
	}

	// Nil context check
	if _, ok := GetAuthUser(nil); ok {
		t.Fatal("expected no auth user on nil ctx")
	}

	// Injected
	expectedUser := AuthUser{
		UserID:    uuid.New(),
		Email:     "user@example.com",
		SessionID: "sess-12345",
		IsGuest:   false,
	}
	ctx = WithAuthUser(ctx, expectedUser)
	user, ok := GetAuthUser(ctx)
	if !ok {
		t.Fatal("expected auth user to be present")
	}
	if user.Email != expectedUser.Email || user.SessionID != expectedUser.SessionID {
		t.Fatalf("expected user %+v, got %+v", expectedUser, user)
	}

	// Check GetAuthUserID & IsAuthenticated
	uid, ok := GetAuthUserID(ctx)
	if !ok || uid != expectedUser.UserID {
		t.Fatalf("expected user id %s, got %s (ok=%v)", expectedUser.UserID, uid, ok)
	}
	if !IsAuthenticated(ctx) {
		t.Fatal("expected IsAuthenticated to be true")
	}

	// Nil context in WithAuthUser
	nilCtx := WithAuthUser(nil, expectedUser)
	user, ok = GetAuthUser(nilCtx)
	if !ok || user.Email != expectedUser.Email {
		t.Fatalf("expected user %+v from nil context injection", expectedUser)
	}

	// Guest user tests
	guestCtx := WithGuestUser(context.Background())
	gUser, gOk := GetAuthUser(guestCtx)
	if !gOk || !gUser.IsGuest {
		t.Fatalf("expected guest user to have IsGuest=true, got %+v", gUser)
	}
	if _, ok := GetAuthUserID(guestCtx); ok {
		t.Fatal("expected GetAuthUserID to return false for guest user")
	}
	if IsAuthenticated(guestCtx) {
		t.Fatal("expected IsAuthenticated to return false for guest user")
	}

	// Nil context in WithGuestUser
	nilGuestCtx := WithGuestUser(nil)
	if gUser, gOk := GetAuthUser(nilGuestCtx); !gOk || !gUser.IsGuest {
		t.Fatal("expected valid guest user from WithGuestUser(nil)")
	}

	// Nil context in helpers
	if _, ok := GetAuthUserID(nil); ok {
		t.Fatal("expected GetAuthUserID(nil) to return false")
	}
	if IsAuthenticated(nil) {
		t.Fatal("expected IsAuthenticated(nil) to return false")
	}
}

func TestRequestID(t *testing.T) {
	ctx := context.Background()

	// Initial empty
	if reqID := GetRequestID(ctx); reqID != "" {
		t.Fatalf("expected empty request_id on empty ctx, got %s", reqID)
	}

	// Nil context check
	if reqID := GetRequestID(nil); reqID != "" {
		t.Fatalf("expected empty request_id on nil ctx, got %s", reqID)
	}

	// Injected
	expectedID := "c138db50-9669-42b4-82a1-12c8a149f1db"
	ctx = WithRequestID(ctx, expectedID)
	if reqID := GetRequestID(ctx); reqID != expectedID {
		t.Fatalf("expected %s, got %s", expectedID, reqID)
	}

	// Nil context in WithRequestID
	nilCtx := WithRequestID(nil, expectedID)
	if reqID := GetRequestID(nilCtx); reqID != expectedID {
		t.Fatalf("expected %s from nil context injection, got %s", expectedID, reqID)
	}
}

func TestAdminAuthUser(t *testing.T) {
	ctx := context.Background()

	// Initial empty
	if _, ok := GetAdminAuthUser(ctx); ok {
		t.Fatal("expected no admin user on empty ctx")
	}
	if _, ok := GetAdminAuthUserID(ctx); ok {
		t.Fatal("expected no admin user id on empty ctx")
	}
	if IsAdminAuthenticated(ctx) {
		t.Fatal("expected IsAdminAuthenticated to be false")
	}

	// Nil context check
	if _, ok := GetAdminAuthUser(nil); ok {
		t.Fatal("expected no admin user on nil ctx")
	}
	if _, ok := GetAdminAuthUserID(nil); ok {
		t.Fatal("expected no admin user id on nil ctx")
	}
	if IsAdminAuthenticated(nil) {
		t.Fatal("expected IsAdminAuthenticated(nil) to be false")
	}

	// Injected
	adminID := uuid.New()
	roleID := uuid.New()
	expectedAdmin := AdminAuthUser{
		AdminID:     adminID,
		Username:    "superadmin",
		FullName:    "Super Administrator",
		RoleID:      roleID,
		RoleName:    "Super Admin",
		Permissions: []string{"*"},
	}

	ctx = WithAdminAuthUser(ctx, expectedAdmin)
	admin, ok := GetAdminAuthUser(ctx)
	if !ok {
		t.Fatal("expected admin auth user to be present")
	}
	if admin.Username != expectedAdmin.Username || admin.AdminID != adminID {
		t.Fatalf("expected admin %+v, got %+v", expectedAdmin, admin)
	}

	id, ok := GetAdminAuthUserID(ctx)
	if !ok || id != adminID {
		t.Fatalf("expected admin id %s, got %s", adminID, id)
	}
	if !IsAdminAuthenticated(ctx) {
		t.Fatal("expected IsAdminAuthenticated to be true")
	}

	// Nil context injection
	nilCtx := WithAdminAuthUser(nil, expectedAdmin)
	admin, ok = GetAdminAuthUser(nilCtx)
	if !ok || admin.AdminID != adminID {
		t.Fatal("expected valid admin from WithAdminAuthUser(nil)")
	}
}

func TestHasAdminPermission(t *testing.T) {
	tests := []struct {
		name         string
		permissions  []string
		requiredPerm string
		expected     bool
	}{
		{
			name:         "empty required perm always passes",
			permissions:  []string{},
			requiredPerm: "",
			expected:     true,
		},
		{
			name:         "wildcard gives access to all",
			permissions:  []string{"*"},
			requiredPerm: "users:read",
			expected:     true,
		},
		{
			name:         "exact match passes",
			permissions:  []string{"users:read", "roles:read"},
			requiredPerm: "users:read",
			expected:     true,
		},
		{
			name:         "prefix wildcard passes",
			permissions:  []string{"users:*"},
			requiredPerm: "users:write",
			expected:     true,
		},
		{
			name:         "unrelated permission fails",
			permissions:  []string{"templates:read"},
			requiredPerm: "users:read",
			expected:     false,
		},
		{
			name:         "different prefix fails",
			permissions:  []string{"roles:*"},
			requiredPerm: "users:delete",
			expected:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			admin := AdminAuthUser{Permissions: tc.permissions}
			result := HasAdminPermission(admin, tc.requiredPerm)
			if result != tc.expected {
				t.Errorf("HasAdminPermission(%v, %q) = %v; expected %v", tc.permissions, tc.requiredPerm, result, tc.expected)
			}
		})
	}
}

func TestHasUserPermission(t *testing.T) {
	tests := []struct {
		name         string
		user         AuthUser
		requiredPerm string
		expected     bool
	}{
		{
			name:         "guest user always fails",
			user:         AuthUser{IsGuest: true, Permissions: []string{"*"}},
			requiredPerm: "export:pdf",
			expected:     false,
		},
		{
			name:         "empty required perm passes",
			user:         AuthUser{Permissions: []string{"recordings:read"}},
			requiredPerm: "",
			expected:     true,
		},
		{
			name:         "wildcard star passes all",
			user:         AuthUser{Permissions: []string{"*"}},
			requiredPerm: "export:pdf",
			expected:     true,
		},
		{
			name:         "exact match passes",
			user:         AuthUser{Permissions: []string{"recordings:create", "export:pdf"}},
			requiredPerm: "export:pdf",
			expected:     true,
		},
		{
			name:         "prefix wildcard passes",
			user:         AuthUser{Permissions: []string{"recordings:*"}},
			requiredPerm: "recordings:share",
			expected:     true,
		},
		{
			name:         "unrelated permission fails",
			user:         AuthUser{Permissions: []string{"recordings:read"}},
			requiredPerm: "export:pdf",
			expected:     false,
		},
		{
			name:         "different prefix fails",
			user:         AuthUser{Permissions: []string{"workspace:*"}},
			requiredPerm: "recordings:share",
			expected:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := HasUserPermission(tc.user, tc.requiredPerm)
			if result != tc.expected {
				t.Errorf("HasUserPermission(%v, %q) = %v; expected %v", tc.user.Permissions, tc.requiredPerm, result, tc.expected)
			}
		})
	}
}


