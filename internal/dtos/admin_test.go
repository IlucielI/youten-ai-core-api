package dtos

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAdminUserListQuery_SetDefaults(t *testing.T) {
	t.Run("defaults invalid values", func(t *testing.T) {
		q := AdminUserListQuery{
			Page:  0,
			Limit: -5,
		}
		q.SetDefaults()

		if q.Page != 1 {
			t.Errorf("expected page 1, got %d", q.Page)
		}
		if q.Limit != 10 {
			t.Errorf("expected limit 10, got %d", q.Limit)
		}
	})

	t.Run("caps limit at 100", func(t *testing.T) {
		q := AdminUserListQuery{
			Page:  2,
			Limit: 250,
		}
		q.SetDefaults()

		if q.Page != 2 {
			t.Errorf("expected page 2, got %d", q.Page)
		}
		if q.Limit != 100 {
			t.Errorf("expected limit 100, got %d", q.Limit)
		}
	})

	t.Run("trims search and status", func(t *testing.T) {
		q := AdminUserListQuery{
			Search: "  alice@example.com  ",
			Status: "  active  ",
			Page:   1,
			Limit:  20,
		}
		q.SetDefaults()

		if q.Search != "alice@example.com" {
			t.Errorf("expected trimmed search, got %q", q.Search)
		}
		if q.Status != "active" {
			t.Errorf("expected trimmed status, got %q", q.Status)
		}
	})
}

func TestAdminUserListResponse(t *testing.T) {
	id := uuid.New()
	now := time.Now()
	resp := AdminUserListResponse{
		Items: []AdminUserListItem{
			{
				ID:             id,
				Email:          "admin_target@example.com",
				FullName:       "Target User",
				Status:         "active",
				DailyQuota:     5,
				QuotaUsedToday: 2,
				EmailVerified:  true,
				CreatedAt:      now,
			},
		},
		Pagination: PaginationMeta{
			CurrentPage: 1,
			PageSize:    10,
			TotalItems:  1,
			TotalPages:  1,
		},
	}

	if len(resp.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(resp.Items))
	}
	if resp.Items[0].Email != "admin_target@example.com" {
		t.Errorf("expected email admin_target@example.com, got %s", resp.Items[0].Email)
	}
	if resp.Pagination.TotalItems != 1 {
		t.Errorf("expected total_items 1, got %d", resp.Pagination.TotalItems)
	}
}

func TestAdminUserQuotaOverrideRequest_Validate(t *testing.T) {
	t.Run("nil override is valid (reset)", func(t *testing.T) {
		req := AdminUserQuotaOverrideRequest{DailyQuotaOverride: nil}
		if err := req.Validate(); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("positive override is valid", func(t *testing.T) {
		v := 25
		req := AdminUserQuotaOverrideRequest{DailyQuotaOverride: &v}
		if err := req.Validate(); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("zero override is valid", func(t *testing.T) {
		v := 0
		req := AdminUserQuotaOverrideRequest{DailyQuotaOverride: &v}
		if err := req.Validate(); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("negative override returns error", func(t *testing.T) {
		v := -1
		req := AdminUserQuotaOverrideRequest{DailyQuotaOverride: &v}
		if err := req.Validate(); err == nil {
			t.Fatal("expected error for negative override, got nil")
		}
	})

	t.Run("exceeding max override returns error", func(t *testing.T) {
		v := 10001
		req := AdminUserQuotaOverrideRequest{DailyQuotaOverride: &v}
		if err := req.Validate(); err == nil {
			t.Fatal("expected error for override exceeding 10000, got nil")
		}
	})
}

func TestAdminRevokeUserSessionsResponse(t *testing.T) {
	id := uuid.New()
	resp := AdminRevokeUserSessionsResponse{
		UserID:  id,
		Revoked: true,
		Message: "all sessions revoked",
	}

	if resp.UserID != id {
		t.Errorf("expected userID %v, got %v", id, resp.UserID)
	}
	if !resp.Revoked {
		t.Error("expected revoked true")
	}
	if resp.Message != "all sessions revoked" {
		t.Errorf("expected message, got %s", resp.Message)
	}
}

func TestAdminRoleListResponse(t *testing.T) {
	id := uuid.New()
	now := time.Now()
	resp := AdminRoleListResponse{
		Items: []AdminRoleItem{
			{
				ID:          id,
				Name:        "Super Admin",
				Description: "Root administrator",
				Permissions: []string{"*"},
				IsSystem:    true,
				CreatedAt:   now,
				UpdatedAt:   now,
			},
		},
	}

	if len(resp.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(resp.Items))
	}
	if resp.Items[0].Name != "Super Admin" {
		t.Errorf("expected name Super Admin, got %s", resp.Items[0].Name)
	}
	if len(resp.Items[0].Permissions) != 1 || resp.Items[0].Permissions[0] != "*" {
		t.Errorf("expected permissions [*], got %v", resp.Items[0].Permissions)
	}
	if !resp.Items[0].IsSystem {
		t.Error("expected is_system true")
	}
}
