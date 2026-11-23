package services_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	gormPostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"

	"code-base-golang/internal/config"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/pkg/apperror"
	"code-base-golang/internal/pkg/crypto"
	"code-base-golang/internal/repositories"
	"code-base-golang/internal/services"
)

type mockGoogleCalendarClient struct {
	authURL        string
	tokenResult    *services.CalendarOAuthTokenResult
	userInfo       *services.CalendarUserInfo
	exchangeErr    error
	refreshResult  *services.CalendarOAuthTokenResult
	refreshErr     error
	syncResult     *services.CalendarSyncResult
	syncErr        error
}

func (m *mockGoogleCalendarClient) GetAuthURL(state string) string {
	if m.authURL != "" {
		return m.authURL
	}
	return "https://accounts.google.com/o/oauth2/v2/auth?state=" + state
}

func (m *mockGoogleCalendarClient) ExchangeCode(ctx context.Context, code string) (*services.CalendarOAuthTokenResult, *services.CalendarUserInfo, error) {
	if m.exchangeErr != nil {
		return nil, nil, m.exchangeErr
	}
	return m.tokenResult, m.userInfo, nil
}

func (m *mockGoogleCalendarClient) RefreshToken(ctx context.Context, refreshToken string) (*services.CalendarOAuthTokenResult, error) {
	if m.refreshErr != nil {
		return nil, m.refreshErr
	}
	return m.refreshResult, nil
}

func (m *mockGoogleCalendarClient) FetchEvents(ctx context.Context, accessToken string, syncToken string, timeMin time.Time) (*services.CalendarSyncResult, error) {
	if m.syncErr != nil {
		return nil, m.syncErr
	}
	return m.syncResult, nil
}

func setupTestCalendarService(t *testing.T) (*services.Service, sqlmock.Sqlmock, *mockGoogleCalendarClient, func()) {
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

	cfg := config.Config{
		CalendarTokenEncryptionKey: "test-calendar-secret-encryption-key-32b",
		UserDailyQuota:             10,
	}
	repo := repositories.New(gormDB)
	mockClient := &mockGoogleCalendarClient{}

	svc := services.New(cfg, repo, nil)
	svc.SetGoogleCalendarClient(mockClient)

	cleanup := func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("there were unfulfilled expectations: %s", err)
		}
		sqlDB.Close()
	}

	return svc, mock, mockClient, cleanup
}

func TestService_GetGoogleCalendarAuthURL(t *testing.T) {
	svc, _, _, cleanup := setupTestCalendarService(t)
	defer cleanup()

	userID := uuid.New()
	res, err := svc.GetGoogleCalendarAuthURL(context.Background(), userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.AuthURL == "" || res.State == "" {
		t.Errorf("expected non-empty AuthURL and State, got %+v", res)
	}

	// Verify state decodes to user ID
	dec, decErr := crypto.Decrypt(res.State, "test-calendar-secret-encryption-key-32b")
	if decErr != nil {
		t.Fatalf("failed to decrypt state: %v", decErr)
	}
	if len(dec) < len(userID.String()) || dec[:len(userID.String())] != userID.String() {
		t.Errorf("decrypted state does not start with userID: %s", dec)
	}
}

func TestService_GetGoogleCalendarAuthURL_NoClient(t *testing.T) {
	svc, _, _, cleanup := setupTestCalendarService(t)
	defer cleanup()

	svc.SetGoogleCalendarClient(nil)
	_, err := svc.GetGoogleCalendarAuthURL(context.Background(), uuid.New())
	if err == nil {
		t.Error("expected error when client is nil, got nil")
	}
	var appErr *apperror.AppError
	if ok := errors.As(err, &appErr); ok && appErr.HTTPStatus != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", appErr.HTTPStatus)
	}
}

func TestService_GetGoogleCalendarStatus_NotConnected(t *testing.T) {
	svc, mock, _, cleanup := setupTestCalendarService(t)
	defer cleanup()

	userID := uuid.New()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "user_google_calendar_connections" WHERE user_id = $1`)).
		WithArgs(userID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	res, err := svc.GetGoogleCalendarStatus(context.Background(), userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Connected {
		t.Error("expected connected to be false")
	}
}

func TestService_HandleGoogleCalendarCallback_InvalidState(t *testing.T) {
	svc, _, _, cleanup := setupTestCalendarService(t)
	defer cleanup()

	userID := uuid.New()
	req := dtos.GoogleCalendarCallbackRequest{
		Code:  "valid-code",
		State: "invalid-or-tampered-state",
	}

	_, err := svc.HandleGoogleCalendarCallback(context.Background(), userID, req)
	if err == nil {
		t.Error("expected error on invalid state, got nil")
	}
}

func TestService_HandleGoogleCalendarCallback_SuccessNew(t *testing.T) {
	svc, mock, mockClient, cleanup := setupTestCalendarService(t)
	defer cleanup()

	userID := uuid.New()
	statePayload := fmt.Sprintf("%s:%d", userID.String(), time.Now().Unix())
	validState, _ := crypto.Encrypt(statePayload, "test-calendar-secret-encryption-key-32b")

	mockClient.tokenResult = &services.CalendarOAuthTokenResult{
		AccessToken:  "mock-access-token",
		RefreshToken: "mock-refresh-token",
		ExpiresIn:    1 * time.Hour,
		TokenType:    "Bearer",
	}
	mockClient.userInfo = &services.CalendarUserInfo{
		ID:    "goog-usr-123",
		Email: "user@example.com",
		Name:  "Example User",
	}

	// 1. Check existing connection -> RecordNotFound
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "user_google_calendar_connections" WHERE user_id = $1`)).
		WithArgs(userID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	// 2. Create connection
	now := time.Now().UTC()
	connID := uuid.New()
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "user_google_calendar_connections"`)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(connID, now, now))
	mock.ExpectCommit()

	req := dtos.GoogleCalendarCallbackRequest{
		Code:  "google-auth-code-123",
		State: validState,
	}

	res, err := svc.HandleGoogleCalendarCallback(context.Background(), userID, req)
	if err != nil {
		t.Fatalf("unexpected callback error: %v", err)
	}

	if !res.Connected || res.GoogleEmail != "user@example.com" {
		t.Errorf("unexpected callback response: %+v", res)
	}
}

func TestService_DisconnectGoogleCalendar(t *testing.T) {
	svc, mock, _, cleanup := setupTestCalendarService(t)
	defer cleanup()

	userID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "user_google_calendar_connections" SET "deleted_at"=$1 WHERE user_id = $2 AND "user_google_calendar_connections"."deleted_at" IS NULL`)).
		WithArgs(sqlmock.AnyArg(), userID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := svc.DisconnectGoogleCalendar(context.Background(), userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestService_ToggleEventAutoRecord(t *testing.T) {
	svc, mock, _, cleanup := setupTestCalendarService(t)
	defer cleanup()

	eventID := uuid.New()
	userID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "scheduled_calendar_events" SET "auto_record_enabled"=$1,"updated_at"=$2 WHERE (id = $3 AND user_id = $4) AND "scheduled_calendar_events"."deleted_at" IS NULL`)).
		WithArgs(false, sqlmock.AnyArg(), eventID, userID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := svc.ToggleEventAutoRecord(context.Background(), eventID, userID, dtos.ToggleEventAutoRecordRequest{AutoRecordEnabled: false})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
