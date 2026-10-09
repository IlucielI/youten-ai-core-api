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

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
	"code-base-golang/internal/services"
)

type mockTestBotProvider struct {
	name    string
	enabled bool
}

func (p *mockTestBotProvider) ProviderName() string {
	return p.name
}

func (p *mockTestBotProvider) IsEnabled() bool {
	return p.enabled
}

func (p *mockTestBotProvider) Dispatch(ctx context.Context, params services.BotDispatchParams) (string, error) {
	return "ext-session-123", nil
}

func (p *mockTestBotProvider) GetStatus(ctx context.Context, externalSessionID string) (*services.BotProviderStatus, error) {
	return &services.BotProviderStatus{
		ExternalSessionID: externalSessionID,
		Status:            constants.BotSessionStatusRecording,
	}, nil
}

func (p *mockTestBotProvider) Stop(ctx context.Context, externalSessionID string) error {
	return nil
}

func TestControllers_DispatchMeetingBot_InvalidJSON(t *testing.T) {
	ctrls, _, cleanup := setupTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/meeting-bot", bytes.NewBufferString(`{invalid json}`))
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req

	ctrls.DispatchMeetingBot(ctx)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid JSON, got %d", w.Code)
	}
}

func TestControllers_DispatchMeetingBot_ValidationErrors(t *testing.T) {
	ctrls, _, cleanup := setupTestControllers(t)
	defer cleanup()

	tests := []struct {
		name    string
		payload string
	}{
		{
			name:    "empty provider",
			payload: `{"provider":"","channel_id":"123"}`,
		},
		{
			name:    "discord without channel_id",
			payload: `{"provider":"discord"}`,
		},
		{
			name:    "google meet without meeting_url",
			payload: `{"provider":"google_meet"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(w)
			req := httptest.NewRequest(http.MethodPost, "/v1/recordings/meeting-bot", bytes.NewBufferString(tt.payload))
			req.Header.Set("Content-Type", "application/json")
			ctx.Request = req

			ctrls.DispatchMeetingBot(ctx)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for validation error, got %d", w.Code)
			}
		})
	}
}

func TestControllers_DispatchMeetingBot_Success(t *testing.T) {
	ctrls, mock, cleanup := setupTestControllers(t)
	defer cleanup()

	provider := &mockTestBotProvider{
		name:    constants.BotProviderDiscord,
		enabled: true,
	}
	ctrls.svc.RegisterBotProvider(provider)

	// Guest quota check
	mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings"`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	recID := uuid.New()
	sessID := uuid.New()
	now := time.Now()

	// Insert recording
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "recordings"`)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(recID, now, now))
	mock.ExpectCommit()

	// Insert bot session
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "bot_sessions"`)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(sessID, now, now))
	mock.ExpectCommit()

	// Update bot session external id
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "bot_sessions" SET "external_session_id"=$1,"updated_at"=$2 WHERE id = $3`)).
		WithArgs("ext-session-123", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	// Update bot session status to recording
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "bot_sessions" SET`)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	payload := `{"provider":"discord","channel_id":"123456","guild_id":"789012","title":"Standup Meeting"}`
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/meeting-bot", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req

	ctrls.DispatchMeetingBot(ctx)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d. Body: %s", w.Code, w.Body.String())
	}

	var res dtos.APIResponse[dtos.DispatchBotResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if res.Data.Provider != constants.BotProviderDiscord {
		t.Errorf("expected provider discord, got %s", res.Data.Provider)
	}
}

