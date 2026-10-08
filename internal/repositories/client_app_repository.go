package repositories

import (
	"context"

	"github.com/google/uuid"

	"code-base-golang/internal/models"
)

// FindClientAppByID retrieves a client app by its UUID.
func (r *Repositories) FindClientAppByID(ctx context.Context, id uuid.UUID) (*models.ClientApp, error) {
	var app models.ClientApp
	if err := r.db.WithContext(ctx).First(&app, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &app, nil
}

// FindClientAppByClientID retrieves a client app by its unique client_id.
func (r *Repositories) FindClientAppByClientID(ctx context.Context, clientID string) (*models.ClientApp, error) {
	var app models.ClientApp
	if err := r.db.WithContext(ctx).Where("client_id = ?", clientID).First(&app).Error; err != nil {
		return nil, err
	}
	return &app, nil
}

// CreateClientApp persists a new client application into the database.
func (r *Repositories) CreateClientApp(ctx context.Context, app *models.ClientApp) error {
	return r.db.WithContext(ctx).Create(app).Error
}
