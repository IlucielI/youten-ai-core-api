package dtos

import (
	"time"

	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/google/uuid"
)

// AdminLoginRequest defines the payload required for staff administrator login.
type AdminLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Validate validates the AdminLoginRequest fields.
func (r AdminLoginRequest) Validate() error {
	return validation.ValidateStruct(&r,
		validation.Field(&r.Username, validation.Required, validation.Length(3, 100)),
		validation.Field(&r.Password, validation.Required, validation.Length(6, 100)),
	)
}

// AdminUserDTO represents staff user details included in authentication responses.
type AdminUserDTO struct {
	ID          uuid.UUID `json:"id"`
	Username    string    `json:"username"`
	FullName    string    `json:"full_name"`
	RoleID      uuid.UUID `json:"role_id"`
	RoleName    string    `json:"role_name"`
	Permissions []string  `json:"permissions"`
}

// AdminLoginResponse defines the response envelope payload for successful admin login.
type AdminLoginResponse struct {
	Token     string       `json:"token"`
	ExpiresAt time.Time    `json:"expires_at"`
	Admin     AdminUserDTO `json:"admin"`
}
