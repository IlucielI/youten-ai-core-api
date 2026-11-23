package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/dtos"
	"code-base-golang/internal/pkg/ctxmeta"
	"code-base-golang/internal/services"
)

type dummyCalendarClient struct{}

func (d *dummyCalendarClient) GetAuthURL(state string) string {
	return "https://accounts.google.com/o/oauth2/v2/auth?state=" + state
}

func (d *dummyCalendarClient) ExchangeCode(ctx context.Context, code string) (*services.CalendarOAuthTokenResult, *services.CalendarUserInfo, error) {
	return &services.CalendarOAuthTokenResult{
		AccessToken:  "access",
		RefreshToken: "refresh",
		ExpiresIn:    time.Hour,
	}, &services.CalendarUserInfo{ID: "usr-1", Email: "usr@example.com"}, nil
}

func (d *dummyCalendarClient) RefreshToken(ctx context.Context, refreshToken string) (*services.CalendarOAuthTokenResult, error) {
	return &services.CalendarOAuthTokenResult{AccessToken: "acc", ExpiresIn: time.Hour}, nil
}

func (d *dummyCalendarClient) FetchEvents(ctx context.Context, accessToken string, syncToken string, timeMin time.Time) (*services.CalendarSyncResult, error) {
	return &services.CalendarSyncResult{Events: nil, NextSyncToken: "next"}, nil
}

func TestController_GetGoogleCalendarAuthURL(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	ctrls.Service().SetGoogleCalendarClient(&dummyCalendarClient{})
	userID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "/v1/integrations/google-calendar/auth-url", nil)
	req = req.WithContext(ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID}))
	c.Request = req

	ctrls.GetGoogleCalendarAuthURL(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var resp dtos.APIResponse[dtos.GoogleCalendarAuthURLResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Data.AuthURL == "" || resp.Data.State == "" {
		t.Errorf("unexpected empty auth url or state: %+v", resp.Data)
	}
}

func TestController_GetGoogleCalendarStatus_NotConnected(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	ctrls.Service().SetGoogleCalendarClient(&dummyCalendarClient{})
	userID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "/v1/integrations/google-calendar/status", nil)
	req = req.WithContext(ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID}))
	c.Request = req

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "user_google_calendar_connections" WHERE user_id = $1`)).
		WithArgs(userID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	ctrls.GetGoogleCalendarStatus(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var resp dtos.APIResponse[dtos.GoogleCalendarConnectionResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Data.Connected {
		t.Errorf("expected connected=false, got true")
	}
}

func TestController_ToggleEventAutoRecord_NotFound(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	eventID := uuid.New()
	userID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: eventID.String()}}

	bodyBytes, _ := json.Marshal(dtos.ToggleEventAutoRecordRequest{AutoRecordEnabled: true})
	req := httptest.NewRequest(http.MethodPatch, "/v1/calendar/events/"+eventID.String()+"/auto-record", bytes.NewReader(bodyBytes))
	req = req.WithContext(ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID}))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "scheduled_calendar_events" SET "auto_record_enabled"=$1,"updated_at"=$2 WHERE (id = $3 AND user_id = $4) AND "scheduled_calendar_events"."deleted_at" IS NULL`)).
		WithArgs(true, sqlmock.AnyArg(), eventID, userID).
		WillReturnResult(sqlmock.NewResult(0, 0)) // 0 rows affected -> record not found
	mock.ExpectCommit()

	ctrls.ToggleEventAutoRecord(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d: %s", w.Code, w.Body.String())
	}
}
