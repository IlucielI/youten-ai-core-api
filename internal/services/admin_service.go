package services

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
	"code-base-golang/internal/pkg/hasher"
	"code-base-golang/internal/pkg/jwt"
	"code-base-golang/internal/repositories"
)

// AdminActionMeta carries administrative actor and request provenance for audit logging.
type AdminActionMeta struct {
	AdminID   uuid.UUID
	IPAddress string
	UserAgent string
}

// AdminListUsers retrieves a paginated and filtered list of users with dynamic daily quota metrics.
func (s *Service) AdminListUsers(ctx context.Context, q dtos.AdminUserListQuery) (*dtos.AdminUserListResponse, error) {
	q.SetDefaults()

	offset := (q.Page - 1) * q.Limit
	repoQuery := repositories.ListUsersQuery{
		Search: q.Search,
		Status: models.UserStatus(q.Status),
		Offset: offset,
		Limit:  q.Limit,
	}

	users, total, err := s.repo.ListUsers(ctx, repoQuery)
	if err != nil {
		return nil, s.wrapError(ctx, err)
	}

	items := make([]dtos.AdminUserListItem, 0, len(users))
	for _, u := range users {
		dailyQuota := resolveDailyQuota(&u)

		countToday, err := s.repo.CountUserRecordingsToday(ctx, u.ID)
		if err != nil {
			countToday = 0
		}

		items = append(items, dtos.AdminUserListItem{
			ID:             u.ID,
			Email:          u.Email,
			FullName:       u.FullName,
			Status:         string(u.Status),
			DailyQuota:     dailyQuota,
			QuotaUsedToday: int(countToday),
			EmailVerified:  u.EmailVerified,
			CreatedAt:      u.CreatedAt,
		})
	}

	totalPages := int(math.Ceil(float64(total) / float64(q.Limit)))
	if totalPages == 0 && total == 0 {
		totalPages = 0
	} else if totalPages == 0 {
		totalPages = 1
	}

	return &dtos.AdminUserListResponse{
		Items: items,
		Pagination: dtos.PaginationMeta{
			CurrentPage: q.Page,
			PageSize:    q.Limit,
			TotalItems:  total,
			TotalPages:  totalPages,
		},
	}, nil
}

// AdminOverrideUserQuota sets or resets a user's daily quota override and records an administrative audit log.
func (s *Service) AdminOverrideUserQuota(ctx context.Context, meta AdminActionMeta, userID uuid.UUID, req dtos.AdminUserQuotaOverrideRequest) (*dtos.AdminUserQuotaResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, constants.ErrBadRequest.WithMessage(err.Error())
	}

	user, err := s.repo.FindUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrNotFound.WithMessage("user not found")
		}
		return nil, s.wrapError(ctx, err)
	}

	prevOverride := user.DailyQuotaOverride

	if err := s.repo.UpdateUserDailyQuotaOverride(ctx, userID, req.DailyQuotaOverride); err != nil {
		return nil, s.wrapError(ctx, err)
	}

	user.DailyQuotaOverride = req.DailyQuotaOverride
	effectiveQuota := resolveDailyQuota(user)

	// Persist administrative audit log
	adminIDVal := meta.AdminID
	entityIDStr := userID.String()
	var ip *string
	var ua *string
	if meta.IPAddress != "" {
		ip = &meta.IPAddress
	}
	if meta.UserAgent != "" {
		ua = &meta.UserAgent
	}

	auditLog := &models.AdminAuditLog{
		AdminID:  &adminIDVal,
		Action:   "user.quota_override",
		Entity:   "user",
		EntityID: &entityIDStr,
		Payload: models.JSONMap{
			"previous_override": prevOverride,
			"new_override":      req.DailyQuotaOverride,
		},
		IPAddress: ip,
		UserAgent: ua,
	}
	if err := s.repo.CreateAdminAuditLog(ctx, auditLog); err != nil {
		return nil, s.wrapError(ctx, err)
	}

	return &dtos.AdminUserQuotaResponse{
		UserID:             userID,
		DailyQuotaOverride: req.DailyQuotaOverride,
		EffectiveQuota:     effectiveQuota,
	}, nil
}

// AdminRevokeUserSessions revokes all active auth and session tokens for a target user and creates an audit trail.
func (s *Service) AdminRevokeUserSessions(ctx context.Context, meta AdminActionMeta, userID uuid.UUID) (*dtos.AdminRevokeUserSessionsResponse, error) {
	user, err := s.repo.FindUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrNotFound.WithMessage("user not found")
		}
		return nil, s.wrapError(ctx, err)
	}

	if err := s.repo.RevokeAllAuthTokensByUserID(ctx, user.ID, ""); err != nil {
		return nil, s.wrapError(ctx, err)
	}

	adminIDVal := meta.AdminID
	entityIDStr := userID.String()
	var ip *string
	var ua *string
	if meta.IPAddress != "" {
		ip = &meta.IPAddress
	}
	if meta.UserAgent != "" {
		ua = &meta.UserAgent
	}

	auditLog := &models.AdminAuditLog{
		AdminID:  &adminIDVal,
		Action:   "user.revoke_sessions",
		Entity:   "user",
		EntityID: &entityIDStr,
		Payload: models.JSONMap{
			"target_email": user.Email,
		},
		IPAddress: ip,
		UserAgent: ua,
	}
	if err := s.repo.CreateAdminAuditLog(ctx, auditLog); err != nil {
		return nil, s.wrapError(ctx, err)
	}

	return &dtos.AdminRevokeUserSessionsResponse{
		UserID:  userID,
		Revoked: true,
		Message: "all active sessions have been successfully revoked",
	}, nil
}

