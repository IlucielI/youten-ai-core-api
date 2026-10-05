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



