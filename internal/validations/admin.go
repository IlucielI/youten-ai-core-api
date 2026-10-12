package validations

import (
	"errors"
	"strings"

	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/google/uuid"

	"code-base-golang/internal/dtos"
)

// ValidateAdminUserQuotaOverrideRequest normalizes and validates quota override parameters.
func ValidateAdminUserQuotaOverrideRequest(r *dtos.AdminUserQuotaOverrideRequest) error {
	if r == nil {
		return errors.New("request is required")
	}
	if r.DailyQuotaOverride != nil {
		if *r.DailyQuotaOverride < 0 {
			return errors.New("daily_quota_override must be greater than or equal to 0")
		}
		if *r.DailyQuotaOverride > 10000 {
			return errors.New("daily_quota_override cannot exceed 10000")
		}
	}
	return nil
}

// ValidateAdminCreateRoleRequest normalizes and validates administrative role creation payloads.
func ValidateAdminCreateRoleRequest(r *dtos.AdminCreateRoleRequest) error {
	if r == nil {
		return errors.New("request is required")
	}
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		return errors.New("role name is required")
	}
	if len(r.Name) > 100 {
		return errors.New("role name cannot exceed 100 characters")
	}
	r.Description = strings.TrimSpace(r.Description)
	if len(r.Description) > 500 {
		return errors.New("role description cannot exceed 500 characters")
	}
	if len(r.Permissions) == 0 {
		return errors.New("at least one permission is required")
	}
	for i, p := range r.Permissions {
		r.Permissions[i] = strings.TrimSpace(p)
		if r.Permissions[i] == "" {
			return errors.New("empty permission item is not allowed")
		}
	}
	return nil
}

// ValidateAdminUpdateRoleRequest normalizes and validates administrative role update payloads.
func ValidateAdminUpdateRoleRequest(r *dtos.AdminUpdateRoleRequest) error {
	if r == nil {
		return errors.New("request is required")
	}
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		return errors.New("role name is required")
	}
	if len(r.Name) > 100 {
		return errors.New("role name cannot exceed 100 characters")
	}
	r.Description = strings.TrimSpace(r.Description)
	if len(r.Description) > 500 {
		return errors.New("role description cannot exceed 500 characters")
	}
	if len(r.Permissions) == 0 {
		return errors.New("at least one permission is required")
	}
	for i, p := range r.Permissions {
		r.Permissions[i] = strings.TrimSpace(p)
		if r.Permissions[i] == "" {
			return errors.New("empty permission item is not allowed")
		}
	}
	return nil
}

// ValidateAdminCreateUserRoleRequest normalizes and validates customer role creation payloads.
func ValidateAdminCreateUserRoleRequest(r *dtos.AdminCreateUserRoleRequest) error {
	if r == nil {
		return errors.New("request is required")
	}
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		return errors.New("name is required")
	}
	if len(r.Name) > 100 {
		return errors.New("name cannot exceed 100 characters")
	}
	r.Code = strings.ToUpper(strings.TrimSpace(r.Code))
	if r.Code == "" {
		return errors.New("code is required")
	}
	if len(r.Code) > 50 {
		return errors.New("code cannot exceed 50 characters")
	}
	r.Description = strings.TrimSpace(r.Description)
	if len(r.Description) > 500 {
		return errors.New("description cannot exceed 500 characters")
	}
	if r.DailyQuota < 1 || r.DailyQuota > 1000 {
		return errors.New("daily_quota must be between 1 and 1000")
	}
	if len(r.Permissions) == 0 {
		return errors.New("at least one permission is required")
	}
	for i, p := range r.Permissions {
		r.Permissions[i] = strings.TrimSpace(p)
		if r.Permissions[i] == "" {
			return errors.New("empty permission item is not allowed")
		}
	}
	return nil
}

// ValidateAdminUpdateUserRoleRequest normalizes and validates customer role update payloads.
func ValidateAdminUpdateUserRoleRequest(r *dtos.AdminUpdateUserRoleRequest) error {
	if r == nil {
		return errors.New("request is required")
	}
	if r.Name != nil {
		*r.Name = strings.TrimSpace(*r.Name)
		if *r.Name == "" {
			return errors.New("name cannot be empty")
		}
		if len(*r.Name) > 100 {
			return errors.New("name cannot exceed 100 characters")
		}
	}
	if r.Description != nil {
		*r.Description = strings.TrimSpace(*r.Description)
		if len(*r.Description) > 500 {
			return errors.New("description cannot exceed 500 characters")
		}
	}
	if r.DailyQuota != nil {
		if *r.DailyQuota < 1 || *r.DailyQuota > 1000 {
			return errors.New("daily_quota must be between 1 and 1000")
		}
	}
	if r.Permissions != nil {
		for i, p := range *r.Permissions {
			(*r.Permissions)[i] = strings.TrimSpace(p)
			if (*r.Permissions)[i] == "" {
				return errors.New("empty permission item is not allowed")
			}
		}
	}
	return nil
}

// ValidateAdminAssignUserRoleRequest validates role assignment payloads.
func ValidateAdminAssignUserRoleRequest(r *dtos.AdminAssignUserRoleRequest) error {
	if r == nil {
		return errors.New("request is required")
	}
	if r.RoleID == uuid.Nil {
		return errors.New("role_id is required")
	}
	return nil
}