func TestControllers_GetBotSessionStatus_InvalidUUID_And_Success(t *testing.T) {
	ctrls, mock, cleanup := setupTestControllers(t)
	defer cleanup()

	t.Run("invalid UUID returns 400", func(t *testing.T) {
		w := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(w)
		ctx.Params = gin.Params{{Key: "id", Value: "invalid-uuid"}}
		req := httptest.NewRequest(http.MethodGet, "/v1/recordings/meeting-bot/invalid-uuid/status", nil)
		ctx.Request = req

		ctrls.GetBotSessionStatus(ctx)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("valid session returns 200", func(t *testing.T) {
		sessID := uuid.New()
		recID := uuid.New()
		now := time.Now()

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "bot_sessions" WHERE id = $1 AND "bot_sessions"."deleted_at" IS NULL ORDER BY "bot_sessions"."id" LIMIT $2`)).
			WithArgs(sessID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "provider", "status", "created_at", "updated_at"}).
				AddRow(sessID, recID, constants.BotProviderDiscord, constants.BotSessionStatusRecording, now, now))

		w := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(w)
		ctx.Params = gin.Params{{Key: "id", Value: sessID.String()}}
		req := httptest.NewRequest(http.MethodGet, "/v1/recordings/meeting-bot/"+sessID.String()+"/status", nil)
		ctx.Request = req

		ctrls.GetBotSessionStatus(ctx)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
		}
	})
}

func TestControllers_StopBotSession_InvalidUUID_And_Success(t *testing.T) {
	ctrls, mock, cleanup := setupTestControllers(t)
	defer cleanup()

	t.Run("invalid UUID returns 400", func(t *testing.T) {
		w := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(w)
		ctx.Params = gin.Params{{Key: "id", Value: "bad-uuid"}}
		req := httptest.NewRequest(http.MethodPost, "/v1/recordings/meeting-bot/bad-uuid/stop", nil)
		ctx.Request = req

		ctrls.StopBotSession(ctx)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("valid session stops and returns 200", func(t *testing.T) {
		sessID := uuid.New()
		recID := uuid.New()
		now := time.Now()

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "bot_sessions" WHERE id = $1 AND "bot_sessions"."deleted_at" IS NULL ORDER BY "bot_sessions"."id" LIMIT $2`)).
			WithArgs(sessID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "provider", "status", "created_at", "updated_at"}).
				AddRow(sessID, recID, constants.BotProviderDiscord, constants.BotSessionStatusRecording, now, now))

		// Update bot session
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "bot_sessions" SET`)).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		// Lookup recording and update status
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "recordings" WHERE id = $1 AND "recordings"."deleted_at" IS NULL ORDER BY "recordings"."id" LIMIT $2`)).
			WithArgs(recID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).AddRow(recID, constants.BotSessionStatusRecording))

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "recordings" SET "status"=$1,"updated_at"=$2 WHERE "recordings"."deleted_at" IS NULL AND "id" = $3`)).
			WithArgs(models.RecordingStatusPending, sqlmock.AnyArg(), recID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		w := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(w)
		ctx.Params = gin.Params{{Key: "id", Value: sessID.String()}}
		req := httptest.NewRequest(http.MethodPost, "/v1/recordings/meeting-bot/"+sessID.String()+"/stop", nil)
		ctx.Request = req

		ctrls.StopBotSession(ctx)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
		}
	})
}

func TestControllers_GetCapabilities_Success(t *testing.T) {
	ctrls, _, cleanup := setupTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "/v1/capabilities", nil)
	ctx.Request = req

	ctrls.GetCapabilities(ctx)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var res dtos.APIResponse[dtos.CapabilitiesResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !res.Data.AudioUpload {
		t.Errorf("expected AudioUpload to be true")
	}
	if !res.Data.LinkImport {
		t.Errorf("expected LinkImport to be true")
	}
}

func TestControllers_HandleGoogleMeetWebhook_InvalidJSON(t *testing.T) {
	ctrls, _, cleanup := setupTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/bot/google-meet", bytes.NewBufferString(`{invalid json}`))
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req

	ctrls.HandleGoogleMeetWebhook(ctx)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid JSON, got %d", w.Code)
	}
}

func TestControllers_HandleGoogleMeetWebhook_ValidationErrors(t *testing.T) {
	ctrls, _, cleanup := setupTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/bot/google-meet", bytes.NewBufferString(`{"external_session_id":"","event":"invalid_event"}`))
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req

	ctrls.HandleGoogleMeetWebhook(ctx)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for validation error, got %d", w.Code)
	}
}

func TestControllers_HandleGoogleMeetWebhook_Success(t *testing.T) {
	ctrls, mock, cleanup := setupTestControllers(t)
	defer cleanup()

	sessID := uuid.New()
	recID := uuid.New()
	extID := "meet-12345"

	// Mock finding bot session by external_session_id
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "bot_sessions" WHERE external_session_id = $1 AND "bot_sessions"."deleted_at" IS NULL ORDER BY "bot_sessions"."id" LIMIT $2`)).
		WithArgs(extID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "external_session_id", "status"}).AddRow(sessID, recID, extID, constants.BotSessionStatusWaitingAdmit))

	// Mock updating status to RECORDING
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "bot_sessions" SET "started_at"=$1,"status"=$2,"updated_at"=$3 WHERE id = $4`)).
		WithArgs(sqlmock.AnyArg(), constants.BotSessionStatusRecording, sqlmock.AnyArg(), sessID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	body := `{"external_session_id":"meet-12345","event":"recording"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/bot/google-meet", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req

	ctrls.HandleGoogleMeetWebhook(ctx)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestControllers_HandleMSTeamsWebhook_InvalidJSON(t *testing.T) {
	ctrls, _, cleanup := setupTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/bot/ms-teams", bytes.NewBufferString(`{invalid json}`))
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req

	ctrls.HandleMSTeamsWebhook(ctx)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid JSON, got %d", w.Code)
	}
}

