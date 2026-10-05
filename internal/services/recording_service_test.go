package services_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"code-base-golang/internal/config"
	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
	"code-base-golang/internal/pkg/ctxmeta"
	"code-base-golang/internal/repositories"
	"code-base-golang/internal/services"
)

type testRecordingStorage struct {
	uploadErr  error
	presignErr error
}

func (m *testRecordingStorage) Ping(ctx context.Context) error { return nil }
func (m *testRecordingStorage) Upload(ctx context.Context, bucketName, objectName string, reader io.Reader, size int64, contentType string) error {
	return m.uploadErr
}
func (m *testRecordingStorage) Download(ctx context.Context, bucketName, objectName string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (m *testRecordingStorage) Delete(ctx context.Context, bucketName, objectName string) error { return nil }
func (m *testRecordingStorage) PresignGetObject(ctx context.Context, bucketName, objectName string, expiry time.Duration) (string, error) {
	if m.presignErr != nil {
		return "", m.presignErr
	}
	return "https://mock.storage/get", nil
}
func (m *testRecordingStorage) PresignPutObject(ctx context.Context, bucketName, objectName string, expiry time.Duration) (string, error) {
	if m.presignErr != nil {
		return "", m.presignErr
	}
	return "https://mock.storage/recordings/upload-url", nil
}

type testEventPublisher struct {
	publishedTopics []string
	publishErr      error
}

func (m *testEventPublisher) Ping(ctx context.Context) error { return nil }
func (m *testEventPublisher) Publish(ctx context.Context, topic string, payload any) error {
	m.publishedTopics = append(m.publishedTopics, topic)
	return m.publishErr
}
func (m *testEventPublisher) MustPublish(ctx context.Context, topic string, payload any) {
	m.publishedTopics = append(m.publishedTopics, topic)
}

func setupRecordingTestService(t *testing.T) (*services.Service, sqlmock.Sqlmock, *testRecordingStorage, *testEventPublisher) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}

	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("there were unfulfilled sqlmock expectations: %s", err)
		}
		db.Close()
	})

	dialector := postgres.New(postgres.Config{
		Conn:       db,
		DriverName: "postgres",
	})
	gormDB, err := gorm.Open(dialector, &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open gorm connection: %v", err)
	}

	repo := repositories.New(gormDB)
	storage := &testRecordingStorage{}
	publisher := &testEventPublisher{}
	cfg := config.Config{S3BucketName: "test-bucket"}
	svc := services.New(cfg, repo, storage, publisher)

	return svc, mock, storage, publisher
}

func TestService_GeneratePresignUpload_InvalidFilename(t *testing.T) {
	svc, _, _, _ := setupRecordingTestService(t)
	_, err := svc.GeneratePresignUpload(context.Background(), dtos.PresignUploadRequest{
		Filename:    "",
		ContentType: "audio/mpeg",
	})
	if err == nil {
		t.Error("expected error for empty filename, got nil")
	}
}

func TestService_GeneratePresignUpload_PreflightChecks(t *testing.T) {
	svc, _, _, _ := setupRecordingTestService(t)

	// Unsupported MIME type
	_, err := svc.GeneratePresignUpload(context.Background(), dtos.PresignUploadRequest{
		Filename:    "file.pdf",
		ContentType: "application/pdf",
	})
	if err != constants.ErrUnsupportedMediaType {
		t.Errorf("expected ErrUnsupportedMediaType, got %v", err)
	}
}

func TestService_GeneratePresignUpload_GuestQuotaExceeded(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	ctx := ctxmeta.WithClientMeta(context.Background(), "192.168.1.10", "Agent")

	mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings" WHERE \(is_guest = TRUE AND guest_ip = \$1 AND created_at >= \$2\) AND "recordings"\."deleted_at" IS NULL`).
		WithArgs("192.168.1.10", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	_, err := svc.GeneratePresignUpload(ctx, dtos.PresignUploadRequest{
		Filename: "guest.mp3",
	})
	if err != constants.ErrGuestDailyQuotaExceeded {
		t.Fatalf("expected ErrGuestDailyQuotaExceeded, got %v", err)
	}
}

func TestService_GeneratePresignUpload_AuthUserInactive(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "users" WHERE id = \$1 AND "users"\."deleted_at" IS NULL ORDER BY "users"\."id" LIMIT \$2`).
		WithArgs(userID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).AddRow(userID, constants.UserStatusSuspended))

	_, err := svc.GeneratePresignUpload(ctx, dtos.PresignUploadRequest{
		Filename: "auth.mp3",
	})
	if err != constants.ErrUserInactive {
		t.Fatalf("expected ErrUserInactive, got %v", err)
	}
}

func TestService_GeneratePresignUpload_AuthUserQuotaExceeded(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "users" WHERE id = \$1 AND "users"\."deleted_at" IS NULL ORDER BY "users"\."id" LIMIT \$2`).
		WithArgs(userID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "daily_quota_override"}).AddRow(userID, constants.UserStatusActive, nil))

	mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings" WHERE \(user_id = \$1 AND created_at >= \$2\) AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(userID, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(constants.DefaultUserDailyQuota))

	_, err := svc.GeneratePresignUpload(ctx, dtos.PresignUploadRequest{
		Filename: "auth.mp3",
	})
	if err != constants.ErrDailyQuotaExceeded {
		t.Fatalf("expected ErrDailyQuotaExceeded, got %v", err)
	}
}

func TestService_GeneratePresignUpload_StoragePresignError(t *testing.T) {
	svc, mock, storage, _ := setupRecordingTestService(t)
	storage.presignErr = errors.New("s3 connection timeout")
	ctx := ctxmeta.WithClientMeta(context.Background(), "127.0.0.1", "Agent")

	mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings"`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	_, err := svc.GeneratePresignUpload(ctx, dtos.PresignUploadRequest{
		Filename: "guest.mp3",
	})
	if err == nil {
		t.Fatal("expected error from storage presign, got nil")
	}
	if !strings.Contains(err.Error(), "s3 connection timeout") {
		t.Errorf("expected error to contain 's3 connection timeout', got %v", err)
	}
}

func TestService_GeneratePresignUpload_Success(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	ctx := ctxmeta.WithClientMeta(context.Background(), "127.0.0.1", "Agent")

	mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings"`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	resp, err := svc.GeneratePresignUpload(ctx, dtos.PresignUploadRequest{
		Filename: "meeting.mp4",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.UploadURL != "https://mock.storage/recordings/upload-url" {
		t.Errorf("expected upload url 'https://mock.storage/recordings/upload-url', got %s", resp.UploadURL)
	}
	if resp.Filename != "meeting.mp4" {
		t.Errorf("expected filename 'meeting.mp4', got %s", resp.Filename)
	}
	if !strings.HasPrefix(resp.ObjectKey, "recordings/") || !strings.HasSuffix(resp.ObjectKey, "/meeting.mp4") {
		t.Errorf("expected object key pattern 'recordings/<uuid>/meeting.mp4', got %s", resp.ObjectKey)
	}
}

func TestService_UploadRecording_InvalidFilename(t *testing.T) {
	svc, _, _, _ := setupRecordingTestService(t)
	_, err := svc.UploadRecording(context.Background(), dtos.UploadRecordingRequest{
		Filename: "",
	})
	if err == nil {
		t.Error("expected error for empty filename, got nil")
	}
}

