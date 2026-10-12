package validations

import (
	"testing"

	"github.com/google/uuid"

	"code-base-golang/internal/dtos"
)

func TestValidateAdminUserQuotaOverrideRequest(t *testing.T) {
	t.Run("nil override is valid (reset)", func(t *testing.T) {
		req := dtos.AdminUserQuotaOverrideRequest{DailyQuotaOverride: nil}
		if err := ValidateAdminUserQuotaOverrideRequest(&req); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("positive override is valid", func(t *testing.T) {
		v := 25
		req := dtos.AdminUserQuotaOverrideRequest{DailyQuotaOverride: &v}
		if err := ValidateAdminUserQuotaOverrideRequest(&req); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("negative override returns error", func(t *testing.T) {
		v := -1
		req := dtos.AdminUserQuotaOverrideRequest{DailyQuotaOverride: &v}
		if err := ValidateAdminUserQuotaOverrideRequest(&req); err == nil {
			t.Fatal("expected error for negative override, got nil")
		}
	})

	t.Run("exceeding max override returns error", func(t *testing.T) {
		v := 10001
		req := dtos.AdminUserQuotaOverrideRequest{DailyQuotaOverride: &v}
		if err := ValidateAdminUserQuotaOverrideRequest(&req); err == nil {
			t.Fatal("expected error for override exceeding 10000, got nil")
		}
	})

	t.Run("nil request returns error", func(t *testing.T) {
		if err := ValidateAdminUserQuotaOverrideRequest(nil); err == nil {
			t.Fatal("expected error for nil request, got nil")
		}
	})
}

func TestValidateAdminCreateRoleRequest(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		req := dtos.AdminCreateRoleRequest{
			Name:        "Billing Admin",
			Description: "Can manage billing",
			Permissions: []string{"billing:read", "billing:write"},
		}
		if err := ValidateAdminCreateRoleRequest(&req); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("empty name returns error", func(t *testing.T) {
		req := dtos.AdminCreateRoleRequest{
			Name:        "   ",
			Permissions: []string{"billing:read"},
		}
		if err := ValidateAdminCreateRoleRequest(&req); err == nil {
			t.Fatal("expected error for empty name, got nil")
		}
	})

	t.Run("empty permissions returns error", func(t *testing.T) {
		req := dtos.AdminCreateRoleRequest{
			Name:        "Editor",
			Permissions: []string{},
		}
		if err := ValidateAdminCreateRoleRequest(&req); err == nil {
			t.Fatal("expected error for empty permissions, got nil")
		}
	})
}

func TestValidateAdminUpdateRoleRequest(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		req := dtos.AdminUpdateRoleRequest{
			Name:        "Updated Role",
			Description: "Updated description",
			Permissions: []string{"admin:read"},
		}
		if err := ValidateAdminUpdateRoleRequest(&req); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("nil request returns error", func(t *testing.T) {
		if err := ValidateAdminUpdateRoleRequest(nil); err == nil {
			t.Fatal("expected error for nil request, got nil")
		}
	})
}

func TestValidateAdminCreateUserRoleRequest(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		req := dtos.AdminCreateUserRoleRequest{
			Name:        "Pro",
			Code:        "pro_tier",
			Description: "Pro tier role",
			Permissions: []string{"ai:summarize"},
			DailyQuota:  20,
		}
		if err := ValidateAdminCreateUserRoleRequest(&req); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if req.Code != "PRO_TIER" {
			t.Errorf("expected code PRO_TIER, got %s", req.Code)
		}
	})

	t.Run("invalid quota returns error", func(t *testing.T) {
		req := dtos.AdminCreateUserRoleRequest{
			Name:        "Pro",
			Code:        "pro",
			Permissions: []string{"ai:summarize"},
			DailyQuota:  0,
		}
		if err := ValidateAdminCreateUserRoleRequest(&req); err == nil {
			t.Fatal("expected error for quota 0, got nil")
		}
	})
}

func TestValidateAdminUpdateUserRoleRequest(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		name := "Enterprise"
		quota := 100
		req := dtos.AdminUpdateUserRoleRequest{
			Name:       &name,
			DailyQuota: &quota,
		}
		if err := ValidateAdminUpdateUserRoleRequest(&req); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})
}

func TestValidateAdminAssignUserRoleRequest(t *testing.T) {
	t.Run("valid role ID", func(t *testing.T) {
		req := dtos.AdminAssignUserRoleRequest{RoleID: uuid.New()}
		if err := ValidateAdminAssignUserRoleRequest(&req); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("nil UUID returns error", func(t *testing.T) {
		req := dtos.AdminAssignUserRoleRequest{RoleID: uuid.Nil}
		if err := ValidateAdminAssignUserRoleRequest(&req); err == nil {
			t.Fatal("expected error for nil UUID, got nil")
		}
	})
}

func TestValidateAdminCreateTemplateRequest(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		req := dtos.AdminCreateTemplateRequest{
			CategoryKey: "meeting",
			Name:        "Executive Summary",
			Description: "Detailed executive notes",
			Prompt:      "Provide a comprehensive executive summary of this transcript.",
			OutputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"summary": map[string]interface{}{"type": "string"},
				},
			},
		}
		if err := ValidateAdminCreateTemplateRequest(&req); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("missing prompt returns error", func(t *testing.T) {
		req := dtos.AdminCreateTemplateRequest{
			CategoryKey: "meeting",
			Name:        "Executive Summary",
			Description: "Detailed executive notes",
			Prompt:      "   ",
		}
		if err := ValidateAdminCreateTemplateRequest(&req); err == nil {
			t.Fatal("expected error for empty prompt, got nil")
		}
	})
}

