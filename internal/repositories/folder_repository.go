package repositories

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/models"
)

// CreateFolder inserts a new folder into the database.
func (r *Repositories) CreateFolder(ctx context.Context, folder *models.Folder) error {
	return r.db.WithContext(ctx).Create(folder).Error
}

// FindFolderByID retrieves a folder by its UUID.
func (r *Repositories) FindFolderByID(ctx context.Context, id uuid.UUID) (*models.Folder, error) {
	var folder models.Folder
	if err := r.db.WithContext(ctx).First(&folder, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &folder, nil
}

// FindFoldersByWorkspaceID retrieves all folders belonging to a workspace ordered by name.
func (r *Repositories) FindFoldersByWorkspaceID(ctx context.Context, workspaceID uuid.UUID) ([]models.Folder, error) {
	var folders []models.Folder
	err := r.db.WithContext(ctx).
		Where("workspace_id = ?", workspaceID).
		Order("name ASC").
		Find(&folders).Error
	if err != nil {
		return nil, err
	}
	return folders, nil
}

// UpdateFolder updates folder fields in the database.
func (r *Repositories) UpdateFolder(ctx context.Context, folder *models.Folder) error {
	return r.db.WithContext(ctx).Save(folder).Error
}

// DeleteFolder soft-deletes or deletes a folder by its ID.
func (r *Repositories) DeleteFolder(ctx context.Context, id uuid.UUID) error {
	res := r.db.WithContext(ctx).Delete(&models.Folder{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// IsDescendantOf checks if candidateParentID is equal to or a descendant of folderID to prevent cyclic trees.
func (r *Repositories) IsDescendantOf(ctx context.Context, folderID, candidateParentID uuid.UUID) (bool, error) {
	if folderID == candidateParentID {
		return true, nil
	}

	visited := make(map[uuid.UUID]struct{})
	curr := candidateParentID
	for {
		if _, exists := visited[curr]; exists {
			return false, errors.New("cycle detected in folder hierarchy")
		}
		visited[curr] = struct{}{}

		var parentFolder models.Folder
		err := r.db.WithContext(ctx).Select("id, parent_id").First(&parentFolder, "id = ?", curr).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return false, nil
			}
			return false, err
		}

		if parentFolder.ParentID == nil {
			return false, nil
		}

		if *parentFolder.ParentID == folderID {
			return true, nil
		}

		curr = *parentFolder.ParentID
	}
}

