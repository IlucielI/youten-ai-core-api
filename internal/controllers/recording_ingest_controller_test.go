package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
	"code-base-golang/internal/pkg/ctxmeta"
)

func TestControllers_PresignUpload_InvalidPayload(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req, err := createJSONRequest(http.MethodPost, "/v1/recordings/presign", dtos.PresignUploadRequest{
		Filename: "",
	})
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	c.Request = req

	ctrls.PresignUpload(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_PresignUpload_UnsupportedMIME(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req, err := createJSONRequest(http.MethodPost, "/v1/recordings/presign", dtos.PresignUploadRequest{
		Filename:    "document.pdf",
		ContentType: "application/pdf",
	})
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	c.Request = req

	ctrls.PresignUpload(c)
	if w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415 Unsupported Media Type, got %d", w.Code)
	}
}

func TestControllers_PresignUpload_Success(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req, err := createJSONRequest(http.MethodPost, "/v1/recordings/presign", dtos.PresignUploadRequest{
		Filename:    "meeting.mp4",
		ContentType: "video/mp4",
	})
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req = req.WithContext(ctxmeta.WithClientMeta(req.Context(), "127.0.0.1", "Agent"))
	c.Request = req

	mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings"`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	ctrls.PresignUpload(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var resp dtos.APIResponse[dtos.PresignUploadResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}
	if resp.Data.UploadURL != "https://mock.storage/presigned-put-url" {
		t.Errorf("expected upload url 'https://mock.storage/presigned-put-url', got %s", resp.Data.UploadURL)
	}
	if resp.Data.Filename != "meeting.mp4" {
		t.Errorf("expected filename 'meeting.mp4', got %s", resp.Data.Filename)
	}
}

func TestControllers_UploadRecording_MissingFilename(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req, err := createJSONRequest(http.MethodPost, "/v1/recordings/upload", dtos.UploadRecordingRequest{
		Filename: "",
	})
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	c.Request = req

	ctrls.UploadRecording(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_UploadRecording_GuestSuccess(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req, err := createJSONRequest(http.MethodPost, "/v1/recordings/upload", dtos.UploadRecordingRequest{
		Filename:  "meeting.mp4",
		ObjectKey: "recordings/123/meeting.mp4",
		Title:     "Board Meeting",
		Template:  "MOM",
		Language:  "id",
	})
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req = req.WithContext(ctxmeta.WithClientMeta(req.Context(), "192.168.1.50", "TestAgent"))
	c.Request = req

	// 1. Quota check
	mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings"`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	// 2. Insert recording
	recID := uuid.New()
	now := time.Now()
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "recordings"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(recID, now, now))
	mock.ExpectCommit()

	ctrls.UploadRecording(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp dtos.APIResponse[dtos.RecordingUploadResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal JSON response: %v", err)
	}
	if resp.Status != constants.ResponseStatusSuccess {
		t.Errorf("expected status 'success', got %s", resp.Status)
	}
	if resp.Data.Title != "Board Meeting" {
		t.Errorf("expected title 'Board Meeting', got %s", resp.Data.Title)
	}
	if !resp.Data.IsGuest {
		t.Error("expected is_guest to be true")
	}
	if resp.Data.OwnershipToken == nil || *resp.Data.OwnershipToken == "" {
		t.Error("expected non-empty ownership_token for guest recording")
	}
}

func TestControllers_UploadRecording_AuthSuccess(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	userID := uuid.New()
	recID := uuid.New()
	now := time.Now()

	req, err := createJSONRequest(http.MethodPost, "/v1/recordings/upload", dtos.UploadRecordingRequest{
		Filename:  "memo.mp3",
		ObjectKey: "recordings/123/memo.mp3",
	})
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req = req.WithContext(ctxmeta.WithAuthUser(req.Context(), ctxmeta.AuthUser{UserID: userID}))
	c.Request = req

	// 1. User lookup
	mock.ExpectQuery(`SELECT \* FROM "users" WHERE id = \$1 AND "users"\."deleted_at" IS NULL ORDER BY "users"\."id" LIMIT \$2`).
		WithArgs(userID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).AddRow(userID, constants.UserStatusActive))

	// 2. Count user recordings today
	mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings" WHERE \(user_id = \$1 AND created_at >= \$2\) AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(userID, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// 3. Insert recording
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "recordings"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(recID, now, now))
	mock.ExpectCommit()

	ctrls.UploadRecording(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp dtos.APIResponse[dtos.RecordingUploadResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal JSON response: %v", err)
	}
	if resp.Data.IsGuest {
		t.Error("expected is_guest to be false")
	}
	if resp.Data.OwnershipToken != nil {
		t.Errorf("expected ownership token to be nil, got %v", resp.Data.OwnershipToken)
	}
}

func TestControllers_UploadRecording_ServiceError(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req, err := createJSONRequest(http.MethodPost, "/v1/recordings/upload", dtos.UploadRecordingRequest{
		Filename: "memo.mp3",
	})
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	c.Request = req

	mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings"`).
		WillReturnError(errors.New("db connection failure"))

	ctrls.UploadRecording(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 Internal Server Error, got %d", w.Code)
	}
}

func TestControllers_ImportURL_InvalidPayload(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req, err := createJSONRequest(http.MethodPost, "/v1/recordings/import-url", dtos.ImportURLRequest{
		URL: "", // missing URL
	})
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	c.Request = req

	ctrls.ImportURL(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_ImportURL_SSRFBlocked(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req, err := createJSONRequest(http.MethodPost, "/v1/recordings/import-url", dtos.ImportURLRequest{
		URL: "http://127.0.0.1:8080/audio.mp3",
	})
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	c.Request = req

	ctrls.ImportURL(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for SSRF blocked target, got %d", w.Code)
	}
}

func TestControllers_ImportURL_ServiceError(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req, err := createJSONRequest(http.MethodPost, "/v1/recordings/import-url", dtos.ImportURLRequest{
		URL: "https://8.8.8.8/audio.mp3",
	})
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	c.Request = req

	mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings"`).
		WillReturnError(errors.New("db error"))

	ctrls.ImportURL(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 Internal Server Error, got %d", w.Code)
	}
}

func TestControllers_ImportURL_Success(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	ctrls.svc.SetMediaFetcher(func(ctx context.Context, targetURL string, timeout time.Duration) (*http.Response, error) {
		resp := &http.Response{
			StatusCode:    http.StatusOK,
			Header:        make(http.Header),
			Body:          io.NopCloser(strings.NewReader("audio stream")),
			ContentLength: 12,
		}
		resp.Header.Set("Content-Type", "audio/mpeg")
		return resp, nil
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req, err := createJSONRequest(http.MethodPost, "/v1/recordings/import-url", dtos.ImportURLRequest{
		URL:   "https://8.8.8.8/audio.mp3",
		Title: "Imported Audio",
	})
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	c.Request = req

	mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings"`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	recID := uuid.New()
	now := time.Now()
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "recordings"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(recID, now, now))
	mock.ExpectCommit()

	ctrls.ImportURL(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp dtos.APIResponse[dtos.RecordingUploadResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal JSON response: %v", err)
	}
	if resp.Data.Title != "Imported Audio" {
		t.Errorf("expected Title 'Imported Audio', got %s", resp.Data.Title)
	}
	if !resp.Data.IsGuest {
		t.Error("expected IsGuest true")
	}
	if resp.Data.OwnershipToken == nil {
		t.Error("expected ownership token for guest")
	}
}

func TestControllers_RetryRecording_InvalidUUID(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "invalid-uuid"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/recordings/invalid-uuid/retry", nil)
	ctrls.RetryRecording(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_RetryRecording_NotFound(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/retry", nil)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	ctrls.RetryRecording(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", w.Code)
	}
}

func TestControllers_RetryRecording_Forbidden(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	ownerID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/retry", nil)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "status"}).
			AddRow(recID, &ownerID, models.RecordingStatusFailed))

	ctrls.RetryRecording(c)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden, got %d", w.Code)
	}
}

func TestControllers_RetryRecording_Conflict_Processing(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	userID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/retry", nil)
	ctx := ctxmeta.WithAuthUser(req.Context(), ctxmeta.AuthUser{UserID: userID})
	c.Request = req.WithContext(ctx)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "status"}).
			AddRow(recID, &userID, models.RecordingStatusTranscribing))

	ctrls.RetryRecording(c)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict, got %d", w.Code)
	}
	var resp dtos.BaseResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse json response: %v", err)
	}
	if resp.Code != "CONFLICT_PROCESSING" {
		t.Errorf("expected code CONFLICT_PROCESSING, got %s", resp.Code)
	}
}