func TestService_UploadRecording_GuestSuccess(t *testing.T) {
	svc, mock, _, pub := setupRecordingTestService(t)
	ctx := ctxmeta.WithClientMeta(context.Background(), "127.0.0.1", "Agent")

	// Quota check
	mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings"`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	recID := uuid.New()
	now := time.Now()
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "recordings"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(recID, now, now))
	mock.ExpectCommit()

	resp, err := svc.UploadRecording(ctx, dtos.UploadRecordingRequest{
		Filename:  "meeting.mp4",
		ObjectKey: "recordings/uuid/meeting.mp4",
		Title:     "Custom Meeting Title",
		Template:  "MOM",
		Language:  "en",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Title != "Custom Meeting Title" {
		t.Errorf("expected title 'Custom Meeting Title', got %q", resp.Title)
	}
	if resp.OriginalFilename != "meeting.mp4" {
		t.Errorf("expected original filename 'meeting.mp4', got %q", resp.OriginalFilename)
	}
	if !resp.IsGuest {
		t.Error("expected is_guest to be true")
	}
	if resp.OwnershipToken == nil || *resp.OwnershipToken == "" {
		t.Error("expected non-empty ownership_token for guest recording")
	}
	if resp.SelectedTemplate != "MOM" {
		t.Errorf("expected template 'MOM', got %q", resp.SelectedTemplate)
	}
	if resp.OutputLanguage != "en" {
		t.Errorf("expected language 'en', got %q", resp.OutputLanguage)
	}

	if len(pub.publishedTopics) == 0 || pub.publishedTopics[0] != constants.TopicRecordingUploaded {
		t.Errorf("expected published topic %s, got %v", constants.TopicRecordingUploaded, pub.publishedTopics)
	}
}

func TestService_UploadRecording_AuthSuccess(t *testing.T) {
	svc, mock, _, pub := setupRecordingTestService(t)
	userID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	override := 10
	mock.ExpectQuery(`SELECT \* FROM "users" WHERE id = \$1 AND "users"\."deleted_at" IS NULL ORDER BY "users"\."id" LIMIT \$2`).
		WithArgs(userID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "daily_quota_override"}).AddRow(userID, constants.UserStatusActive, &override))

	mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings" WHERE \(user_id = \$1 AND created_at >= \$2\) AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(userID, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	recID := uuid.New()
	now := time.Now()
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "recordings"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(recID, now, now))
	mock.ExpectCommit()

	resp, err := svc.UploadRecording(ctx, dtos.UploadRecordingRequest{
		Filename: "recording.m4a",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Title != "recording" {
		t.Errorf("expected title 'recording', got %q", resp.Title)
	}
	if resp.IsGuest {
		t.Error("expected is_guest to be false")
	}
	if resp.OwnershipToken != nil {
		t.Errorf("expected ownership_token to be omitted for authenticated user, got %v", resp.OwnershipToken)
	}

	if len(pub.publishedTopics) == 0 || pub.publishedTopics[0] != constants.TopicRecordingUploaded {
		t.Errorf("expected published topic %s, got %v", constants.TopicRecordingUploaded, pub.publishedTopics)
	}
}

func TestService_UploadRecording_PublisherError(t *testing.T) {
	svc, mock, _, pub := setupRecordingTestService(t)
	pub.publishErr = errors.New("rabbitmq broker down")
	ctx := ctxmeta.WithClientMeta(context.Background(), "127.0.0.1", "Agent")

	mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings"`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	recID := uuid.New()
	now := time.Now()
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "recordings"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(recID, now, now))
	mock.ExpectCommit()

	_, err := svc.UploadRecording(ctx, dtos.UploadRecordingRequest{
		Filename: "test.mp3",
	})
	if err == nil {
		t.Fatal("expected error from failed publisher, got nil")
	}
	if !strings.Contains(err.Error(), "rabbitmq broker down") {
		t.Errorf("expected error to contain 'rabbitmq broker down', got %v", err)
	}
}

func TestService_CleanupExpiredRecordings(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	now := time.Now().Add(-2 * time.Hour)
	recID := uuid.New()
	audioKey := "recordings/test/audio.mp3"

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE \(expires_at IS NOT NULL AND expires_at <= \$1\) AND "recordings"\."deleted_at" IS NULL ORDER BY expires_at ASC LIMIT \$2`).
		WithArgs(sqlmock.AnyArg(), 100).
		WillReturnRows(sqlmock.NewRows([]string{"id", "audio_url", "expires_at"}).
			AddRow(recID, &audioKey, now))

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM "recordings" WHERE id = \$1`).
		WithArgs(recID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	count, err := svc.CleanupExpiredRecordings(context.Background(), 100)
	if err != nil {
		t.Fatalf("unexpected cleanup error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 cleaned recording, got %d", count)
	}
}

func TestService_ImportRecordingFromURL_SSRFBlocked(t *testing.T) {
	svc, _, _, _ := setupRecordingTestService(t)

	// Loopback IP
	_, err := svc.ImportRecordingFromURL(context.Background(), dtos.ImportURLRequest{
		URL: "http://127.0.0.1:8080/audio.mp3",
	})
	if !errors.Is(err, constants.ErrSSRFBlocked) {
		t.Fatalf("expected ErrSSRFBlocked for loopback, got %v", err)
	}

	// Private IP RFC 1918
	_, err = svc.ImportRecordingFromURL(context.Background(), dtos.ImportURLRequest{
		URL: "http://192.168.1.100/recording.wav",
	})
	if !errors.Is(err, constants.ErrSSRFBlocked) {
		t.Fatalf("expected ErrSSRFBlocked for private IP, got %v", err)
	}

	// AWS Cloud Metadata IP
	_, err = svc.ImportRecordingFromURL(context.Background(), dtos.ImportURLRequest{
		URL: "http://169.254.169.254/latest/meta-data/",
	})
	if !errors.Is(err, constants.ErrSSRFBlocked) {
		t.Fatalf("expected ErrSSRFBlocked for metadata IP, got %v", err)
	}
}

func TestService_ImportRecordingFromURL_InvalidScheme(t *testing.T) {
	svc, _, _, _ := setupRecordingTestService(t)

	_, err := svc.ImportRecordingFromURL(context.Background(), dtos.ImportURLRequest{
		URL: "ftp://example.com/audio.mp3",
	})
	if !errors.Is(err, constants.ErrInvalidImportURL) {
		t.Fatalf("expected ErrInvalidImportURL, got %v", err)
	}
}

func TestService_ImportRecordingFromURL_GuestQuotaExceeded(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)

	mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings" WHERE \(is_guest = TRUE AND guest_ip = \$1 AND created_at >= \$2\) AND "recordings"\."deleted_at" IS NULL`).
		WithArgs("198.51.100.99", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(constants.DefaultGuestDailyQuota))

	ctx := ctxmeta.WithClientMeta(context.Background(), "198.51.100.99", "")
	_, err := svc.ImportRecordingFromURL(ctx, dtos.ImportURLRequest{
		URL: "https://8.8.8.8/podcast.mp3",
	})
	if !errors.Is(err, constants.ErrGuestDailyQuotaExceeded) {
		t.Fatalf("expected ErrGuestDailyQuotaExceeded, got %v", err)
	}
}

func TestService_ImportRecordingFromURL_AuthQuotaExceeded(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "users" WHERE id = \$1 AND "users"\."deleted_at" IS NULL ORDER BY "users"\."id" LIMIT \$2`).
		WithArgs(userID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "daily_quota_override"}).AddRow(userID, constants.UserStatusActive, nil))

	mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings" WHERE \(user_id = \$1 AND created_at >= \$2\) AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(userID, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(constants.DefaultUserDailyQuota))

	_, err := svc.ImportRecordingFromURL(ctx, dtos.ImportURLRequest{
		URL: "https://8.8.8.8/podcast.mp3",
	})
	if !errors.Is(err, constants.ErrDailyQuotaExceeded) {
		t.Fatalf("expected ErrDailyQuotaExceeded, got %v", err)
	}
}

func TestService_ImportRecordingFromURL_FetchFailed(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)

	mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings" WHERE \(is_guest = TRUE AND guest_ip = \$1 AND created_at >= \$2\) AND "recordings"\."deleted_at" IS NULL`).
		WithArgs("", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	svc.SetMediaFetcher(func(ctx context.Context, targetURL string, timeout time.Duration) (*http.Response, error) {
		return nil, errors.New("upstream connection timed out")
	})

	_, err := svc.ImportRecordingFromURL(context.Background(), dtos.ImportURLRequest{
		URL: "https://8.8.8.8/podcast.mp3",
	})
	if err == nil || !strings.Contains(err.Error(), "upstream connection timed out") {
		t.Fatalf("expected fetch error, got %v", err)
	}
}

