package services

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
	"code-base-golang/internal/queue"
	"code-base-golang/internal/repositories"
	"code-base-golang/internal/validations"
)

// AdminListRoles retrieves paginated administrative RBAC roles with decoded permission lists,
// ordering default system roles first.
func (s *Service) AdminListRoles(ctx context.Context, q dtos.AdminRoleListQuery) (*dtos.AdminRoleListResponse, error) {
	q.SetDefaults()

	offset := (q.Page - 1) * q.Limit
	repoQuery := repositories.ListRolesQuery{
		Search: q.Search,
		Offset: offset,
		Limit:  q.Limit,
	}

	roles, total, err := s.repo.ListRolesPaginated(ctx, repoQuery)
	if err != nil {
		return nil, s.wrapError(ctx, err)
	}

	items := make([]dtos.AdminRoleItem, 0, len(roles))
	for _, r := range roles {
		rawPerms := r.PermissionsList()
		perms := make([]string, 0, len(rawPerms))
		for _, p := range rawPerms {
			p = strings.TrimSpace(p)
			if p != "" {
				perms = append(perms, p)
			}
		}
		items = append(items, dtos.AdminRoleItem{
			ID:          r.ID,
			Name:        r.Name,
			Description: r.Description,
			Permissions: perms,
			IsSystem:    r.IsSystem,
			CreatedAt:   r.CreatedAt,
			UpdatedAt:   r.UpdatedAt,
		})
	}

	return &dtos.AdminRoleListResponse{
		Items:      items,
		Pagination: dtos.NewPaginationMeta(q.Page, q.Limit, total),
	}, nil
}

// AdminCreateRole creates a new custom administrative role and writes an audit log.
func (s *Service) AdminCreateRole(ctx context.Context, meta dtos.AdminActionMeta, req dtos.AdminCreateRoleRequest) (*dtos.AdminRoleItem, error) {
	if err := validations.ValidateAdminCreateRoleRequest(&req); err != nil {
		return nil, constants.ErrBadRequest.WithMessage(err.Error())
	}

	existing, err := s.repo.FindRoleByName(ctx, req.Name)
	if err == nil && existing != nil {
		return nil, constants.ErrConflict.WithMessage("role with this name already exists")
	}

	permsJSON, err := json.Marshal(req.Permissions)
	if err != nil {
		return nil, constants.ErrBadRequest.WithMessage("invalid permissions format")
	}

	role := &models.AdminRole{
		Name:        req.Name,
		Description: req.Description,
		Permissions: permsJSON,
		IsSystem:    false,
	}

	if err := s.repo.CreateRole(ctx, role); err != nil {
		return nil, s.wrapError(ctx, err)
	}

	roleIDStr := role.ID.String()
	auditLog := queue.AdminAuditPayload{
		AdminID:  meta.AdminIDPtr(),
		Action:   constants.AuditActionRoleCreate,
		Entity:   constants.AuditEntityRole,
		EntityID: &roleIDStr,
		Payload: map[string]any{
			"name":        role.Name,
			"permissions": req.Permissions,
		},
		IPAddress: meta.IPPtr(),
		UserAgent: meta.UAPtr(),
	}
	s.emitAdminAuditLog(ctx, auditLog)

	return &dtos.AdminRoleItem{
		ID:          role.ID,
		Name:        role.Name,
		Description: role.Description,
		Permissions: req.Permissions,
		IsSystem:    role.IsSystem,
		CreatedAt:   role.CreatedAt,
		UpdatedAt:   role.UpdatedAt,
	}, nil
}

