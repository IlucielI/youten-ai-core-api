package services

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
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



