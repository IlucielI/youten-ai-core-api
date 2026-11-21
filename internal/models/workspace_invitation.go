package models

import (
	"time"

	"code-base-golang/internal/constants"

	"github.com/google/uuid"
)

// WorkspaceInvitation represents an email invitation to join a workspace.
type WorkspaceInvitation struct {
	BaseModel
	WorkspaceID     uuid.UUID               `gorm:"type:uuid;not null;index:idx_workspace_invitations_workspace_id" json:"workspace_id"`
	Workspace       *Workspace              `gorm:"foreignKey:WorkspaceID" json:"workspace,omitempty"`
	Email           string                  `gorm:"type:varchar(255);not null" json:"email"`
	InviteToken     string                  `gorm:"type:varchar(64);not null;uniqueIndex:uidx_workspace_invitations_token,where:deleted_at IS NULL" json:"invite_token"`
	Role            constants.WorkspaceRole `gorm:"type:varchar(32);not null;default:'member'" json:"role"`
	InvitedByUserID uuid.UUID               `gorm:"type:uuid;not null" json:"invited_by_user_id"`
	InvitedBy       *User                   `gorm:"foreignKey:InvitedByUserID" json:"invited_by,omitempty"`
	ExpiresAt       time.Time               `gorm:"not null" json:"expires_at"`
	AcceptedAt      *time.Time              `json:"accepted_at,omitempty"`
}

// TableName returns the table name for GORM.
func (WorkspaceInvitation) TableName() string {
	return "workspace_invitations"
}
