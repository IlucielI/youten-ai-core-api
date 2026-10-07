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

func TestAdminCreateRoleRequest_Validate(t *testing.T) {
	t.Run("valid payload succeeds", func(t *testing.T) {
		req := AdminCreateRoleRequest{
			Name:        "Support Agent",
			Description: "Customer support role",
			Permissions: []string{"users:read", "recordings:read"},
		}
		if err := req.Validate(); err != nil {
			t.Fatalf("expected valid, got error: %v", err)
		}
		if req.Name != "Support Agent" {
			t.Errorf("expected trimmed name, got %q", req.Name)
		}
	})

	t.Run("empty name returns error", func(t *testing.T) {
		req := AdminCreateRoleRequest{
			Name:        "   ",
			Permissions: []string{"users:read"},
		}
		if err := req.Validate(); err == nil {
			t.Fatal("expected error for empty name, got nil")
		}
	})

	t.Run("empty permissions returns error", func(t *testing.T) {
		req := AdminCreateRoleRequest{
			Name:        "Editor",
			Permissions: []string{},
		}
		if err := req.Validate(); err == nil {
			t.Fatal("expected error for empty permissions, got nil")
		}
	})

	t.Run("blank permission item returns error", func(t *testing.T) {
		req := AdminCreateRoleRequest{
			Name:        "Editor",
			Permissions: []string{"users:read", "  "},
		}
		if err := req.Validate(); err == nil {
			t.Fatal("expected error for blank permission item, got nil")
		}
	})
}

func TestAdminUpdateRoleRequest_Validate(t *testing.T) {
	t.Run("valid payload succeeds", func(t *testing.T) {
		req := AdminUpdateRoleRequest{
			Name:        "Senior Support",
			Description: "Updated description",
			Permissions: []string{"users:read", "users:write"},
		}
		if err := req.Validate(); err != nil {
			t.Fatalf("expected valid, got error: %v", err)
		}
	})

	t.Run("empty name returns error", func(t *testing.T) {
		req := AdminUpdateRoleRequest{
			Name:        "",
			Permissions: []string{"users:read"},
		}
		if err := req.Validate(); err == nil {
			t.Fatal("expected error for empty name, got nil")
		}
	})

	t.Run("empty permissions returns error", func(t *testing.T) {
		req := AdminUpdateRoleRequest{
			Name:        "Support",
			Permissions: []string{},
		}
		if err := req.Validate(); err == nil {
			t.Fatal("expected error for empty permissions, got nil")
		}
	})
}

func TestAdminTemplateListResponse(t *testing.T) {
	id := uuid.New()
	now := time.Now()
	resp := AdminTemplateListResponse{
		Items: []AdminTemplateItem{
			{
				ID:          id,
				CategoryKey: "MOM",
				Name:        "Minutes of Meeting",
				Description: "Standard meeting minutes",
				Prompt:      "Analyze meeting notes...",
				OutputSchema: map[string]interface{}{
					"type": "object",
				},
				Version:   1,
				IsActive:  true,
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
	}

	if len(resp.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(resp.Items))
	}
	if resp.Items[0].CategoryKey != "MOM" {
		t.Errorf("expected CategoryKey MOM, got %s", resp.Items[0].CategoryKey)
	}
	if !resp.Items[0].IsActive {
		t.Error("expected IsActive true")
	}
}

func TestAdminCreateTemplateRequest_Validate(t *testing.T) {
	t.Run("valid payload succeeds", func(t *testing.T) {
		req := AdminCreateTemplateRequest{
			CategoryKey: "TUTORIAL",
			Name:        "Tutorial Video",
			Description: "Educational tutorial",
			Prompt:      "Extract tutorial steps...",
			OutputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"steps": map[string]interface{}{"type": "array"},
				},
			},
		}
		if err := req.Validate(); err != nil {
			t.Fatalf("expected valid payload, got error: %v", err)
		}
	})

	t.Run("empty category key returns error", func(t *testing.T) {
		req := AdminCreateTemplateRequest{
			CategoryKey: "",
			Name:        "Name",
			Prompt:      "Prompt",
			OutputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"title": map[string]interface{}{"type": "string"},
				},
			},
		}
		if err := req.Validate(); err == nil {
			t.Fatal("expected error for empty category key, got nil")
		}
	})

	t.Run("empty prompt returns error", func(t *testing.T) {
		req := AdminCreateTemplateRequest{
			CategoryKey: "TEST",
			Name:        "Name",
			Prompt:      "   ",
			OutputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"title": map[string]interface{}{"type": "string"},
				},
			},
		}
		if err := req.Validate(); err == nil {
			t.Fatal("expected error for empty prompt, got nil")
		}
	})

	t.Run("non-object schema type returns error", func(t *testing.T) {
		req := AdminCreateTemplateRequest{
			CategoryKey: "TEST",
			Name:        "Name",
			Prompt:      "Prompt",
			OutputSchema: map[string]interface{}{
				"type": "array",
			},
		}
		if err := req.Validate(); err == nil {
			t.Fatal("expected error for non-object schema type, got nil")
		}
	})

	t.Run("empty properties returns error", func(t *testing.T) {
		req := AdminCreateTemplateRequest{
			CategoryKey: "TEST",
			Name:        "Name",
			Prompt:      "Prompt",
			OutputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		}
		if err := req.Validate(); err == nil {
			t.Fatal("expected error for empty schema properties, got nil")
		}
	})
}

