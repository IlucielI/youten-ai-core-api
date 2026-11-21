package models

import (
	"github.com/google/uuid"
)

// Folder represents a hierarchical organizational unit within a Workspace.
type Folder struct {
	BaseModel
	WorkspaceID uuid.UUID   `gorm:"type:uuid;not null;index:idx_folders_workspace_id" json:"workspace_id"`
	Workspace   *Workspace  `gorm:"foreignKey:WorkspaceID;constraint:OnDelete:CASCADE" json:"workspace,omitempty"`
	ParentID    *uuid.UUID  `gorm:"type:uuid;index:idx_folders_parent_id" json:"parent_id,omitempty"`
	Parent      *Folder     `gorm:"foreignKey:ParentID;constraint:OnDelete:CASCADE" json:"parent,omitempty"`
	Children    []Folder    `gorm:"foreignKey:ParentID" json:"children,omitempty"`
	Name        string      `gorm:"type:varchar(255);not null" json:"name"`
	Color       string      `gorm:"type:varchar(50);not null;default:'#4F46E5'" json:"color"`
	Recordings  []Recording `gorm:"foreignKey:FolderID" json:"recordings,omitempty"`
}

// TableName returns the table name for the Folder model.
func (Folder) TableName() string {
	return "folders"
}

