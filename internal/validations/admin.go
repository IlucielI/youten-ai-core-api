package validations

import (
	"errors"
	"strings"

	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/go-ozzo/ozzo-validation/v4/is"
	"github.com/google/uuid"

	"code-base-golang/internal/dtos"
)

var notEmptyUUID = validation.By(func(value interface{}) error {
	id, ok := value.(uuid.UUID)
	if !ok || id == uuid.Nil {
		return errors.New("cannot be blank")
	}
	return nil
})

// ValidateAdminUserQuotaOverrideRequest normalizes and validates quota override parameters.
func ValidateAdminUserQuotaOverrideRequest(r *dtos.AdminUserQuotaOverrideRequest) error {
	return validation.ValidateStruct(r,
		validation.Field(&r.DailyQuotaOverride, validation.Min(0), validation.Max(10000)),
	)
}

// ValidateAdminCreateRoleRequest normalizes and validates administrative role creation payloads.
func ValidateAdminCreateRoleRequest(r *dtos.AdminCreateRoleRequest) error {
	r.Name = strings.TrimSpace(r.Name)
	r.Description = strings.TrimSpace(r.Description)
	for i, p := range r.Permissions {
		r.Permissions[i] = strings.TrimSpace(p)
	}

	return validation.ValidateStruct(r,
		validation.Field(&r.Name, validation.Required, validation.Length(1, 100)),
		validation.Field(&r.Description, validation.Length(0, 500)),
		validation.Field(&r.Permissions, validation.Required, validation.Each(validation.Required)),
	)
}

// ValidateAdminUpdateRoleRequest normalizes and validates administrative role update payloads.
func ValidateAdminUpdateRoleRequest(r *dtos.AdminUpdateRoleRequest) error {
	r.Name = strings.TrimSpace(r.Name)
	r.Description = strings.TrimSpace(r.Description)
	for i, p := range r.Permissions {
		r.Permissions[i] = strings.TrimSpace(p)
	}

	return validation.ValidateStruct(r,
		validation.Field(&r.Name, validation.Required, validation.Length(1, 100)),
		validation.Field(&r.Description, validation.Length(0, 500)),
		validation.Field(&r.Permissions, validation.Required, validation.Each(validation.Required)),
	)
}

// ValidateAdminCreateUserRoleRequest normalizes and validates customer role creation payloads.
func ValidateAdminCreateUserRoleRequest(r *dtos.AdminCreateUserRoleRequest) error {
	r.Name = strings.TrimSpace(r.Name)
	r.Code = strings.ToUpper(strings.TrimSpace(r.Code))
	r.Description = strings.TrimSpace(r.Description)
	for i, p := range r.Permissions {
		r.Permissions[i] = strings.TrimSpace(p)
	}

	return validation.ValidateStruct(r,
		validation.Field(&r.Name, validation.Required, validation.Length(1, 100)),
		validation.Field(&r.Code, validation.Required, validation.Length(1, 50)),
		validation.Field(&r.Description, validation.Length(0, 500)),
		validation.Field(&r.DailyQuota, validation.Required, validation.Min(1), validation.Max(1000)),
		validation.Field(&r.Permissions, validation.Required, validation.Each(validation.Required)),
	)
}

// ValidateAdminUpdateUserRoleRequest normalizes and validates customer role update payloads.
func ValidateAdminUpdateUserRoleRequest(r *dtos.AdminUpdateUserRoleRequest) error {
	if r.Name != nil {
		*r.Name = strings.TrimSpace(*r.Name)
	}
	if r.Description != nil {
		*r.Description = strings.TrimSpace(*r.Description)
	}
	if r.Permissions != nil {
		for i, p := range *r.Permissions {
			(*r.Permissions)[i] = strings.TrimSpace(p)
		}
	}

	rules := []*validation.FieldRules{
		validation.Field(&r.Name, validation.NilOrNotEmpty, validation.Length(1, 100)),
		validation.Field(&r.Description, validation.Length(0, 500)),
		validation.Field(&r.DailyQuota, validation.Min(1), validation.Max(1000)),
	}
	if r.Permissions != nil {
		rules = append(rules, validation.Field(r.Permissions, validation.Each(validation.Required)))
	}

	return validation.ValidateStruct(r, rules...)
}

