package controllers

import (
	"bytes"
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
	gormPostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"

	"code-base-golang/internal/config"
	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/pkg/ctxmeta"
	"code-base-golang/internal/repositories"
	"code-base-golang/internal/services"
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