// ValidateAdminCreateTemplateRequest normalizes and validates prompt template creation payloads.
func ValidateAdminCreateTemplateRequest(r *dtos.AdminCreateTemplateRequest) error {
	if r == nil {
		return errors.New("request is required")
	}
	r.CategoryKey = strings.TrimSpace(r.CategoryKey)
	if r.CategoryKey == "" {
		return errors.New("category_key is required")
	}
	if len(r.CategoryKey) > 100 {
		return errors.New("category_key cannot exceed 100 characters")
	}
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		return errors.New("template name is required")
	}
	if len(r.Name) > 255 {
		return errors.New("template name cannot exceed 255 characters")
	}
	r.Description = strings.TrimSpace(r.Description)
	r.Prompt = strings.TrimSpace(r.Prompt)
	if r.Prompt == "" {
		return errors.New("prompt is required")
	}

	if len(r.OutputSchema) == 0 {
		return errors.New("output_schema is required")
	}

	schemaType, ok := r.OutputSchema["type"].(string)
	if !ok || schemaType != "object" {
		return errors.New("output_schema must have type 'object'")
	}
	props, ok := r.OutputSchema["properties"].(map[string]interface{})
	if !ok || len(props) == 0 {
		return errors.New("output_schema must define non-empty 'properties'")
	}

	return nil
}

// ValidateAdminUpdateTemplateRequest normalizes and validates prompt template update payloads.
func ValidateAdminUpdateTemplateRequest(r *dtos.AdminUpdateTemplateRequest) error {
	if r == nil {
		return errors.New("request is required")
	}
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		return errors.New("template name is required")
	}
	if len(r.Name) > 255 {
		return errors.New("template name cannot exceed 255 characters")
	}
	r.Description = strings.TrimSpace(r.Description)
	r.Prompt = strings.TrimSpace(r.Prompt)
	if r.Prompt == "" {
		return errors.New("prompt is required")
	}

	if len(r.OutputSchema) == 0 {
		return errors.New("output_schema is required")
	}

	schemaType, ok := r.OutputSchema["type"].(string)
	if !ok || schemaType != "object" {
		return errors.New("output_schema must have type 'object'")
	}
	props, ok := r.OutputSchema["properties"].(map[string]interface{})
	if !ok || len(props) == 0 {
		return errors.New("output_schema must define non-empty 'properties'")
	}

	return nil
}

// ValidateAdminTestTemplateRequest normalizes and validates prompt template test payloads.
func ValidateAdminTestTemplateRequest(r *dtos.AdminTestTemplateRequest) error {
	if r == nil {
		return errors.New("request is required")
	}
	r.Prompt = strings.TrimSpace(r.Prompt)
	if r.Prompt == "" {
		return errors.New("prompt is required")
	}
	if len(r.Prompt) > 20000 {
		return errors.New("prompt exceeds maximum length of 20000 characters")
	}
	r.SampleTranscript = strings.TrimSpace(r.SampleTranscript)
	if r.SampleTranscript == "" {
		return errors.New("sample_transcript is required")
	}
	if len(r.SampleTranscript) > 100000 {
		return errors.New("sample_transcript exceeds maximum length of 100000 characters")
	}
	if len(r.OutputSchema) == 0 {
		return errors.New("output_schema is required")
	}

	schemaType, ok := r.OutputSchema["type"].(string)
	if !ok || schemaType != "object" {
		return errors.New("output_schema must have type 'object'")
	}
	props, ok := r.OutputSchema["properties"].(map[string]interface{})
	if !ok || len(props) == 0 {
		return errors.New("output_schema must define non-empty 'properties'")
	}

	return nil
}

// ValidateDLQRetryRequest validates dead-letter message retry payloads.
func ValidateDLQRetryRequest(r *dtos.DLQRetryRequest) error {
	if r == nil {
		return errors.New("request is required")
	}
	if r.RecordingID == uuid.Nil {
		return errors.New("recording_id is required")
	}
	return nil
}

// ValidateUpdateSystemConfigRequest validates system configuration update payloads.
func ValidateUpdateSystemConfigRequest(r *dtos.UpdateSystemConfigRequest) error {
	if r == nil {
		return errors.New("request is required")
	}
	if r.MaintenanceMode == nil && r.AllowGuestUploads == nil && r.BotWaitlistEnabled == nil {
		return errors.New("at least one configuration flag must be provided")
	}
	return nil
}

// ValidateResolveReportRequest normalizes and validates report resolution payloads.
func ValidateResolveReportRequest(r *dtos.ResolveReportRequest) error {
	if r == nil {
		return errors.New("request is required")
	}
	r.Action = strings.ToUpper(strings.TrimSpace(r.Action))
	switch r.Action {
	case "DISMISS", "SUSPEND_RECORDING", "BAN_USER":
		// valid
	default:
		return errors.New("action must be DISMISS, SUSPEND_RECORDING, or BAN_USER")
	}
	r.ResolutionNote = strings.TrimSpace(r.ResolutionNote)
	return nil
}

// ValidateAdminLoginRequest validates administrator login credentials.
func ValidateAdminLoginRequest(r *dtos.AdminLoginRequest) error {
	if r == nil {
		return errors.New("request is required")
	}
	return validation.ValidateStruct(r,
		validation.Field(&r.Username, validation.Required, validation.Length(3, 100)),
		validation.Field(&r.Password, validation.Required, validation.Length(6, 100)),
	)
}

// ValidateAdminJobListQuery validates administrative job listing pagination queries.
func ValidateAdminJobListQuery(q *dtos.AdminJobListQuery) error {
	if q == nil {
		return errors.New("query is required")
	}
	return validation.ValidateStruct(q,
		validation.Field(&q.Page, validation.Min(1)),
		validation.Field(&q.Limit, validation.Min(1), validation.Max(100)),
	)
}