func TestValidateAdminUpdateTemplateRequest(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		req := dtos.AdminUpdateTemplateRequest{
			Name:   "Updated Template",
			Prompt: "Updated prompt content",
			OutputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"summary": map[string]interface{}{"type": "string"},
				},
			},
		}
		if err := ValidateAdminUpdateTemplateRequest(&req); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})
}

func TestValidateAdminTestTemplateRequest(t *testing.T) {
	t.Run("valid test request", func(t *testing.T) {
		req := dtos.AdminTestTemplateRequest{
			Prompt:           "Generate summary notes",
			SampleTranscript: "Speaker 1: Hello everyone.",
			OutputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"summary": map[string]interface{}{"type": "string"},
				},
			},
		}
		if err := ValidateAdminTestTemplateRequest(&req); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})
}

func TestValidateDLQRetryRequest(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		req := dtos.DLQRetryRequest{RecordingID: uuid.New()}
		if err := ValidateDLQRetryRequest(&req); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("nil uuid returns error", func(t *testing.T) {
		req := dtos.DLQRetryRequest{RecordingID: uuid.Nil}
		if err := ValidateDLQRetryRequest(&req); err == nil {
			t.Fatal("expected error for nil uuid, got nil")
		}
	})
}

func TestValidateUpdateSystemConfigRequest(t *testing.T) {
	t.Run("valid request with maintenance flag", func(t *testing.T) {
		m := true
		req := dtos.UpdateSystemConfigRequest{MaintenanceMode: &m}
		if err := ValidateUpdateSystemConfigRequest(&req); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("empty flags returns error", func(t *testing.T) {
		req := dtos.UpdateSystemConfigRequest{}
		if err := ValidateUpdateSystemConfigRequest(&req); err == nil {
			t.Fatal("expected error for empty flags, got nil")
		}
	})
}

func TestValidateResolveReportRequest(t *testing.T) {
	t.Run("valid action DISMISS", func(t *testing.T) {
		req := dtos.ResolveReportRequest{Action: "DISMISS"}
		if err := ValidateResolveReportRequest(&req); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("valid action lowercase", func(t *testing.T) {
		req := dtos.ResolveReportRequest{Action: "suspend_recording"}
		if err := ValidateResolveReportRequest(&req); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if req.Action != "SUSPEND_RECORDING" {
			t.Errorf("expected SUSPEND_RECORDING, got %s", req.Action)
		}
	})

	t.Run("invalid action returns error", func(t *testing.T) {
		req := dtos.ResolveReportRequest{Action: "INVALID_ACTION"}
		if err := ValidateResolveReportRequest(&req); err == nil {
			t.Fatal("expected error for invalid action, got nil")
		}
	})
}

func TestValidateAdminLoginRequest(t *testing.T) {
	t.Run("valid credentials", func(t *testing.T) {
		req := dtos.AdminLoginRequest{
			Username: "admin",
			Password: "securepassword",
		}
		if err := ValidateAdminLoginRequest(&req); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("short username returns error", func(t *testing.T) {
		req := dtos.AdminLoginRequest{
			Username: "ab",
			Password: "securepassword",
		}
		if err := ValidateAdminLoginRequest(&req); err == nil {
			t.Fatal("expected error for short username, got nil")
		}
	})
}

func TestValidateAdminJobListQuery(t *testing.T) {
	t.Run("valid query", func(t *testing.T) {
		q := dtos.AdminJobListQuery{Page: 1, Limit: 20}
		if err := ValidateAdminJobListQuery(&q); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("invalid page returns error", func(t *testing.T) {
		q := dtos.AdminJobListQuery{Page: 0, Limit: 20}
		if err := ValidateAdminJobListQuery(&q); err == nil {
			t.Fatal("expected error for page 0, got nil")
		}
	})
}

func TestValidateAdminRoleListQuery(t *testing.T) {
	t.Run("nil query returns error", func(t *testing.T) {
		if err := ValidateAdminRoleListQuery(nil); err == nil {
			t.Fatal("expected error for nil query, got nil")
		}
	})

	t.Run("valid query with search", func(t *testing.T) {
		q := dtos.AdminRoleListQuery{
			Search: "Admin",
			Page:   1,
			Limit:  10,
		}
		if err := ValidateAdminRoleListQuery(&q); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("invalid page returns error", func(t *testing.T) {
		q := dtos.AdminRoleListQuery{Page: 0, Limit: 10}
		if err := ValidateAdminRoleListQuery(&q); err == nil {
			t.Fatal("expected error for page 0, got nil")
		}
	})

	t.Run("invalid limit returns error", func(t *testing.T) {
		q := dtos.AdminRoleListQuery{Page: 1, Limit: 101}
		if err := ValidateAdminRoleListQuery(&q); err == nil {
			t.Fatal("expected error for limit > 100, got nil")
		}
	})
}

func TestValidateAdminUserRoleListQuery(t *testing.T) {
	t.Run("nil query returns error", func(t *testing.T) {
		if err := ValidateAdminUserRoleListQuery(nil); err == nil {
			t.Fatal("expected error for nil query, got nil")
		}
	})

	t.Run("valid query with search", func(t *testing.T) {
		q := dtos.AdminUserRoleListQuery{
			Search: "PRO",
			Page:   1,
			Limit:  10,
		}
		if err := ValidateAdminUserRoleListQuery(&q); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("invalid page returns error", func(t *testing.T) {
		q := dtos.AdminUserRoleListQuery{Page: 0, Limit: 10}
		if err := ValidateAdminUserRoleListQuery(&q); err == nil {
			t.Fatal("expected error for page 0, got nil")
		}
	})

	t.Run("invalid limit returns error", func(t *testing.T) {
		q := dtos.AdminUserRoleListQuery{Page: 1, Limit: 101}
		if err := ValidateAdminUserRoleListQuery(&q); err == nil {
			t.Fatal("expected error for limit > 100, got nil")
		}
	})
}

func TestValidateAdminForgotPasswordRequest(t *testing.T) {
	t.Run("nil request returns error", func(t *testing.T) {
		if err := ValidateAdminForgotPasswordRequest(nil); err == nil {
			t.Fatal("expected error for nil request, got nil")
		}
	})

	t.Run("valid email", func(t *testing.T) {
		req := dtos.AdminForgotPasswordRequest{Email: "admin@youten.ai"}
		if err := ValidateAdminForgotPasswordRequest(&req); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("invalid email format returns error", func(t *testing.T) {
		req := dtos.AdminForgotPasswordRequest{Email: "not-an-email"}
		if err := ValidateAdminForgotPasswordRequest(&req); err == nil {
			t.Fatal("expected error for invalid email format, got nil")
		}
	})

	t.Run("empty email returns error", func(t *testing.T) {
		req := dtos.AdminForgotPasswordRequest{Email: ""}
		if err := ValidateAdminForgotPasswordRequest(&req); err == nil {
			t.Fatal("expected error for empty email, got nil")
		}
	})
}

func TestValidateAdminResetPasswordRequest(t *testing.T) {
	t.Run("nil request returns error", func(t *testing.T) {
		if err := ValidateAdminResetPasswordRequest(nil); err == nil {
			t.Fatal("expected error for nil request, got nil")
		}
	})

	t.Run("valid payload without confirm_password", func(t *testing.T) {
		req := dtos.AdminResetPasswordRequest{
			Token:       "valid-token-string",
			NewPassword: "newpassword1",
		}
		if err := ValidateAdminResetPasswordRequest(&req); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("valid payload with matching confirm_password", func(t *testing.T) {
		req := dtos.AdminResetPasswordRequest{
			Token:           "valid-token-string",
			NewPassword:     "newpassword1",
			ConfirmPassword: "newpassword1",
		}
		if err := ValidateAdminResetPasswordRequest(&req); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("mismatched confirm_password returns error", func(t *testing.T) {
		req := dtos.AdminResetPasswordRequest{
			Token:           "valid-token-string",
			NewPassword:     "newpassword1",
			ConfirmPassword: "differentpassword2",
		}
		if err := ValidateAdminResetPasswordRequest(&req); err == nil {
			t.Fatal("expected error for mismatched confirm_password, got nil")
		}
	})

	t.Run("short password returns error", func(t *testing.T) {
		req := dtos.AdminResetPasswordRequest{
			Token:       "valid-token-string",
			NewPassword: "short1",
		}
		if err := ValidateAdminResetPasswordRequest(&req); err == nil {
			t.Fatal("expected error for short password, got nil")
		}
	})

	t.Run("password without digit returns error", func(t *testing.T) {
		req := dtos.AdminResetPasswordRequest{
			Token:       "valid-token-string",
			NewPassword: "passwordwithoutdigit",
		}
		if err := ValidateAdminResetPasswordRequest(&req); err == nil {
			t.Fatal("expected error for password without digit, got nil")
		}
	})

	t.Run("empty token returns error", func(t *testing.T) {
		req := dtos.AdminResetPasswordRequest{
			Token:       "",
			NewPassword: "newpassword1",
		}
		if err := ValidateAdminResetPasswordRequest(&req); err == nil {
			t.Fatal("expected error for empty token, got nil")
		}
	})
}

