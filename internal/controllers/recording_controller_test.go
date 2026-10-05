package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	gormPostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"

	"code-base-golang/internal/config"
	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
	"code-base-golang/internal/pkg/ctxmeta"
	"code-base-golang/internal/repositories"
	"code-base-golang/internal/services"
	"code-base-golang/internal/sse"
)

type dummyStorage struct {
	presignErr error
}

func (d *dummyStorage) Ping(ctx context.Context) error { return nil }
func (d *dummyStorage) Upload(ctx context.Context, bucketName, objectName string, reader io.Reader, size int64, contentType string) error {
	return nil
}
func (d *dummyStorage) Download(ctx context.Context, bucketName, objectName string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (d *dummyStorage) Delete(ctx context.Context, bucketName, objectName string) error { return nil }
func (d *dummyStorage) PresignGetObject(ctx context.Context, bucketName, objectName string, expiry time.Duration) (string, error) {
	if d.presignErr != nil {
		return "", d.presignErr
	}
	return "https://mock.storage/get", nil
}
func (d *dummyStorage) PresignPutObject(ctx context.Context, bucketName, objectName string, expiry time.Duration) (string, error) {
	if d.presignErr != nil {
		return "", d.presignErr
	}
	return "https://mock.storage/presigned-put-url", nil
}

type dummyPublisher struct{}

func (p *dummyPublisher) Ping(ctx context.Context) error { return nil }
func (p *dummyPublisher) Publish(ctx context.Context, topic string, payload any) error {
	return nil
}
func (p *dummyPublisher) MustPublish(ctx context.Context, topic string, payload any) {}

func setupRecordingTestControllers(t *testing.T) (*Controllers, sqlmock.Sqlmock, *dummyStorage, func()) {
	gin.SetMode(gin.TestMode)

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}

	gormDB, err := gorm.Open(gormPostgres.New(gormPostgres.Config{
		Conn: sqlDB,
	}), &gorm.Config{})
	if err != nil {
		sqlDB.Close()
		t.Fatalf("failed to initialize gorm: %v", err)
	}

	repo := repositories.New(gormDB)
	storage := &dummyStorage{}
	publisher := &dummyPublisher{}
	cfg := config.Config{
		AppName:      "youten-test",
		S3BucketName: "test-bucket",
	}
	svc := services.New(cfg, repo, storage, publisher)
	ctrls := New(cfg, svc)

	cleanup := func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("there were unfulfilled sqlmock expectations: %s", err)
		}
		sqlDB.Close()
	}

	return ctrls, mock, storage, cleanup
}

func createJSONRequest(method, url string, body any) (*http.Request, error) {
	jsonBytes, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(method, url, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

func TestControllers_PresignUpload_NilService(t *testing.T) {
	ctrls := &Controllers{}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req, err := createJSONRequest(http.MethodPost, "/v1/recordings/presign", dtos.PresignUploadRequest{
		Filename: "test.mp3",
	})
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	c.Request = req

	ctrls.PresignUpload(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 Internal Server Error, got %d", w.Code)
	}
}

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

func TestControllers_UploadRecording_NilService(t *testing.T) {
	ctrls := &Controllers{}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req, err := createJSONRequest(http.MethodPost, "/v1/recordings/upload", dtos.UploadRecordingRequest{
		Filename: "memo.mp3",
	})
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	c.Request = req

	ctrls.UploadRecording(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 Internal Server Error, got %d", w.Code)
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

func TestControllers_ImportURL_NilService(t *testing.T) {
	ctrls := &Controllers{}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req, err := createJSONRequest(http.MethodPost, "/v1/recordings/import-url", dtos.ImportURLRequest{
		URL: "https://8.8.8.8/audio.mp3",
	})
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	c.Request = req

	ctrls.ImportURL(c)

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

func TestControllers_GetRecordingDetail_NilService(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings/test", nil)

	ctrls := &Controllers{}
	ctrls.GetRecordingDetail(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 Internal Server Error, got %d", w.Code)
	}
}

func TestControllers_GetRecordingDetail_InvalidUUID(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings/not-a-uuid", nil)
	c.Params = gin.Params{{Key: "id", Value: "not-a-uuid"}}

	ctrls.GetRecordingDetail(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestControllers_GetRecordingDetail_NotFound(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings/"+recID.String(), nil)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	ctrls.GetRecordingDetail(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestControllers_GetRecordingDetail_Forbidden(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings/"+recID.String(), nil)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "is_guest"}).
			AddRow(recID, "secret-token", true))

	ctrls.GetRecordingDetail(c)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestControllers_GetRecordingDetail_Success_QueryToken(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	token := "valid-token-123"
	now := time.Now()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := httptest.NewRequest(http.MethodGet, "/v1/recordings/"+recID.String()+"?token="+token, nil)
	c.Request = req
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "title", "original_filename", "audio_url", "source_type",
			"status", "selected_template", "output_language", "is_guest", "ownership_token", "created_at", "updated_at",
		}).AddRow(
			recID, "Meeting Recording", "meeting.mp3", "recordings/"+recID.String()+"/meeting.mp3", "UPLOAD",
			"COMPLETED", "MOM", "id", true, token, now, now,
		))

	mock.ExpectQuery(`SELECT \* FROM "transcript_segments" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND is_active = TRUE`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	mock.ExpectQuery(`SELECT \* FROM "chapters" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC LIMIT \$2`).
		WithArgs(recID, 100).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	mock.ExpectQuery(`SELECT \* FROM "highlights" WHERE recording_id = \$1 ORDER BY start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	ctrls.GetRecordingDetail(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp dtos.APIResponse[dtos.RecordingDetailResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}
	if resp.Data.ID != recID.String() {
		t.Errorf("expected ID %s, got %s", recID.String(), resp.Data.ID)
	}
	if resp.Data.Title != "Meeting Recording" {
		t.Errorf("expected title Meeting Recording, got %s", resp.Data.Title)
	}
}

func TestControllers_GetRecordingDetail_Success_HeaderToken(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	token := "header-token-456"
	now := time.Now()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := httptest.NewRequest(http.MethodGet, "/v1/recordings/"+recID.String(), nil)
	req.Header.Set("X-Ownership-Token", token)
	c.Request = req
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "title", "original_filename", "audio_url", "source_type",
			"status", "selected_template", "output_language", "is_guest", "ownership_token", "created_at", "updated_at",
		}).AddRow(
			recID, "Header Auth Meeting", "voice.wav", "", "UPLOAD",
			"PENDING", "GENERAL", "en", true, token, now, now,
		))

	mock.ExpectQuery(`SELECT \* FROM "transcript_segments" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND is_active = TRUE`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	mock.ExpectQuery(`SELECT \* FROM "chapters" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC LIMIT \$2`).
		WithArgs(recID, 100).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	mock.ExpectQuery(`SELECT \* FROM "highlights" WHERE recording_id = \$1 ORDER BY start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	ctrls.GetRecordingDetail(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp dtos.APIResponse[dtos.RecordingDetailResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}
	if resp.Data.ID != recID.String() {
		t.Errorf("expected ID %s, got %s", recID.String(), resp.Data.ID)
	}
}

func TestControllers_ListRecordings_NilService(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings", nil)

	var nilCtrls *Controllers
	nilCtrls.ListRecordings(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for nil controller, got %d", w.Code)
	}

	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings", nil)

	ctrls := &Controllers{}
	ctrls.ListRecordings(c2)

	if w2.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for nil service, got %d", w2.Code)
	}
}

func TestControllers_ListRecordings_Unauthorized(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings", nil)

	ctrls.ListRecordings(c)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d", w.Code)
	}
}

func TestControllers_ListRecordings_Success(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	userID := uuid.New()
	recID := uuid.New()
	now := time.Now()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := httptest.NewRequest(http.MethodGet, "/v1/recordings?search=sprint&status=COMPLETED&template=MOM&page=1&limit=10&sort_by=created_at&sort_order=desc", nil)
	ctx := ctxmeta.WithAuthUser(req.Context(), ctxmeta.AuthUser{UserID: userID})
	c.Request = req.WithContext(ctx)

	mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings" WHERE user_id = \$1 AND \(LOWER\(title\) LIKE \$2 OR LOWER\(original_filename\) LIKE \$3\) AND status = \$4 AND selected_template = \$5 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(userID, "%sprint%", "%sprint%", "COMPLETED", "MOM").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE user_id = \$1 AND \(LOWER\(title\) LIKE \$2 OR LOWER\(original_filename\) LIKE \$3\) AND status = \$4 AND selected_template = \$5 AND "recordings"\."deleted_at" IS NULL ORDER BY created_at DESC LIMIT \$6`).
		WithArgs(userID, "%sprint%", "%sprint%", "COMPLETED", "MOM", 10).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "title", "original_filename", "file_size_bytes", "duration_seconds",
			"source_type", "status", "selected_template", "output_language", "is_guest", "created_at", "updated_at",
		}).AddRow(
			recID, userID, "Sprint Planning Meeting", "sprint.mp3", 2048, 180.0,
			"UPLOAD", "COMPLETED", "MOM", "id", false, now, now,
		))

	ctrls.ListRecordings(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp dtos.APIResponse[dtos.RecordingListResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if len(resp.Data.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(resp.Data.Items))
	}
	if resp.Data.Items[0].Title != "Sprint Planning Meeting" {
		t.Errorf("expected Title 'Sprint Planning Meeting', got %s", resp.Data.Items[0].Title)
	}
	if resp.Data.Pagination.TotalItems != 1 {
		t.Errorf("expected total items 1, got %d", resp.Data.Pagination.TotalItems)
	}
}