func TestControllers_RetryRecording_Conflict_AlreadyCompleted(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	userID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/retry", nil)
	ctx := ctxmeta.WithAuthUser(req.Context(), ctxmeta.AuthUser{UserID: userID})
	c.Request = req.WithContext(ctx)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "status"}).
			AddRow(recID, &userID, models.RecordingStatusCompleted))

	ctrls.RetryRecording(c)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict, got %d", w.Code)
	}
	var resp dtos.BaseResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse json response: %v", err)
	}
	if resp.Code != "ERR_ALREADY_COMPLETED" {
		t.Errorf("expected code ERR_ALREADY_COMPLETED, got %s", resp.Code)
	}
}

func TestControllers_RetryRecording_Success_BodyToken(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	guestToken := "guest-tok-body"
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	reqBody := `{"ownership_token":"guest-tok-body"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/retry", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	// Single lookup with guest credentials
	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "is_guest", "status", "ownership_token", "audio_url", "selected_template", "output_language"}).
			AddRow(recID, true, models.RecordingStatusFailed, guestToken, nil, "GENERAL", "en"))

	mock.ExpectQuery(`SELECT \* FROM "transcript_segments" WHERE recording_id = \$1.*`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND is_active = TRUE.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	mock.ExpectQuery(`SELECT .* FROM "transcript_chunks" WHERE recording_id = \$1.*`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings" SET .* WHERE id = .*`).
		WithArgs(models.RecordingStatusQueued, sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	ctrls.RetryRecording(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d (body: %s)", w.Code, w.Body.String())
	}
	var resp dtos.APIResponse[dtos.RetryRecordingResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse json response: %v", err)
	}
	if resp.Data.ID != recID.String() {
		t.Errorf("expected ID %s, got %s", recID.String(), resp.Data.ID)
	}
	if resp.Data.Stage != models.RecordingStatusExtracting {
		t.Errorf("expected stage EXTRACTING, got %s", resp.Data.Stage)
	}
	if resp.Data.Status != models.RecordingStatusQueued {
		t.Errorf("expected status QUEUED, got %s", resp.Data.Status)
	}
}

