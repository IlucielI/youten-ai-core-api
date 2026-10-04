package repositories

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/models"
)

// CreateAuthToken stores a new authentication or verification token.
func (r *Repositories) CreateAuthToken(ctx context.Context, token *models.AuthToken) error {
	return r.db.WithContext(ctx).Create(token).Error
}

// FindAuthTokenByHashAndType retrieves a valid (unrevoked, unexpired) token by its hash and type.
func (r *Repositories) FindAuthTokenByHashAndType(ctx context.Context, tokenHash string, tokenType string) (*models.AuthToken, error) {
	var token models.AuthToken
	err := r.db.WithContext(ctx).
		Where("token_hash = ? AND type = ? AND revoked_at IS NULL AND expires_at > ?", tokenHash, tokenType, time.Now()).
		First(&token).Error
	if err != nil {
		return nil, err
	}
	return &token, nil
}

// RevokeAuthToken revokes a specific token by its ID.
func (r *Repositories) RevokeAuthToken(ctx context.Context, id uuid.UUID) error {
	now := time.Now()
	res := r.db.WithContext(ctx).Model(&models.AuthToken{}).Where("id = ?", id).Update("revoked_at", now)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// RevokeAllAuthTokensByUserID revokes all active tokens for a user, optionally filtered by token type.
func (r *Repositories) RevokeAllAuthTokensByUserID(ctx context.Context, userID uuid.UUID, tokenType string) error {
	now := time.Now()
	query := r.db.WithContext(ctx).Model(&models.AuthToken{}).Where("user_id = ? AND revoked_at IS NULL", userID)
	if tokenType != "" {
		query = query.Where("type = ?", tokenType)
	}
	return query.Update("revoked_at", now).Error
}

// DeleteExpiredAuthTokens removes permanently expired tokens to keep the table clean.
func (r *Repositories) DeleteExpiredAuthTokens(ctx context.Context) error {
	return r.db.WithContext(ctx).Where("expires_at < ?", time.Now()).Delete(&models.AuthToken{}).Error
}