func TestControllers_ListRecordings_ServiceError(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	userID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := httptest.NewRequest(http.MethodGet, "/v1/recordings", nil)
	ctx := ctxmeta.WithAuthUser(req.Context(), ctxmeta.AuthUser{UserID: userID})
	c.Request = req.WithContext(ctx)

	mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings" WHERE user_id = \$1 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(userID).
		WillReturnError(errors.New("db query error"))

	ctrls.ListRecordings(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 Internal Server Error, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestControllers_DeleteRecording_NilReceiver(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "00000000-0000-0000-0000-000000000001"}}
	c.Request = httptest.NewRequest(http.MethodDelete, "/v1/recordings/00000000-0000-0000-0000-000000000001", nil)

	var nilCtrls *Controllers
	nilCtrls.DeleteRecording(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for nil controller receiver, got %d", w.Code)
	}
}

func TestControllers_DeleteRecording_NilService(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "00000000-0000-0000-0000-000000000001"}}
	c.Request = httptest.NewRequest(http.MethodDelete, "/v1/recordings/00000000-0000-0000-0000-000000000001", nil)

	ctrls := &Controllers{}
	ctrls.DeleteRecording(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for nil service, got %d", w.Code)
	}
}

func TestControllers_DeleteRecording_InvalidUUID(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "invalid-uuid"}}
	c.Request = httptest.NewRequest(http.MethodDelete, "/v1/recordings/invalid-uuid", nil)

	ctrls.DeleteRecording(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_DeleteRecording_Unauthorized(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	c.Request = httptest.NewRequest(http.MethodDelete, "/v1/recordings/"+recID.String(), nil)

	ctrls.DeleteRecording(c)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d", w.Code)
	}
}

func TestControllers_DeleteRecording_NotFound(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	userID := uuid.New()
	recID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}

	req := httptest.NewRequest(http.MethodDelete, "/v1/recordings/"+recID.String(), nil)
	ctx := ctxmeta.WithAuthUser(req.Context(), ctxmeta.AuthUser{UserID: userID})
	c.Request = req.WithContext(ctx)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	ctrls.DeleteRecording(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", w.Code)
	}
}

func TestControllers_DeleteRecording_Forbidden(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	userID := uuid.New()
	otherUserID := uuid.New()
	recID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}

	req := httptest.NewRequest(http.MethodDelete, "/v1/recordings/"+recID.String(), nil)
	ctx := ctxmeta.WithAuthUser(req.Context(), ctxmeta.AuthUser{UserID: userID})
	c.Request = req.WithContext(ctx)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id"}).
			AddRow(recID, otherUserID))

	ctrls.DeleteRecording(c)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden, got %d", w.Code)
	}
}

func TestControllers_DeleteRecording_Success(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	userID := uuid.New()
	recID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}

	req := httptest.NewRequest(http.MethodDelete, "/v1/recordings/"+recID.String(), nil)
	ctx := ctxmeta.WithAuthUser(req.Context(), ctxmeta.AuthUser{UserID: userID})
	c.Request = req.WithContext(ctx)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id"}).
			AddRow(recID, userID))

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings" SET "deleted_at"=\$1 WHERE id = \$2 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	ctrls.DeleteRecording(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp dtos.BaseResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}
	if resp.Message != "recording deleted successfully" {
		t.Errorf("expected Message 'recording deleted successfully', got %s", resp.Message)
	}
}

func TestControllers_ClaimRecording_NilReceiver(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/recordings/test/claim", nil)

	var ctrls *Controllers
	ctrls.ClaimRecording(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 Internal Server Error, got %d", w.Code)
	}
}

func TestControllers_ClaimRecording_NilService(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/recordings/test/claim", nil)

	ctrls := &Controllers{}
	ctrls.ClaimRecording(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 Internal Server Error, got %d", w.Code)
	}
}

func TestControllers_ClaimRecording_InvalidUUID(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "invalid-uuid"}}

	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/invalid-uuid/claim", strings.NewReader(`{"ownership_token":"tok"}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	ctrls.ClaimRecording(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_ClaimRecording_InvalidJSON(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}

	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/claim", strings.NewReader(`{invalid-json}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	ctrls.ClaimRecording(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_ClaimRecording_ValidationError(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}

	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/claim", strings.NewReader(`{"ownership_token":""}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	ctrls.ClaimRecording(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_ClaimRecording_ServiceError(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	userID := uuid.New()
	recID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}

	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/claim", strings.NewReader(`{"ownership_token":"token-123"}`))
	req.Header.Set("Content-Type", "application/json")
	ctx := ctxmeta.WithAuthUser(req.Context(), ctxmeta.AuthUser{UserID: userID})
	c.Request = req.WithContext(ctx)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "is_guest", "user_id", "ownership_token"}).
			AddRow(recID, false, nil, "token-123"))

	ctrls.ClaimRecording(c)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestControllers_ClaimRecording_Success(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	userID := uuid.New()
	recID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}

	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/claim", strings.NewReader(`{"ownership_token":"valid-token"}`))
	req.Header.Set("Content-Type", "application/json")
	ctx := ctxmeta.WithAuthUser(req.Context(), ctxmeta.AuthUser{UserID: userID})
	c.Request = req.WithContext(ctx)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "is_guest", "user_id", "ownership_token"}).
			AddRow(recID, true, nil, "valid-token"))

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings" SET "is_guest"=\$1,"user_id"=\$2,"updated_at"=\$3 WHERE \(id = \$4 AND ownership_token = \$5 AND is_guest = TRUE\) AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(false, userID, sqlmock.AnyArg(), recID, "valid-token").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	ctrls.ClaimRecording(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp dtos.BaseResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}
	if resp.Message != "recording claimed successfully" {
		t.Errorf("expected Message 'recording claimed successfully', got %s", resp.Message)
	}
}

func TestControllers_ClaimBulkRecordings_NilReceiver(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/recordings/claim", nil)

	var ctrls *Controllers
	ctrls.ClaimBulkRecordings(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 Internal Server Error, got %d", w.Code)
	}
}

func TestControllers_ClaimBulkRecordings_NilService(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/recordings/claim", nil)

	ctrls := &Controllers{}
	ctrls.ClaimBulkRecordings(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 Internal Server Error, got %d", w.Code)
	}
}

