package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	gormPostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"

	"code-base-golang/internal/config"
	"code-base-golang/internal/dtos"
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

type dummyEmbeddingProvider struct {
	embeddings [][]float32
	err        error
}

func (d *dummyEmbeddingProvider) CreateEmbeddings(ctx context.Context, texts []string) ([][]float32, error) {
	if d.err != nil {
		return nil, d.err
	}
	if len(d.embeddings) > 0 {
		return d.embeddings, nil
	}
	res := make([][]float32, len(texts))
	for i := range texts {
		res[i] = make([]float32, 1024)
	}
	return res, nil
}

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