func TestService_ImportRecordingFromURL_UnsupportedMediaType(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)

	mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings" WHERE \(is_guest = TRUE AND guest_ip = \$1 AND created_at >= \$2\) AND "recordings"\."deleted_at" IS NULL`).
		WithArgs("", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	svc.SetMediaFetcher(func(ctx context.Context, targetURL string, timeout time.Duration) (*http.Response, error) {
		resp := &http.Response{
			StatusCode:    http.StatusOK,
			Header:        make(http.Header),
			Body:          io.NopCloser(strings.NewReader("<html>Not audio</html>")),
			ContentLength: 20,
		}
		resp.Header.Set("Content-Type", "text/html")
		return resp, nil
	})

	_, err := svc.ImportRecordingFromURL(context.Background(), dtos.ImportURLRequest{
		URL: "https://8.8.8.8/document.html",
	})
	if !errors.Is(err, constants.ErrUnsupportedMediaType) {
		t.Fatalf("expected ErrUnsupportedMediaType, got %v", err)
	}
}

func TestService_ImportRecordingFromURL_Success_Auth(t *testing.T) {
	svc, mock, _, pub := setupRecordingTestService(t)
	userID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "users" WHERE id = \$1 AND "users"\."deleted_at" IS NULL ORDER BY "users"\."id" LIMIT \$2`).
		WithArgs(userID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).AddRow(userID, constants.UserStatusActive))

	mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings" WHERE \(user_id = \$1 AND created_at >= \$2\) AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(userID, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	svc.SetMediaFetcher(func(ctx context.Context, targetURL string, timeout time.Duration) (*http.Response, error) {
		resp := &http.Response{
			StatusCode:    http.StatusOK,
			Header:        make(http.Header),
			Body:          io.NopCloser(strings.NewReader("dummy audio bytes")),
			ContentLength: 17,
		}
		resp.Header.Set("Content-Type", "audio/mpeg")
		return resp, nil
	})

	recID := uuid.New()
	now := time.Now()
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "recordings"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(recID, now, now))
	mock.ExpectCommit()

	resp, err := svc.ImportRecordingFromURL(ctx, dtos.ImportURLRequest{
		URL:      "https://8.8.8.8/audio.mp3",
		Title:    "Custom Title",
		Template: "MOM",
		Language: "en",
	})
	if err != nil {
		t.Fatalf("unexpected import error: %v", err)
	}

	if resp.Title != "Custom Title" {
		t.Errorf("expected Title Custom Title, got %s", resp.Title)
	}
	if resp.IsGuest {
		t.Error("expected IsGuest false for authenticated user")
	}
	if resp.Status != "PENDING" {
		t.Errorf("expected status PENDING, got %s", resp.Status)
	}
	if len(pub.publishedTopics) == 0 || pub.publishedTopics[0] != constants.TopicRecordingUploaded {
		t.Errorf("expected event published to %s, got %v", constants.TopicRecordingUploaded, pub.publishedTopics)
	}
}

func TestService_ImportRecordingFromURL_Success_Guest(t *testing.T) {
	svc, mock, _, pub := setupRecordingTestService(t)

	mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings" WHERE \(is_guest = TRUE AND guest_ip = \$1 AND created_at >= \$2\) AND "recordings"\."deleted_at" IS NULL`).
		WithArgs("203.0.113.5", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	svc.SetMediaFetcher(func(ctx context.Context, targetURL string, timeout time.Duration) (*http.Response, error) {
		resp := &http.Response{
			StatusCode:    http.StatusOK,
			Header:        make(http.Header),
			Body:          io.NopCloser(strings.NewReader("wav audio stream")),
			ContentLength: 16,
		}
		resp.Header.Set("Content-Type", "audio/wav")
		return resp, nil
	})

	recID := uuid.New()
	now := time.Now()
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "recordings"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(recID, now, now))
	mock.ExpectCommit()

	ctx := ctxmeta.WithClientMeta(context.Background(), "203.0.113.5", "")
	resp, err := svc.ImportRecordingFromURL(ctx, dtos.ImportURLRequest{
		URL: "https://8.8.8.8/sample.wav",
	})
	if err != nil {
		t.Fatalf("unexpected import error: %v", err)
	}

	if !resp.IsGuest {
		t.Error("expected IsGuest true for unauthenticated client")
	}
	if resp.OwnershipToken == nil || *resp.OwnershipToken == "" {
		t.Error("expected ownership token for guest")
	}
	if len(pub.publishedTopics) == 0 {
		t.Error("expected publication to background queue")
	}
}

func TestService_GetRecordingDetail_NotFound(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	_, err := svc.GetRecordingDetail(context.Background(), recID, "")
	if !errors.Is(err, constants.ErrRecordingNotFound) {
		t.Fatalf("expected ErrRecordingNotFound, got %v", err)
	}
}