func TestControllers_ClaimBulkRecordings_InvalidJSON(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/claim", strings.NewReader(`{invalid}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	ctrls.ClaimBulkRecordings(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_ClaimBulkRecordings_ValidationError(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/claim", strings.NewReader(`{"tokens":[]}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	ctrls.ClaimBulkRecordings(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_ClaimBulkRecordings_ServiceError(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/claim", strings.NewReader(`{"tokens":["tok-1"]}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req // No auth user

	ctrls.ClaimBulkRecordings(c)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestControllers_ClaimBulkRecordings_Success(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	userID := uuid.New()
	recID1 := uuid.New()
	recID2 := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/claim", strings.NewReader(`{"tokens":["tok-1","tok-2"]}`))
	req.Header.Set("Content-Type", "application/json")
	ctx := ctxmeta.WithAuthUser(req.Context(), ctxmeta.AuthUser{UserID: userID})
	c.Request = req.WithContext(ctx)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE \(ownership_token IN \(\$1,\$2\) AND is_guest = TRUE\) AND "recordings"\."deleted_at" IS NULL`).
		WithArgs("tok-1", "tok-2").
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "is_guest"}).
			AddRow(recID1, "tok-1", true).
			AddRow(recID2, "tok-2", true))

	mock.ExpectExec(`UPDATE "recordings" SET "is_guest"=\$1,"user_id"=\$2,"updated_at"=\$3 WHERE id IN \(\$4,\$5\) AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(false, userID, sqlmock.AnyArg(), recID1, recID2).
		WillReturnResult(sqlmock.NewResult(1, 2))
	mock.ExpectCommit()

	ctrls.ClaimBulkRecordings(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp dtos.APIResponse[dtos.BulkClaimResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}
	if resp.Data.ClaimedCount != 2 {
		t.Errorf("expected ClaimedCount 2, got %d", resp.Data.ClaimedCount)
	}
	if len(resp.Data.RecordingIDs) != 2 {
		t.Errorf("expected 2 RecordingIDs, got %d", len(resp.Data.RecordingIDs))
	}
}

func TestControllers_ToggleRecordingShare_NilReceiver(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPatch, "/v1/recordings/test/share", nil)

	var ctrls *Controllers
	ctrls.ToggleRecordingShare(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 Internal Server Error, got %d", w.Code)
	}
}

func TestControllers_ToggleRecordingShare_NilService(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPatch, "/v1/recordings/test/share", nil)

	ctrls := &Controllers{}
	ctrls.ToggleRecordingShare(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 Internal Server Error, got %d", w.Code)
	}
}

func TestControllers_ToggleRecordingShare_InvalidUUID(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "invalid-uuid"}}

	req := httptest.NewRequest(http.MethodPatch, "/v1/recordings/invalid-uuid/share", strings.NewReader(`{"is_share_enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	ctrls.ToggleRecordingShare(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_ToggleRecordingShare_InvalidJSON(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}

	req := httptest.NewRequest(http.MethodPatch, "/v1/recordings/"+recID.String()+"/share", strings.NewReader(`{invalid}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	ctrls.ToggleRecordingShare(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_ToggleRecordingShare_ValidationError(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}

	req := httptest.NewRequest(http.MethodPatch, "/v1/recordings/"+recID.String()+"/share", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	ctrls.ToggleRecordingShare(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_ToggleRecordingShare_ServiceError(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}

	req := httptest.NewRequest(http.MethodPatch, "/v1/recordings/"+recID.String()+"/share", strings.NewReader(`{"is_share_enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req // No auth user

	ctrls.ToggleRecordingShare(c)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestControllers_ToggleRecordingShare_Success(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	userID := uuid.New()
	recID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}

	req := httptest.NewRequest(http.MethodPatch, "/v1/recordings/"+recID.String()+"/share", strings.NewReader(`{"is_share_enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	ctx := ctxmeta.WithAuthUser(req.Context(), ctxmeta.AuthUser{UserID: userID})
	c.Request = req.WithContext(ctx)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "share_token", "is_share_enabled"}).
			AddRow(recID, userID, nil, false))

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings" SET "is_share_enabled"=\$1,"share_token"=\$2,"updated_at"=\$3 WHERE id = \$4 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(true, sqlmock.AnyArg(), sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	ctrls.ToggleRecordingShare(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp dtos.APIResponse[dtos.ShareToggleResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}
	if !resp.Data.IsShareEnabled {
		t.Error("expected IsShareEnabled true")
	}
	if resp.Data.ShareToken == nil {
		t.Error("expected ShareToken non-nil")
	}
	if resp.Data.ShareURL == nil || !strings.Contains(*resp.Data.ShareURL, *resp.Data.ShareToken) {
		t.Errorf("expected ShareURL containing token, got %v", resp.Data.ShareURL)
	}
}

func TestControllers_GetSharedRecording_NilReceiver(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings/shared/test-token", nil)

	var ctrls *Controllers
	ctrls.GetSharedRecording(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 Internal Server Error, got %d", w.Code)
	}
}

func TestControllers_GetSharedRecording_NilService(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings/shared/test-token", nil)

	ctrls := &Controllers{}
	ctrls.GetSharedRecording(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 Internal Server Error, got %d", w.Code)
	}
}

func TestControllers_GetSharedRecording_EmptyToken(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "token", Value: "  "}}
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings/shared/%20", nil)

	ctrls.GetSharedRecording(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", w.Code)
	}
}

func TestControllers_GetSharedRecording_NotFound(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "token", Value: "non-existent-token"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings/shared/non-existent-token", nil)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE \(share_token = \$1 AND is_share_enabled = TRUE\) AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs("non-existent-token", 1).
		WillReturnError(gorm.ErrRecordNotFound)

	ctrls.GetSharedRecording(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", w.Code)
	}
}

func TestControllers_GetSharedRecording_Success(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	audioKey := "recordings/audio.mp3"

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "token", Value: "valid-share-token"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings/shared/valid-share-token", nil)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE \(share_token = \$1 AND is_share_enabled = TRUE\) AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs("valid-share-token", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "title", "duration_seconds", "audio_url", "is_share_enabled", "share_token"}).
			AddRow(recID, "Public Demo", 120.0, audioKey, true, "valid-share-token"))

	mock.ExpectQuery(`SELECT \* FROM "transcript_segments" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "text"}).
			AddRow(uuid.New(), recID, "Public transcript"))

	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND is_active = TRUE.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "markdown_content", "is_active"}).
			AddRow(uuid.New(), recID, "Public summary", true))

	mock.ExpectQuery(`SELECT \* FROM "chapters" WHERE recording_id = \$1.*LIMIT \$2`).
		WithArgs(recID, 100).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "title"}).
			AddRow(uuid.New(), recID, "Public Chapter"))

	mock.ExpectQuery(`SELECT \* FROM "highlights" WHERE recording_id = \$1 ORDER BY start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "source"}).
			AddRow(uuid.New(), recID, "KEY_POINT"))

	ctrls.GetSharedRecording(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp dtos.APIResponse[dtos.SharedRecordingResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}
	if resp.Data.ID != recID.String() {
		t.Errorf("expected ID %s, got %s", recID.String(), resp.Data.ID)
	}
	if resp.Data.Title != "Public Demo" {
		t.Errorf("expected Title 'Public Demo', got %s", resp.Data.Title)
	}
	if len(resp.Data.Segments) != 1 {
		t.Errorf("expected 1 segment, got %d", len(resp.Data.Segments))
	}
}

func TestControllers_StreamRecordingProgress_NilReceiver(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings/test/progress", nil)

	var ctrls *Controllers
	ctrls.StreamRecordingProgress(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 Internal Server Error, got %d", w.Code)
	}
}

func TestControllers_StreamRecordingProgress_NilService(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings/test/progress", nil)

	ctrls := &Controllers{}
	ctrls.StreamRecordingProgress(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 Internal Server Error, got %d", w.Code)
	}
}

func TestControllers_StreamRecordingProgress_InvalidUUID(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "invalid-uuid"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings/invalid-uuid/progress", nil)

	ctrls.StreamRecordingProgress(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_StreamRecordingProgress_NotFound(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings/"+recID.String()+"/progress", nil)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	ctrls.StreamRecordingProgress(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", w.Code)
	}
}

func TestControllers_StreamRecordingProgress_Forbidden(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	userID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings/"+recID.String()+"/progress", nil)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "ownership_token", "is_share_enabled"}).
			AddRow(recID, &userID, "token-1", false))

	ctrls.StreamRecordingProgress(c)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden, got %d", w.Code)
	}
}

func TestControllers_StreamRecordingProgress_TerminalInitial_Completed(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	userID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req := httptest.NewRequest(http.MethodGet, "/v1/recordings/"+recID.String()+"/progress", nil)
	ctx := ctxmeta.WithAuthUser(req.Context(), ctxmeta.AuthUser{UserID: userID})
	c.Request = req.WithContext(ctx)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "status", "ownership_token"}).
			AddRow(recID, &userID, "COMPLETED", "token-1"))

	ctrls.StreamRecordingProgress(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}
	if !strings.Contains(w.Header().Get("Content-Type"), "text/event-stream") {
		t.Errorf("expected text/event-stream Content-Type, got %s", w.Header().Get("Content-Type"))
	}
	body := w.Body.String()
	if !strings.Contains(body, "event: progress\n") || !strings.Contains(body, `"status":"COMPLETED"`) {
		t.Errorf("expected progress event with COMPLETED, got: %s", body)
	}
}

func TestControllers_StreamRecordingProgress_StreamTransitions(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	userID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req := httptest.NewRequest(http.MethodGet, "/v1/recordings/"+recID.String()+"/progress", nil)
	ctx := ctxmeta.WithAuthUser(req.Context(), ctxmeta.AuthUser{UserID: userID})
	c.Request = req.WithContext(ctx)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "status", "ownership_token"}).
			AddRow(recID, &userID, "EXTRACTING", "token-1"))

	// In background, broadcast next events to hub, culminating in COMPLETED
	go func() {
		time.Sleep(50 * time.Millisecond)
		ctrls.Service().SSEHub().Publish(recID, sse.ProgressEvent{
			RecordingID: recID.String(),
			Status:      "TRANSCRIBING",
			Stage:       "TRANSCRIBING",
			Progress:    55,
		})
		time.Sleep(50 * time.Millisecond)
		ctrls.Service().SSEHub().Publish(recID, sse.ProgressEvent{
			RecordingID: recID.String(),
			Status:      "COMPLETED",
			Stage:       "COMPLETED",
			Progress:    100,
		})
	}()

	ctrls.StreamRecordingProgress(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"status":"EXTRACTING"`) {
		t.Errorf("expected initial EXTRACTING status, got: %s", body)
	}
	if !strings.Contains(body, `"status":"TRANSCRIBING"`) {
		t.Errorf("expected streamed TRANSCRIBING status, got: %s", body)
	}
	if !strings.Contains(body, `"status":"COMPLETED"`) {
		t.Errorf("expected final COMPLETED status, got: %s", body)
	}
}

func TestControllers_StreamRecordingProgress_ClientDisconnect(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	userID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}

	reqCtx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/v1/recordings/"+recID.String()+"/progress", nil).WithContext(reqCtx)
	ctx := ctxmeta.WithAuthUser(req.Context(), ctxmeta.AuthUser{UserID: userID})
	c.Request = req.WithContext(ctx)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "status", "ownership_token"}).
			AddRow(recID, &userID, "EXTRACTING", "token-1"))

	// Cancel context shortly after start
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	ctrls.StreamRecordingProgress(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"status":"EXTRACTING"`) {
		t.Errorf("expected initial EXTRACTING status, got: %s", body)
	}
}

func TestControllers_RetryRecording_NilReceiver(t *testing.T) {
	var ctrls *Controllers
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	ctrls.RetryRecording(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 Internal Server Error, got %d", w.Code)
	}
}

func TestControllers_RetryRecording_NilService(t *testing.T) {
	ctrls := New(config.Config{}, nil)
	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/retry", nil)
	ctrls.RetryRecording(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 Internal Server Error, got %d", w.Code)
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

func TestControllers_StreamRecordingChat_NilController(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: uuid.New().String()}}
	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/"+uuid.New().String()+"/chat", strings.NewReader(`{"message":"hello"}`))
	c.Request = req

	var nilCtrls *Controllers
	nilCtrls.StreamRecordingChat(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 Internal Server Error, got %d", w.Code)
	}
}