// AdminListRoles retrieves all administrative RBAC roles with decoded permission lists.
func (s *Service) AdminListRoles(ctx context.Context) (*dtos.AdminRoleListResponse, error) {
	roles, err := s.repo.ListRoles(ctx)
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

	return &dtos.AdminRoleListResponse{Items: items}, nil
}

// AdminCreateRole creates a new custom administrative role and writes an audit log.
func (s *Service) AdminCreateRole(ctx context.Context, meta AdminActionMeta, req dtos.AdminCreateRoleRequest) (*dtos.AdminRoleItem, error) {
	if err := req.Validate(); err != nil {
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

	adminIDVal := meta.AdminID
	roleIDStr := role.ID.String()
	var ip *string
	var ua *string
	if meta.IPAddress != "" {
		ip = &meta.IPAddress
	}
	if meta.UserAgent != "" {
		ua = &meta.UserAgent
	}

	auditLog := &models.AdminAuditLog{
		AdminID:  &adminIDVal,
		Action:   "role.create",
		Entity:   "role",
		EntityID: &roleIDStr,
		Payload: models.JSONMap{
			"name":        role.Name,
			"permissions": req.Permissions,
		},
		IPAddress: ip,
		UserAgent: ua,
	}
	if err := s.repo.CreateAdminAuditLog(ctx, auditLog); err != nil {
		return nil, s.wrapError(ctx, err)
	}

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
func (s *Service) AdminUpdateRole(ctx context.Context, meta AdminActionMeta, roleID uuid.UUID, req dtos.AdminUpdateRoleRequest) (*dtos.AdminRoleItem, error) {
	if err := req.Validate(); err != nil {
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

	adminIDVal := meta.AdminID
	roleIDStr := role.ID.String()
	var ip *string
	var ua *string
	if meta.IPAddress != "" {
		ip = &meta.IPAddress
	}
	if meta.UserAgent != "" {
		ua = &meta.UserAgent
	}

	auditLog := &models.AdminAuditLog{
		AdminID:  &adminIDVal,
		Action:   "role.update",
		Entity:   "role",
		EntityID: &roleIDStr,
		Payload: models.JSONMap{
			"name":        role.Name,
			"permissions": req.Permissions,
		},
		IPAddress: ip,
		UserAgent: ua,
	}
	if err := s.repo.CreateAdminAuditLog(ctx, auditLog); err != nil {
		return nil, s.wrapError(ctx, err)
	}

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

// AdminListUserRoles retrieves all customer user roles with permissions.
func (s *Service) AdminListUserRoles(ctx context.Context) (*dtos.AdminUserRoleListResponse, error) {
	roles, err := s.repo.ListUserRoles(ctx)
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

	return &dtos.AdminUserRoleListResponse{Items: items}, nil
}

// AdminCreateUserRole creates a new customer user role and writes an audit log.
func (s *Service) AdminCreateUserRole(ctx context.Context, meta AdminActionMeta, req dtos.AdminCreateUserRoleRequest) (*dtos.AdminUserRoleItem, error) {
	if err := req.Validate(); err != nil {
		return nil, constants.ErrBadRequest.WithMessage(err.Error())
	}

	existingName, err := s.repo.FindUserRoleByName(ctx, req.Name)
	if err == nil && existingName != nil {
		return nil, constants.ErrConflict.WithMessage("user role with this name already exists")
	}

	existingCode, err := s.repo.FindUserRoleByCode(ctx, req.Code)
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

	if req.IsDefault {
		if err := s.repo.ClearDefaultUserRoles(ctx, uuid.Nil); err != nil {
			return nil, s.wrapError(ctx, err)
		}
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

	adminIDVal := meta.AdminID
	roleIDStr := role.ID.String()
	var ip *string
	var ua *string
	if meta.IPAddress != "" {
		ip = &meta.IPAddress
	}
	if meta.UserAgent != "" {
		ua = &meta.UserAgent
	}

	auditLog := &models.AdminAuditLog{
		AdminID:  &adminIDVal,
		Action:   "user_role.create",
		Entity:   "user_role",
		EntityID: &roleIDStr,
		Payload: models.JSONMap{
			"name":        role.Name,
			"code":        role.Code,
			"daily_quota": role.DailyQuota,
			"permissions": perms,
			"is_default":  role.IsDefault,
		},
		IPAddress: ip,
		UserAgent: ua,
	}
	if err := s.repo.CreateAdminAuditLog(ctx, auditLog); err != nil {
		return nil, s.wrapError(ctx, err)
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

// AdminUpdateUserRole modifies an existing customer user role.
func (s *Service) AdminUpdateUserRole(ctx context.Context, meta AdminActionMeta, roleID uuid.UUID, req dtos.AdminUpdateUserRoleRequest) (*dtos.AdminUserRoleItem, error) {
	if err := req.Validate(); err != nil {
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

	adminIDVal := meta.AdminID
	roleIDStr := role.ID.String()
	var ip *string
	var ua *string
	if meta.IPAddress != "" {
		ip = &meta.IPAddress
	}
	if meta.UserAgent != "" {
		ua = &meta.UserAgent
	}

	auditLog := &models.AdminAuditLog{
		AdminID:  &adminIDVal,
		Action:   "user_role.update",
		Entity:   "user_role",
		EntityID: &roleIDStr,
		Payload: models.JSONMap{
			"name":        role.Name,
			"code":        role.Code,
			"daily_quota": role.DailyQuota,
			"is_default":  role.IsDefault,
		},
		IPAddress: ip,
		UserAgent: ua,
	}
	if err := s.repo.CreateAdminAuditLog(ctx, auditLog); err != nil {
		return nil, s.wrapError(ctx, err)
	}

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

// AdminAssignUserRole assigns a customer user role to a user.
func (s *Service) AdminAssignUserRole(ctx context.Context, meta AdminActionMeta, userID uuid.UUID, req dtos.AdminAssignUserRoleRequest) (*dtos.UserResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, constants.ErrBadRequest.WithMessage(err.Error())
	}

	user, err := s.repo.FindUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrNotFound.WithMessage("user not found")
		}
		return nil, s.wrapError(ctx, err)
	}

	role, err := s.repo.FindUserRoleByID(ctx, req.RoleID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrNotFound.WithMessage("user role not found")
		}
		return nil, s.wrapError(ctx, err)
	}

	if err := s.repo.UpdateUserRoleID(ctx, userID, req.RoleID); err != nil {
		return nil, s.wrapError(ctx, err)
	}

	updatedUser, err := s.repo.FindUserByIDWithRole(ctx, userID)
	if err != nil {
		return nil, s.wrapError(ctx, err)
	}

	adminIDVal := meta.AdminID
	userIDStr := user.ID.String()
	var ip *string
	var ua *string
	if meta.IPAddress != "" {
		ip = &meta.IPAddress
	}
	if meta.UserAgent != "" {
		ua = &meta.UserAgent
	}

	auditLog := &models.AdminAuditLog{
		AdminID:  &adminIDVal,
		Action:   "user.assign_role",
		Entity:   "user",
		EntityID: &userIDStr,
		Payload: models.JSONMap{
			"previous_role_id": user.RoleID,
			"new_role_id":      req.RoleID,
			"new_role_code":    role.Code,
			"new_role_name":    role.Name,
		},
		IPAddress: ip,
		UserAgent: ua,
	}
	if err := s.repo.CreateAdminAuditLog(ctx, auditLog); err != nil {
		return nil, s.wrapError(ctx, err)
	}

	resp := newUserResponse(updatedUser)
	return &resp, nil
}

// AdminListTemplates retrieves all prompt templates for administrative management.
func (s *Service) AdminListTemplates(ctx context.Context) (*dtos.AdminTemplateListResponse, error) {
	templates, err := s.repo.ListAllTemplates(ctx)
	if err != nil {
		return nil, s.wrapError(ctx, err)
	}

	items := make([]dtos.AdminTemplateItem, 0, len(templates))
	for _, t := range templates {
		items = append(items, dtos.AdminTemplateItem{
			ID:           t.ID,
			CategoryKey:  t.CategoryKey,
			Name:         t.Name,
			Description:  t.Description,
			Prompt:       t.Prompt,
			OutputSchema: t.OutputSchema,
			Version:      t.Version,
			IsActive:     t.IsActive,
			CreatedAt:    t.CreatedAt,
			UpdatedAt:    t.UpdatedAt,
		})
	}

	return &dtos.AdminTemplateListResponse{Items: items}, nil
}

// AdminCreateTemplate creates a new prompt template and writes an administrative audit log.
func (s *Service) AdminCreateTemplate(ctx context.Context, meta AdminActionMeta, req dtos.AdminCreateTemplateRequest) (*dtos.AdminTemplateItem, error) {
	if err := req.Validate(); err != nil {
		return nil, constants.ErrBadRequest.WithMessage(err.Error())
	}

	existing, err := s.repo.FindAnyTemplateByCategoryKey(ctx, req.CategoryKey)
	if err == nil && existing != nil {
		return nil, constants.ErrConflict.WithMessage("template with this category key already exists")
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	tmpl := &models.Template{
		CategoryKey:  req.CategoryKey,
		Name:         req.Name,
		Description:  req.Description,
		Prompt:       req.Prompt,
		OutputSchema: models.JSONMap(req.OutputSchema),
		Version:      1,
		IsActive:     isActive,
	}

	if err := s.repo.CreateTemplate(ctx, tmpl); err != nil {
		return nil, s.wrapError(ctx, err)
	}

	adminIDVal := meta.AdminID
	tmplIDStr := tmpl.ID.String()
	var ip *string
	var ua *string
	if meta.IPAddress != "" {
		ip = &meta.IPAddress
	}
	if meta.UserAgent != "" {
		ua = &meta.UserAgent
	}

	auditLog := &models.AdminAuditLog{
		AdminID:  &adminIDVal,
		Action:   "template.create",
		Entity:   "template",
		EntityID: &tmplIDStr,
		Payload: models.JSONMap{
			"category_key": tmpl.CategoryKey,
			"name":         tmpl.Name,
			"version":      tmpl.Version,
		},
		IPAddress: ip,
		UserAgent: ua,
	}
	if err := s.repo.CreateAdminAuditLog(ctx, auditLog); err != nil {
		return nil, s.wrapError(ctx, err)
	}

	return &dtos.AdminTemplateItem{
		ID:           tmpl.ID,
		CategoryKey:  tmpl.CategoryKey,
		Name:         tmpl.Name,
		Description:  tmpl.Description,
		Prompt:       tmpl.Prompt,
		OutputSchema: map[string]interface{}(tmpl.OutputSchema),
		Version:      tmpl.Version,
		IsActive:     tmpl.IsActive,
		CreatedAt:    tmpl.CreatedAt,
		UpdatedAt:    tmpl.UpdatedAt,
	}, nil
}

// AdminUpdateTemplate modifies an existing prompt template and increments version when prompt or schema changes.
func (s *Service) AdminUpdateTemplate(ctx context.Context, meta AdminActionMeta, templateID uuid.UUID, req dtos.AdminUpdateTemplateRequest) (*dtos.AdminTemplateItem, error) {
	if err := req.Validate(); err != nil {
		return nil, constants.ErrBadRequest.WithMessage(err.Error())
	}

	tmpl, err := s.repo.FindTemplateByID(ctx, templateID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrNotFound.WithMessage("template not found")
		}
		return nil, s.wrapError(ctx, err)
	}

	promptChanged := tmpl.Prompt != req.Prompt
	schemaChanged := !reflect.DeepEqual(map[string]interface{}(tmpl.OutputSchema), req.OutputSchema)

	if promptChanged || schemaChanged {
		tmpl.Version = tmpl.Version + 1
	}

	tmpl.Name = req.Name
	tmpl.Description = req.Description
	tmpl.Prompt = req.Prompt
	tmpl.OutputSchema = models.JSONMap(req.OutputSchema)
	if req.IsActive != nil {
		tmpl.IsActive = *req.IsActive
	}

	if err := s.repo.UpdateTemplate(ctx, tmpl); err != nil {
		return nil, s.wrapError(ctx, err)
	}

	adminIDVal := meta.AdminID
	tmplIDStr := tmpl.ID.String()
	var ip *string
	var ua *string
	if meta.IPAddress != "" {
		ip = &meta.IPAddress
	}
	if meta.UserAgent != "" {
		ua = &meta.UserAgent
	}

	auditLog := &models.AdminAuditLog{
		AdminID:  &adminIDVal,
		Action:   "template.update",
		Entity:   "template",
		EntityID: &tmplIDStr,
		Payload: models.JSONMap{
			"category_key":   tmpl.CategoryKey,
			"name":           tmpl.Name,
			"version":        tmpl.Version,
			"prompt_changed": promptChanged,
			"schema_changed": schemaChanged,
		},
		IPAddress: ip,
		UserAgent: ua,
	}
	if err := s.repo.CreateAdminAuditLog(ctx, auditLog); err != nil {
		return nil, s.wrapError(ctx, err)
	}

	return &dtos.AdminTemplateItem{
		ID:           tmpl.ID,
		CategoryKey:  tmpl.CategoryKey,
		Name:         tmpl.Name,
		Description:  tmpl.Description,
		Prompt:       tmpl.Prompt,
		OutputSchema: map[string]interface{}(tmpl.OutputSchema),
		Version:      tmpl.Version,
		IsActive:     tmpl.IsActive,
		CreatedAt:    tmpl.CreatedAt,
		UpdatedAt:    tmpl.UpdatedAt,
	}, nil
}

// AdminTestTemplate executes a sandbox dry run of a prompt and schema against a sample transcript.
func (s *Service) AdminTestTemplate(ctx context.Context, meta AdminActionMeta, req dtos.AdminTestTemplateRequest) (*dtos.AdminTestTemplateResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, constants.ErrBadRequest.WithMessage(err.Error())
	}

	if s.llm == nil {
		return nil, constants.ErrInternalServerError.WithMessage("llm provider is not configured")
	}

	startTime := time.Now()
	structuredRes, err := s.llm.GenerateStructured(ctx, req.Prompt, req.SampleTranscript, req.OutputSchema)
	executionTimeMs := time.Since(startTime).Milliseconds()
	if err != nil {
		return nil, s.wrapError(ctx, err)
	}

	var parsedJSON interface{}
	if structuredRes.RawJSON != "" {
		_ = json.Unmarshal([]byte(structuredRes.RawJSON), &parsedJSON)
	}

	return &dtos.AdminTestTemplateResponse{
		RawOutput:       structuredRes.RawJSON,
		ParsedJSON:      parsedJSON,
		ExecutionTimeMs: executionTimeMs,
		TokensUsed:      structuredRes.Usage.TotalTokens,
	}, nil
}

// AdminGetDLQMessages retrieves failed and stuck pipeline items for DLQ monitoring.
func (s *Service) AdminGetDLQMessages(ctx context.Context, meta AdminActionMeta, page, limit int) (*dtos.DLQMessagesResponse, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	// Recordings stuck in intermediate states for > 15 minutes are considered stuck
	stuckThreshold := 15 * time.Minute

	recs, total, failedCount, stuckCount, err := s.repo.ListDLQRecordings(ctx, stuckThreshold, limit, offset)
	if err != nil {
		return nil, s.wrapError(ctx, err)
	}

	items := make([]dtos.DLQMessageItem, 0, len(recs))
	for _, rec := range recs {
		queueName, routingKey := resolveDLQQueueAndTopic(rec.Status, rec.SourceType)

		item := dtos.DLQMessageItem{
			RecordingID: rec.ID,
			Queue:       queueName,
			RoutingKey:  routingKey,
			Stage:       rec.Status,
			Status:      rec.Status,
			RetryCount:  0,
			FailedAt:    rec.UpdatedAt,
			Title:       rec.Title,
		}

		if rec.ErrorCode != nil {
			item.ErrorCode = *rec.ErrorCode
		}
		if rec.ErrorMessage != nil {
			item.ErrorMessage = *rec.ErrorMessage
		}
		if rec.User != nil {
			item.UserName = rec.User.FullName
			item.UserEmail = rec.User.Email
		}

		items = append(items, item)
	}

	return &dtos.DLQMessagesResponse{
		Total:       total,
		FailedCount: failedCount,
		StuckCount:  stuckCount,
		Page:        page,
		Limit:       limit,
		Items:       items,
	}, nil
}

// AdminListJobs retrieves paginated generation and pipeline recordings across all users and statuses.
func (s *Service) AdminListJobs(ctx context.Context, meta AdminActionMeta, query dtos.AdminJobListQuery) (*dtos.AdminJobListResponse, error) {
	query.SetDefaults()
	offset := (query.Page - 1) * query.Limit

	recs, total, err := s.repo.ListAllRecordingsForAdmin(ctx, query.Status, query.Search, query.Limit, offset)
	if err != nil {
		return nil, s.wrapError(ctx, err)
	}

	items := make([]dtos.AdminJobItem, 0, len(recs))
	for _, rec := range recs {
		item := dtos.AdminJobItem{
			ID:               rec.ID,
			Title:            rec.Title,
			OriginalFilename: rec.OriginalFilename,
			FileSizeBytes:    rec.FileSizeBytes,
			DurationSeconds:  rec.DurationSeconds,
			SourceType:       rec.SourceType,
			Status:           rec.Status,
			SelectedTemplate: rec.SelectedTemplate,
			DetectedLanguage: rec.DetectedLanguage,
			OutputLanguage:   rec.OutputLanguage,
			ErrorMessage:     rec.ErrorMessage,
			ErrorCode:        rec.ErrorCode,
			IsGuest:          rec.IsGuest,
			CreatedAt:        rec.CreatedAt,
			UpdatedAt:        rec.UpdatedAt,
		}
		if rec.User != nil {
			item.UserName = &rec.User.FullName
			item.UserEmail = &rec.User.Email
		}
		items = append(items, item)
	}

	totalPages := 0
	if query.Limit > 0 {
		totalPages = int((total + int64(query.Limit) - 1) / int64(query.Limit))
	}

	return &dtos.AdminJobListResponse{
		Items:      items,
		Total:      total,
		Page:       query.Page,
		Limit:      query.Limit,
		TotalPages: totalPages,
	}, nil
}

func resolveDLQQueueAndTopic(status, sourceType string) (string, string) {
	switch status {
	case models.RecordingStatusQueued, models.RecordingStatusExtracting:
		if sourceType == constants.RecordingSourceTypeLink {
			return constants.TopicRecordingImport, constants.TopicRecordingImport
		}
		return constants.TopicRecordingExtract, constants.TopicRecordingExtract
	case models.RecordingStatusTranscribing:
		return constants.TopicRecordingTranscribe, constants.TopicRecordingTranscribe
	case models.RecordingStatusSummarizing:
		return constants.TopicRecordingSummarize, constants.TopicRecordingSummarize
	case models.RecordingStatusIndexing:
		return constants.TopicRecordingIndex, constants.TopicRecordingIndex
	case models.RecordingStatusFailed:
		return "recording.dlq", constants.TopicRecordingFailed
	default:
		return "pipeline.unknown", "pipeline.unknown"
	}
}

// AdminRetryDLQJob reprocesses a dead-lettered or stuck pipeline job, recovers state, and logs the administrative action.
func (s *Service) AdminRetryDLQJob(ctx context.Context, meta AdminActionMeta, req dtos.DLQRetryRequest) (*dtos.DLQRetryResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, constants.ErrBadRequest.WithMessage(err.Error())
	}

	rec, err := s.repo.FindRecordingByID(ctx, req.RecordingID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrRecordingNotFound
		}
		return nil, s.wrapError(ctx, err)
	}

	if strings.ToUpper(strings.TrimSpace(rec.Status)) == models.RecordingStatusCompleted {
		return nil, constants.ErrRecordingAlreadyCompleted
	}

	previousStatus := rec.Status
	previousStage := req.Stage

	// If job was stuck in non-terminal processing state, reset status to FAILED so resume can proceed
	if rec.Status != models.RecordingStatusFailed {
		rec.Status = models.RecordingStatusFailed
	}

	// Execute smart pipeline resume
	p, err := s.ResumeRecordingPipeline(ctx, rec)
	if err != nil {
		return nil, s.wrapError(ctx, err)
	}

	status := p.Status
	if status == "" {
		status = models.RecordingStatusQueued
	}
	stage := p.Stage
	if stage == "" {
		stage = models.RecordingStatusExtracting
	}

	// Persist admin audit log
	adminIDVal := meta.AdminID
	entityIDStr := req.RecordingID.String()
	var ip *string
	var ua *string
	if meta.IPAddress != "" {
		ip = &meta.IPAddress
	}
	if meta.UserAgent != "" {
		ua = &meta.UserAgent
	}

	auditLog := &models.AdminAuditLog{
		AdminID:  &adminIDVal,
		Action:   "pipeline.dlq_retry",
		Entity:   "recording",
		EntityID: &entityIDStr,
		Payload: models.JSONMap{
			"previous_status": previousStatus,
			"requested_stage": previousStage,
			"resumed_status":  status,
			"resumed_stage":   stage,
		},
		IPAddress: ip,
		UserAgent: ua,
	}
	if err := s.repo.CreateAdminAuditLog(ctx, auditLog); err != nil {
		return nil, s.wrapError(ctx, err)
	}

	return &dtos.DLQRetryResponse{
		RecordingID: rec.ID,
		Status:      status,
		Stage:       stage,
		Message:     "pipeline job retry initiated successfully",
		RetriedAt:   time.Now().UTC(),
	}, nil
}

// IsMaintenanceMode checks if maintenance mode is enabled in Redis/in-memory config.
func (s *Service) IsMaintenanceMode(ctx context.Context) bool {
	if s == nil || s.repo == nil {
		return false
	}
	return s.repo.IsMaintenanceMode(ctx)
}

// AdminGetSystemConfig retrieves current system flags and maintenance status.
func (s *Service) AdminGetSystemConfig(ctx context.Context, meta AdminActionMeta) (*dtos.SystemConfigResponse, error) {
	cfg, err := s.repo.GetSystemConfig(ctx)
	if err != nil {
		return nil, s.wrapError(ctx, err)
	}

	return &dtos.SystemConfigResponse{
		MaintenanceMode:    cfg.MaintenanceMode,
		AllowGuestUploads:  cfg.AllowGuestUploads,
		BotWaitlistEnabled: cfg.BotWaitlistEnabled,
	}, nil
}

// AdminUpdateSystemConfig updates system flags and persists an audit log.
func (s *Service) AdminUpdateSystemConfig(ctx context.Context, meta AdminActionMeta, req dtos.UpdateSystemConfigRequest) (*dtos.SystemConfigResponse, error) {
	cfg, err := s.repo.UpdateSystemConfig(ctx, req.MaintenanceMode, req.AllowGuestUploads, req.BotWaitlistEnabled)
	if err != nil {
		return nil, s.wrapError(ctx, err)
	}

	// Persist admin audit log
	adminIDVal := meta.AdminID
	var ip *string
	var ua *string
	if meta.IPAddress != "" {
		ip = &meta.IPAddress
	}
	if meta.UserAgent != "" {
		ua = &meta.UserAgent
	}

	payload := models.JSONMap{
		"maintenance_mode":     cfg.MaintenanceMode,
		"allow_guest_uploads":  cfg.AllowGuestUploads,
		"bot_waitlist_enabled": cfg.BotWaitlistEnabled,
	}

	auditLog := &models.AdminAuditLog{
		AdminID:   &adminIDVal,
		Action:    "config.update_flags",
		Entity:    "system_config",
		Payload:   payload,
		IPAddress: ip,
		UserAgent: ua,
	}
	if err := s.repo.CreateAdminAuditLog(ctx, auditLog); err != nil {
		return nil, s.wrapError(ctx, err)
	}

	return &dtos.SystemConfigResponse{
		MaintenanceMode:    cfg.MaintenanceMode,
		AllowGuestUploads:  cfg.AllowGuestUploads,
		BotWaitlistEnabled: cfg.BotWaitlistEnabled,
	}, nil
}

// AdminListReports queries moderation reports with preloaded target and handler metadata.
func (s *Service) AdminListReports(ctx context.Context, q dtos.AdminReportListQuery) (*dtos.AdminReportListResponse, error) {
	q.SetDefaults()

	offset := (q.Page - 1) * q.Limit
	reports, total, err := s.repo.ListReportsByStatus(ctx, q.Status, q.Limit, offset)
	if err != nil {
		return nil, s.wrapError(ctx, err)
	}

	items := make([]dtos.AdminReportListItem, 0, len(reports))
	for _, r := range reports {
		item := dtos.AdminReportListItem{
			ID:             r.ID,
			RecordingID:    r.RecordingID,
			ReporterType:   r.ReporterType,
			ReporterRef:    r.ReporterRef,
			Reason:         r.Reason,
			Status:         r.Status,
			ResolutionNote: r.ResolutionNote,
			HandledBy:      r.HandledBy,
			CreatedAt:      r.CreatedAt,
			UpdatedAt:      r.UpdatedAt,
		}
		if r.Recording != nil {
			item.RecordingTitle = r.Recording.Title
		}
		if r.Handler != nil {
			handlerName := r.Handler.FullName
			if handlerName == "" {
				handlerName = r.Handler.Username
			}
			item.HandlerName = &handlerName
		}
		items = append(items, item)
	}

	totalPages := int(math.Ceil(float64(total) / float64(q.Limit)))
	if totalPages < 1 {
		totalPages = 1
	}

	return &dtos.AdminReportListResponse{
		Items: items,
		Pagination: dtos.PaginationMeta{
			CurrentPage: q.Page,
			PageSize:    q.Limit,
			TotalItems:  total,
			TotalPages:  totalPages,
		},
	}, nil
}

// AdminResolveReport executes administrative resolution on a reported recording ticket.
func (s *Service) AdminResolveReport(ctx context.Context, meta AdminActionMeta, reportID uuid.UUID, req dtos.ResolveReportRequest) (*dtos.ResolveReportResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, constants.ErrBadRequest.WithMessage(err.Error())
	}

	report, err := s.repo.FindReportByID(ctx, reportID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrNotFound.WithMessage("report ticket not found")
		}
		return nil, s.wrapError(ctx, err)
	}

	var status string
	switch req.Action {
	case "DISMISS":
		status = "dismissed"
	case "SUSPEND_RECORDING":
		status = "resolved"
		if report.RecordingID != uuid.Nil {
			if err := s.repo.UpdateRecordingShareSettings(ctx, report.RecordingID, false, nil); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, s.wrapError(ctx, err)
			}
		}
	case "BAN_USER":
		status = "resolved"
		if report.Recording != nil && report.Recording.UserID != nil {
			if err := s.repo.UpdateUserStatus(ctx, *report.Recording.UserID, models.UserStatusSuspended); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, s.wrapError(ctx, err)
			}
		}
		if report.RecordingID != uuid.Nil {
			if err := s.repo.UpdateRecordingShareSettings(ctx, report.RecordingID, false, nil); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, s.wrapError(ctx, err)
			}
		}
	}

	resolvedAt := time.Now()
	var notePtr *string
	if req.ResolutionNote != "" {
		notePtr = &req.ResolutionNote
	}

	if err := s.repo.UpdateReportResolution(ctx, reportID, status, notePtr, meta.AdminID); err != nil {
		return nil, s.wrapError(ctx, err)
	}

	// Persist admin audit log
	adminIDVal := meta.AdminID
	var ip *string
	var ua *string
	if meta.IPAddress != "" {
		ip = &meta.IPAddress
	}
	if meta.UserAgent != "" {
		ua = &meta.UserAgent
	}

	payload := models.JSONMap{
		"report_id":       reportID.String(),
		"recording_id":    report.RecordingID.String(),
		"action":          req.Action,
		"resolution_note": req.ResolutionNote,
		"status":          status,
	}

	entityIDStr := reportID.String()
	auditLog := &models.AdminAuditLog{
		AdminID:   &adminIDVal,
		Action:    "report.resolve",
		Entity:    "report",
		EntityID:  &entityIDStr,
		Payload:   payload,
		IPAddress: ip,
		UserAgent: ua,
	}
	if err := s.repo.CreateAdminAuditLog(ctx, auditLog); err != nil {
		return nil, s.wrapError(ctx, err)
	}

	return &dtos.ResolveReportResponse{
		ReportID:       reportID,
		Status:         status,
		Action:         req.Action,
		HandledBy:      meta.AdminID,
		ResolutionNote: req.ResolutionNote,
		ResolvedAt:     resolvedAt,
	}, nil
}

