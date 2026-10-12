package repositories

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/models"
)

// FindAdminByUsername retrieves a staff user by their username with preloaded role.
func (r *Repositories) FindAdminByUsername(ctx context.Context, username string) (*models.AdminUser, error) {
	var admin models.AdminUser
	err := r.db.WithContext(ctx).
		Preload("Role").
		Where("username = ? AND deleted_at IS NULL", username).
		First(&admin).Error
	if err != nil {
		return nil, err
	}
	return &admin, nil
}

// FindAdminByEmail retrieves a staff user by their email address with preloaded role.
func (r *Repositories) FindAdminByEmail(ctx context.Context, email string) (*models.AdminUser, error) {
	var admin models.AdminUser
	err := r.db.WithContext(ctx).
		Preload("Role").
		Where("LOWER(email) = LOWER(?) AND deleted_at IS NULL", email).
		First(&admin).Error
	if err != nil {
		return nil, err
	}
	return &admin, nil
}

// UpdateAdminPassword updates the hashed password for an admin user.
func (r *Repositories) UpdateAdminPassword(ctx context.Context, id uuid.UUID, passwordHash string) error {
	return r.db.WithContext(ctx).
		Model(&models.AdminUser{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"password_hash": passwordHash,
			"updated_at":    time.Now(),
		}).Error
}

// CreateAdminAuthToken stores a new admin authentication or recovery token.
func (r *Repositories) CreateAdminAuthToken(ctx context.Context, token *models.AdminAuthToken) error {
	return r.db.WithContext(ctx).Create(token).Error
}

// FindAdminAuthTokenByHashAndType retrieves a valid (unrevoked, unexpired) admin token by its hash and type.
func (r *Repositories) FindAdminAuthTokenByHashAndType(ctx context.Context, tokenHash string, tokenType constants.AuthTokenType) (*models.AdminAuthToken, error) {
	var token models.AdminAuthToken
	err := r.db.WithContext(ctx).
		Where("token_hash = ? AND type = ? AND revoked_at IS NULL AND expires_at > ?", tokenHash, tokenType, time.Now()).
		First(&token).Error
	if err != nil {
		return nil, err
	}
	return &token, nil
}

// RevokeAllAdminAuthTokens revokes all active tokens for an admin by token type.
func (r *Repositories) RevokeAllAdminAuthTokens(ctx context.Context, adminID uuid.UUID, tokenType constants.AuthTokenType) error {
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&models.AdminAuthToken{}).
		Where("admin_id = ? AND type = ? AND revoked_at IS NULL", adminID, tokenType).
		Update("revoked_at", now).Error
}

// RevokeAdminAuthToken revokes a specific admin token by its ID.
func (r *Repositories) RevokeAdminAuthToken(ctx context.Context, id uuid.UUID) error {
	now := time.Now()
	res := r.db.WithContext(ctx).
		Model(&models.AdminAuthToken{}).
		Where("id = ?", id).
		Update("revoked_at", now)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// RotateAdminAuthToken atomically revokes the old token and inserts the new token within a database transaction.
func (r *Repositories) RotateAdminAuthToken(ctx context.Context, revokeTokenID uuid.UUID, newToken *models.AdminAuthToken) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		res := tx.Model(&models.AdminAuthToken{}).Where("id = ? AND revoked_at IS NULL", revokeTokenID).Update("revoked_at", now)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return tx.Create(newToken).Error
	})
}

// FindAdminByID retrieves a staff user by their UUID with preloaded role.
func (r *Repositories) FindAdminByID(ctx context.Context, id uuid.UUID) (*models.AdminUser, error) {
	var admin models.AdminUser
	err := r.db.WithContext(ctx).
		Preload("Role").
		Where("id = ? AND deleted_at IS NULL", id).
		First(&admin).Error
	if err != nil {
		return nil, err
	}
	return &admin, nil
}

// CreateAdmin creates a new staff user.
func (r *Repositories) CreateAdmin(ctx context.Context, admin *models.AdminUser) error {
	return r.db.WithContext(ctx).Create(admin).Error
}

// UpdateAdminLastLogin updates the last login timestamp for an admin user.
func (r *Repositories) UpdateAdminLastLogin(ctx context.Context, id uuid.UUID, lastLoginAt time.Time) error {
	return r.db.WithContext(ctx).
		Model(&models.AdminUser{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Update("last_login_at", lastLoginAt).Error
}
