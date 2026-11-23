package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/models"
)

// CalendarEventFilter encapsulates query scoping parameters for scheduled calendar events.
type CalendarEventFilter struct {
	StartDate      *time.Time
	EndDate        *time.Time
	Platform       string
	DispatchStatus string
}

func (f CalendarEventFilter) scope(db *gorm.DB) *gorm.DB {
	if f.StartDate != nil {
		db = db.Where("start_time >= ?", *f.StartDate)
	}
	if f.EndDate != nil {
		db = db.Where("start_time <= ?", *f.EndDate)
	}
	if f.Platform != "" {
		db = db.Where("meeting_platform = ?", f.Platform)
	}
	if f.DispatchStatus != "" {
		db = db.Where("dispatch_status = ?", f.DispatchStatus)
	}
	return db
}

// CreateGoogleCalendarConnection stores a new Google Calendar integration record.
func (r *Repositories) CreateGoogleCalendarConnection(ctx context.Context, conn *models.UserGoogleCalendarConnection) error {
	if conn == nil {
		return errors.New("calendar connection cannot be nil")
	}
	return r.db.WithContext(ctx).Create(conn).Error
}

// GetGoogleCalendarConnectionByUserID retrieves the active Google Calendar integration for a user.
func (r *Repositories) GetGoogleCalendarConnectionByUserID(ctx context.Context, userID uuid.UUID) (*models.UserGoogleCalendarConnection, error) {
	var conn models.UserGoogleCalendarConnection
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		First(&conn).Error
	if err != nil {
		return nil, err
	}
	return &conn, nil
}

// UpdateGoogleCalendarConnection updates an existing Google Calendar integration record.
func (r *Repositories) UpdateGoogleCalendarConnection(ctx context.Context, conn *models.UserGoogleCalendarConnection) error {
	if conn == nil {
		return errors.New("calendar connection cannot be nil")
	}
	return r.db.WithContext(ctx).Save(conn).Error
}

// DeleteGoogleCalendarConnection deletes the user's Google Calendar connection.
func (r *Repositories) DeleteGoogleCalendarConnection(ctx context.Context, userID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Delete(&models.UserGoogleCalendarConnection{}).Error
}

// ListActiveGoogleCalendarConnections lists all active connections for the background sync worker.
func (r *Repositories) ListActiveGoogleCalendarConnections(ctx context.Context, limit, offset int) ([]models.UserGoogleCalendarConnection, error) {
	var conns []models.UserGoogleCalendarConnection
	err := r.db.WithContext(ctx).
		Where("status = ?", constants.CalendarConnectionStatusActive).
		Order("last_synced_at ASC NULLS FIRST").
		Limit(limit).
		Offset(offset).
		Find(&conns).Error
	if err != nil {
		return nil, err
	}
	return conns, nil
}

// UpsertScheduledCalendarEvents stores new calendar events or updates existing ones while preserving manual toggles.
func (r *Repositories) UpsertScheduledCalendarEvents(ctx context.Context, events []models.ScheduledCalendarEvent) error {
	if len(events) == 0 {
		return nil
	}

	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "user_id"},
			{Name: "google_event_id"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"title",
			"description",
			"start_time",
			"end_time",
			"scheduled_dispatch_at",
			"meeting_url",
			"meeting_platform",
			"organizer_email",
			"user_response_status",
			"updated_at",
		}),
	}).Create(&events).Error
}

// GetScheduledCalendarEventByID retrieves a single scheduled calendar event by ID.
func (r *Repositories) GetScheduledCalendarEventByID(ctx context.Context, id uuid.UUID) (*models.ScheduledCalendarEvent, error) {
	var evt models.ScheduledCalendarEvent
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&evt).Error
	if err != nil {
		return nil, err
	}
	return &evt, nil
}

// ListScheduledCalendarEvents retrieves paginated calendar events for a user with filters.
func (r *Repositories) ListScheduledCalendarEvents(ctx context.Context, userID uuid.UUID, filter CalendarEventFilter, limit, offset int) ([]models.ScheduledCalendarEvent, int64, error) {
	var events []models.ScheduledCalendarEvent
	var total int64

	base := r.db.WithContext(ctx).Model(&models.ScheduledCalendarEvent{}).
		Where("user_id = ?", userID).
		Scopes(filter.scope)

	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := base.Order("start_time ASC").
		Limit(limit).
		Offset(offset).
		Find(&events).Error
	if err != nil {
		return nil, 0, err
	}

	return events, total, nil
}

// ToggleEventAutoRecord updates the auto_record_enabled flag for an individual meeting event.
func (r *Repositories) ToggleEventAutoRecord(ctx context.Context, eventID uuid.UUID, userID uuid.UUID, enabled bool) error {
	res := r.db.WithContext(ctx).Model(&models.ScheduledCalendarEvent{}).
		Where("id = ? AND user_id = ?", eventID, userID).
		Update("auto_record_enabled", enabled)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// GetPendingDispatches retrieves events due for bot dispatch across all users.
func (r *Repositories) GetPendingDispatches(ctx context.Context, scheduledBefore time.Time, limit int) ([]models.ScheduledCalendarEvent, error) {
	var events []models.ScheduledCalendarEvent
	err := r.db.WithContext(ctx).
		Where("auto_record_enabled = ? AND dispatch_status = ? AND scheduled_dispatch_at <= ?", true, constants.CalendarDispatchStatusPending, scheduledBefore).
		Order("scheduled_dispatch_at ASC").
		Limit(limit).
		Find(&events).Error
	if err != nil {
		return nil, err
	}
	return events, nil
}

// ClaimEventForDispatch attempts to atomically transition an event from PENDING to DISPATCHING.
// Returns true if claimed, false if another worker already claimed it.
func (r *Repositories) ClaimEventForDispatch(ctx context.Context, eventID uuid.UUID) (bool, error) {
	res := r.db.WithContext(ctx).Model(&models.ScheduledCalendarEvent{}).
		Where("id = ? AND dispatch_status = ? AND auto_record_enabled = ?", eventID, constants.CalendarDispatchStatusPending, true).
		Update("dispatch_status", constants.CalendarDispatchStatusDispatching)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// MarkEventDispatched updates a scheduled event with its associated bot session and recording.
func (r *Repositories) MarkEventDispatched(ctx context.Context, eventID uuid.UUID, botSessionID uuid.UUID, recordingID uuid.UUID) error {
	updates := map[string]interface{}{
		"dispatch_status": constants.CalendarDispatchStatusDispatched,
		"bot_session_id":  botSessionID,
		"recording_id":    recordingID,
		"updated_at":      time.Now().UTC(),
	}
	return r.db.WithContext(ctx).Model(&models.ScheduledCalendarEvent{}).
		Where("id = ?", eventID).
		Updates(updates).Error
}

// MarkEventSkippedOrFailed updates a scheduled event status to skipped or failed with an error message.
func (r *Repositories) MarkEventSkippedOrFailed(ctx context.Context, eventID uuid.UUID, status string, errorMsg *string) error {
	updates := map[string]interface{}{
		"dispatch_status": status,
		"error_message":   errorMsg,
		"updated_at":      time.Now().UTC(),
	}
	return r.db.WithContext(ctx).Model(&models.ScheduledCalendarEvent{}).
		Where("id = ?", eventID).
		Updates(updates).Error
}