// AdminListAuditLogs retrieves a paginated and filtered audit trail of administrative actions.
func (s *Service) AdminListAuditLogs(ctx context.Context, q dtos.AdminAuditLogListQuery) (*dtos.AdminAuditLogListResponse, error) {
	q.SetDefaults()

	offset := (q.Page - 1) * q.Limit
	filter := repositories.AdminAuditLogFilter{
		Action:  q.Action,
		Entity:  q.Entity,
		AdminID: q.AdminID,
		Limit:   q.Limit,
		Offset:  offset,
	}

	logs, total, err := s.repo.ListAdminAuditLogsFiltered(ctx, filter)
	if err != nil {
		return nil, s.wrapError(ctx, err)
	}

	items := make([]dtos.AdminAuditLogListItem, 0, len(logs))
	for _, l := range logs {
		item := dtos.AdminAuditLogListItem{
			ID:        l.ID,
			AdminID:   l.AdminID,
			Action:    l.Action,
			Entity:    l.Entity,
			EntityID:  l.EntityID,
			Payload:   map[string]interface{}(l.Payload),
			IPAddress: l.IPAddress,
			UserAgent: l.UserAgent,
			CreatedAt: l.CreatedAt,
		}
		if l.Admin != nil {
			u := l.Admin.Username
			item.AdminUsername = &u
			fn := l.Admin.FullName
			item.AdminFullName = &fn
		}
		items = append(items, item)
	}

	totalPages := int(math.Ceil(float64(total) / float64(q.Limit)))
	if totalPages < 1 {
		totalPages = 1
	}

	return &dtos.AdminAuditLogListResponse{
		Items: items,
		Pagination: dtos.PaginationMeta{
			CurrentPage: q.Page,
			PageSize:    q.Limit,
			TotalItems:  total,
			TotalPages:  totalPages,
		},
	}, nil
}

