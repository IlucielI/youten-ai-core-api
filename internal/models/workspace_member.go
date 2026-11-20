package models

import (
	"code-base-golang/internal/constants"

	"github.com/google/uuid"
)

// WorkspaceMember represents a user's membership and RBAC role in a workspace.
type WorkspaceMember struct {
	BaseModel
	WorkspaceID uuid.UUID               `gorm:"type:uuid;not null;index:idx_workspace_members_workspace_id;uniqueIndex:uidx_workspace_members_ws_user,priority:1,where:deleted_at IS NULL" json:"workspace_id"`
	Workspace   *Workspace              `gorm:"foreignKey:WorkspaceID" json:"workspace,omitempty"`
	UserID      uuid.UUID               `gorm:"type:uuid;not null;index:idx_workspace_members_user_id;uniqueIndex:uidx_workspace_members_ws_user,priority:2,where:deleted_at IS NULL" json:"user_id"`
	User        *User                   `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Role        constants.WorkspaceRole `gorm:"type:varchar(30);not null;default:'member'" json:"role"`
}

// TableName returns the table name for GORM.
func (WorkspaceMember) TableName() string {
	return "workspace_members"
}

