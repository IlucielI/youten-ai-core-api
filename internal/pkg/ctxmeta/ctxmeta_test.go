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

