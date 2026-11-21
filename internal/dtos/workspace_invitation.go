package dtos

import (
	"time"

	"github.com/google/uuid"
)

// InviteMemberRequest defines the payload to invite a new member to a workspace.
type InviteMemberRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

// WorkspaceInvitationResponse represents the public format of an invitation.
type WorkspaceInvitationResponse struct {
	ID              uuid.UUID  `json:"id"`
	WorkspaceID     uuid.UUID  `json:"workspace_id"`
	WorkspaceName   string     `json:"workspace_name,omitempty"`
	Email           string     `json:"email"`
	Role            string     `json:"role"`
	InvitedByUserID uuid.UUID  `json:"invited_by_user_id"`
	InvitedByName   string     `json:"invited_by_name,omitempty"`
	ExpiresAt       time.Time  `json:"expires_at"`
	AcceptedAt      *time.Time `json:"accepted_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

// InvitationDetailResponse represents the public details returned when an invite token is checked.
type InvitationDetailResponse struct {
	Token         string    `json:"token"`
	WorkspaceID   uuid.UUID `json:"workspace_id"`
	WorkspaceName string    `json:"workspace_name"`
	Email         string    `json:"email"`
	Role          string    `json:"role"`
	InvitedByName string    `json:"invited_by_name"`
	ExpiresAt     time.Time `json:"expires_at"`
	IsExpired     bool      `json:"is_expired"`
	IsAccepted    bool      `json:"is_accepted"`
}

// AcceptInvitationResponse represents the result of accepting an invitation.
type AcceptInvitationResponse struct {
	WorkspaceID   uuid.UUID `json:"workspace_id"`
	WorkspaceName string    `json:"workspace_name"`
	Role          string    `json:"role"`
	Message       string    `json:"message"`
}
