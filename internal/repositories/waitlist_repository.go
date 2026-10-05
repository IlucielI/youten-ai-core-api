package repositories

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"code-base-golang/internal/models"
)

// UpsertBotWaitlist inserts a waitlist applicant or updates existing entry if the email already exists.
func (r *Repositories) UpsertBotWaitlist(ctx context.Context, entry *models.BotWaitlist) (*models.BotWaitlist, error) {
	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "email"}},
			DoUpdates: clause.AssignmentColumns([]string{"platform", "company_size", "updated_at"}),
		}).
		Create(entry).Error
	if err != nil {
		return nil, err
	}
	return entry, nil
}

// FindBotWaitlistByEmail finds a waitlist applicant by email.
func (r *Repositories) FindBotWaitlistByEmail(ctx context.Context, email string) (*models.BotWaitlist, error) {
	var entry models.BotWaitlist
	err := r.db.WithContext(ctx).Where("LOWER(email) = LOWER(?)", email).First(&entry).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &entry, nil
}