func TestControllers_StreamRecordingChat_NilService(t *testing.T) {
	cfg := config.Config{}
	ctrls := New(cfg, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: uuid.New().String()}}
	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/"+uuid.New().String()+"/chat", strings.NewReader(`{"message":"hello"}`))
	c.Request = req

	ctrls.StreamRecordingChat(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 Internal Server Error, got %d", w.Code)
	}
}

func TestControllers_StreamRecordingChat_InvalidUUID(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "invalid-uuid-format"}}
	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/invalid/chat", strings.NewReader(`{"message":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	ctrls.StreamRecordingChat(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_StreamRecordingChat_InvalidJSON(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/chat", strings.NewReader(`{invalid-json`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	ctrls.StreamRecordingChat(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_StreamRecordingChat_ValidationError(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/chat", strings.NewReader(`{"message":""}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	ctrls.StreamRecordingChat(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_StreamRecordingChat_NotFound(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/chat", strings.NewReader(`{"message":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	ctrls.StreamRecordingChat(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", w.Code)
	}
}

func TestControllers_StreamRecordingChat_Forbidden(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	ownerID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/chat", strings.NewReader(`{"message":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "ownership_token"}).
			AddRow(recID, &ownerID, "secret-token"))

	ctrls.StreamRecordingChat(c)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden, got %d", w.Code)
	}
}

type mockControllerLLM struct {
	streamFunc     func(ctx context.Context, systemPrompt string, messages []dtos.ChatMessageInput, opts dtos.ChatOptions) (<-chan dtos.StreamChunk, error)
	structuredFunc func(ctx context.Context, systemPrompt string, userPrompt string, schema map[string]interface{}) (*dtos.StructuredResponse, error)
}

func (m *mockControllerLLM) GenerateStructured(ctx context.Context, systemPrompt string, userPrompt string, schema map[string]interface{}) (*dtos.StructuredResponse, error) {
	if m.structuredFunc != nil {
		return m.structuredFunc(ctx, systemPrompt, userPrompt, schema)
	}
	return &dtos.StructuredResponse{
		RawJSON: `{"overview": "summary", "markdown_content": "# Regenerated Summary"}`,
	}, nil
}

func (m *mockControllerLLM) GenerateChatResponse(ctx context.Context, systemPrompt string, messages []dtos.ChatMessageInput, opts dtos.ChatOptions) (*dtos.ChatResponse, error) {
	return nil, nil
}

func (m *mockControllerLLM) StreamChatResponse(ctx context.Context, systemPrompt string, messages []dtos.ChatMessageInput, opts dtos.ChatOptions) (<-chan dtos.StreamChunk, error) {
	if m.streamFunc != nil {
		return m.streamFunc(ctx, systemPrompt, messages, opts)
	}
	out := make(chan dtos.StreamChunk, 2)
	go func() {
		defer close(out)
		out <- dtos.StreamChunk{Content: "Hello "}
		out <- dtos.StreamChunk{Content: "world [00:10]"}
	}()
	return out, nil
}

func TestControllers_StreamRecordingChat_Success(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	guestToken := "guest-ownership-token"
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/chat", strings.NewReader(`{"message":"Tell me about the goals"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Ownership-Token", guestToken)
	c.Request = req

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token"}).
			AddRow(recID, guestToken))

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "chat_messages"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(uuid.New()))
	mock.ExpectCommit()

	mockLLM := &mockControllerLLM{
		streamFunc: func(ctx context.Context, systemPrompt string, messages []dtos.ChatMessageInput, opts dtos.ChatOptions) (<-chan dtos.StreamChunk, error) {
			out := make(chan dtos.StreamChunk, 2)
			go func() {
				defer close(out)
				out <- dtos.StreamChunk{Content: "Goal is to expand [01:23] "}
				out <- dtos.StreamChunk{Content: "market reach."}
			}()
			return out, nil
		},
	}
	ctrls.svc.SetLLM(mockLLM)

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "chat_messages"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(uuid.New()))
	mock.ExpectCommit()

	ctrls.StreamRecordingChat(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if !strings.HasPrefix(contentType, "text/event-stream") {
		t.Errorf("expected Content-Type text/event-stream, got %s", contentType)
	}

	body := w.Body.String()
	if !strings.Contains(body, "event: token") {
		t.Errorf("expected body to contain token events, got: %s", body)
	}
	if !strings.Contains(body, "event: done") {
		t.Errorf("expected body to contain done event, got: %s", body)
	}
	if !strings.Contains(body, `"citations":["01:23"]`) {
		t.Errorf("expected body to contain parsed citation, got: %s", body)
	}
}

func TestControllers_StreamRecordingChat_StreamError(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	guestToken := "guest-ownership-token"
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/chat", strings.NewReader(`{"message":"Tell me about the goals"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Ownership-Token", guestToken)
	c.Request = req

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token"}).
			AddRow(recID, guestToken))

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "chat_messages"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(uuid.New()))
	mock.ExpectCommit()

	mockLLM := &mockControllerLLM{
		streamFunc: func(ctx context.Context, systemPrompt string, messages []dtos.ChatMessageInput, opts dtos.ChatOptions) (<-chan dtos.StreamChunk, error) {
			out := make(chan dtos.StreamChunk, 2)
			go func() {
				defer close(out)
				out <- dtos.StreamChunk{Content: "Partial "}
				out <- dtos.StreamChunk{Err: errors.New("upstream connection reset")}
			}()
			return out, nil
		},
	}
	ctrls.svc.SetLLM(mockLLM)

	ctrls.StreamRecordingChat(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "event: error") {
		t.Errorf("expected body to contain error event, got: %s", body)
	}
	if !strings.Contains(body, "upstream connection reset") {
		t.Errorf("expected body to contain error message, got: %s", body)
	}
}

func TestControllers_UpdateTranscriptSpeakers_NilController(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: uuid.New().String()}}
	req := httptest.NewRequest(http.MethodPut, "/v1/recordings/"+uuid.New().String()+"/speakers", strings.NewReader(`{"speakers":{"SPEAKER_00":"Bayu"}}`))
	c.Request = req

	var nilCtrls *Controllers
	nilCtrls.UpdateTranscriptSpeakers(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 Internal Server Error, got %d", w.Code)
	}
}

func TestControllers_UpdateTranscriptSpeakers_NilService(t *testing.T) {
	cfg := config.Config{}
	ctrls := New(cfg, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: uuid.New().String()}}
	req := httptest.NewRequest(http.MethodPut, "/v1/recordings/"+uuid.New().String()+"/speakers", strings.NewReader(`{"speakers":{"SPEAKER_00":"Bayu"}}`))
	c.Request = req

	ctrls.UpdateTranscriptSpeakers(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 Internal Server Error, got %d", w.Code)
	}
}

func TestControllers_UpdateTranscriptSpeakers_InvalidUUID(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "invalid-uuid"}}
	req := httptest.NewRequest(http.MethodPut, "/v1/recordings/invalid/speakers", strings.NewReader(`{"speakers":{"SPEAKER_00":"Bayu"}}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	ctrls.UpdateTranscriptSpeakers(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_UpdateTranscriptSpeakers_InvalidJSON(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req := httptest.NewRequest(http.MethodPut, "/v1/recordings/"+recID.String()+"/speakers", strings.NewReader(`{invalid-json`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	ctrls.UpdateTranscriptSpeakers(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_UpdateTranscriptSpeakers_ValidationError(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req := httptest.NewRequest(http.MethodPut, "/v1/recordings/"+recID.String()+"/speakers", strings.NewReader(`{"speakers":{}}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	ctrls.UpdateTranscriptSpeakers(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_UpdateTranscriptSpeakers_NotFound(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req := httptest.NewRequest(http.MethodPut, "/v1/recordings/"+recID.String()+"/speakers", strings.NewReader(`{"speakers":{"SPEAKER_00":"Bayu"}}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	ctrls.UpdateTranscriptSpeakers(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", w.Code)
	}
}

func TestControllers_UpdateTranscriptSpeakers_Forbidden(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	ownerID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req := httptest.NewRequest(http.MethodPut, "/v1/recordings/"+recID.String()+"/speakers", strings.NewReader(`{"speakers":{"SPEAKER_00":"Bayu"}}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "ownership_token"}).
			AddRow(recID, &ownerID, "secret-token"))

	ctrls.UpdateTranscriptSpeakers(c)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden, got %d", w.Code)
	}
}

func TestControllers_UpdateTranscriptSpeakers_Success_TokenInBody(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	guestToken := "guest-ownership-token"
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req := httptest.NewRequest(http.MethodPut, "/v1/recordings/"+recID.String()+"/speakers", strings.NewReader(`{"speakers":{"SPEAKER_00":"Bayu"},"ownership_token":"`+guestToken+`"}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token"}).
			AddRow(recID, guestToken))

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "transcript_segments" SET "speaker_name"=\$1,"updated_at"=\$2 WHERE recording_id = \$3 AND speaker_label = \$4`).
		WithArgs("Bayu", sqlmock.AnyArg(), recID, "SPEAKER_00").
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectCommit()

	ctrls.UpdateTranscriptSpeakers(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d (body: %s)", w.Code, w.Body.String())
	}

	var resp dtos.APIResponse[dtos.UpdateSpeakersResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse json response: %v", err)
	}
	if resp.Data.UpdatedCount != 3 {
		t.Errorf("expected UpdatedCount 3, got %d", resp.Data.UpdatedCount)
	}
	if resp.Data.Speakers["SPEAKER_00"] != "Bayu" {
		t.Errorf("expected speaker name Bayu, got %s", resp.Data.Speakers["SPEAKER_00"])
	}
}

func TestControllers_UpdateTranscriptSpeakers_Success_HeaderToken(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	guestToken := "guest-ownership-token"
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req := httptest.NewRequest(http.MethodPut, "/v1/recordings/"+recID.String()+"/speakers", strings.NewReader(`{"speakers":{"SPEAKER_01":"Alice"}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Ownership-Token", guestToken)
	c.Request = req

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token"}).
			AddRow(recID, guestToken))

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "transcript_segments" SET "speaker_name"=\$1,"updated_at"=\$2 WHERE recording_id = \$3 AND speaker_label = \$4`).
		WithArgs("Alice", sqlmock.AnyArg(), recID, "SPEAKER_01").
		WillReturnResult(sqlmock.NewResult(0, 5))
	mock.ExpectCommit()

	ctrls.UpdateTranscriptSpeakers(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d (body: %s)", w.Code, w.Body.String())
	}

	var resp dtos.APIResponse[dtos.UpdateSpeakersResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse json response: %v", err)
	}
	if resp.Data.UpdatedCount != 5 {
		t.Errorf("expected UpdatedCount 5, got %d", resp.Data.UpdatedCount)
	}
}

func TestControllers_RegenerateSummary_NilController(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: uuid.New().String()}}
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/recordings/"+uuid.New().String()+"/regenerate", nil)

	var ctrls *Controllers
	ctrls.RegenerateSummary(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 Internal Server Error, got %d", w.Code)
	}
}

func TestControllers_RegenerateSummary_NilService(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: uuid.New().String()}}
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/recordings/"+uuid.New().String()+"/regenerate", nil)

	ctrls := &Controllers{}
	ctrls.RegenerateSummary(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 Internal Server Error, got %d", w.Code)
	}
}

