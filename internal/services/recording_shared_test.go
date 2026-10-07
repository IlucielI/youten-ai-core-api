package services_test

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"code-base-golang/internal/config"
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
func (m *testRecordingStorage) Delete(ctx context.Context, bucketName, objectName string) error {
	return nil
}
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

type mockMediaLinkExtractor struct {
	supportsFunc      func(rawURL string) bool
	extractIDFunc     func(rawURL string) (string, error)
	normalizeURLFunc  func(rawURL string) (string, error)
	fetchMetadataFunc func(ctx context.Context, rawURL string) (*services.MediaMetadata, error)
	extractAudioFunc  func(ctx context.Context, rawURL string) (*services.ExtractedAudio, error)
}

func (m *mockMediaLinkExtractor) Supports(rawURL string) bool {
	if m.supportsFunc != nil {
		return m.supportsFunc(rawURL)
	}
	return false
}

func (m *mockMediaLinkExtractor) ExtractID(rawURL string) (string, error) {
	if m.extractIDFunc != nil {
		return m.extractIDFunc(rawURL)
	}
	return "", nil
}

func (m *mockMediaLinkExtractor) NormalizeURL(rawURL string) (string, error) {
	if m.normalizeURLFunc != nil {
		return m.normalizeURLFunc(rawURL)
	}
	return rawURL, nil
}

func (m *mockMediaLinkExtractor) FetchMetadata(ctx context.Context, rawURL string) (*services.MediaMetadata, error) {
	if m.fetchMetadataFunc != nil {
		return m.fetchMetadataFunc(ctx, rawURL)
	}
	return &services.MediaMetadata{ID: "test-id", Title: "Test Title"}, nil
}

func (m *mockMediaLinkExtractor) ExtractAudio(ctx context.Context, rawURL string) (*services.ExtractedAudio, error) {
	if m.extractAudioFunc != nil {
		return m.extractAudioFunc(ctx, rawURL)
	}
	return nil, nil
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
