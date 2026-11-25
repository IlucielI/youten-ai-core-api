package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
	"code-base-golang/internal/pkg/ctxmeta"
	"code-base-golang/internal/pkg/strutil"
)

// CreateWorkspace provisions a new collaborative team workspace with the caller as owner.
func (s *Service) CreateWorkspace(ctx context.Context, req dtos.CreateWorkspaceRequest) (*dtos.WorkspaceResponse, error) {
	userID, ok := ctxmeta.GetAuthUserID(ctx)
	if !ok {
		return nil, constants.ErrUnauthorized
	}

	slugBase := strutil.Slugify(req.Name)
	slug := fmt.Sprintf("%s-%s", slugBase, uuid.New().String()[:8])

	ws := &models.Workspace{
		BaseModel:   models.BaseModel{ID: uuid.New()},
		Name:        req.Name,
		Slug:        slug,
		Type:        constants.WorkspaceTypeTeam,
		OwnerUserID: userID,
		LogoURL:     req.LogoURL,
	}

	if err := s.repo.CreateWorkspace(ctx, ws); err != nil {
		return nil, s.wrapError(ctx, fmt.Errorf("failed to create workspace: %w", err))
	}

	return &dtos.WorkspaceResponse{
		ID:           ws.ID,
		Name:         ws.Name,
		Slug:         ws.Slug,
		Type:         string(ws.Type),
		Role:         string(constants.WorkspaceRoleOwner),
		OwnerUserID:  ws.OwnerUserID,
		LogoURL:      ws.LogoURL,
		MembersCount: 1,
		CreatedAt:    ws.CreatedAt,
		UpdatedAt:    ws.UpdatedAt,
	}, nil
}

// ListUserWorkspaces returns all workspaces the authenticated user belongs to.
func (s *Service) ListUserWorkspaces(ctx context.Context) ([]dtos.WorkspaceResponse, error) {
	userID, ok := ctxmeta.GetAuthUserID(ctx)
	if !ok {
		return nil, constants.ErrUnauthorized
	}

	workspaces, err := s.repo.FindWorkspacesByUserID(ctx, userID)
	if err != nil {
		return nil, s.wrapError(ctx, fmt.Errorf("failed to find workspaces for user: %w", err))
	}

	if len(workspaces) == 0 {
		workspaces = s.autoProvisionPersonalWorkspaceSlice(ctx, userID)
	}

	res := make([]dtos.WorkspaceResponse, 0, len(workspaces))
	for _, ws := range workspaces {
		member, err := s.repo.FindWorkspaceMember(ctx, ws.ID, userID)
		if err != nil {
			continue
		}

		count, err := s.repo.CountWorkspaceMembers(ctx, ws.ID)
		if err != nil {
			count = 1
		}

		res = append(res, dtos.WorkspaceResponse{
			ID:           ws.ID,
			Name:         ws.Name,
			Slug:         ws.Slug,
			Type:         string(ws.Type),
			Role:         string(member.Role),
			OwnerUserID:  ws.OwnerUserID,
			LogoURL:      ws.LogoURL,
			MembersCount: count,
			CreatedAt:    ws.CreatedAt,
			UpdatedAt:    ws.UpdatedAt,
		})
	}

	return res, nil
}

// GetWorkspaceDetail retrieves workspace metadata and the caller's role in it.
func (s *Service) GetWorkspaceDetail(ctx context.Context, workspaceID uuid.UUID) (*dtos.WorkspaceResponse, error) {
	userID, ok := ctxmeta.GetAuthUserID(ctx)
	if !ok {
		return nil, constants.ErrUnauthorized
	}

	member, err := s.repo.FindWorkspaceMember(ctx, workspaceID, userID)
	if err != nil {
		return nil, constants.ErrWorkspaceAccessDenied
	}

	ws, err := s.repo.FindWorkspaceByID(ctx, workspaceID)
	if err != nil {
		return nil, constants.ErrWorkspaceNotFound
	}

	count, err := s.repo.CountWorkspaceMembers(ctx, workspaceID)
	if err != nil {
		count = 1
	}

	return &dtos.WorkspaceResponse{
		ID:           ws.ID,
		Name:         ws.Name,
		Slug:         ws.Slug,
		Type:         string(ws.Type),
		Role:         string(member.Role),
		OwnerUserID:  ws.OwnerUserID,
		LogoURL:      ws.LogoURL,
		MembersCount: count,
		CreatedAt:    ws.CreatedAt,
		UpdatedAt:    ws.UpdatedAt,
	}, nil
}