func TestControllers_RegenerateSummary_InvalidUUID(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "invalid-uuid"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/recordings/invalid-uuid/regenerate", nil)

	ctrls.RegenerateSummary(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_RegenerateSummary_InvalidJSON(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/regenerate", strings.NewReader(`{invalid json`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	ctrls.RegenerateSummary(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_RegenerateSummary_ValidationError(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	tooLongCategory := strings.Repeat("c", 101)
	body, _ := json.Marshal(dtos.RegenerateSummaryRequest{TemplateCategory: tooLongCategory})
	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/regenerate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	ctrls.RegenerateSummary(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request on validation error, got %d", w.Code)
	}
}

func TestControllers_RegenerateSummary_NotFound(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/regenerate", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	ctrls.RegenerateSummary(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", w.Code)
	}
}

func TestControllers_RegenerateSummary_Forbidden(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/regenerate", strings.NewReader(`{"ownership_token":"wrong"}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token"}).
			AddRow(recID, "secret-token"))

	ctrls.RegenerateSummary(c)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden, got %d", w.Code)
	}
}

func TestControllers_RegenerateSummary_ConflictProcessing(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	guestToken := "guest-token"
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/regenerate", strings.NewReader(`{"ownership_token":"`+guestToken+`"}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "status"}).
			AddRow(recID, guestToken, models.RecordingStatusTranscribing))

	ctrls.RegenerateSummary(c)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict, got %d", w.Code)
	}

	var resp dtos.BaseResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Code != "CONFLICT_PROCESSING" {
		t.Errorf("expected code CONFLICT_PROCESSING, got %s", resp.Code)
	}
}

func TestControllers_RegenerateSummary_VersionLimit(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	guestToken := "guest-token"
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/regenerate", strings.NewReader(`{"ownership_token":"`+guestToken+`"}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "status"}).
			AddRow(recID, guestToken, models.RecordingStatusCompleted))

	mock.ExpectQuery(`SELECT count\(\*\) FROM "summaries" WHERE recording_id = \$1`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(5))

	ctrls.RegenerateSummary(c)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict, got %d", w.Code)
	}

	var resp dtos.BaseResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Code != "SUMMARY_VERSION_LIMIT" {
		t.Errorf("expected code SUMMARY_VERSION_LIMIT, got %s", resp.Code)
	}
}

