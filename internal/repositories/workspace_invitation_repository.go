package repositories

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/models"
)

// CreateInvitation persists a new workspace invitation.
func (r *Repositories) CreateInvitation(ctx context.Context, inv *models.WorkspaceInvitation) error {
	return r.db.WithContext(ctx).Create(inv).Error
}

// FindInvitationByToken retrieves an invitation by its token, preloading Workspace and InvitedBy.
func (r *Repositories) FindInvitationByToken(ctx context.Context, token string) (*models.WorkspaceInvitation, error) {
	var inv models.WorkspaceInvitation
	err := r.db.WithContext(ctx).
		Preload("Workspace").
		Preload("InvitedBy").
		First(&inv, "invite_token = ?", token).Error
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

// FindInvitationByID retrieves an invitation by its UUID.
func (r *Repositories) FindInvitationByID(ctx context.Context, id uuid.UUID) (*models.WorkspaceInvitation, error) {
	var inv models.WorkspaceInvitation
	err := r.db.WithContext(ctx).First(&inv, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

// FindPendingInvitationsByWorkspaceID returns active unaccepted and non-expired invites.
func (r *Repositories) FindPendingInvitationsByWorkspaceID(ctx context.Context, workspaceID uuid.UUID) ([]models.WorkspaceInvitation, error) {
	var list []models.WorkspaceInvitation
	err := r.db.WithContext(ctx).
		Preload("InvitedBy").
		Where("workspace_id = ? AND accepted_at IS NULL AND expires_at > ?", workspaceID, time.Now()).
		Order("created_at DESC").
		Find(&list).Error
	return list, err
}

// FindPendingInvitationByWorkspaceAndEmail returns pending unaccepted invite for a specific email in a workspace.
func (r *Repositories) FindPendingInvitationByWorkspaceAndEmail(ctx context.Context, workspaceID uuid.UUID, email string) (*models.WorkspaceInvitation, error) {
	var inv models.WorkspaceInvitation
	err := r.db.WithContext(ctx).
		Where("workspace_id = ? AND LOWER(email) = LOWER(?) AND accepted_at IS NULL AND expires_at > ?", workspaceID, email, time.Now()).
		First(&inv).Error
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

// DeleteInvitation soft-deletes an invitation by its ID.
func (r *Repositories) DeleteInvitation(ctx context.Context, id uuid.UUID) error {
	res := r.db.WithContext(ctx).Delete(&models.WorkspaceInvitation{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// AcceptInvitationInTransaction marks the invitation as accepted and creates the workspace member record atomically.
func (r *Repositories) AcceptInvitationInTransaction(ctx context.Context, invID uuid.UUID, member *models.WorkspaceMember) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		res := tx.Model(&models.WorkspaceInvitation{}).
			Where("id = ? AND accepted_at IS NULL", invID).
			Update("accepted_at", now)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}

		return tx.Create(member).Error
	})
}