func TestService_GetRecordingDetail_Forbidden_UnauthenticatedNoToken(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "is_guest"}).
			AddRow(recID, "secret-token", true))

	_, err := svc.GetRecordingDetail(context.Background(), recID, "")
	if !errors.Is(err, constants.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestService_GetRecordingDetail_Forbidden_UnauthenticatedWrongToken(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "is_guest"}).
			AddRow(recID, "secret-token", true))

	_, err := svc.GetRecordingDetail(context.Background(), recID, "invalid-token")
	if !errors.Is(err, constants.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestService_GetRecordingDetail_Forbidden_AuthenticatedDifferentUser(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	ownerID := uuid.New()
	callerID := uuid.New()

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: callerID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "ownership_token", "is_guest"}).
			AddRow(recID, ownerID, "owner-token", false))

	_, err := svc.GetRecordingDetail(ctx, recID, "")
	if !errors.Is(err, constants.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestService_GetRecordingDetail_Success_AuthenticatedOwner(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	ownerID := uuid.New()
	audioPath := "recordings/" + recID.String() + "/speech.mp3"
	now := time.Now()

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: ownerID})

	// 1. FindRecordingByID
	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "title", "original_filename", "audio_url", "source_type",
			"status", "selected_template", "output_language", "is_guest", "created_at", "updated_at",
		}).AddRow(
			recID, ownerID, "Architecture Review", "speech.mp3", audioPath, "UPLOAD",
			"COMPLETED", "MOM", "en", false, now, now,
		))

	// 2. ListTranscriptSegmentsByRecordingID
	segID := uuid.New()
	mock.ExpectQuery(`SELECT \* FROM "transcript_segments" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "recording_id", "speaker_label", "speaker_name", "start_time", "end_time", "text", "sequence_order",
		}).AddRow(segID, recID, "Speaker 0", "Alice", 0.0, 4.5, "Welcome everyone", 0))

	// 3. FindActiveSummaryByRecordingID
	sumID := uuid.New()
	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND is_active = TRUE`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "recording_id", "template_category", "version", "is_active", "markdown_content", "created_at", "updated_at",
		}).AddRow(sumID, recID, "MOM", 1, true, "## Key Decisions", now, now))

	// 4. ListChaptersByRecordingID
	chapID := uuid.New()
	mock.ExpectQuery(`SELECT \* FROM "chapters" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC LIMIT \$2`).
		WithArgs(recID, 100).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "recording_id", "title", "start_time", "end_time", "summary", "sequence_order", "created_at",
		}).AddRow(chapID, recID, "Introduction", 0.0, 60.0, "Session kickoff", 0, now))

	// 5. ListHighlightsByRecordingID
	hlID := uuid.New()
	mock.ExpectQuery(`SELECT \* FROM "highlights" WHERE recording_id = \$1 ORDER BY start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "recording_id", "start_time", "end_time", "source", "created_at",
		}).AddRow(hlID, recID, 10.0, 15.0, "manual", now))

	resp, err := svc.GetRecordingDetail(ctx, recID, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ID != recID.String() {
		t.Errorf("expected ID %s, got %s", recID.String(), resp.ID)
	}
	if resp.Title != "Architecture Review" {
		t.Errorf("expected title Architecture Review, got %s", resp.Title)
	}
	if resp.PlaybackURL == nil || *resp.PlaybackURL != "https://mock.storage/get" {
		t.Errorf("expected presigned playback URL, got %v", resp.PlaybackURL)
	}
	if len(resp.Segments) != 1 || resp.Segments[0].Text != "Welcome everyone" {
		t.Errorf("unexpected segments: %+v", resp.Segments)
	}
	if resp.ActiveSummary == nil || resp.ActiveSummary.MarkdownContent != "## Key Decisions" {
		t.Errorf("unexpected active summary: %+v", resp.ActiveSummary)
	}
	if len(resp.Chapters) != 1 || resp.Chapters[0].Title != "Introduction" {
		t.Errorf("unexpected chapters: %+v", resp.Chapters)
	}
	if len(resp.Highlights) != 1 || resp.Highlights[0].Source != "manual" {
		t.Errorf("unexpected highlights: %+v", resp.Highlights)
	}
}

func TestService_GetRecordingDetail_Success_GuestWithOwnershipToken(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	guestToken := "guest-valid-token-789"
	now := time.Now()

	// 1. FindRecordingByID
	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "title", "original_filename", "audio_url", "source_type",
			"status", "selected_template", "output_language", "is_guest", "ownership_token", "created_at", "updated_at",
		}).AddRow(
			recID, "Guest Meeting", "guest.mp3", "https://external.cdn/guest.mp3", "LINK",
			"COMPLETED", "GENERAL", "id", true, guestToken, now, now,
		))

	// 2. Empty child queries
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

	resp, err := svc.GetRecordingDetail(context.Background(), recID, guestToken)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ID != recID.String() {
		t.Errorf("expected ID %s, got %s", recID.String(), resp.ID)
	}
	if !resp.IsGuest {
		t.Error("expected IsGuest true")
	}
	if resp.PlaybackURL == nil || *resp.PlaybackURL != "https://external.cdn/guest.mp3" {
		t.Errorf("expected external playback URL retained, got %v", resp.PlaybackURL)
	}
}

func TestService_GetRecordingDetail_Success_ShareToken(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	shareToken := "public-share-token-xyz"
	now := time.Now()

	// 1. FindRecordingByID
	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "title", "original_filename", "audio_url", "source_type",
			"status", "selected_template", "output_language", "is_guest", "is_share_enabled", "share_token", "ownership_token", "created_at", "updated_at",
		}).AddRow(
			recID, "Shared Presentation", "pres.mp4", "", "UPLOAD",
			"COMPLETED", "GENERAL", "en", false, true, shareToken, "owner-only-token", now, now,
		))

	// 2. Empty child queries
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

	resp, err := svc.GetRecordingDetail(context.Background(), recID, shareToken)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ID != recID.String() {
		t.Errorf("expected ID %s, got %s", recID.String(), resp.ID)
	}
}

func TestService_ListRecordings_Unauthorized(t *testing.T) {
	svc, _, _, _ := setupRecordingTestService(t)

	_, err := svc.ListRecordings(context.Background(), dtos.RecordingFilterQuery{})
	if !errors.Is(err, constants.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestService_ListRecordings_Success(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	recID := uuid.New()
	now := time.Now()

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings" WHERE user_id = \$1 AND \(LOWER\(title\) LIKE \$2 OR LOWER\(original_filename\) LIKE \$3\) AND status = \$4 AND selected_template = \$5 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(userID, "%sprint%", "%sprint%", "COMPLETED", "MOM").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE user_id = \$1 AND \(LOWER\(title\) LIKE \$2 OR LOWER\(original_filename\) LIKE \$3\) AND status = \$4 AND selected_template = \$5 AND "recordings"\."deleted_at" IS NULL ORDER BY created_at DESC LIMIT \$6`).
		WithArgs(userID, "%sprint%", "%sprint%", "COMPLETED", "MOM", 10).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "title", "original_filename", "file_size_bytes", "duration_seconds",
			"source_type", "status", "selected_template", "output_language", "is_guest", "created_at", "updated_at",
		}).AddRow(
			recID, userID, "Sprint Planning", "sprint.mp3", 1024, 120.0,
			"UPLOAD", "COMPLETED", "MOM", "en", false, now, now,
		))

	resp, err := svc.ListRecordings(ctx, dtos.RecordingFilterQuery{
		Search:   "sprint",
		Status:   "COMPLETED",
		Template: "MOM",
		Page:     1,
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(resp.Items))
	}
	if resp.Items[0].Title != "Sprint Planning" {
		t.Errorf("expected Title Sprint Planning, got %s", resp.Items[0].Title)
	}
	if resp.Pagination.TotalItems != 1 {
		t.Errorf("expected TotalItems 1, got %d", resp.Pagination.TotalItems)
	}
	if resp.Pagination.TotalPages != 1 {
		t.Errorf("expected TotalPages 1, got %d", resp.Pagination.TotalPages)
	}
}