func TestControllers_RegenerateSummary_Success_BodyToken(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	ctrls.svc.SetLLM(&mockControllerLLM{})

	recID := uuid.New()
	guestToken := "guest-token"
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	bodyJSON := `{"template_category":"MOM","custom_angle":"Focus on decisions","ownership_token":"` + guestToken + `"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/regenerate", strings.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "status", "detected_language", "output_language", "selected_template"}).
			AddRow(recID, guestToken, models.RecordingStatusCompleted, "en", "en", "GENERAL"))

	mock.ExpectQuery(`SELECT count\(\*\) FROM "summaries" WHERE recording_id = \$1`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	mock.ExpectQuery(`SELECT \* FROM "transcript_segments" WHERE recording_id = \$1 ORDER BY sequence_order ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "speaker_name", "text", "start_time", "end_time"}).
			AddRow(uuid.New(), recID, "Bayu", "Discussing database schema.", 0.0, 10.0))

	mock.ExpectQuery(`SELECT \* FROM "templates" WHERE category_key = \$1 AND is_active = TRUE.*LIMIT \$2`).
		WithArgs("MOM", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "category_key", "prompt", "output_schema"}).
			AddRow(uuid.New(), "MOM", "MOM prompt", "{}"))

	// SaveNewSummaryVersion expectations
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT count\(\*\) FROM "summaries" WHERE recording_id = \$1`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`SELECT COALESCE\(MAX\(version\), 0\) FROM "summaries" WHERE recording_id = \$1`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"coalesce"}).AddRow(1))
	mock.ExpectExec(`UPDATE "summaries" SET "is_active"=\$1,"updated_at"=\$2 WHERE recording_id = \$3`).
		WithArgs(false, sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(`INSERT INTO "summaries"`).
		WithArgs(recID, "MOM", sqlmock.AnyArg(), 2, true, sqlmock.AnyArg(), "# Regenerated Summary", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(uuid.New(), time.Now(), time.Now()))
	mock.ExpectCommit()

	ctrls.RegenerateSummary(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d (body: %s)", w.Code, w.Body.String())
	}

	var resp dtos.APIResponse[dtos.SummaryVersionResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse json response: %v", err)
	}
	if resp.Data.Version != 2 {
		t.Errorf("expected version 2, got %d", resp.Data.Version)
	}
	if resp.Data.TemplateCategory != "MOM" {
		t.Errorf("expected template MOM, got %s", resp.Data.TemplateCategory)
	}
	if resp.Data.CustomAngle == nil || *resp.Data.CustomAngle != "Focus on decisions" {
		t.Errorf("expected custom angle 'Focus on decisions', got %v", resp.Data.CustomAngle)
	}
	if resp.Data.MarkdownContent != "# Regenerated Summary" {
		t.Errorf("unexpected MarkdownContent: %s", resp.Data.MarkdownContent)
	}
}

func TestControllers_RegenerateSummary_Success_HeaderToken(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	ctrls.svc.SetLLM(&mockControllerLLM{})

	recID := uuid.New()
	guestToken := "header-ownership-token"
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/regenerate", nil)
	req.Header.Set("X-Ownership-Token", guestToken)
	c.Request = req

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "status", "detected_language", "output_language", "selected_template"}).
			AddRow(recID, guestToken, models.RecordingStatusCompleted, "id", "id", "GENERAL"))

	mock.ExpectQuery(`SELECT count\(\*\) FROM "summaries" WHERE recording_id = \$1`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	mock.ExpectQuery(`SELECT \* FROM "transcript_segments" WHERE recording_id = \$1 ORDER BY sequence_order ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "speaker_name", "text", "start_time", "end_time"}).
			AddRow(uuid.New(), recID, "Alice", "Pembahasan sistem.", 0.0, 10.0))

	mock.ExpectQuery(`SELECT \* FROM "templates" WHERE category_key = \$1 AND is_active = TRUE.*LIMIT \$2`).
		WithArgs("GENERAL", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "category_key", "prompt", "output_schema"}).
			AddRow(uuid.New(), "GENERAL", "General prompt", "{}"))

	// SaveNewSummaryVersion expectations
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT count\(\*\) FROM "summaries" WHERE recording_id = \$1`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectQuery(`SELECT COALESCE\(MAX\(version\), 0\) FROM "summaries" WHERE recording_id = \$1`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"coalesce"}).AddRow(2))
	mock.ExpectExec(`UPDATE "summaries" SET "is_active"=\$1,"updated_at"=\$2 WHERE recording_id = \$3`).
		WithArgs(false, sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 2))
	mock.ExpectQuery(`INSERT INTO "summaries"`).
		WithArgs(recID, "GENERAL", nil, 3, true, sqlmock.AnyArg(), "# Regenerated Summary", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(uuid.New(), time.Now(), time.Now()))
	mock.ExpectCommit()

	ctrls.RegenerateSummary(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d (body: %s)", w.Code, w.Body.String())
	}

	var resp dtos.APIResponse[dtos.SummaryVersionResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse json response: %v", err)
	}
	if resp.Data.Version != 3 {
		t.Errorf("expected version 3, got %d", resp.Data.Version)
	}
}

func TestControllers_ListSummaryVersions_NilController(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/v1/recordings/test/summaries", nil)

	var ctrls *Controllers
	ctrls.ListSummaryVersions(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestControllers_ListSummaryVersions_NilService(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/v1/recordings/test/summaries", nil)

	ctrls := &Controllers{}
	ctrls.ListSummaryVersions(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestControllers_ListSummaryVersions_InvalidUUID(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "invalid-uuid"}}
	c.Request, _ = http.NewRequest(http.MethodGet, "/v1/recordings/invalid-uuid/summaries", nil)

	ctrls.ListSummaryVersions(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_ListSummaryVersions_NotFound(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	c.Request, _ = http.NewRequest(http.MethodGet, "/v1/recordings/"+recID.String()+"/summaries", nil)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	ctrls.ListSummaryVersions(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", w.Code)
	}
}

func TestControllers_ListSummaryVersions_Forbidden(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	c.Request, _ = http.NewRequest(http.MethodGet, "/v1/recordings/"+recID.String()+"/summaries", nil)
	c.Request.Header.Set("X-Ownership-Token", "wrong-token")

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id"}).
			AddRow(recID, "valid-token", nil))

	ctrls.ListSummaryVersions(c)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden, got %d", w.Code)
	}
}

func TestControllers_ListSummaryVersions_Success_GuestWithHeader(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	guestToken := "guest-token-123"

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req, _ := http.NewRequest(http.MethodGet, "/v1/recordings/"+recID.String()+"/summaries", nil)
	req.Header.Set("X-Ownership-Token", guestToken)
	c.Request = req

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id"}).
			AddRow(recID, guestToken, nil))

	sum1ID := uuid.New()
	sum2ID := uuid.New()
	now := time.Now()

	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 ORDER BY version ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "version", "template_category", "structured_data", "markdown_content", "is_active", "created_at"}).
			AddRow(sum1ID, recID, 1, "GENERAL", models.JSONMap{"summary": "v1"}, "# V1", false, now.Add(-10*time.Minute)).
			AddRow(sum2ID, recID, 2, "MOM", models.JSONMap{"summary": "v2"}, "# V2", true, now))

	ctrls.ListSummaryVersions(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d (body: %s)", w.Code, w.Body.String())
	}

	var resp dtos.APIResponse[[]dtos.SummaryVersionResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode json response: %v", err)
	}

	if len(resp.Data) != 2 {
		t.Fatalf("expected 2 summary versions, got %d", len(resp.Data))
	}
	if resp.Data[0].Version != 1 || resp.Data[0].IsActive {
		t.Errorf("unexpected version 1 data: %+v", resp.Data[0])
	}
	if resp.Data[1].Version != 2 || !resp.Data[1].IsActive {
		t.Errorf("unexpected version 2 data: %+v", resp.Data[1])
	}
}

func TestControllers_ListSummaryVersions_Success_GuestWithQuery(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	guestToken := "guest-token-123"

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req, _ := http.NewRequest(http.MethodGet, "/v1/recordings/"+recID.String()+"/summaries?token="+guestToken, nil)
	c.Request = req

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id"}).
			AddRow(recID, guestToken, nil))

	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 ORDER BY version ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "version", "template_category", "structured_data", "markdown_content", "is_active", "created_at"}).
			AddRow(uuid.New(), recID, 1, "GENERAL", models.JSONMap{"summary": "v1"}, "# V1", true, time.Now()))

	ctrls.ListSummaryVersions(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestControllers_ListSummaryVersions_Success_AuthenticatedOwner(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	userID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req, _ := http.NewRequest(http.MethodGet, "/v1/recordings/"+recID.String()+"/summaries", nil)
	c.Request = req.WithContext(ctxmeta.WithAuthUser(req.Context(), ctxmeta.AuthUser{UserID: userID}))

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id"}).
			AddRow(recID, "token", &userID))

	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 ORDER BY version ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "version", "template_category", "structured_data", "markdown_content", "is_active", "created_at"}).
			AddRow(uuid.New(), recID, 1, "GENERAL", models.JSONMap{"summary": "v1"}, "# V1", true, time.Now()))

	ctrls.ListSummaryVersions(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestControllers_ActivateSummaryVersion_NilController(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPatch, "/v1/recordings/test/summaries/1/activate", nil)

	var ctrls *Controllers
	ctrls.ActivateSummaryVersion(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestControllers_ActivateSummaryVersion_NilService(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPatch, "/v1/recordings/test/summaries/1/activate", nil)

	ctrls := &Controllers{}
	ctrls.ActivateSummaryVersion(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestControllers_ActivateSummaryVersion_InvalidID(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: "invalid-uuid"},
		{Key: "versionId", Value: "1"},
	}
	c.Request, _ = http.NewRequest(http.MethodPatch, "/v1/recordings/invalid-uuid/summaries/1/activate", nil)

	ctrls.ActivateSummaryVersion(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_ActivateSummaryVersion_NotFound_Recording(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: recID.String()},
		{Key: "versionId", Value: "1"},
	}
	c.Request, _ = http.NewRequest(http.MethodPatch, "/v1/recordings/"+recID.String()+"/summaries/1/activate", nil)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	ctrls.ActivateSummaryVersion(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", w.Code)
	}
}

func TestControllers_ActivateSummaryVersion_NotFound_Summary(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	missingSumID := uuid.New()
	guestToken := "guest-token"

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: recID.String()},
		{Key: "versionId", Value: missingSumID.String()},
	}
	req, _ := http.NewRequest(http.MethodPatch, "/v1/recordings/"+recID.String()+"/summaries/"+missingSumID.String()+"/activate", nil)
	req.Header.Set("X-Ownership-Token", guestToken)
	c.Request = req

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id"}).
			AddRow(recID, guestToken, nil))

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND id = \$2.*LIMIT \$3`).
		WithArgs(recID, missingSumID, 1).
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectRollback()

	ctrls.ActivateSummaryVersion(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestControllers_ActivateSummaryVersion_Forbidden(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: recID.String()},
		{Key: "versionId", Value: "1"},
	}
	req, _ := http.NewRequest(http.MethodPatch, "/v1/recordings/"+recID.String()+"/summaries/1/activate", nil)
	req.Header.Set("X-Ownership-Token", "wrong-token")
	c.Request = req

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id"}).
			AddRow(recID, "valid-token", nil))

	ctrls.ActivateSummaryVersion(c)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden, got %d", w.Code)
	}
}