func TestControllers_HandleMSTeamsWebhook_ValidationErrors(t *testing.T) {
	ctrls, _, cleanup := setupTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/bot/ms-teams", bytes.NewBufferString(`{"external_session_id":"","event":"invalid_event"}`))
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req

	ctrls.HandleMSTeamsWebhook(ctx)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for validation error, got %d", w.Code)
	}
}

func TestControllers_HandleMSTeamsWebhook_Success(t *testing.T) {
	ctrls, mock, cleanup := setupTestControllers(t)
	defer cleanup()

	sessID := uuid.New()
	recID := uuid.New()
	extID := "teams-12345"

	// Mock finding bot session by external_session_id
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "bot_sessions" WHERE external_session_id = $1 AND "bot_sessions"."deleted_at" IS NULL ORDER BY "bot_sessions"."id" LIMIT $2`)).
		WithArgs(extID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "external_session_id", "status"}).AddRow(sessID, recID, extID, constants.BotSessionStatusWaitingAdmit))

	// Mock updating status to RECORDING
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "bot_sessions" SET "started_at"=$1,"status"=$2,"updated_at"=$3 WHERE id = $4`)).
		WithArgs(sqlmock.AnyArg(), constants.BotSessionStatusRecording, sqlmock.AnyArg(), sessID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	body := `{"external_session_id":"teams-12345","event":"recording_started"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/bot/ms-teams", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req

	ctrls.HandleMSTeamsWebhook(ctx)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestControllers_HandleZoomWebhook_InvalidJSON(t *testing.T) {
	ctrls, _, cleanup := setupTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/bot/zoom", bytes.NewBufferString(`{invalid json}`))
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req

	ctrls.HandleZoomWebhook(ctx)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid JSON, got %d", w.Code)
	}
}

func TestControllers_HandleZoomWebhook_ValidationErrors(t *testing.T) {
	ctrls, _, cleanup := setupTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/bot/zoom", bytes.NewBufferString(`{"external_session_id":"","event":"invalid_event"}`))
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req

	ctrls.HandleZoomWebhook(ctx)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for validation error, got %d", w.Code)
	}
}

func TestControllers_HandleZoomWebhook_Success(t *testing.T) {
	ctrls, mock, cleanup := setupTestControllers(t)
	defer cleanup()

	sessID := uuid.New()
	recID := uuid.New()
	extID := "zoom-12345"

	// Mock finding bot session by external_session_id
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "bot_sessions" WHERE external_session_id = $1 AND "bot_sessions"."deleted_at" IS NULL ORDER BY "bot_sessions"."id" LIMIT $2`)).
		WithArgs(extID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "external_session_id", "status"}).AddRow(sessID, recID, extID, constants.BotSessionStatusWaitingAdmit))

	// Mock updating status to RECORDING
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "bot_sessions" SET "started_at"=$1,"status"=$2,"updated_at"=$3 WHERE id = $4`)).
		WithArgs(sqlmock.AnyArg(), constants.BotSessionStatusRecording, sqlmock.AnyArg(), sessID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	body := `{"external_session_id":"zoom-12345","event":"recording_started"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/bot/zoom", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req

	ctrls.HandleZoomWebhook(ctx)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}