func TestService_ListRecordings_EmptyResults(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings" WHERE user_id = \$1 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE user_id = \$1 AND "recordings"\."deleted_at" IS NULL ORDER BY created_at DESC LIMIT \$2`).
		WithArgs(userID, 10).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	resp, err := svc.ListRecordings(ctx, dtos.RecordingFilterQuery{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Items) != 0 {
		t.Errorf("expected 0 items, got %d", len(resp.Items))
	}
	if resp.Pagination.TotalItems != 0 {
		t.Errorf("expected TotalItems 0, got %d", resp.Pagination.TotalItems)
	}
	if resp.Pagination.TotalPages != 0 {
		t.Errorf("expected TotalPages 0, got %d", resp.Pagination.TotalPages)
	}
}

func TestService_ListRecordings_DBError(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings" WHERE user_id = \$1 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(userID).
		WillReturnError(errors.New("db connection failure"))

	_, err := svc.ListRecordings(ctx, dtos.RecordingFilterQuery{})
	if err == nil || !strings.Contains(err.Error(), "db connection failure") {
		t.Fatalf("expected db connection error, got %v", err)
	}
}

func TestService_DeleteRecording_Unauthorized(t *testing.T) {
	svc, _, _, _ := setupRecordingTestService(t)
	recID := uuid.New()

	err := svc.DeleteRecording(context.Background(), recID)
	if !errors.Is(err, constants.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestService_DeleteRecording_Unauthorized_ContextNilUserID(t *testing.T) {
	svc, _, _, _ := setupRecordingTestService(t)
	recID := uuid.New()

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: uuid.Nil})
	err := svc.DeleteRecording(ctx, recID)
	if !errors.Is(err, constants.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestService_DeleteRecording_NotFound(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	recID := uuid.New()

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	err := svc.DeleteRecording(ctx, recID)
	if !errors.Is(err, constants.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestService_DeleteRecording_FindError(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	recID := uuid.New()

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(errors.New("db error"))

	err := svc.DeleteRecording(ctx, recID)
	if err == nil || !strings.Contains(err.Error(), "db error") {
		t.Fatalf("expected db error, got %v", err)
	}
}

func TestService_DeleteRecording_Forbidden_RecordingHasNilUser(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	recID := uuid.New()

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id"}).
			AddRow(recID, nil))

	err := svc.DeleteRecording(ctx, recID)
	if !errors.Is(err, constants.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for guest/nil user recording, got %v", err)
	}
}

func TestService_DeleteRecording_Forbidden_DifferentUser(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	otherUserID := uuid.New()
	recID := uuid.New()

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id"}).
			AddRow(recID, otherUserID))

	err := svc.DeleteRecording(ctx, recID)
	if !errors.Is(err, constants.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for other user recording, got %v", err)
	}
}

func TestService_DeleteRecording_DeleteError(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	recID := uuid.New()

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id"}).
			AddRow(recID, userID))

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings" SET "deleted_at"=\$1 WHERE id = \$2 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(sqlmock.AnyArg(), recID).
		WillReturnError(errors.New("delete failed"))
	mock.ExpectRollback()

	err := svc.DeleteRecording(ctx, recID)
	if err == nil || !strings.Contains(err.Error(), "delete failed") {
		t.Fatalf("expected delete error, got %v", err)
	}
}

func TestService_DeleteRecording_Success(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	recID := uuid.New()

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id"}).
			AddRow(recID, userID))

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings" SET "deleted_at"=\$1 WHERE id = \$2 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := svc.DeleteRecording(ctx, recID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestService_ClaimRecording_Unauthorized(t *testing.T) {
	svc, _, _, _ := setupRecordingTestService(t)
	recID := uuid.New()

	ctx := context.Background() // No auth
	err := svc.ClaimRecording(ctx, recID, dtos.ClaimRecordingRequest{OwnershipToken: "token-123"})
	if !errors.Is(err, constants.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestService_ClaimRecording_EmptyToken(t *testing.T) {
	svc, _, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	recID := uuid.New()

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})
	err := svc.ClaimRecording(ctx, recID, dtos.ClaimRecordingRequest{OwnershipToken: "   "})
	if !errors.Is(err, constants.ErrBadRequest) {
		t.Fatalf("expected ErrBadRequest for empty token, got %v", err)
	}
}

func TestService_ClaimRecording_NotFound(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	recID := uuid.New()

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	err := svc.ClaimRecording(ctx, recID, dtos.ClaimRecordingRequest{OwnershipToken: "token-123"})
	if !errors.Is(err, constants.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestService_ClaimRecording_FindError(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	recID := uuid.New()

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(errors.New("db error"))

	err := svc.ClaimRecording(ctx, recID, dtos.ClaimRecordingRequest{OwnershipToken: "token-123"})
	if err == nil || !strings.Contains(err.Error(), "failed to retrieve recording") {
		t.Fatalf("expected find error, got %v", err)
	}
}

func TestService_ClaimRecording_AlreadyClaimed_NotGuest(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	recID := uuid.New()

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "is_guest", "user_id", "ownership_token"}).
			AddRow(recID, false, nil, "token-123"))

	err := svc.ClaimRecording(ctx, recID, dtos.ClaimRecordingRequest{OwnershipToken: "token-123"})
	if !errors.Is(err, constants.ErrConflict) {
		t.Fatalf("expected ErrConflict when is_guest is false, got %v", err)
	}
}

func TestService_ClaimRecording_AlreadyClaimed_UserIDNotNull(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	existingOwner := uuid.New()
	recID := uuid.New()

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "is_guest", "user_id", "ownership_token"}).
			AddRow(recID, true, existingOwner, "token-123"))

	err := svc.ClaimRecording(ctx, recID, dtos.ClaimRecordingRequest{OwnershipToken: "token-123"})
	if !errors.Is(err, constants.ErrConflict) {
		t.Fatalf("expected ErrConflict when user_id is already assigned, got %v", err)
	}
}

func TestService_ClaimRecording_MismatchedToken(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	recID := uuid.New()

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "is_guest", "user_id", "ownership_token"}).
			AddRow(recID, true, nil, "correct-token"))

	err := svc.ClaimRecording(ctx, recID, dtos.ClaimRecordingRequest{OwnershipToken: "wrong-token"})
	if !errors.Is(err, constants.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for wrong ownership token, got %v", err)
	}
}

func TestService_ClaimRecording_ClaimRepoNotFound(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	recID := uuid.New()

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "is_guest", "user_id", "ownership_token"}).
			AddRow(recID, true, nil, "valid-token"))

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings" SET "is_guest"=\$1,"user_id"=\$2,"updated_at"=\$3 WHERE \(id = \$4 AND ownership_token = \$5 AND is_guest = TRUE\) AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(false, userID, sqlmock.AnyArg(), recID, "valid-token").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	err := svc.ClaimRecording(ctx, recID, dtos.ClaimRecordingRequest{OwnershipToken: "valid-token"})
	if !errors.Is(err, constants.ErrNotFound) {
		t.Fatalf("expected ErrNotFound when claim rows affected is 0, got %v", err)
	}
}

func TestService_ClaimRecording_ClaimRepoError(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	recID := uuid.New()

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "is_guest", "user_id", "ownership_token"}).
			AddRow(recID, true, nil, "valid-token"))

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings" SET "is_guest"=\$1,"user_id"=\$2,"updated_at"=\$3 WHERE \(id = \$4 AND ownership_token = \$5 AND is_guest = TRUE\) AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(false, userID, sqlmock.AnyArg(), recID, "valid-token").
		WillReturnError(errors.New("db update error"))
	mock.ExpectRollback()

	err := svc.ClaimRecording(ctx, recID, dtos.ClaimRecordingRequest{OwnershipToken: "valid-token"})
	if err == nil || !strings.Contains(err.Error(), "failed to claim recording") {
		t.Fatalf("expected claim failure, got %v", err)
	}
}

func TestService_ClaimRecording_Success(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	recID := uuid.New()

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "is_guest", "user_id", "ownership_token"}).
			AddRow(recID, true, nil, "valid-token"))

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings" SET "is_guest"=\$1,"user_id"=\$2,"updated_at"=\$3 WHERE \(id = \$4 AND ownership_token = \$5 AND is_guest = TRUE\) AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(false, userID, sqlmock.AnyArg(), recID, "valid-token").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := svc.ClaimRecording(ctx, recID, dtos.ClaimRecordingRequest{OwnershipToken: "valid-token"})
	if err != nil {
		t.Fatalf("unexpected claim error: %v", err)
	}
}

func TestService_ClaimBulkRecordings_Unauthorized(t *testing.T) {
	svc, _, _, _ := setupRecordingTestService(t)
	ctx := context.Background()

	_, err := svc.ClaimBulkRecordings(ctx, dtos.BulkClaimRequest{
		Tokens: []string{"tok-1"},
	})
	if !errors.Is(err, constants.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestService_ClaimBulkRecordings_EmptyTokens(t *testing.T) {
	svc, _, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	_, err := svc.ClaimBulkRecordings(ctx, dtos.BulkClaimRequest{
		Tokens: []string{"  ", ""},
	})
	if !errors.Is(err, constants.ErrBadRequest) {
		t.Fatalf("expected ErrBadRequest, got %v", err)
	}
}

func TestService_ClaimBulkRecordings_ZeroMatch(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE \(ownership_token IN \(\$1,\$2\) AND is_guest = TRUE\) AND "recordings"\."deleted_at" IS NULL`).
		WithArgs("tok-1", "tok-2").
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "is_guest"}))
	mock.ExpectCommit()

	resp, err := svc.ClaimBulkRecordings(ctx, dtos.BulkClaimRequest{
		Tokens: []string{"tok-1", "tok-2"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ClaimedCount != 0 {
		t.Errorf("expected ClaimedCount 0, got %d", resp.ClaimedCount)
	}
	if len(resp.RecordingIDs) != 0 {
		t.Errorf("expected 0 RecordingIDs, got %d", len(resp.RecordingIDs))
	}
}

func TestService_ClaimBulkRecordings_PartialMatch(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	recID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE \(ownership_token IN \(\$1,\$2\) AND is_guest = TRUE\) AND "recordings"\."deleted_at" IS NULL`).
		WithArgs("tok-1", "tok-2").
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "is_guest"}).
			AddRow(recID, "tok-1", true))

	mock.ExpectExec(`UPDATE "recordings" SET "is_guest"=\$1,"user_id"=\$2,"updated_at"=\$3 WHERE id IN \(\$4\) AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(false, userID, sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	resp, err := svc.ClaimBulkRecordings(ctx, dtos.BulkClaimRequest{
		Tokens: []string{"tok-1", "tok-2"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ClaimedCount != 1 {
		t.Errorf("expected ClaimedCount 1, got %d", resp.ClaimedCount)
	}
	if len(resp.RecordingIDs) != 1 || resp.RecordingIDs[0] != recID.String() {
		t.Errorf("expected RecordingIDs [%s], got %v", recID.String(), resp.RecordingIDs)
	}
}

func TestService_ClaimBulkRecordings_RepoError(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE \(ownership_token IN \(\$1\) AND is_guest = TRUE\) AND "recordings"\."deleted_at" IS NULL`).
		WithArgs("tok-1").
		WillReturnError(errors.New("db query error"))
	mock.ExpectRollback()

	_, err := svc.ClaimBulkRecordings(ctx, dtos.BulkClaimRequest{
		Tokens: []string{"tok-1"},
	})
	if err == nil || !strings.Contains(err.Error(), "failed to bulk claim recordings") {
		t.Fatalf("expected error, got %v", err)
	}
}

