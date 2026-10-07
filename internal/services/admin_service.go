package services

import (
	"context"
	"errors"
	"math"
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
