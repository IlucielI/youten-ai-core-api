package dtos

import (
	"time"

	"github.com/google/uuid"
)

// CreateFolderRequest represents the payload to create a folder in a workspace.
type CreateFolderRequest struct {
	Name     string     `json:"name"`
	Color    string     `json:"color"`
	ParentID *uuid.UUID `json:"parent_id,omitempty"`
}

// UpdateFolderRequest represents the payload to update folder name, color, or parent folder.
type UpdateFolderRequest struct {
	Name     *string    `json:"name,omitempty"`
	Color    *string    `json:"color,omitempty"`
	ParentID *uuid.UUID `json:"parent_id,omitempty"`
}

// FolderResponse represents the detailed view of a folder, including its nested child folders.
type FolderResponse struct {
	ID          string           `json:"id"`
	WorkspaceID string           `json:"workspace_id"`
	ParentID    *string          `json:"parent_id,omitempty"`
	Name        string           `json:"name"`
	Color       string           `json:"color"`
	Children    []FolderResponse `json:"children,omitempty"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

