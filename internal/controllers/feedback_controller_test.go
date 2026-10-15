package controllers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"code-base-golang/internal/dtos"
)

func TestControllers_SubmitFeedback_InvalidJSON(t *testing.T) {
	ctrls, _, cleanup := setupTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/feedback", bytes.NewBufferString(`invalid json`))
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req

	ctrls.SubmitFeedback(ctx)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid JSON, got %d", w.Code)
	}
}

func TestControllers_SubmitFeedback_ValidationErrors(t *testing.T) {
	ctrls, _, cleanup := setupTestControllers(t)
	defer cleanup()

	tests := []struct {
		name    string
		payload string
	}{
		{
			name:    "empty title",
			payload: `{"title":"","message":"Valid length feedback message."}`,
		},
		{
			name:    "title too short",
			payload: `{"title":"Hi","message":"Valid length feedback message."}`,
		},
		{
			name:    "message too short",
			payload: `{"title":"Valid Title","message":"Short"}`,
		},
		{
			name:    "invalid category",
			payload: `{"title":"Valid Title","category":"invalid_cat","message":"Valid length feedback message."}`,
		},
		{
			name:    "invalid email",
			payload: `{"title":"Valid Title","message":"Valid length feedback message.","user_email":"bad-email"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(w)
			req := httptest.NewRequest(http.MethodPost, "/v1/feedback", bytes.NewBufferString(tt.payload))
			req.Header.Set("Content-Type", "application/json")
			ctx.Request = req

			ctrls.SubmitFeedback(ctx)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 on validation failure, got %d (body: %s)", w.Code, w.Body.String())
			}
		})
	}
}

func TestControllers_SubmitFeedback_Success(t *testing.T) {
	ctrls, mock, cleanup := setupTestControllers(t)
	defer cleanup()

	feedbackID := uuid.New()
	now := time.Now()
	key := "header-idem-key-123"

	// 1. Idempotency check query (no existing record)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "feedbacks" WHERE idempotency_key = $1 ORDER BY "feedbacks"."id" LIMIT $2`)).
		WithArgs(key, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	// 2. Insert new feedback
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "feedbacks"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(feedbackID, now, now))
	mock.ExpectCommit()

	payload := dtos.FeedbackRequest{
		Title:   "Audio playback fails in Safari",
		Message: "When playing audio recordings on Safari 17.5, audio stalls after 2 seconds.",
	}
	bodyBytes, _ := json.Marshal(payload)

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/feedback", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", key)
	ctx.Request = req

	ctrls.SubmitFeedback(ctx)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d (body: %s)", w.Code, w.Body.String())
	}

	var resp struct {
		Status string                 `json:"status"`
		Data   *dtos.FeedbackResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Data == nil || resp.Data.ID != feedbackID {
		t.Fatalf("unexpected feedback data in response: %+v", resp.Data)
	}
	if resp.Data.IdempotencyKey == nil || *resp.Data.IdempotencyKey != key {
		t.Errorf("expected idempotency key %s, got %v", key, resp.Data.IdempotencyKey)
	}
}

func TestControllers_SubmitFeedback_ServiceError(t *testing.T) {
	ctrls, mock, cleanup := setupTestControllers(t)
	defer cleanup()

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "feedbacks"`).
		WillReturnError(errors.New("db down"))
	mock.ExpectRollback()

	payload := `{"title":"System crash","message":"Encountered an internal crash while browsing."}`

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/feedback", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req

	ctrls.SubmitFeedback(ctx)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 on db error, got %d", w.Code)
	}
}
