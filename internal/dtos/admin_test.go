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


func TestSystemConfigResponse(t *testing.T) {
	resp := SystemConfigResponse{
		MaintenanceMode:    true,
		AllowGuestUploads:  false,
		BotWaitlistEnabled: true,
	}

	if !resp.MaintenanceMode || resp.AllowGuestUploads || !resp.BotWaitlistEnabled {
		t.Fatalf("unexpected SystemConfigResponse values: %+v", resp)
	}
}

func TestAdminReportListQuery_SetDefaults(t *testing.T) {
	t.Run("defaults invalid values", func(t *testing.T) {
		q := AdminReportListQuery{
			Page:  0,
			Limit: -1,
		}
		q.SetDefaults()

		if q.Page != 1 {
			t.Errorf("expected page 1, got %d", q.Page)
		}
		if q.Limit != 10 {
			t.Errorf("expected limit 10, got %d", q.Limit)
		}
	})

	t.Run("caps limit at 100 and trims status", func(t *testing.T) {
		q := AdminReportListQuery{
			Page:   2,
			Limit:  200,
			Status: "  OPEN  ",
		}
		q.SetDefaults()

		if q.Limit != 100 {
			t.Errorf("expected limit 100, got %d", q.Limit)
		}
		if q.Status != "open" {
			t.Errorf("expected status open, got %q", q.Status)
		}
	})
}

func TestAdminReportListResponse(t *testing.T) {
	id := uuid.New()
	recID := uuid.New()
	handler := "Admin Moderator"
	now := time.Now()

	resp := AdminReportListResponse{
		Items: []AdminReportListItem{
			{
				ID:             id,
				RecordingID:    recID,
				RecordingTitle: "Abusive Title",
				ReporterType:   "guest",
				Reason:         "Spam content",
				Status:         "open",
				HandlerName:    &handler,
				CreatedAt:      now,
				UpdatedAt:      now,
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
	if resp.Items[0].RecordingTitle != "Abusive Title" {
		t.Errorf("expected title 'Abusive Title', got %q", resp.Items[0].RecordingTitle)
	}
}


func TestResolveReportResponse(t *testing.T) {
	repID := uuid.New()
	adminID := uuid.New()
	now := time.Now()

	resp := ResolveReportResponse{
		ReportID:       repID,
		Status:         "resolved",
		Action:         "SUSPEND_RECORDING",
		HandledBy:      adminID,
		ResolutionNote: "Takedown completed",
		ResolvedAt:     now,
	}

	if resp.ReportID != repID || resp.Status != "resolved" || resp.Action != "SUSPEND_RECORDING" {
		t.Fatalf("unexpected ResolveReportResponse values: %+v", resp)
	}
}

func TestAdminAuditLogListQuery_SetDefaults(t *testing.T) {
	t.Run("defaults invalid values", func(t *testing.T) {
		q := AdminAuditLogListQuery{
			Page:  -1,
			Limit: 0,
		}
		q.SetDefaults()

		if q.Page != 1 {
			t.Errorf("expected page 1, got %d", q.Page)
		}
		if q.Limit != 20 {
			t.Errorf("expected limit 20, got %d", q.Limit)
		}
	})

	t.Run("caps limit at 100 and trims query fields", func(t *testing.T) {
		q := AdminAuditLogListQuery{
			Action: "  report.resolve  ",
			Entity: "  report  ",
			Page:   2,
			Limit:  500,
		}
		q.SetDefaults()

		if q.Limit != 100 {
			t.Errorf("expected limit 100, got %d", q.Limit)
		}
		if q.Action != "report.resolve" {
			t.Errorf("expected trimmed action, got %q", q.Action)
		}
		if q.Entity != "report" {
			t.Errorf("expected trimmed entity, got %q", q.Entity)
		}
	})
}

func TestAdminAuditLogListResponse(t *testing.T) {
	id := uuid.New()
	adminID := uuid.New()
	username := "sysadmin"
	fullname := "System Admin"
	now := time.Now()

	resp := AdminAuditLogListResponse{
		Items: []AdminAuditLogListItem{
			{
				ID:            id,
				AdminID:       &adminID,
				AdminUsername: &username,
				AdminFullName: &fullname,
				Action:        "user.quota_override",
				Entity:        "user",
				CreatedAt:     now,
			},
		},
		Pagination: PaginationMeta{
			CurrentPage: 1,
			PageSize:    20,
			TotalItems:  1,
			TotalPages:  1,
		},
	}

	if len(resp.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(resp.Items))
	}
	if resp.Items[0].Action != "user.quota_override" {
		t.Errorf("expected action 'user.quota_override', got %q", resp.Items[0].Action)
	}
}

func TestSystemOverviewStatsResponse(t *testing.T) {
	resp := SystemOverviewStatsResponse{
		TotalUsers:           100,
		ActiveUsers:          90,
		TotalRecordings:      500,
		CompletedRecordings:  480,
		FailedRecordings:     20,
		TotalStorageBytes:    1048576,
		TotalDurationSeconds: 3600.0,
	}

	if resp.TotalUsers != 100 || resp.ActiveUsers != 90 || resp.TotalRecordings != 500 {
		t.Fatalf("unexpected stats response values: %+v", resp)
	}
}

func TestCostOversightResponse(t *testing.T) {
	resp := CostOversightResponse{
		TotalAudioMinutes:     60.0,
		EstimatedSTTCostUSD:   0.36,
		TotalLLMTokens:        18000,
		EstimatedLLMCostUSD:   0.005,
		TotalEstimatedCostUSD: 0.365,
		Currency:              "USD",
	}

	if resp.TotalAudioMinutes != 60.0 || resp.EstimatedSTTCostUSD != 0.36 || resp.Currency != "USD" {
		t.Fatalf("unexpected cost oversight values: %+v", resp)
	}
}