func TestService_ClaimBulkRecordings_Success(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	recID1 := uuid.New()
	recID2 := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

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

	resp, err := svc.ClaimBulkRecordings(ctx, dtos.BulkClaimRequest{
		Tokens: []string{"tok-1", "tok-2"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ClaimedCount != 2 {
		t.Errorf("expected ClaimedCount 2, got %d", resp.ClaimedCount)
	}
	if len(resp.RecordingIDs) != 2 {
		t.Errorf("expected 2 recording IDs, got %d", len(resp.RecordingIDs))
	}
}

func TestService_ToggleRecordingShare_Unauthorized(t *testing.T) {
	svc, _, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	enabled := true

	ctx := context.Background()
	_, err := svc.ToggleRecordingShare(ctx, recID, dtos.ShareToggleRequest{IsShareEnabled: &enabled})
	if !errors.Is(err, constants.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestService_ToggleRecordingShare_NilRequest(t *testing.T) {
	svc, _, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	userID := uuid.New()

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})
	_, err := svc.ToggleRecordingShare(ctx, recID, dtos.ShareToggleRequest{IsShareEnabled: nil})
	if !errors.Is(err, constants.ErrBadRequest) {
		t.Fatalf("expected ErrBadRequest, got %v", err)
	}
}

func TestService_ToggleRecordingShare_NotFound(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	userID := uuid.New()
	enabled := true

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	_, err := svc.ToggleRecordingShare(ctx, recID, dtos.ShareToggleRequest{IsShareEnabled: &enabled})
	if !errors.Is(err, constants.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestService_ToggleRecordingShare_Forbidden_NotOwner(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	userID := uuid.New()
	otherUserID := uuid.New()
	enabled := true

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id"}).
			AddRow(recID, otherUserID))

	_, err := svc.ToggleRecordingShare(ctx, recID, dtos.ShareToggleRequest{IsShareEnabled: &enabled})
	if !errors.Is(err, constants.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestService_ToggleRecordingShare_Forbidden_Guest(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	userID := uuid.New()
	enabled := true

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id"}).
			AddRow(recID, nil))

	_, err := svc.ToggleRecordingShare(ctx, recID, dtos.ShareToggleRequest{IsShareEnabled: &enabled})
	if !errors.Is(err, constants.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestService_ToggleRecordingShare_Enable_NewToken(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	userID := uuid.New()
	enabled := true

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "share_token", "is_share_enabled"}).
			AddRow(recID, userID, nil, false))

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings" SET "is_share_enabled"=\$1,"share_token"=\$2,"updated_at"=\$3 WHERE id = \$4 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(true, sqlmock.AnyArg(), sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	resp, err := svc.ToggleRecordingShare(ctx, recID, dtos.ShareToggleRequest{IsShareEnabled: &enabled})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.IsShareEnabled {
		t.Error("expected IsShareEnabled true")
	}
	if resp.ShareToken == nil || *resp.ShareToken == "" {
		t.Error("expected non-empty ShareToken")
	}
	if resp.ShareURL == nil || !strings.Contains(*resp.ShareURL, *resp.ShareToken) {
		t.Errorf("expected ShareURL containing token, got %v", resp.ShareURL)
	}
}

func TestService_ToggleRecordingShare_Enable_ExistingToken(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	userID := uuid.New()
	existingToken := "existing-token-abc"
	enabled := true

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "share_token", "is_share_enabled"}).
			AddRow(recID, userID, &existingToken, false))

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings" SET "is_share_enabled"=\$1,"share_token"=\$2,"updated_at"=\$3 WHERE id = \$4 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(true, existingToken, sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	resp, err := svc.ToggleRecordingShare(ctx, recID, dtos.ShareToggleRequest{IsShareEnabled: &enabled})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.IsShareEnabled {
		t.Error("expected IsShareEnabled true")
	}
	if resp.ShareToken == nil || *resp.ShareToken != existingToken {
		t.Errorf("expected ShareToken %s, got %v", existingToken, resp.ShareToken)
	}
}

func TestService_ToggleRecordingShare_Disable(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	userID := uuid.New()
	existingToken := "existing-token-abc"
	disabled := false

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "share_token", "is_share_enabled"}).
			AddRow(recID, userID, &existingToken, true))

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings" SET "is_share_enabled"=\$1,"updated_at"=\$2 WHERE id = \$3 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(false, sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	resp, err := svc.ToggleRecordingShare(ctx, recID, dtos.ShareToggleRequest{IsShareEnabled: &disabled})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.IsShareEnabled {
		t.Error("expected IsShareEnabled false")
	}
	if resp.ShareToken != nil {
		t.Errorf("expected nil ShareToken when disabled, got %v", resp.ShareToken)
	}
	if resp.ShareURL != nil {
		t.Errorf("expected nil ShareURL when disabled, got %v", resp.ShareURL)
	}
}

func TestService_ToggleRecordingShare_DBError(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	userID := uuid.New()
	enabled := true

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "share_token", "is_share_enabled"}).
			AddRow(recID, userID, nil, false))

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings" SET "is_share_enabled"=\$1,"share_token"=\$2,"updated_at"=\$3 WHERE id = \$4 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(true, sqlmock.AnyArg(), sqlmock.AnyArg(), recID).
		WillReturnError(errors.New("db update error"))
	mock.ExpectRollback()

	_, err := svc.ToggleRecordingShare(ctx, recID, dtos.ShareToggleRequest{IsShareEnabled: &enabled})
	if err == nil || !strings.Contains(err.Error(), "failed to update share settings") {
		t.Fatalf("expected update error, got %v", err)
	}
}

func TestService_GetSharedRecording_EmptyToken(t *testing.T) {
	svc, _, _, _ := setupRecordingTestService(t)
	ctx := context.Background()

	_, err := svc.GetSharedRecording(ctx, "   ")
	if !errors.Is(err, constants.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for empty token, got %v", err)
	}
}