func TestControllers_ActivateSummaryVersion_Success_GuestWithHeader(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	targetSumID := uuid.New()
	guestToken := "guest-token"

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: recID.String()},
		{Key: "versionId", Value: targetSumID.String()},
	}
	req, _ := http.NewRequest(http.MethodPatch, "/v1/recordings/"+recID.String()+"/summaries/"+targetSumID.String()+"/activate", nil)
	req.Header.Set("X-Ownership-Token", guestToken)
	c.Request = req

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id"}).
			AddRow(recID, guestToken, nil))

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND id = \$2.*LIMIT \$3`).
		WithArgs(recID, targetSumID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "version", "template_category", "structured_data", "markdown_content", "is_active", "created_at"}).
			AddRow(targetSumID, recID, 2, "GENERAL", models.JSONMap{"summary": "v2"}, "# V2", false, time.Now()))
	mock.ExpectExec(`UPDATE "summaries" SET "is_active"=\$1,"updated_at"=\$2 WHERE recording_id = \$3`).
		WithArgs(false, sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 2))
	mock.ExpectExec(`UPDATE "summaries" SET "is_active"=\$1,"updated_at"=\$2 WHERE id = \$3`).
		WithArgs(true, sqlmock.AnyArg(), targetSumID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	ctrls.ActivateSummaryVersion(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d (body: %s)", w.Code, w.Body.String())
	}

	var resp dtos.APIResponse[dtos.SummaryVersionResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response json: %v", err)
	}

	if resp.Data.ID != targetSumID.String() || !resp.Data.IsActive || resp.Data.Version != 2 {
		t.Errorf("unexpected activated response data: %+v", resp.Data)
	}
}

func TestControllers_ActivateSummaryVersion_Success_AuthenticatedOwner(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	targetSumID := uuid.New()
	userID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: recID.String()},
		{Key: "versionId", Value: "3"},
	}
	req, _ := http.NewRequest(http.MethodPatch, "/v1/recordings/"+recID.String()+"/summaries/3/activate", nil)
	c.Request = req.WithContext(ctxmeta.WithAuthUser(req.Context(), ctxmeta.AuthUser{UserID: userID}))

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id"}).
			AddRow(recID, "token", &userID))

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND version = \$2.*LIMIT \$3`).
		WithArgs(recID, 3, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "version", "template_category", "structured_data", "markdown_content", "is_active", "created_at"}).
			AddRow(targetSumID, recID, 3, "EXECUTIVE", models.JSONMap{"summary": "v3"}, "# V3", false, time.Now()))
	mock.ExpectExec(`UPDATE "summaries" SET "is_active"=\$1,"updated_at"=\$2 WHERE recording_id = \$3`).
		WithArgs(false, sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 3))
	mock.ExpectExec(`UPDATE "summaries" SET "is_active"=\$1,"updated_at"=\$2 WHERE id = \$3`).
		WithArgs(true, sqlmock.AnyArg(), targetSumID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	ctrls.ActivateSummaryVersion(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestControllers_CreateInlineComment_NilReceiver(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/recordings/test/comments", nil)

	var ctrls *Controllers
	ctrls.CreateInlineComment(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 Internal Server Error, got %d", w.Code)
	}
}

func TestControllers_CreateInlineComment_NilService(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/recordings/test/comments", nil)

	ctrls := &Controllers{}
	ctrls.CreateInlineComment(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 Internal Server Error, got %d", w.Code)
	}
}

func TestControllers_CreateInlineComment_InvalidUUID(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "invalid-uuid"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/recordings/invalid-uuid/comments", strings.NewReader(`{}`))
	c.Request.Header.Set("Content-Type", "application/json")

	ctrls.CreateInlineComment(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_CreateInlineComment_BindError(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/comments", strings.NewReader("invalid-json"))
	c.Request.Header.Set("Content-Type", "application/json")

	ctrls.CreateInlineComment(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_CreateInlineComment_ValidationError(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/comments", strings.NewReader(`{"comment_text":""}`))
	c.Request.Header.Set("Content-Type", "application/json")

	ctrls.CreateInlineComment(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_CreateInlineComment_RecordingNotFound(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	body := `{"comment_text":"Good point","author_name":"Tester"}`
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/comments", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	ctrls.CreateInlineComment(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", w.Code)
	}
}

func TestControllers_CreateInlineComment_Forbidden(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	body := `{"comment_text":"Good point","author_name":"Tester"}`
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/comments", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id", "is_share_enabled"}).
			AddRow(recID, "secret-token", nil, false))

	ctrls.CreateInlineComment(c)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden, got %d", w.Code)
	}
}

func TestControllers_CreateInlineComment_Success_GuestWithHeader(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	guestToken := "guest-token-123"
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	body := `{"comment_text":"Great explanation here!","author_name":"Alice","timestamp_sec":12.5}`
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/comments", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("X-Ownership-Token", guestToken)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id", "is_share_enabled"}).
			AddRow(recID, guestToken, nil, false))

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "inline_comments"`).
		WithArgs(recID, nil, nil, 12.5, nil, "Alice", "Great explanation here!", nil, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).
			AddRow(uuid.New(), time.Now(), time.Now()))
	mock.ExpectCommit()

	ctrls.CreateInlineComment(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp dtos.APIResponse[dtos.CommentResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode json: %v", err)
	}
	if resp.Data.CommentText != "Great explanation here!" || resp.Data.AuthorName != "Alice" {
		t.Errorf("unexpected comment data: %+v", resp.Data)
	}
}

func TestControllers_CreateInlineComment_Success_ReplyToParent(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	parentID := uuid.New()
	guestToken := "guest-token-123"
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	body := fmt.Sprintf(`{"comment_text":"I agree with this","parent_id":"%s","ownership_token":"%s"}`, parentID.String(), guestToken)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/comments", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id", "is_share_enabled"}).
			AddRow(recID, guestToken, nil, false))

	mock.ExpectQuery(`SELECT \* FROM "inline_comments" WHERE id = \$1.*LIMIT \$2`).
		WithArgs(parentID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "comment_text"}).
			AddRow(parentID, recID, "Parent comment"))
	mock.ExpectQuery(`SELECT \* FROM "inline_comments" WHERE "inline_comments"\."parent_id" = \$1`).
		WithArgs(parentID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "parent_id"}))

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "inline_comments"`).
		WithArgs(recID, nil, nil, 0.0, nil, "Anonymous", "I agree with this", parentID, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).
			AddRow(uuid.New(), time.Now(), time.Now()))
	mock.ExpectCommit()

	ctrls.CreateInlineComment(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp dtos.APIResponse[dtos.CommentResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode json: %v", err)
	}
	if resp.Data.ParentID == nil || *resp.Data.ParentID != parentID.String() {
		t.Errorf("expected parent ID %s, got %v", parentID.String(), resp.Data.ParentID)
	}
}

func TestControllers_ListInlineComments_NilReceiver(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings/test/comments", nil)

	var ctrls *Controllers
	ctrls.ListInlineComments(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 Internal Server Error, got %d", w.Code)
	}
}

func TestControllers_ListInlineComments_NilService(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings/test/comments", nil)

	ctrls := &Controllers{}
	ctrls.ListInlineComments(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 Internal Server Error, got %d", w.Code)
	}
}

func TestControllers_ListInlineComments_InvalidUUID(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "invalid-uuid"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings/invalid-uuid/comments", nil)

	ctrls.ListInlineComments(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_ListInlineComments_RecordingNotFound(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings/"+recID.String()+"/comments", nil)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	ctrls.ListInlineComments(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", w.Code)
	}
}

func TestControllers_ListInlineComments_Forbidden(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings/"+recID.String()+"/comments", nil)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id", "is_share_enabled"}).
			AddRow(recID, "secret-token", nil, false))

	ctrls.ListInlineComments(c)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden, got %d", w.Code)
	}
}

func TestControllers_ListInlineComments_Success_GuestWithHeader(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	commID := uuid.New()
	replyID := uuid.New()
	guestToken := "guest-token-123"

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req := httptest.NewRequest(http.MethodGet, "/v1/recordings/"+recID.String()+"/comments", nil)
	req.Header.Set("X-Ownership-Token", guestToken)
	c.Request = req

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id", "is_share_enabled"}).
			AddRow(recID, guestToken, nil, false))

	mock.ExpectQuery(`SELECT \* FROM "inline_comments" WHERE recording_id = \$1 AND parent_id IS NULL ORDER BY timestamp_sec ASC, created_at ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "timestamp_sec", "author_name", "comment_text", "parent_id", "created_at", "updated_at"}).
			AddRow(commID, recID, 14.0, "Alice", "Top question", nil, time.Now(), time.Now()))

	mock.ExpectQuery(`SELECT \* FROM "inline_comments" WHERE "inline_comments"\."parent_id" = \$1`).
		WithArgs(commID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "timestamp_sec", "author_name", "comment_text", "parent_id", "created_at", "updated_at"}).
			AddRow(replyID, recID, 14.0, "Bob", "Reply here", commID, time.Now(), time.Now()))

	ctrls.ListInlineComments(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp dtos.APIResponse[[]dtos.CommentResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode json: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(resp.Data))
	}
	if resp.Data[0].ID != commID.String() {
		t.Errorf("expected ID %s, got %s", commID.String(), resp.Data[0].ID)
	}
	if len(resp.Data[0].Replies) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(resp.Data[0].Replies))
	}
	if resp.Data[0].Replies[0].ID != replyID.String() {
		t.Errorf("expected reply ID %s, got %s", replyID.String(), resp.Data[0].Replies[0].ID)
	}
}

func TestControllers_ListInlineComments_Success_AuthenticatedOwner(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	userID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req := httptest.NewRequest(http.MethodGet, "/v1/recordings/"+recID.String()+"/comments", nil)
	c.Request = req.WithContext(ctxmeta.WithAuthUser(req.Context(), ctxmeta.AuthUser{UserID: userID}))

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id", "is_share_enabled"}).
			AddRow(recID, "token", &userID, false))

	mock.ExpectQuery(`SELECT \* FROM "inline_comments" WHERE recording_id = \$1 AND parent_id IS NULL ORDER BY timestamp_sec ASC, created_at ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "timestamp_sec", "comment_text", "parent_id"}))

	ctrls.ListInlineComments(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp dtos.APIResponse[[]dtos.CommentResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode json: %v", err)
	}
	if len(resp.Data) != 0 {
		t.Fatalf("expected 0 comments, got %d", len(resp.Data))
	}
}

func TestControllers_DeleteInlineComment_NilReceiver(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodDelete, "/v1/recordings/test/comments/test", nil)

	var ctrls *Controllers
	ctrls.DeleteInlineComment(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 Internal Server Error, got %d", w.Code)
	}
}

func TestControllers_DeleteInlineComment_NilService(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodDelete, "/v1/recordings/test/comments/test", nil)

	ctrls := &Controllers{}
	ctrls.DeleteInlineComment(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 Internal Server Error, got %d", w.Code)
	}
}

func TestControllers_DeleteInlineComment_InvalidRecordingUUID(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: "invalid-uuid"},
		{Key: "commentId", Value: uuid.New().String()},
	}
	c.Request = httptest.NewRequest(http.MethodDelete, "/v1/recordings/invalid-uuid/comments/test", nil)

	ctrls.DeleteInlineComment(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_DeleteInlineComment_InvalidCommentUUID(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: uuid.New().String()},
		{Key: "commentId", Value: "invalid-comment-uuid"},
	}
	c.Request = httptest.NewRequest(http.MethodDelete, "/v1/recordings/test/comments/invalid", nil)

	ctrls.DeleteInlineComment(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_DeleteInlineComment_RecordingNotFound(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	commID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: recID.String()},
		{Key: "commentId", Value: commID.String()},
	}
	c.Request = httptest.NewRequest(http.MethodDelete, "/v1/recordings/"+recID.String()+"/comments/"+commID.String(), nil)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	ctrls.DeleteInlineComment(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", w.Code)
	}
}