// ValidateAdminAssignUserRoleRequest validates role assignment payloads.
func ValidateAdminAssignUserRoleRequest(r *dtos.AdminAssignUserRoleRequest) error {
	return validation.ValidateStruct(r,
		validation.Field(&r.RoleID, notEmptyUUID),
	)
}

// ValidateAdminCreateTemplateRequest normalizes and validates prompt template creation payloads.
func ValidateAdminCreateTemplateRequest(r *dtos.AdminCreateTemplateRequest) error {
	r.CategoryKey = strings.TrimSpace(r.CategoryKey)
	r.Name = strings.TrimSpace(r.Name)
	r.Description = strings.TrimSpace(r.Description)
	r.Prompt = strings.TrimSpace(r.Prompt)

	return validation.ValidateStruct(r,
		validation.Field(&r.CategoryKey, validation.Required, validation.Length(1, 100)),
		validation.Field(&r.Name, validation.Required, validation.Length(1, 255)),
		validation.Field(&r.Description, validation.Length(0, 500)),
		validation.Field(&r.Prompt, validation.Required),
		validation.Field(&r.OutputSchema, validation.Required, validation.By(validateTemplateOutputSchema)),
	)
}

// ValidateAdminUpdateTemplateRequest normalizes and validates prompt template update payloads.
func ValidateAdminUpdateTemplateRequest(r *dtos.AdminUpdateTemplateRequest) error {
	r.Name = strings.TrimSpace(r.Name)
	r.Description = strings.TrimSpace(r.Description)
	r.Prompt = strings.TrimSpace(r.Prompt)

	return validation.ValidateStruct(r,
		validation.Field(&r.Name, validation.Required, validation.Length(1, 255)),
		validation.Field(&r.Description, validation.Length(0, 500)),
		validation.Field(&r.Prompt, validation.Required),
		validation.Field(&r.OutputSchema, validation.Required, validation.By(validateTemplateOutputSchema)),
	)
}

// ValidateAdminTestTemplateRequest normalizes and validates prompt template test payloads.
func ValidateAdminTestTemplateRequest(r *dtos.AdminTestTemplateRequest) error {
	r.Prompt = strings.TrimSpace(r.Prompt)
	r.SampleTranscript = strings.TrimSpace(r.SampleTranscript)

	return validation.ValidateStruct(r,
		validation.Field(&r.Prompt, validation.Required, validation.Length(1, 20000)),
		validation.Field(&r.SampleTranscript, validation.Required, validation.Length(1, 100000)),
		validation.Field(&r.OutputSchema, validation.Required, validation.By(validateTemplateOutputSchema)),
	)
}

func validateTemplateOutputSchema(value interface{}) error {
	schema, ok := value.(map[string]interface{})
	if !ok || len(schema) == 0 {
		return errors.New("is required")
	}
	schemaType, ok := schema["type"].(string)
	if !ok || schemaType != "object" {
		return errors.New("must have type 'object'")
	}
	props, ok := schema["properties"].(map[string]interface{})
	if !ok || len(props) == 0 {
		return errors.New("must define non-empty 'properties'")
	}
	return nil
}

// ValidateDLQRetryRequest validates dead-letter message retry payloads.
func ValidateDLQRetryRequest(r *dtos.DLQRetryRequest) error {
	return validation.ValidateStruct(r,
		validation.Field(&r.RecordingID, notEmptyUUID),
	)
}

// ValidateUpdateSystemConfigRequest validates system configuration update payloads.
func ValidateUpdateSystemConfigRequest(r *dtos.UpdateSystemConfigRequest) error {
	return validation.Validate(r, validation.By(func(value interface{}) error {
		if r.MaintenanceMode == nil && r.AllowGuestUploads == nil && r.BotWaitlistEnabled == nil {
			return errors.New("at least one configuration flag must be provided")
		}
		return nil
	}))
}