func TestService_GetSharedRecording_NotFound(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE \(share_token = \$1 AND is_share_enabled = TRUE\) AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs("invalid-token", 1).
		WillReturnError(gorm.ErrRecordNotFound)

	_, err := svc.GetSharedRecording(ctx, "invalid-token")
	if !errors.Is(err, constants.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestService_GetSharedRecording_DBError(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE \(share_token = \$1 AND is_share_enabled = TRUE\) AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs("token-123", 1).
		WillReturnError(errors.New("db query error"))

	_, err := svc.GetSharedRecording(ctx, "token-123")
	if err == nil || !strings.Contains(err.Error(), "failed to retrieve shared recording") {
		t.Fatalf("expected error, got %v", err)
	}
}

func TestService_GetSharedRecording_Success(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	ctx := context.Background()
	recID := uuid.New()
	audioKey := "recordings/audio.mp3"

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE \(share_token = \$1 AND is_share_enabled = TRUE\) AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs("valid-token", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "title", "duration_seconds", "audio_url", "is_share_enabled", "share_token"}).
			AddRow(recID, "Public Meeting", 180.0, audioKey, true, "valid-token"))

	// ListTranscriptSegmentsByRecordingID
	mock.ExpectQuery(`SELECT \* FROM "transcript_segments" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "speaker_label", "text"}).
			AddRow(uuid.New(), recID, "spk_0", "Hello public world"))

	// FindActiveSummaryByRecordingID
	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND is_active = TRUE.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "markdown_content", "is_active"}).
			AddRow(uuid.New(), recID, "Public summary markdown", true))

	// ListChaptersByRecordingID
	mock.ExpectQuery(`SELECT \* FROM "chapters" WHERE recording_id = \$1.*LIMIT \$2`).
		WithArgs(recID, 100).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "title"}).
			AddRow(uuid.New(), recID, "Introduction"))

	// ListHighlightsByRecordingID
	mock.ExpectQuery(`SELECT \* FROM "highlights" WHERE recording_id = \$1 ORDER BY start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "source"}).
			AddRow(uuid.New(), recID, "KEY_POINT"))

	resp, err := svc.GetSharedRecording(ctx, "valid-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ID != recID.String() {
		t.Errorf("expected ID %s, got %s", recID.String(), resp.ID)
	}
	if resp.Title != "Public Meeting" {
		t.Errorf("expected Title 'Public Meeting', got %s", resp.Title)
	}
	if len(resp.Segments) != 1 {
		t.Errorf("expected 1 segment, got %d", len(resp.Segments))
	}
	if resp.ActiveSummary == nil || resp.ActiveSummary.MarkdownContent != "Public summary markdown" {
		t.Errorf("expected active summary markdown content, got %v", resp.ActiveSummary)
	}
	if len(resp.Chapters) != 1 {
		t.Errorf("expected 1 chapter, got %d", len(resp.Chapters))
	}
	if len(resp.Highlights) != 1 {
		t.Errorf("expected 1 highlight, got %d", len(resp.Highlights))
	}
}

func TestService_GetRecordingProgress_NotFound(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	_, _, _, err := svc.GetRecordingProgress(ctx, recID, "")
	if !errors.Is(err, constants.ErrRecordingNotFound) {
		t.Fatalf("expected ErrRecordingNotFound, got %v", err)
	}
}

func TestService_GetRecordingProgress_Forbidden_Unauthorized(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	userID := uuid.New()
	ctx := context.Background() // Not authenticated, no token

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "ownership_token", "is_share_enabled"}).
			AddRow(recID, &userID, "token-1", false))

	_, _, _, err := svc.GetRecordingProgress(ctx, recID, "")
	if !errors.Is(err, constants.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestService_GetRecordingProgress_Forbidden_NotOwner(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	ownerID := uuid.New()
	callerID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: callerID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "ownership_token", "is_share_enabled"}).
			AddRow(recID, &ownerID, "token-1", false))

	_, _, _, err := svc.GetRecordingProgress(ctx, recID, "")
	if !errors.Is(err, constants.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestService_GetRecordingProgress_Success_AuthUser(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	userID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "status", "ownership_token"}).
			AddRow(recID, &userID, "TRANSCRIBING", "token-1"))

	initial, subCh, unsub, err := svc.GetRecordingProgress(ctx, recID, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer unsub()

	if initial.RecordingID != recID.String() {
		t.Errorf("expected recording ID %s, got %s", recID.String(), initial.RecordingID)
	}
	if initial.Status != "TRANSCRIBING" || initial.Progress != 55 {
		t.Errorf("expected status TRANSCRIBING and progress 55, got %s / %d", initial.Status, initial.Progress)
	}
	if subCh == nil {
		t.Error("expected non-nil subscription channel")
	}
}

func TestService_GetRecordingProgress_Success_GuestToken(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "is_guest", "status", "ownership_token"}).
			AddRow(recID, true, "EXTRACTING", "guest-token-123"))

	initial, subCh, unsub, err := svc.GetRecordingProgress(ctx, recID, "guest-token-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer unsub()

	if initial.Status != "EXTRACTING" || initial.Progress != 30 {
		t.Errorf("expected EXTRACTING / 30, got %s / %d", initial.Status, initial.Progress)
	}
	if subCh == nil {
		t.Error("expected non-nil sub channel")
	}
}

func TestService_GetRecordingProgress_Success_ShareToken(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	shareToken := "share-token-xyz"
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "is_share_enabled", "share_token", "status"}).
			AddRow(recID, true, &shareToken, "COMPLETED"))

	initial, subCh, unsub, err := svc.GetRecordingProgress(ctx, recID, "share-token-xyz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer unsub()

	if initial.Status != "COMPLETED" || initial.Progress != 100 {
		t.Errorf("expected COMPLETED / 100, got %s / %d", initial.Status, initial.Progress)
	}
	if subCh == nil {
		t.Error("expected non-nil sub channel")
	}
}

func TestService_PollRecordingProgress_Success(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).
			AddRow(recID, "SUMMARIZING"))

	event, err := svc.PollRecordingProgress(ctx, recID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.Status != "SUMMARIZING" || event.Progress != 75 {
		t.Errorf("expected SUMMARIZING / 75, got %s / %d", event.Status, event.Progress)
	}
}

func TestService_PollRecordingProgress_NotFound(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	_, err := svc.PollRecordingProgress(ctx, recID)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected ErrRecordNotFound, got %v", err)
	}
}

func TestService_RetryRecordingPipeline_NotFound(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	_, err := svc.RetryRecordingPipeline(ctx, recID, "")
	if !errors.Is(err, constants.ErrRecordingNotFound) {
		t.Fatalf("expected ErrRecordingNotFound, got %v", err)
	}
}

func TestService_RetryRecordingPipeline_Forbidden_Unauthorized(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "is_guest", "status", "ownership_token"}).
			AddRow(recID, true, models.RecordingStatusFailed, "guest-tok-123"))

	_, err := svc.RetryRecordingPipeline(ctx, recID, "")
	if !errors.Is(err, constants.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestService_RetryRecordingPipeline_Forbidden_NotOwner(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	callerID := uuid.New()
	ownerID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: callerID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "status"}).
			AddRow(recID, &ownerID, models.RecordingStatusFailed))

	_, err := svc.RetryRecordingPipeline(ctx, recID, "")
	if !errors.Is(err, constants.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestService_RetryRecordingPipeline_Conflict_Processing(t *testing.T) {
	activeStatuses := []string{
		models.RecordingStatusPending,
		models.RecordingStatusQueued,
		models.RecordingStatusValidating,
		models.RecordingStatusExtracting,
		models.RecordingStatusTranscribing,
		models.RecordingStatusSummarizing,
		models.RecordingStatusIndexing,
		"PROCESSING",
	}

	for _, status := range activeStatuses {
		t.Run(status, func(t *testing.T) {
			svc, mock, _, _ := setupRecordingTestService(t)
			recID := uuid.New()
			userID := uuid.New()
			ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

			mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
				WithArgs(recID, 1).
				WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "status"}).
					AddRow(recID, &userID, status))

			_, err := svc.RetryRecordingPipeline(ctx, recID, "")
			if !errors.Is(err, constants.ErrConflictProcessing) {
				t.Fatalf("expected ErrConflictProcessing for status %s, got %v", status, err)
			}
		})
	}
}