func TestControllers_DeleteInlineComment_CommentNotFound(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	commID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: recID.String()},
		{Key: "commentId", Value: commID.String()},
	}
	c.Request = httptest.NewRequest(http.MethodDelete, "/v1/recordings/"+recID.String()+"/comments/"+commID.String(), nil)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id", "is_share_enabled"}).
			AddRow(recID, "token", nil, false))

	mock.ExpectQuery(`SELECT \* FROM "inline_comments" WHERE id = \$1.*LIMIT \$2`).
		WithArgs(commID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	ctrls.DeleteInlineComment(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", w.Code)
	}
}

func TestControllers_DeleteInlineComment_Forbidden(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	commID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: recID.String()},
		{Key: "commentId", Value: commID.String()},
	}
	c.Request = httptest.NewRequest(http.MethodDelete, "/v1/recordings/"+recID.String()+"/comments/"+commID.String(), nil)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id", "is_share_enabled"}).
			AddRow(recID, "secret-token", nil, false))

	mock.ExpectQuery(`SELECT \* FROM "inline_comments" WHERE id = \$1.*LIMIT \$2`).
		WithArgs(commID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "author_name", "comment_text"}).
			AddRow(commID, recID, "Alice", "Test comment"))
	mock.ExpectQuery(`SELECT \* FROM "inline_comments" WHERE "inline_comments"\."parent_id" = \$1`).
		WithArgs(commID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "parent_id"}))

	ctrls.DeleteInlineComment(c)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden, got %d", w.Code)
	}
}

func TestControllers_DeleteInlineComment_Success_RecordingOwner(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	commID := uuid.New()
	guestToken := "guest-token-123"

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: recID.String()},
		{Key: "commentId", Value: commID.String()},
	}
	req := httptest.NewRequest(http.MethodDelete, "/v1/recordings/"+recID.String()+"/comments/"+commID.String(), nil)
	req.Header.Set("X-Ownership-Token", guestToken)
	c.Request = req

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id", "is_share_enabled"}).
			AddRow(recID, guestToken, nil, false))

	mock.ExpectQuery(`SELECT \* FROM "inline_comments" WHERE id = \$1.*LIMIT \$2`).
		WithArgs(commID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "author_name", "comment_text"}).
			AddRow(commID, recID, "Someone", "Comment text"))
	mock.ExpectQuery(`SELECT \* FROM "inline_comments" WHERE "inline_comments"\."parent_id" = \$1`).
		WithArgs(commID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "parent_id"}))

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM "inline_comments" WHERE id = \$1`).
		WithArgs(commID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	ctrls.DeleteInlineComment(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestControllers_DeleteInlineComment_Success_CommentAuthor(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	commID := uuid.New()
	ownerID := uuid.New()
	authorID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: recID.String()},
		{Key: "commentId", Value: commID.String()},
	}
	req := httptest.NewRequest(http.MethodDelete, "/v1/recordings/"+recID.String()+"/comments/"+commID.String(), nil)
	c.Request = req.WithContext(ctxmeta.WithAuthUser(req.Context(), ctxmeta.AuthUser{UserID: authorID}))

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id", "is_share_enabled"}).
			AddRow(recID, "token", &ownerID, false))

	mock.ExpectQuery(`SELECT \* FROM "inline_comments" WHERE id = \$1.*LIMIT \$2`).
		WithArgs(commID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "user_id", "author_name", "comment_text"}).
			AddRow(commID, recID, &authorID, "Author Name", "My comment"))
	mock.ExpectQuery(`SELECT \* FROM "inline_comments" WHERE "inline_comments"\."parent_id" = \$1`).
		WithArgs(commID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "parent_id"}))

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM "inline_comments" WHERE id = \$1`).
		WithArgs(commID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	ctrls.DeleteInlineComment(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestControllers_ExportRecording_NilReceiver(t *testing.T) {
	var ctrls *Controllers
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: uuid.New().String()}}
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings/"+uuid.New().String()+"/export", nil)

	ctrls.ExportRecording(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 Internal Server Error, got %d", w.Code)
	}
}

func TestControllers_ExportRecording_NilService(t *testing.T) {
	ctrls := &Controllers{}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: uuid.New().String()}}
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings/"+uuid.New().String()+"/export", nil)

	ctrls.ExportRecording(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 Internal Server Error, got %d", w.Code)
	}
}

func TestControllers_ExportRecording_InvalidUUID(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "invalid-uuid"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings/invalid-uuid/export", nil)

	ctrls.ExportRecording(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_ExportRecording_RecordingNotFound(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings/"+recID.String()+"/export?format=markdown", nil)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	ctrls.ExportRecording(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", w.Code)
	}
}

func TestControllers_ExportRecording_Forbidden(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	ownerID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings/"+recID.String()+"/export?format=markdown", nil)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id", "is_share_enabled"}).
			AddRow(recID, "secret-token", &ownerID, false))

	ctrls.ExportRecording(c)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden, got %d", w.Code)
	}
}

func TestControllers_ExportRecording_Success_Markdown(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	guestToken := "guest-token-123"
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req := httptest.NewRequest(http.MethodGet, "/v1/recordings/"+recID.String()+"/export?format=markdown", nil)
	req.Header.Set("X-Ownership-Token", guestToken)
	c.Request = req

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "title", "duration_seconds", "status", "created_at", "user_id", "ownership_token", "is_share_enabled"}).
			AddRow(recID, "Product Sync", 600.0, "COMPLETED", time.Now(), nil, guestToken, false))

	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND is_active = TRUE ORDER BY version DESC.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "version", "is_active", "markdown_content", "structured_data"}).
			AddRow(uuid.New(), recID, 1, true, "Product sync markdown summary", `{"executive_summary":"Sync summary"}`))

	mock.ExpectQuery(`SELECT \* FROM "transcript_segments" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "speaker_name", "speaker_label", "start_time", "end_time", "text"}).
			AddRow(uuid.New(), recID, "PM", "Speaker 0", 0.0, 5.0, "Let's review the roadmap."))

	mock.ExpectQuery(`SELECT \* FROM "chapters" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC.*`).
		WithArgs(recID, 100).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id"}))

	mock.ExpectQuery(`SELECT \* FROM "highlights" WHERE recording_id = \$1 ORDER BY start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id"}))

	ctrls.ExportRecording(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d, body: %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Content-Type") != "text/markdown; charset=utf-8" {
		t.Errorf("expected text/markdown, got %s", w.Header().Get("Content-Type"))
	}
	if !strings.Contains(w.Header().Get("Content-Disposition"), "attachment; filename=") {
		t.Errorf("expected Content-Disposition attachment header, got %s", w.Header().Get("Content-Disposition"))
	}
}

func TestControllers_ExportRecording_Success_JSON(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings/"+recID.String()+"/export?format=json&token=shared-token", nil)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "title", "duration_seconds", "status", "created_at", "user_id", "ownership_token", "is_share_enabled"}).
			AddRow(recID, "API Planning", 300.0, "COMPLETED", time.Now(), nil, "shared-token", false))

	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND is_active = TRUE ORDER BY version DESC.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	mock.ExpectQuery(`SELECT \* FROM "transcript_segments" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id"}))

	mock.ExpectQuery(`SELECT \* FROM "chapters" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC.*`).
		WithArgs(recID, 100).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id"}))

	mock.ExpectQuery(`SELECT \* FROM "highlights" WHERE recording_id = \$1 ORDER BY start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id"}))

	ctrls.ExportRecording(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}
	if w.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Errorf("expected application/json, got %s", w.Header().Get("Content-Type"))
	}
}

func TestControllers_ExportRecording_Success_PDF(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings/"+recID.String()+"/export?format=pdf", nil)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "title", "duration_seconds", "status", "created_at", "user_id", "ownership_token", "is_share_enabled"}).
			AddRow(recID, "PDF Export Title", 100.0, "COMPLETED", time.Now(), nil, "token", true))

	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND is_active = TRUE ORDER BY version DESC.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	mock.ExpectQuery(`SELECT \* FROM "transcript_segments" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id"}))

	mock.ExpectQuery(`SELECT \* FROM "chapters" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC.*`).
		WithArgs(recID, 100).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id"}))

	mock.ExpectQuery(`SELECT \* FROM "highlights" WHERE recording_id = \$1 ORDER BY start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id"}))

	ctrls.ExportRecording(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}
	if w.Header().Get("Content-Type") != "application/pdf" {
		t.Errorf("expected application/pdf, got %s", w.Header().Get("Content-Type"))
	}
	if !bytes.HasPrefix(w.Body.Bytes(), []byte("%PDF-1.4")) {
		t.Errorf("expected PDF header in body")
	}
}

func TestControllers_ExportRecording_Success_Txt(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings/"+recID.String()+"/export?format=txt", nil)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "title", "duration_seconds", "status", "created_at", "user_id", "ownership_token", "is_share_enabled"}).
			AddRow(recID, "Txt Export Title", 100.0, "COMPLETED", time.Now(), nil, "token", true))

	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND is_active = TRUE ORDER BY version DESC.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	mock.ExpectQuery(`SELECT \* FROM "transcript_segments" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id"}))

	mock.ExpectQuery(`SELECT \* FROM "chapters" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC.*`).
		WithArgs(recID, 100).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id"}))

	mock.ExpectQuery(`SELECT \* FROM "highlights" WHERE recording_id = \$1 ORDER BY start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id"}))

	ctrls.ExportRecording(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}
	if w.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Errorf("expected text/plain, got %s", w.Header().Get("Content-Type"))
	}
}









