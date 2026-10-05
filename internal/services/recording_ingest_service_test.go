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
	"gorm.io/gorm"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
	"code-base-golang/internal/pkg/ctxmeta"
)

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