// ValidateResolveReportRequest normalizes and validates report resolution payloads.
func ValidateResolveReportRequest(r *dtos.ResolveReportRequest) error {
	r.Action = strings.ToUpper(strings.TrimSpace(r.Action))
	r.ResolutionNote = strings.TrimSpace(r.ResolutionNote)

	return validation.ValidateStruct(r,
		validation.Field(&r.Action, validation.Required, validation.In("DISMISS", "SUSPEND_RECORDING", "BAN_USER")),
		validation.Field(&r.ResolutionNote, validation.Length(0, 1000)),
	)
}

// ValidateAdminLoginRequest validates administrator login credentials.
func ValidateAdminLoginRequest(r *dtos.AdminLoginRequest) error {
	return validation.ValidateStruct(r,
		validation.Field(&r.Username, validation.Required, validation.Length(3, 100)),
		validation.Field(&r.Password, validation.Required, validation.Length(6, 100)),
	)
}

// ValidateAdminJobListQuery validates administrative job listing pagination queries.
func ValidateAdminJobListQuery(q *dtos.AdminJobListQuery) error {
	return validation.ValidateStruct(q,
		validation.Field(&q.Page, validation.Min(1)),
		validation.Field(&q.Limit, validation.Min(1), validation.Max(100)),
	)
}

// ValidateAdminRoleListQuery validates administrative role listing pagination queries.
func ValidateAdminRoleListQuery(q *dtos.AdminRoleListQuery) error {
	return validation.ValidateStruct(q,
		validation.Field(&q.Page, validation.Min(1)),
		validation.Field(&q.Limit, validation.Min(1), validation.Max(100)),
		validation.Field(&q.Search, validation.Length(0, 100)),
	)
}

// ValidateAdminUserRoleListQuery validates customer user role listing pagination queries.
func ValidateAdminUserRoleListQuery(q *dtos.AdminUserRoleListQuery) error {
	return validation.ValidateStruct(q,
		validation.Field(&q.Page, validation.Min(1)),
		validation.Field(&q.Limit, validation.Min(1), validation.Max(100)),
		validation.Field(&q.Search, validation.Length(0, 100)),
	)
}

// ValidateAdminTemplateListQuery validates prompt template listing pagination queries.
func ValidateAdminTemplateListQuery(q *dtos.AdminTemplateListQuery) error {
	return validation.ValidateStruct(q,
		validation.Field(&q.Page, validation.Min(1)),
		validation.Field(&q.Limit, validation.Min(1), validation.Max(100)),
		validation.Field(&q.Search, validation.Length(0, 100)),
	)
}

// ValidateAdminForgotPasswordRequest validates a password reset request payload for admin.
func ValidateAdminForgotPasswordRequest(r *dtos.AdminForgotPasswordRequest) error {
	r.Email = strings.TrimSpace(strings.ToLower(r.Email))
	return validation.ValidateStruct(r,
		validation.Field(&r.Email,
			validation.Required.Error("email is required"),
			is.EmailFormat.Error("invalid email format"),
		),
	)
}

// ValidateAdminResetPasswordRequest validates an admin password reset confirmation payload.
func ValidateAdminResetPasswordRequest(r *dtos.AdminResetPasswordRequest) error {
	r.Token = strings.TrimSpace(r.Token)
	return validation.ValidateStruct(r,
		validation.Field(&r.Token,
			validation.Required.Error("token is required"),
		),
		validation.Field(&r.NewPassword,
			validation.Required.Error("new_password is required"),
			validation.Length(8, 72).Error("new_password must be between 8 and 72 characters"),
			validation.Match(digitRegex).Error("new_password must contain at least one digit"),
		),
		validation.Field(&r.ConfirmPassword, validation.By(func(value interface{}) error {
			if r.ConfirmPassword != "" && r.NewPassword != r.ConfirmPassword {
				return errors.New("confirm_password does not match new_password")
			}
			return nil
		})),
	)
}

// ValidateAdminDLQListQuery validates DLQ monitoring pagination queries.
func ValidateAdminDLQListQuery(q *dtos.AdminDLQListQuery) error {
	return validation.ValidateStruct(q,
		validation.Field(&q.Page, validation.Min(1)),
		validation.Field(&q.Limit, validation.Min(1), validation.Max(100)),
	)
}
