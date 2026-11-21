package services

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
	"code-base-golang/internal/pkg/ctxmeta"
)

// CreateFolder creates a new folder in the specified workspace.
func (s *Service) CreateFolder(ctx context.Context, workspaceID uuid.UUID, req dtos.CreateFolderRequest) (*dtos.FolderResponse, error) {
	userID, ok := ctxmeta.GetAuthUserID(ctx)
	if !ok {
		return nil, constants.ErrUnauthorized
	}

	member, err := s.repo.FindWorkspaceMember(ctx, workspaceID, userID)
	if err != nil {
		return nil, constants.ErrWorkspaceAccessDenied
	}
	if !member.Role.CanEditContent() {
		return nil, constants.ErrForbidden
	}

	var parentID *uuid.UUID
	if req.ParentID != nil && *req.ParentID != uuid.Nil {
		parent, err := s.repo.FindFolderByID(ctx, *req.ParentID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, constants.ErrFolderNotFound
			}
			return nil, err
		}
		if parent.WorkspaceID != workspaceID {
			return nil, constants.ErrFolderNotFound
		}
		parentID = req.ParentID
	}

	folder := &models.Folder{
		BaseModel:   models.BaseModel{ID: uuid.New()},
		WorkspaceID: workspaceID,
		ParentID:    parentID,
		Name:        req.Name,
		Color:       req.Color,
	}

	if err := s.repo.CreateFolder(ctx, folder); err != nil {
		return nil, err
	}

	var parentIDStr *string
	if folder.ParentID != nil && *folder.ParentID != uuid.Nil {
		str := folder.ParentID.String()
		parentIDStr = &str
	}

	return &dtos.FolderResponse{
		ID:          folder.ID.String(),
		WorkspaceID: folder.WorkspaceID.String(),
		ParentID:    parentIDStr,
		Name:        folder.Name,
		Color:       folder.Color,
		Children:    []dtos.FolderResponse{},
		CreatedAt:   folder.CreatedAt,
		UpdatedAt:   folder.UpdatedAt,
	}, nil
}

// ListWorkspaceFolders returns all folders of a workspace organized as a hierarchical tree.
func (s *Service) ListWorkspaceFolders(ctx context.Context, workspaceID uuid.UUID) ([]dtos.FolderResponse, error) {
	userID, ok := ctxmeta.GetAuthUserID(ctx)
	if !ok {
		return nil, constants.ErrUnauthorized
	}

	_, err := s.repo.FindWorkspaceMember(ctx, workspaceID, userID)
	if err != nil {
		return nil, constants.ErrWorkspaceAccessDenied
	}

	folders, err := s.repo.FindFoldersByWorkspaceID(ctx, workspaceID)
	if err != nil {
		return nil, err
	}

	return buildFolderTree(folders), nil
}

// UpdateFolder modifies a folder's name, color, or parent (hierarchy move).
func (s *Service) UpdateFolder(ctx context.Context, workspaceID, folderID uuid.UUID, req dtos.UpdateFolderRequest) (*dtos.FolderResponse, error) {
	userID, ok := ctxmeta.GetAuthUserID(ctx)
	if !ok {
		return nil, constants.ErrUnauthorized
	}

	member, err := s.repo.FindWorkspaceMember(ctx, workspaceID, userID)
	if err != nil {
		return nil, constants.ErrWorkspaceAccessDenied
	}
	if !member.Role.CanEditContent() {
		return nil, constants.ErrForbidden
	}

	folder, err := s.repo.FindFolderByID(ctx, folderID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrFolderNotFound
		}
		return nil, err
	}
	if folder.WorkspaceID != workspaceID {
		return nil, constants.ErrFolderNotFound
	}

	if req.ParentID != nil {
		if *req.ParentID == folderID {
			return nil, constants.ErrCircularFolderReference
		}
		if *req.ParentID != uuid.Nil {
			isDesc, err := s.repo.IsDescendantOf(ctx, folderID, *req.ParentID)
			if err != nil {
				return nil, err
			}
			if isDesc {
				return nil, constants.ErrCircularFolderReference
			}

			parent, err := s.repo.FindFolderByID(ctx, *req.ParentID)
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil, constants.ErrFolderNotFound
				}
				return nil, err
			}
			if parent.WorkspaceID != workspaceID {
				return nil, constants.ErrFolderNotFound
			}
			folder.ParentID = req.ParentID
		} else {
			folder.ParentID = nil
		}
	}

	if req.Name != nil && *req.Name != "" {
		folder.Name = *req.Name
	}
	if req.Color != nil && *req.Color != "" {
		folder.Color = *req.Color
	}

	if err := s.repo.UpdateFolder(ctx, folder); err != nil {
		return nil, err
	}

	var parentIDStr *string
	if folder.ParentID != nil && *folder.ParentID != uuid.Nil {
		str := folder.ParentID.String()
		parentIDStr = &str
	}

	return &dtos.FolderResponse{
		ID:          folder.ID.String(),
		WorkspaceID: folder.WorkspaceID.String(),
		ParentID:    parentIDStr,
		Name:        folder.Name,
		Color:       folder.Color,
		Children:    []dtos.FolderResponse{},
		CreatedAt:   folder.CreatedAt,
		UpdatedAt:   folder.UpdatedAt,
	}, nil
}

