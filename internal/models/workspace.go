package models

import (
	"code-base-golang/internal/constants"

	"github.com/google/uuid"
)

// Workspace represents an individual personal workspace or a collaborative team workspace.
type Workspace struct {
	BaseModel
	Name        string                  `gorm:"type:varchar(150);not null" json:"name"`
	Slug        string                  `gorm:"type:varchar(150);not null;uniqueIndex:uidx_workspaces_slug,where:deleted_at IS NULL" json:"slug"`
	Type        constants.WorkspaceType `gorm:"type:varchar(30);not null;default:'team'" json:"type"`
	OwnerUserID uuid.UUID               `gorm:"type:uuid;not null;index:idx_workspaces_owner_user_id" json:"owner_user_id"`
	Owner       *User                   `gorm:"foreignKey:OwnerUserID" json:"owner,omitempty"`
	LogoURL     *string                 `gorm:"type:text" json:"logo_url,omitempty"`
	Members     []WorkspaceMember       `gorm:"foreignKey:WorkspaceID" json:"members,omitempty"`
	Recordings  []Recording             `gorm:"foreignKey:WorkspaceID" json:"recordings,omitempty"`
}

// TableName returns the table name for GORM.
func (Workspace) TableName() string {
	return "workspaces"
}