func TestControllers_RetryRecording_Success_HeaderToken(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	audioURL := "https://s3.amazonaws.com/bucket/audio.mp3"
	guestToken := "guest-tok-header"
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/retry", nil)
	req.Header.Set("X-Ownership-Token", guestToken)
	c.Request = req

	// Single lookup with guest header credentials
	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "is_guest", "status", "ownership_token", "audio_url", "selected_template", "output_language"}).
			AddRow(recID, true, models.RecordingStatusFailed, guestToken, &audioURL, "GENERAL", "en"))

	mock.ExpectQuery(`SELECT \* FROM "transcript_segments" WHERE recording_id = \$1.*`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND is_active = TRUE.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	mock.ExpectQuery(`SELECT .* FROM "transcript_chunks" WHERE recording_id = \$1.*`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings" SET .* WHERE id = .*`).
		WithArgs(models.RecordingStatusQueued, sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	ctrls.RetryRecording(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d (body: %s)", w.Code, w.Body.String())
	}
	var resp dtos.APIResponse[dtos.RetryRecordingResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse json response: %v", err)
	}
	if resp.Data.Stage != models.RecordingStatusTranscribing {
		t.Errorf("expected stage TRANSCRIBING, got %s", resp.Data.Stage)
	}
	if resp.Data.Status != models.RecordingStatusQueued {
		t.Errorf("expected status QUEUED, got %s", resp.Data.Status)
	}
}
