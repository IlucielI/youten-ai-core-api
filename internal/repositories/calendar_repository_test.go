package repositories

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	gormPostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/models"
)

func setupMockCalendarDB(t *testing.T) (*Repositories, sqlmock.Sqlmock, func()) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}

	gormDB, err := gorm.Open(gormPostgres.New(gormPostgres.Config{
		Conn: sqlDB,
	}), &gorm.Config{})
	if err != nil {
		sqlDB.Close()
		t.Fatalf("failed to open gorm db: %v", err)
	}

	repo := New(gormDB)
	cleanup := func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("there were unfulfilled expectations: %s", err)
		}
		sqlDB.Close()
	}
	return repo, mock, cleanup
}

func TestRepositories_CreateGoogleCalendarConnection(t *testing.T) {
	repo, mock, cleanup := setupMockCalendarDB(t)
	defer cleanup()

	userID := uuid.New()
	connID := uuid.New()
	now := time.Now().UTC()

	conn := &models.UserGoogleCalendarConnection{
		BaseModel: models.BaseModel{
			ID:        connID,
			CreatedAt: now,
			UpdatedAt: now,
		},
		UserID:                 userID,
		GoogleUserID:           "goog-123",
		GoogleEmail:            "test@example.com",
		EncryptedRefreshToken:  "enc-refresh",
		TokenExpiresAt:         now.Add(1 * time.Hour),
		IsAutoRecordEnabled:    true,
		LeadTimeMinutes:        2,
		Status:                 constants.CalendarConnectionStatusActive,
	}

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "user_google_calendar_connections"`)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(connID, now, now))
	mock.ExpectCommit()

	err := repo.CreateGoogleCalendarConnection(context.Background(), conn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRepositories_GetGoogleCalendarConnectionByUserID(t *testing.T) {
	repo, mock, cleanup := setupMockCalendarDB(t)
	defer cleanup()

	userID := uuid.New()
	connID := uuid.New()
	now := time.Now().UTC()

	rows := sqlmock.NewRows([]string{
		"id", "user_id", "google_user_id", "google_email", "encrypted_access_token", "encrypted_refresh_token",
		"token_expires_at", "sync_token", "is_auto_record_enabled", "lead_time_minutes", "status", "created_at", "updated_at",
	}).AddRow(
		connID, userID, "goog-123", "test@example.com", "enc-access", "enc-refresh",
		now.Add(1*time.Hour), "sync-123", true, 2, constants.CalendarConnectionStatusActive, now, now,
	)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "user_google_calendar_connections" WHERE user_id = $1`)).
		WithArgs(userID, 1).
		WillReturnRows(rows)

	res, err := repo.GetGoogleCalendarConnectionByUserID(context.Background(), userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.GoogleEmail != "test@example.com" {
		t.Errorf("expected email test@example.com, got %s", res.GoogleEmail)
	}
}

func TestRepositories_DeleteGoogleCalendarConnection(t *testing.T) {
	repo, mock, cleanup := setupMockCalendarDB(t)
	defer cleanup()

	userID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "user_google_calendar_connections" SET "deleted_at"=$1 WHERE user_id = $2 AND "user_google_calendar_connections"."deleted_at" IS NULL`)).
		WithArgs(sqlmock.AnyArg(), userID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := repo.DeleteGoogleCalendarConnection(context.Background(), userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRepositories_ToggleEventAutoRecord(t *testing.T) {
	repo, mock, cleanup := setupMockCalendarDB(t)
	defer cleanup()

	eventID := uuid.New()
	userID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "scheduled_calendar_events" SET "auto_record_enabled"=$1,"updated_at"=$2 WHERE (id = $3 AND user_id = $4) AND "scheduled_calendar_events"."deleted_at" IS NULL`)).
		WithArgs(false, sqlmock.AnyArg(), eventID, userID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := repo.ToggleEventAutoRecord(context.Background(), eventID, userID, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRepositories_ClaimEventForDispatch(t *testing.T) {
	repo, mock, cleanup := setupMockCalendarDB(t)
	defer cleanup()

	eventID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "scheduled_calendar_events" SET "dispatch_status"=$1,"updated_at"=$2 WHERE (id = $3 AND dispatch_status = $4 AND auto_record_enabled = $5) AND "scheduled_calendar_events"."deleted_at" IS NULL`)).
		WithArgs(constants.CalendarDispatchStatusDispatching, sqlmock.AnyArg(), eventID, constants.CalendarDispatchStatusPending, true).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	claimed, err := repo.ClaimEventForDispatch(context.Background(), eventID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !claimed {
		t.Error("expected event to be claimed successfully")
	}
}

func TestRepositories_MarkEventDispatched(t *testing.T) {
	repo, mock, cleanup := setupMockCalendarDB(t)
	defer cleanup()

	eventID := uuid.New()
	botSessionID := uuid.New()
	recordingID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "scheduled_calendar_events" SET`)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := repo.MarkEventDispatched(context.Background(), eventID, botSessionID, recordingID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
