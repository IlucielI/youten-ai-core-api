package repositories

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/models"
)

// CreateWorkspace persists a new workspace and initializes the owner membership in a transaction.
func (r *Repositories) CreateWorkspace(ctx context.Context, ws *models.Workspace) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(ws).Error; err != nil {
			return err
		}

		member := models.WorkspaceMember{
			WorkspaceID: ws.ID,
			UserID:      ws.OwnerUserID,
			Role:        constants.WorkspaceRoleOwner,
		}
		return tx.Create(&member).Error
	})
}

// FindWorkspaceByID retrieves a workspace by its unique ID.
func (r *Repositories) FindWorkspaceByID(ctx context.Context, id uuid.UUID) (*models.Workspace, error) {
	var ws models.Workspace
	if err := r.db.WithContext(ctx).First(&ws, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &ws, nil
}

// FindWorkspaceBySlug retrieves a workspace by its unique slug.
func (r *Repositories) FindWorkspaceBySlug(ctx context.Context, slug string) (*models.Workspace, error) {
	var ws models.Workspace
	if err := r.db.WithContext(ctx).First(&ws, "slug = ?", slug).Error; err != nil {
		return nil, err
	}
	return &ws, nil
}

// FindWorkspacesByUserID retrieves all workspaces where the given user is an active member.
func (r *Repositories) FindWorkspacesByUserID(ctx context.Context, userID uuid.UUID) ([]models.Workspace, error) {
	var list []models.Workspace
	err := r.db.WithContext(ctx).
		Table("workspaces").
		Joins("INNER JOIN workspace_members ON workspace_members.workspace_id = workspaces.id").
		Where("workspace_members.user_id = ? AND workspace_members.deleted_at IS NULL AND workspaces.deleted_at IS NULL", userID).
		Order("workspaces.type ASC, workspaces.created_at ASC").
		Find(&list).Error
	return list, err
}

// FindPersonalWorkspaceByUserID retrieves the personal workspace owned by the user.
func (r *Repositories) FindPersonalWorkspaceByUserID(ctx context.Context, userID uuid.UUID) (*models.Workspace, error) {
	var ws models.Workspace
	err := r.db.WithContext(ctx).
		Where("owner_user_id = ? AND type = ?", userID, constants.WorkspaceTypePersonal).
		First(&ws).Error
	if err != nil {
		return nil, err
	}
	return &ws, nil
}

// UpdateWorkspace updates mutable fields of a workspace.
func (r *Repositories) UpdateWorkspace(ctx context.Context, ws *models.Workspace) error {
	updates := map[string]interface{}{
		"name":       ws.Name,
		"logo_url":   ws.LogoURL,
		"updated_at": time.Now(),
	}
	return r.db.WithContext(ctx).Model(&models.Workspace{}).Where("id = ?", ws.ID).Updates(updates).Error
}

// DeleteWorkspace soft-deletes a workspace.
func (r *Repositories) DeleteWorkspace(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&models.Workspace{}, "id = ?", id).Error
}

// AddWorkspaceMember adds a new member to the workspace.
func (r *Repositories) AddWorkspaceMember(ctx context.Context, member *models.WorkspaceMember) error {
	return r.db.WithContext(ctx).Create(member).Error
}

// FindWorkspaceMember finds a member record by workspace and user ID.
func (r *Repositories) FindWorkspaceMember(ctx context.Context, workspaceID, userID uuid.UUID) (*models.WorkspaceMember, error) {
	var member models.WorkspaceMember
	err := r.db.WithContext(ctx).
		First(&member, "workspace_id = ? AND user_id = ?", workspaceID, userID).Error
	if err != nil {
		return nil, err
	}
	return &member, nil
}

// ListWorkspaceMembers lists all members of a workspace with user details preloaded.
func (r *Repositories) ListWorkspaceMembers(ctx context.Context, workspaceID uuid.UUID) ([]models.WorkspaceMember, error) {
	var members []models.WorkspaceMember
	err := r.db.WithContext(ctx).
		Preload("User").
		Where("workspace_id = ?", workspaceID).
		Order("CASE role WHEN 'owner' THEN 1 WHEN 'admin' THEN 2 WHEN 'member' THEN 3 ELSE 4 END, created_at ASC").
		Find(&members).Error
	return members, err
}

// UpdateWorkspaceMemberRole updates the role of a workspace member.
func (r *Repositories) UpdateWorkspaceMemberRole(ctx context.Context, workspaceID, userID uuid.UUID, role constants.WorkspaceRole) error {
	return r.db.WithContext(ctx).
		Model(&models.WorkspaceMember{}).
		Where("workspace_id = ? AND user_id = ?", workspaceID, userID).
		Update("role", role).Error
}

// RemoveWorkspaceMember removes a member from a workspace.
func (r *Repositories) RemoveWorkspaceMember(ctx context.Context, workspaceID, userID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Delete(&models.WorkspaceMember{}, "workspace_id = ? AND user_id = ?", workspaceID, userID).Error
}

// CountWorkspaceMembers returns the total active members count in a workspace.
func (r *Repositories) CountWorkspaceMembers(ctx context.Context, workspaceID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&models.WorkspaceMember{}).
		Where("workspace_id = ?", workspaceID).
		Count(&count).Error
	return count, err
}