func TestService_RetryRecordingPipeline_Conflict_Completed(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	userID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "status"}).
			AddRow(recID, &userID, models.RecordingStatusCompleted))

	_, err := svc.RetryRecordingPipeline(ctx, recID, "")
	if !errors.Is(err, constants.ErrRecordingAlreadyCompleted) {
		t.Fatalf("expected ErrRecordingAlreadyCompleted, got %v", err)
	}
}

func TestService_RetryRecordingPipeline_Success_AudioMissing(t *testing.T) {
	svc, mock, _, publisher := setupRecordingTestService(t)
	recID := uuid.New()
	userID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	// Single lookup in RetryRecordingPipeline passed directly to ResumeRecordingPipeline
	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "audio_url", "status", "selected_template", "output_language"}).
			AddRow(recID, &userID, nil, models.RecordingStatusFailed, "GENERAL", "en"))

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

	publisher.publishedTopics = nil
	resp, err := svc.RetryRecordingPipeline(ctx, recID, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ID != recID.String() {
		t.Errorf("expected ID %s, got %s", recID.String(), resp.ID)
	}
	if resp.Stage != models.RecordingStatusExtracting {
		t.Errorf("expected stage EXTRACTING, got %s", resp.Stage)
	}
	if resp.Status != models.RecordingStatusQueued {
		t.Errorf("expected status QUEUED, got %s", resp.Status)
	}
	if len(publisher.publishedTopics) == 0 || publisher.publishedTopics[0] != constants.TopicRecordingUploaded {
		t.Errorf("expected TopicRecordingUploaded, got %v", publisher.publishedTopics)
	}
}

func TestService_RetryRecordingPipeline_Success_TranscribeMissing(t *testing.T) {
	svc, mock, _, publisher := setupRecordingTestService(t)
	recID := uuid.New()
	audioURL := "https://s3.amazonaws.com/bucket/audio.mp3"
	guestToken := "guest-token-abc"
	ctx := context.Background()

	// Single lookup with guest credentials
	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "is_guest", "ownership_token", "audio_url", "status", "selected_template", "output_language"}).
			AddRow(recID, true, guestToken, &audioURL, models.RecordingStatusFailed, "GENERAL", "en"))

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

	publisher.publishedTopics = nil
	resp, err := svc.RetryRecordingPipeline(ctx, recID, guestToken)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Stage != models.RecordingStatusTranscribing {
		t.Errorf("expected stage TRANSCRIBING, got %s", resp.Stage)
	}
	if resp.Status != models.RecordingStatusQueued {
		t.Errorf("expected status QUEUED, got %s", resp.Status)
	}
	if len(publisher.publishedTopics) == 0 || publisher.publishedTopics[0] != constants.TopicRecordingTranscribe {
		t.Errorf("expected TopicRecordingTranscribe, got %v", publisher.publishedTopics)
	}
}

func TestService_RetryRecordingPipeline_Success_ShareToken(t *testing.T) {
	svc, mock, _, publisher := setupRecordingTestService(t)
	recID := uuid.New()
	audioURL := "https://s3.amazonaws.com/bucket/audio.mp3"
	shareToken := "share-tok-999"
	ctx := context.Background()

	// Single lookup with share token credentials
	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "is_share_enabled", "share_token", "audio_url", "status", "selected_template", "output_language"}).
			AddRow(recID, true, &shareToken, &audioURL, models.RecordingStatusFailed, "GENERAL", "en"))

	mock.ExpectQuery(`SELECT \* FROM "transcript_segments" WHERE recording_id = \$1.*`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "text"}).
			AddRow(uuid.New(), recID, "Segment 1 text"))

	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND is_active = TRUE.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	mock.ExpectQuery(`SELECT .* FROM "transcript_chunks" WHERE recording_id = \$1.*`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings" SET .* WHERE id = .*`).
		WithArgs(models.RecordingStatusTranscribing, sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	publisher.publishedTopics = nil
	resp, err := svc.RetryRecordingPipeline(ctx, recID, shareToken)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Stage != models.RecordingStatusTranscribing {
		t.Errorf("expected stage TRANSCRIBING, got %s", resp.Stage)
	}
	if resp.Status != models.RecordingStatusTranscribing {
		t.Errorf("expected status TRANSCRIBING, got %s", resp.Status)
	}
	foundSummarize := false
	foundIndex := false
	for _, topic := range publisher.publishedTopics {
		if topic == constants.TopicRecordingSummarize {
			foundSummarize = true
		}
		if topic == constants.TopicRecordingIndex {
			foundIndex = true
		}
	}
	if !foundSummarize || !foundIndex {
		t.Errorf("expected fanout to summarize and index, got %v", publisher.publishedTopics)
	}
}

func TestService_UpdateTranscriptSpeakers_NotFound(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	_, err := svc.UpdateTranscriptSpeakers(ctx, recID, "token", dtos.UpdateSpeakersRequest{
		Speakers: map[string]string{"SPEAKER_00": "Bayu"},
	})
	if !errors.Is(err, constants.ErrRecordingNotFound) {
		t.Fatalf("expected ErrRecordingNotFound, got %v", err)
	}
}

func TestService_UpdateTranscriptSpeakers_Forbidden_Unauthorized(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	ownerID := uuid.New()
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "ownership_token"}).
			AddRow(recID, &ownerID, "secret-token"))

	_, err := svc.UpdateTranscriptSpeakers(ctx, recID, "wrong-token", dtos.UpdateSpeakersRequest{
		Speakers: map[string]string{"SPEAKER_00": "Bayu"},
	})
	if !errors.Is(err, constants.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestService_UpdateTranscriptSpeakers_Forbidden_WrongUser(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	ownerID := uuid.New()
	otherUserID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: otherUserID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "ownership_token"}).
			AddRow(recID, &ownerID, "secret-token"))

	_, err := svc.UpdateTranscriptSpeakers(ctx, recID, "", dtos.UpdateSpeakersRequest{
		Speakers: map[string]string{"SPEAKER_00": "Bayu"},
	})
	if !errors.Is(err, constants.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestService_UpdateTranscriptSpeakers_Success_Guest(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	guestToken := "guest-ownership-token"
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token"}).
			AddRow(recID, guestToken))

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "transcript_segments" SET "speaker_name"=\$1,"updated_at"=\$2 WHERE recording_id = \$3 AND speaker_label = \$4`).
		WithArgs("Bayu", sqlmock.AnyArg(), recID, "SPEAKER_00").
		WillReturnResult(sqlmock.NewResult(0, 4))
	mock.ExpectCommit()

	resp, err := svc.UpdateTranscriptSpeakers(ctx, recID, guestToken, dtos.UpdateSpeakersRequest{
		Speakers: map[string]string{"SPEAKER_00": "Bayu"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.UpdatedCount != 4 {
		t.Errorf("expected UpdatedCount 4, got %d", resp.UpdatedCount)
	}
	if resp.Speakers["SPEAKER_00"] != "Bayu" {
		t.Errorf("expected speaker name 'Bayu', got %s", resp.Speakers["SPEAKER_00"])
	}
}

func TestService_UpdateTranscriptSpeakers_Success_AuthenticatedOwner(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	userID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id"}).
			AddRow(recID, &userID))

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "transcript_segments" SET "speaker_name"=\$1,"updated_at"=\$2 WHERE recording_id = \$3 AND speaker_label = \$4`).
		WithArgs("Alice", sqlmock.AnyArg(), recID, "SPEAKER_01").
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()

	resp, err := svc.UpdateTranscriptSpeakers(ctx, recID, "", dtos.UpdateSpeakersRequest{
		Speakers: map[string]string{"SPEAKER_01": "Alice"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.UpdatedCount != 2 {
		t.Errorf("expected UpdatedCount 2, got %d", resp.UpdatedCount)
	}
	if resp.Speakers["SPEAKER_01"] != "Alice" {
		t.Errorf("expected speaker name 'Alice', got %s", resp.Speakers["SPEAKER_01"])
	}
}




