package validations

import (
	"strings"

	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/go-ozzo/ozzo-validation/v4/is"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
)

// ValidateInviteMemberRequest validates the member invitation payload.
func ValidateInviteMemberRequest(r *dtos.InviteMemberRequest) error {
	r.Email = strings.TrimSpace(strings.ToLower(r.Email))
	r.Role = strings.TrimSpace(strings.ToLower(r.Role))
	if r.Role == "" {
		r.Role = string(constants.WorkspaceRoleMember)
	}

	return validation.ValidateStruct(r,
		validation.Field(&r.Email, validation.Required, is.Email, validation.Length(1, 255)),
		validation.Field(&r.Role, validation.Required, validation.In(
			string(constants.WorkspaceRoleAdmin),
			string(constants.WorkspaceRoleMember),
			string(constants.WorkspaceRoleViewer),
		)),
	)
}