// AdminGetOverviewStats returns system-wide metrics across users, recordings, and storage.
func (s *Service) AdminGetOverviewStats(ctx context.Context) (*dtos.SystemOverviewStatsResponse, error) {
	stats, err := s.repo.GetSystemOverviewStats(ctx)
	if err != nil {
		return nil, s.wrapError(ctx, err)
	}

	return &dtos.SystemOverviewStatsResponse{
		TotalUsers:           stats.TotalUsers,
		ActiveUsers:          stats.ActiveUsers,
		TotalRecordings:      stats.TotalRecordings,
		CompletedRecordings:  stats.CompletedRecordings,
		FailedRecordings:     stats.FailedRecordings,
		TotalStorageBytes:    stats.TotalStorageBytes,
		TotalDurationSeconds: stats.TotalDurationSeconds,
	}, nil
}

// AdminGetCostOversight calculates estimates for transcription (STT) and intelligence (LLM) expenses.
func (s *Service) AdminGetCostOversight(ctx context.Context) (*dtos.CostOversightResponse, error) {
	stats, err := s.repo.GetSystemOverviewStats(ctx)
	if err != nil {
		return nil, s.wrapError(ctx, err)
	}

	totalMinutes := stats.TotalDurationSeconds / 60.0

	// Whisper/Deepgram STT cost benchmark: ~$0.006 per audio minute
	sttRatePerMinute := 0.006
	sttCost := math.Round(totalMinutes*sttRatePerMinute*1000) / 1000

	// LLM benchmark estimate: ~150 words/min = ~200 tokens/min input + ~100 tokens/min output = ~300 tokens/min
	// GPT-4o-mini / Gemini Flash blended cost: ~$0.0003 per 1K tokens
	estimatedTokens := int64(math.Round(totalMinutes * 300))
	llmRatePer1kTokens := 0.0003
	llmCost := math.Round((float64(estimatedTokens)/1000.0)*llmRatePer1kTokens*1000) / 1000

	totalCost := math.Round((sttCost+llmCost)*1000) / 1000

	return &dtos.CostOversightResponse{
		TotalAudioMinutes:     math.Round(totalMinutes*100) / 100,
		EstimatedSTTCostUSD:   sttCost,
		TotalLLMTokens:        estimatedTokens,
		EstimatedLLMCostUSD:   llmCost,
		TotalEstimatedCostUSD: totalCost,
		Currency:              "USD",
	}, nil
}