// AdminUpdateRole modifies an existing administrative role (protecting system roles).
func (s *Service) AdminUpdateRole(ctx context.Context, meta dtos.AdminActionMeta, roleID uuid.UUID, req dtos.AdminUpdateRoleRequest) (*dtos.AdminRoleItem, error) {
	if err := validations.ValidateAdminUpdateRoleRequest(&req); err != nil {
		return nil, constants.ErrBadRequest.WithMessage(err.Error())
	}

	role, err := s.repo.FindRoleByID(ctx, roleID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrNotFound.WithMessage("role not found")
		}
		return nil, s.wrapError(ctx, err)
	}

	if role.IsSystem {
		return nil, constants.ErrForbidden.WithMessage("system roles cannot be modified")
	}

	if role.Name != req.Name {
		existing, err := s.repo.FindRoleByName(ctx, req.Name)
		if err == nil && existing != nil && existing.ID != role.ID {
			return nil, constants.ErrConflict.WithMessage("role with this name already exists")
		}
	}

	permsJSON, err := json.Marshal(req.Permissions)
	if err != nil {
		return nil, constants.ErrBadRequest.WithMessage("invalid permissions format")
	}

	role.Name = req.Name
	role.Description = req.Description
	role.Permissions = permsJSON

	if err := s.repo.UpdateRole(ctx, role); err != nil {
		return nil, s.wrapError(ctx, err)
	}

	roleIDStr := role.ID.String()
	auditLog := queue.AdminAuditPayload{
		AdminID:  meta.AdminIDPtr(),
		Action:   constants.AuditActionRoleUpdate,
		Entity:   constants.AuditEntityRole,
		EntityID: &roleIDStr,
		Payload: map[string]any{
			"name":        role.Name,
			"permissions": req.Permissions,
		},
		IPAddress: meta.IPPtr(),
		UserAgent: meta.UAPtr(),
	}
	s.emitAdminAuditLog(ctx, auditLog)

	return &dtos.AdminRoleItem{
		ID:          role.ID,
		Name:        role.Name,
		Description: role.Description,
		Permissions: req.Permissions,
		IsSystem:    role.IsSystem,
		CreatedAt:   role.CreatedAt,
		UpdatedAt:   role.UpdatedAt,
	}, nil
}

// AdminListUserRoles retrieves paginated customer user roles with permissions,
// ordering default user roles first.
func (s *Service) AdminListUserRoles(ctx context.Context, q dtos.AdminUserRoleListQuery) (*dtos.AdminUserRoleListResponse, error) {
	q.SetDefaults()

	offset := (q.Page - 1) * q.Limit
	repoQuery := repositories.ListUserRolesQuery{
		Search: q.Search,
		Offset: offset,
		Limit:  q.Limit,
	}

	roles, total, err := s.repo.ListUserRolesPaginated(ctx, repoQuery)
	if err != nil {
		return nil, s.wrapError(ctx, err)
	}

	items := make([]dtos.AdminUserRoleItem, 0, len(roles))
	for _, r := range roles {
		rawPerms := r.PermissionsList()
		perms := make([]string, 0, len(rawPerms))
		for _, p := range rawPerms {
			p = strings.TrimSpace(p)
			if p != "" {
				perms = append(perms, p)
			}
		}
		items = append(items, dtos.AdminUserRoleItem{
			ID:          r.ID,
			Name:        r.Name,
			Code:        r.Code,
			Description: r.Description,
			Permissions: perms,
			DailyQuota:  r.DailyQuota,
			IsDefault:   r.IsDefault,
			CreatedAt:   r.CreatedAt,
			UpdatedAt:   r.UpdatedAt,
		})
	}

	return &dtos.AdminUserRoleListResponse{
		Items:      items,
		Pagination: dtos.NewPaginationMeta(q.Page, q.Limit, total),
	}, nil
}

// AdminCreateUserRole creates a new customer user role and writes an audit log.
func (s *Service) AdminCreateUserRole(ctx context.Context, meta dtos.AdminActionMeta, req dtos.AdminCreateUserRoleRequest) (*dtos.AdminUserRoleItem, error) {
	if err := validations.ValidateAdminCreateUserRoleRequest(&req); err != nil {
		return nil, constants.ErrBadRequest.WithMessage(err.Error())
	}

	existingName, err := s.repo.FindUserRoleByName(ctx, req.Name)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, s.wrapError(ctx, err)
	}
	if err == nil && existingName != nil {
		return nil, constants.ErrConflict.WithMessage("user role with this name already exists")
	}

	existingCode, err := s.repo.FindUserRoleByCode(ctx, req.Code)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, s.wrapError(ctx, err)
	}
	if err == nil && existingCode != nil {
		return nil, constants.ErrConflict.WithMessage("user role with this code already exists")
	}

	perms := req.Permissions
	if perms == nil {
		perms = []string{}
	}
	permsJSON, err := json.Marshal(perms)
	if err != nil {
		return nil, constants.ErrBadRequest.WithMessage("invalid permissions format")
	}

	role := &models.UserRole{
		Name:        req.Name,
		Code:        strings.ToUpper(req.Code),
		Description: req.Description,
		Permissions: permsJSON,
		DailyQuota:  req.DailyQuota,
		IsDefault:   req.IsDefault,
	}

	if err := s.repo.CreateUserRole(ctx, role); err != nil {
		return nil, s.wrapError(ctx, err)
	}

	if req.IsDefault {
		if err := s.repo.ClearDefaultUserRoles(ctx, role.ID); err != nil {
			return nil, s.wrapError(ctx, err)
		}
	}

	roleIDStr := role.ID.String()
	auditLog := queue.AdminAuditPayload{
		AdminID:  meta.AdminIDPtr(),
		Action:   constants.AuditActionUserRoleCreate,
		Entity:   constants.AuditEntityUserRole,
		EntityID: &roleIDStr,
		Payload: map[string]any{
			"name":        role.Name,
			"code":        role.Code,
			"daily_quota": role.DailyQuota,
			"permissions": perms,
			"is_default":  role.IsDefault,
		},
		IPAddress: meta.IPPtr(),
		UserAgent: meta.UAPtr(),
	}
	s.emitAdminAuditLog(ctx, auditLog)

	return &dtos.AdminUserRoleItem{
		ID:          role.ID,
		Name:        role.Name,
		Code:        role.Code,
		Description: role.Description,
		Permissions: perms,
		DailyQuota:  role.DailyQuota,
		IsDefault:   role.IsDefault,
		CreatedAt:   role.CreatedAt,
		UpdatedAt:   role.UpdatedAt,
	}, nil
}

