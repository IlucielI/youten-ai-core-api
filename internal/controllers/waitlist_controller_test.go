package controllers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
)

func TestControllers_JoinBotWaitlist_NilService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctrls := &Controllers{}

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/waitlist/bot", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req

	ctrls.JoinBotWaitlist(ctx)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for nil service, got %d", w.Code)
	}
}

func TestControllers_JoinBotWaitlist_InvalidJSON(t *testing.T) {
	ctrls, _, cleanup := setupTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/waitlist/bot", bytes.NewBufferString(`invalid json`))
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req

	ctrls.JoinBotWaitlist(ctx)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid JSON, got %d", w.Code)
	}
}

func TestControllers_JoinBotWaitlist_ValidationErrors(t *testing.T) {
	ctrls, _, cleanup := setupTestControllers(t)
	defer cleanup()

	tests := []struct {
		name    string
		payload string
	}{
		{
			name:    "empty email",
			payload: `{"email":"","platform":"zoom"}`,
		},
		{
			name:    "invalid email format",
			payload: `{"email":"not-an-email","platform":"zoom"}`,
		},
		{
			name:    "whitespace only email",
			payload: `{"email":"   ","platform":"zoom"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(w)
			req := httptest.NewRequest(http.MethodPost, "/v1/waitlist/bot", bytes.NewBufferString(tt.payload))
			req.Header.Set("Content-Type", "application/json")
			ctx.Request = req

			ctrls.JoinBotWaitlist(ctx)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 on validation failure, got %d", w.Code)
			}
		})
	}
}

func TestControllers_JoinBotWaitlist_ServiceError(t *testing.T) {
	ctrls, mock, cleanup := setupTestControllers(t)
	defer cleanup()

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "bot_waitlists"`).
		WithArgs("dberr@example.com", "google_meet", "1-10", "PENDING").
		WillReturnError(errors.New("db error"))
	mock.ExpectRollback()

	body := `{"email":"dberr@example.com"}`
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/waitlist/bot", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req

	ctrls.JoinBotWaitlist(ctx)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 on db error, got %d body: %s", w.Code, w.Body.String())
	}
}

func TestControllers_JoinBotWaitlist_Success(t *testing.T) {
	ctrls, mock, cleanup := setupTestControllers(t)
	defer cleanup()

	entryID := uuid.New()
	now := time.Now()

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "bot_waitlists"`).
		WithArgs("beta@company.com", "zoom", "11-50", "PENDING").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(entryID, now, now))
	mock.ExpectCommit()

	payload := dtos.WaitlistRequest{
		Email:       "Beta@Company.com",
		Platform:    "zoom",
		CompanySize: "11-50",
	}
	bodyBytes, _ := json.Marshal(payload)

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/waitlist/bot", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req

	ctrls.JoinBotWaitlist(ctx)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 on success, got %d body: %s", w.Code, w.Body.String())
	}

	var resp dtos.APIResponse[*dtos.WaitlistResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != constants.ResponseStatusSuccess {
		t.Errorf("expected status success, got %s", resp.Status)
	}
	if resp.Data == nil {
		t.Fatal("expected non-nil response data")
	}
	if resp.Data.Email != "beta@company.com" {
		t.Errorf("expected normalized email beta@company.com, got %s", resp.Data.Email)
	}
	if resp.Data.Platform != "zoom" {
		t.Errorf("expected platform zoom, got %s", resp.Data.Platform)
	}
	if resp.Data.CompanySize != "11-50" {
		t.Errorf("expected company size 11-50, got %s", resp.Data.CompanySize)
	}
	if resp.Data.Status != "PENDING" {
		t.Errorf("expected status PENDING, got %s", resp.Data.Status)
	}
}