// AdminLogin authenticates staff administrator credentials and issues an admin access token.
func (s *Service) AdminLogin(ctx context.Context, req dtos.AdminLoginRequest, meta *AdminActionMeta) (*dtos.AdminLoginResponse, error) {
	admin, err := s.repo.FindAdminByUsername(ctx, req.Username)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrInvalidCredentials
		}
		return nil, s.wrapError(ctx, err)
	}

	if admin.Status != constants.UserStatusActive {
		return nil, constants.ErrUserInactive
	}

	if !hasher.VerifyPassword(admin.PasswordHash, req.Password) {
		return nil, constants.ErrInvalidCredentials
	}

	now := time.Now().UTC()
	if err := s.repo.UpdateAdminLastLogin(ctx, admin.ID, now); err != nil {
		return nil, s.wrapError(ctx, err)
	}

	tokenStr, err := jwt.GenerateAdminToken(s.cfg, admin.ID, admin.Username)
	if err != nil {
		return nil, s.wrapError(ctx, err)
	}

	// Record audit log
	var ipPtr, uaPtr *string
	if meta != nil {
		if meta.IPAddress != "" {
			ip := meta.IPAddress
			ipPtr = &ip
		}
		if meta.UserAgent != "" {
			ua := meta.UserAgent
			uaPtr = &ua
		}
	}
	entityIDStr := admin.ID.String()
	if err := s.repo.CreateAdminAuditLog(ctx, &models.AdminAuditLog{
		AdminID:   &admin.ID,
		Action:    "ADMIN_LOGIN",
		Entity:    "ADMIN_USER",
		EntityID:  &entityIDStr,
		IPAddress: ipPtr,
		UserAgent: uaPtr,
	}); err != nil {
		return nil, s.wrapError(ctx, err)
	}

	roleName := ""
	var perms []string
	if admin.Role != nil {
		roleName = admin.Role.Name
		perms = admin.Role.PermissionsList()
	}

	return &dtos.AdminLoginResponse{
		Token:     tokenStr,
		ExpiresAt: now.Add(s.cfg.JWTAccessExpiration),
		Admin: dtos.AdminUserDTO{
			ID:          admin.ID,
			Username:    admin.Username,
			FullName:    admin.FullName,
			RoleID:      admin.RoleID,
			RoleName:    roleName,
			Permissions: perms,
		},
	}, nil
}