// AdminUpdateUserRole modifies an existing customer user role.
func (s *Service) AdminUpdateUserRole(ctx context.Context, meta dtos.AdminActionMeta, roleID uuid.UUID, req dtos.AdminUpdateUserRoleRequest) (*dtos.AdminUserRoleItem, error) {
	if err := validations.ValidateAdminUpdateUserRoleRequest(&req); err != nil {
		return nil, constants.ErrBadRequest.WithMessage(err.Error())
	}

	role, err := s.repo.FindUserRoleByID(ctx, roleID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrNotFound.WithMessage("user role not found")
		}
		return nil, s.wrapError(ctx, err)
	}

	if req.Name != nil && *req.Name != role.Name {
		existing, err := s.repo.FindUserRoleByName(ctx, *req.Name)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, s.wrapError(ctx, err)
		}
		if err == nil && existing != nil && existing.ID != role.ID {
			return nil, constants.ErrConflict.WithMessage("user role with this name already exists")
		}
		role.Name = *req.Name
	}

	if req.Description != nil {
		role.Description = *req.Description
	}

	if req.DailyQuota != nil {
		role.DailyQuota = *req.DailyQuota
	}

	if req.Permissions != nil {
		permsJSON, err := json.Marshal(*req.Permissions)
		if err != nil {
			return nil, constants.ErrBadRequest.WithMessage("invalid permissions format")
		}
		role.Permissions = permsJSON
	}

	if req.IsDefault != nil {
		if *req.IsDefault {
			if err := s.repo.ClearDefaultUserRoles(ctx, role.ID); err != nil {
				return nil, s.wrapError(ctx, err)
			}
			role.IsDefault = true
		} else {
			role.IsDefault = false
		}
	}

	if err := s.repo.UpdateUserRole(ctx, role); err != nil {
		return nil, s.wrapError(ctx, err)
	}

	roleIDStr := role.ID.String()
	auditLog := queue.AdminAuditPayload{
		AdminID:  meta.AdminIDPtr(),
		Action:   constants.AuditActionUserRoleUpdate,
		Entity:   constants.AuditEntityUserRole,
		EntityID: &roleIDStr,
		Payload: map[string]any{
			"name":        role.Name,
			"code":        role.Code,
			"daily_quota": role.DailyQuota,
			"is_default":  role.IsDefault,
		},
		IPAddress: meta.IPPtr(),
		UserAgent: meta.UAPtr(),
	}
	s.emitAdminAuditLog(ctx, auditLog)

	rawPerms := role.PermissionsList()
	perms := make([]string, 0, len(rawPerms))
	for _, p := range rawPerms {
		p = strings.TrimSpace(p)
		if p != "" {
			perms = append(perms, p)
		}
	}

	return &dtos.AdminUserRoleItem{
		ID:          role.ID,
		Name:        role.Name,
		Code:        role.Code,
		Description: role.Description,
		Permissions: perms,
		DailyQuota:  role.DailyQuota,
		IsDefault:   role.IsDefault,
		CreatedAt:   role.CreatedAt,
		UpdatedAt:   role.UpdatedAt,
	}, nil
}