// UpdateWorkspace modifies workspace metadata (Owner or Admin only).
func (s *Service) UpdateWorkspace(ctx context.Context, workspaceID uuid.UUID, req dtos.UpdateWorkspaceRequest) (*dtos.WorkspaceResponse, error) {
	userID, ok := ctxmeta.GetAuthUserID(ctx)
	if !ok {
		return nil, constants.ErrUnauthorized
	}

	member, err := s.repo.FindWorkspaceMember(ctx, workspaceID, userID)
	if err != nil {
		return nil, constants.ErrWorkspaceAccessDenied
	}
	if !member.Role.CanManageMembers() {
		return nil, constants.ErrForbidden
	}

	ws, err := s.repo.FindWorkspaceByID(ctx, workspaceID)
	if err != nil {
		return nil, constants.ErrWorkspaceNotFound
	}

	if req.Name != nil && *req.Name != "" {
		ws.Name = *req.Name
	}
	if req.LogoURL != nil {
		ws.LogoURL = req.LogoURL
	}

	if err := s.repo.UpdateWorkspace(ctx, ws); err != nil {
		return nil, s.wrapError(ctx, fmt.Errorf("failed to update workspace: %w", err))
	}

	count, _ := s.repo.CountWorkspaceMembers(ctx, workspaceID)
	return &dtos.WorkspaceResponse{
		ID:           ws.ID,
		Name:         ws.Name,
		Slug:         ws.Slug,
		Type:         string(ws.Type),
		Role:         string(member.Role),
		OwnerUserID:  ws.OwnerUserID,
		LogoURL:      ws.LogoURL,
		MembersCount: count,
		CreatedAt:    ws.CreatedAt,
		UpdatedAt:    ws.UpdatedAt,
	}, nil
}

// DeleteWorkspace terminates a team workspace (Owner only). Personal workspaces cannot be deleted.
func (s *Service) DeleteWorkspace(ctx context.Context, workspaceID uuid.UUID) error {
	userID, ok := ctxmeta.GetAuthUserID(ctx)
	if !ok {
		return constants.ErrUnauthorized
	}

	ws, err := s.repo.FindWorkspaceByID(ctx, workspaceID)
	if err != nil {
		return constants.ErrWorkspaceNotFound
	}
	if ws.Type == constants.WorkspaceTypePersonal {
		return constants.ErrCannotDeletePersonalWorkspace
	}
	if ws.OwnerUserID != userID {
		return constants.ErrForbidden
	}

	return s.repo.DeleteWorkspace(ctx, workspaceID)
}

// ListWorkspaceMembers returns all registered members of a workspace.
func (s *Service) ListWorkspaceMembers(ctx context.Context, workspaceID uuid.UUID) ([]dtos.WorkspaceMemberResponse, error) {
	userID, ok := ctxmeta.GetAuthUserID(ctx)
	if !ok {
		return nil, constants.ErrUnauthorized
	}

	_, err := s.repo.FindWorkspaceMember(ctx, workspaceID, userID)
	if err != nil {
		return nil, constants.ErrWorkspaceAccessDenied
	}

	members, err := s.repo.ListWorkspaceMembers(ctx, workspaceID)
	if err != nil {
		return nil, s.wrapError(ctx, fmt.Errorf("failed to list workspace members: %w", err))
	}

	res := make([]dtos.WorkspaceMemberResponse, 0, len(members))
	for _, m := range members {
		email := ""
		fullName := ""
		if m.User != nil {
			email = m.User.Email
			fullName = m.User.FullName
		}
		res = append(res, dtos.WorkspaceMemberResponse{
			ID:          m.ID,
			WorkspaceID: m.WorkspaceID,
			UserID:      m.UserID,
			Email:       email,
			FullName:    fullName,
			Role:        string(m.Role),
			CreatedAt:   m.CreatedAt,
		})
	}
	return res, nil
}

