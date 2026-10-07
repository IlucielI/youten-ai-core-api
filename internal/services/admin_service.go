package services

import (
	"context"
	"math"

	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
	"code-base-golang/internal/repositories"
)

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