func TestAdminUpdateTemplateRequest_Validate(t *testing.T) {
	t.Run("valid payload succeeds", func(t *testing.T) {
		req := AdminUpdateTemplateRequest{
			Name:        "Updated Tutorial",
			Description: "Updated description",
			Prompt:      "Updated prompt...",
			OutputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"title": map[string]interface{}{"type": "string"},
				},
			},
		}
		if err := req.Validate(); err != nil {
			t.Fatalf("expected valid payload, got error: %v", err)
		}
	})

	t.Run("empty name returns error", func(t *testing.T) {
		req := AdminUpdateTemplateRequest{
			Name:   "",
			Prompt: "Prompt",
			OutputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"title": map[string]interface{}{"type": "string"},
				},
			},
		}
		if err := req.Validate(); err == nil {
			t.Fatal("expected error for empty name, got nil")
		}
	})

	t.Run("empty prompt returns error", func(t *testing.T) {
		req := AdminUpdateTemplateRequest{
			Name:   "Name",
			Prompt: "",
			OutputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"title": map[string]interface{}{"type": "string"},
				},
			},
		}
		if err := req.Validate(); err == nil {
			t.Fatal("expected error for empty prompt, got nil")
		}
	})

	t.Run("non-object schema returns error", func(t *testing.T) {
		req := AdminUpdateTemplateRequest{
			Name:   "Name",
			Prompt: "Prompt",
			OutputSchema: map[string]interface{}{
				"type": "string",
			},
		}
		if err := req.Validate(); err == nil {
			t.Fatal("expected error for non-object schema, got nil")
		}
	})
}

func TestAdminTestTemplateRequest_Validate(t *testing.T) {
	t.Run("valid payload succeeds", func(t *testing.T) {
		req := AdminTestTemplateRequest{
			Prompt:           "Extract action items",
			SampleTranscript: "Speaker A: Let's do this tomorrow.",
			OutputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"action_items": map[string]interface{}{"type": "array"},
				},
			},
		}
		if err := req.Validate(); err != nil {
			t.Fatalf("expected valid payload, got error: %v", err)
		}
	})

	t.Run("empty prompt returns error", func(t *testing.T) {
		req := AdminTestTemplateRequest{
			Prompt:           "",
			SampleTranscript: "Sample transcript",
			OutputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"summary": map[string]interface{}{"type": "string"},
				},
			},
		}
		if err := req.Validate(); err == nil {
			t.Fatal("expected error for empty prompt, got nil")
		}
	})

	t.Run("empty sample transcript returns error", func(t *testing.T) {
		req := AdminTestTemplateRequest{
			Prompt:           "Prompt",
			SampleTranscript: "",
			OutputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"summary": map[string]interface{}{"type": "string"},
				},
			},
		}
		if err := req.Validate(); err == nil {
			t.Fatal("expected error for empty sample transcript, got nil")
		}
	})

	t.Run("invalid schema returns error", func(t *testing.T) {
		req := AdminTestTemplateRequest{
			Prompt:           "Prompt",
			SampleTranscript: "Sample",
			OutputSchema: map[string]interface{}{
				"type": "array",
			},
		}
		if err := req.Validate(); err == nil {
			t.Fatal("expected error for non-object schema, got nil")
		}
	})
}

func TestDLQMessagesResponse(t *testing.T) {
	recID := uuid.New()
	now := time.Now()
	resp := DLQMessagesResponse{
		Total:       2,
		FailedCount: 1,
		StuckCount:  1,
		Page:        1,
		Limit:       20,
		Items: []DLQMessageItem{
			{
				RecordingID:  recID,
				Queue:        "recording.dlq",
				RoutingKey:   "recording.failed",
				Stage:        "FAILED",
				Status:       "FAILED",
				ErrorCode:    "ERR_TRANSCRIPTION_FAILED",
				ErrorMessage: "stt timeout",
				RetryCount:   0,
				FailedAt:     now,
				Title:        "Failed Audio",
				UserName:     "John Doe",
				UserEmail:    "john@example.com",
			},
		},
	}

	if resp.Total != 2 || len(resp.Items) != 1 {
		t.Fatalf("unexpected DLQ response structure: %+v", resp)
	}
	if resp.Items[0].RecordingID != recID {
		t.Fatalf("expected recording ID %s, got %s", recID, resp.Items[0].RecordingID)
	}
}

func TestDLQRetryRequest_Validate(t *testing.T) {
	t.Run("valid request succeeds", func(t *testing.T) {
		req := DLQRetryRequest{
			RecordingID: uuid.New(),
			Stage:       "TRANSCRIBING",
		}
		if err := req.Validate(); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("nil recording ID returns error", func(t *testing.T) {
		req := DLQRetryRequest{
			RecordingID: uuid.Nil,
		}
		if err := req.Validate(); err == nil {
			t.Fatal("expected error for nil recording ID, got nil")
		}
	})
}