// UpdateWorkspaceMemberRole updates a member's role (Owner or Admin).
func (s *Service) UpdateWorkspaceMemberRole(ctx context.Context, workspaceID, targetUserID uuid.UUID, req dtos.UpdateWorkspaceMemberRoleRequest) error {
	callerID, ok := ctxmeta.GetAuthUserID(ctx)
	if !ok {
		return constants.ErrUnauthorized
	}

	caller, err := s.repo.FindWorkspaceMember(ctx, workspaceID, callerID)
	if err != nil {
		return constants.ErrWorkspaceAccessDenied
	}
	if !caller.Role.CanManageMembers() {
		return constants.ErrForbidden
	}

	target, err := s.repo.FindWorkspaceMember(ctx, workspaceID, targetUserID)
	if err != nil {
		return constants.ErrNotFound
	}
	if target.Role == constants.WorkspaceRoleOwner {
		return constants.ErrCannotRemoveWorkspaceOwner
	}

	newRole := constants.WorkspaceRole(req.Role)
	if !newRole.IsValid() {
		return constants.ErrInvalidWorkspaceRole
	}
	if newRole == constants.WorkspaceRoleOwner && caller.Role != constants.WorkspaceRoleOwner {
		return constants.ErrForbidden
	}

	return s.repo.UpdateWorkspaceMemberRole(ctx, workspaceID, targetUserID, newRole)
}

// RemoveWorkspaceMember removes a member or allows a member to leave a workspace.
func (s *Service) RemoveWorkspaceMember(ctx context.Context, workspaceID, targetUserID uuid.UUID) error {
	callerID, ok := ctxmeta.GetAuthUserID(ctx)
	if !ok {
		return constants.ErrUnauthorized
	}

	// Case 1: Member leaves voluntarily
	if callerID == targetUserID {
		return s.leaveWorkspace(ctx, workspaceID, callerID)
	}

	// Case 2: Admin or Owner removes another member
	return s.kickWorkspaceMember(ctx, workspaceID, callerID, targetUserID)
}

func (s *Service) leaveWorkspace(ctx context.Context, workspaceID, userID uuid.UUID) error {
	member, err := s.repo.FindWorkspaceMember(ctx, workspaceID, userID)
	if err != nil {
		return constants.ErrWorkspaceAccessDenied
	}
	if member.Role == constants.WorkspaceRoleOwner {
		return constants.ErrCannotRemoveWorkspaceOwner
	}
	return s.repo.RemoveWorkspaceMember(ctx, workspaceID, userID)
}

func (s *Service) kickWorkspaceMember(ctx context.Context, workspaceID, callerID, targetUserID uuid.UUID) error {
	caller, err := s.repo.FindWorkspaceMember(ctx, workspaceID, callerID)
	if err != nil {
		return constants.ErrWorkspaceAccessDenied
	}
	if !caller.Role.CanManageMembers() {
		return constants.ErrForbidden
	}

	target, err := s.repo.FindWorkspaceMember(ctx, workspaceID, targetUserID)
	if err != nil {
		return constants.ErrNotFound
	}
	if target.Role == constants.WorkspaceRoleOwner {
		return constants.ErrCannotRemoveWorkspaceOwner
	}

	return s.repo.RemoveWorkspaceMember(ctx, workspaceID, targetUserID)
}

// EnsureUserPersonalWorkspace guarantees that a user has a personal workspace on login/registration.
func (s *Service) EnsureUserPersonalWorkspace(ctx context.Context, user *models.User) (*models.Workspace, error) {
	if user == nil {
		return nil, constants.ErrBadRequest
	}

	existing, err := s.repo.FindPersonalWorkspaceByUserID(ctx, user.ID)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, s.wrapError(ctx, err)
	}

	name := "My Workspace"
	if user.FullName != "" {
		name = fmt.Sprintf("%s's Workspace", user.FullName)
	}

	ws := &models.Workspace{
		BaseModel:   models.BaseModel{ID: uuid.New()},
		Name:        name,
		Slug:        fmt.Sprintf("personal-%s", user.ID.String()),
		Type:        constants.WorkspaceTypePersonal,
		OwnerUserID: user.ID,
	}

	if err := s.repo.CreateWorkspace(ctx, ws); err != nil {
		return nil, s.wrapError(ctx, fmt.Errorf("failed to create workspace: %w", err))
	}
	return ws, nil
}

// autoProvisionPersonalWorkspaceSlice ensures a personal workspace exists when a user has no workspace memberships yet.
func (s *Service) autoProvisionPersonalWorkspaceSlice(ctx context.Context, userID uuid.UUID) []models.Workspace {
	user, err := s.repo.FindUserByID(ctx, userID)
	if err != nil || user == nil {
		return nil
	}
	ws, err := s.EnsureUserPersonalWorkspace(ctx, user)
	if err != nil || ws == nil {
		return nil
	}
	return []models.Workspace{*ws}
}