// DeleteFolder removes a folder from the workspace.
func (s *Service) DeleteFolder(ctx context.Context, workspaceID, folderID uuid.UUID) error {
	userID, ok := ctxmeta.GetAuthUserID(ctx)
	if !ok {
		return constants.ErrUnauthorized
	}

	member, err := s.repo.FindWorkspaceMember(ctx, workspaceID, userID)
	if err != nil {
		return constants.ErrWorkspaceAccessDenied
	}
	if !member.Role.CanEditContent() {
		return constants.ErrForbidden
	}

	folder, err := s.repo.FindFolderByID(ctx, folderID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return constants.ErrFolderNotFound
		}
		return err
	}
	if folder.WorkspaceID != workspaceID {
		return constants.ErrFolderNotFound
	}

	return s.repo.DeleteFolder(ctx, folderID)
}

// MoveRecording reassigns a recording to a different folder within its workspace.
func (s *Service) MoveRecording(ctx context.Context, recordingID uuid.UUID, req dtos.MoveRecordingRequest) error {
	userID, ok := ctxmeta.GetAuthUserID(ctx)
	if !ok {
		return constants.ErrUnauthorized
	}

	rec, err := s.repo.FindRecordingByID(ctx, recordingID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return constants.ErrRecordingNotFound
		}
		return err
	}

	// Validate authorization: user must own recording or have edit rights in workspace
	if rec.UserID != nil && *rec.UserID == userID {
		// Recording owner is authorized.
	} else {
		if rec.WorkspaceID == nil {
			return constants.ErrForbidden
		}
		member, err := s.repo.FindWorkspaceMember(ctx, *rec.WorkspaceID, userID)
		if err != nil || !member.Role.CanEditContent() {
			return constants.ErrForbidden
		}
	}

	// If moving to a target folder, ensure the target folder exists and matches the workspace
	if req.FolderID != nil && *req.FolderID != uuid.Nil {
		folder, err := s.repo.FindFolderByID(ctx, *req.FolderID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return constants.ErrFolderNotFound
			}
			return err
		}
		if rec.WorkspaceID == nil {
			if folder.WorkspaceID != uuid.Nil {
				return constants.ErrForbidden
			}
		} else if folder.WorkspaceID != *rec.WorkspaceID {
			return constants.ErrForbidden
		}
	}

	return s.repo.MoveRecording(ctx, recordingID, req.FolderID)
}

// buildFolderTree transforms a flat list of folders into a nested tree.
func buildFolderTree(folders []models.Folder) []dtos.FolderResponse {
	childrenMap := make(map[uuid.UUID][]models.Folder)
	var roots []models.Folder
	for _, f := range folders {
		if f.ParentID == nil || *f.ParentID == uuid.Nil {
			roots = append(roots, f)
		} else {
			childrenMap[*f.ParentID] = append(childrenMap[*f.ParentID], f)
		}
	}

	var buildSubtree func(parent models.Folder) dtos.FolderResponse
	buildSubtree = func(parent models.Folder) dtos.FolderResponse {
		var parentIDStr *string
		if parent.ParentID != nil && *parent.ParentID != uuid.Nil {
			s := parent.ParentID.String()
			parentIDStr = &s
		}
		resp := dtos.FolderResponse{
			ID:          parent.ID.String(),
			WorkspaceID: parent.WorkspaceID.String(),
			ParentID:    parentIDStr,
			Name:        parent.Name,
			Color:       parent.Color,
			Children:    []dtos.FolderResponse{},
			CreatedAt:   parent.CreatedAt,
			UpdatedAt:   parent.UpdatedAt,
		}
		for _, child := range childrenMap[parent.ID] {
			resp.Children = append(resp.Children, buildSubtree(child))
		}
		return resp
	}

	res := make([]dtos.FolderResponse, 0, len(roots))
	for _, root := range roots {
		res = append(res, buildSubtree(root))
	}
	return res
}
