package dtos

import (
	"time"

	"github.com/google/uuid"
)

// AdminLoginRequest defines the payload required for staff administrator login.
type AdminLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
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
