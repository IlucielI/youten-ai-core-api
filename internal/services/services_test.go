package services

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"code-base-golang/internal/config"
	"code-base-golang/internal/constants"
)

// mockStorage implements FileStorage for unit testing without S3 or MinIO.
type mockStorage struct {
	pingErr error
}

func (m *mockStorage) Ping(ctx context.Context) error {
	return m.pingErr
}

func (m *mockStorage) Upload(ctx context.Context, bucketName, objectName string, reader io.Reader, size int64, contentType string) error {
	return nil
}

func (m *mockStorage) Download(ctx context.Context, bucketName, objectName string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("mock content")), nil
}

func (m *mockStorage) Delete(ctx context.Context, bucketName, objectName string) error {
	return nil
}

func (m *mockStorage) PresignGetObject(ctx context.Context, bucketName, objectName string, expiry time.Duration) (string, error) {
	return "https://mock.storage/get", nil
}

func (m *mockStorage) PresignPutObject(ctx context.Context, bucketName, objectName string, expiry time.Duration) (string, error) {
	return "https://mock.storage/put", nil
}

func TestService_FileStorage_Mock(t *testing.T) {
	mock := &mockStorage{}
	svc := New(config.Config{}, nil, mock)

	if svc.Storage() == nil {
		t.Fatal("expected storage to be non-nil")
	}

	// Test SetStorage override
	mock2 := &mockStorage{pingErr: errors.New("storage unreachable")}
	svc.SetStorage(mock2)

	health := svc.CheckHealth(context.Background())
	if health.S3 != constants.IntegrationStatusDisconnected {
		t.Errorf("expected storage disconnected on error, got %s", health.S3)
	}

	// Test healthy storage
	svc.SetStorage(&mockStorage{pingErr: nil})
	health = svc.CheckHealth(context.Background())
	if health.S3 != constants.IntegrationStatusConnected {
		t.Errorf("expected storage connected, got %s", health.S3)
	}
}

// mockPublisher implements EventPublisher for unit testing without RabbitMQ or NATS.
type mockPublisher struct {
	pingErr    error
	publishErr error
	published  []string
}

func (m *mockPublisher) Ping(ctx context.Context) error {
	return m.pingErr
}

func (m *mockPublisher) Publish(ctx context.Context, topic string, payload any) error {
	if m.publishErr != nil {
		return m.publishErr
	}
	m.published = append(m.published, topic)
	return nil
}

func (m *mockPublisher) MustPublish(ctx context.Context, topic string, payload any) {
	if err := m.Publish(ctx, topic, payload); err != nil {
		panic(err)
	}
}

func TestService_EventPublisher_Mock(t *testing.T) {
	pub := &mockPublisher{}
	svc := New(config.Config{}, nil, nil, pub)

	if svc.Publisher() == nil {
		t.Fatal("expected publisher to be non-nil")
	}

	// Test publishing
	err := svc.Publisher().Publish(context.Background(), "user.registered", map[string]string{"id": "123"})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(pub.published) != 1 || pub.published[0] != "user.registered" {
		t.Fatalf("unexpected published topics: %v", pub.published)
	}

	// Test health check with broker error
	svc.SetPublisher(&mockPublisher{pingErr: errors.New("broker down")})
	health := svc.CheckHealth(context.Background())
	if health.RabbitMQ != constants.IntegrationStatusDisconnected {
		t.Errorf("expected rabbitmq disconnected on ping error, got %s", health.RabbitMQ)
	}
}

// mockMailer implements EmailSender for unit testing without SMTP server.
type mockMailer struct {
	pingErr error
	sendErr error
	sent    []EmailMessage
}

func (m *mockMailer) Ping(ctx context.Context) error {
	return m.pingErr
}

func (m *mockMailer) Send(ctx context.Context, msg EmailMessage) error {
	if m.sendErr != nil {
		return m.sendErr
	}
	m.sent = append(m.sent, msg)
	return nil
}

func TestService_EmailSender_Mock(t *testing.T) {
	mailer := &mockMailer{}
	svc := New(config.Config{}, nil, nil).WithMailer(mailer)

	if svc.Mailer() == nil {
		t.Fatal("expected mailer to be non-nil")
	}

	// Test sending
	err := svc.Mailer().Send(context.Background(), EmailMessage{
		To:      []string{"test@example.com"},
		Subject: "Test Subject",
	})
	if err != nil {
		t.Fatalf("expected nil error sending email, got %v", err)
	}
	if len(mailer.sent) != 1 || mailer.sent[0].Subject != "Test Subject" {
		t.Fatalf("unexpected sent email: %v", mailer.sent)
	}

	// Test health check
	health := svc.CheckHealth(context.Background())
	if health.SMTP != constants.IntegrationStatusConnected {
		t.Errorf("expected smtp connected, got %s", health.SMTP)
	}

	// Test health check with SMTP error
	svc.SetMailer(&mockMailer{pingErr: errors.New("smtp down")})
	health = svc.CheckHealth(context.Background())
	if health.SMTP != constants.IntegrationStatusDisconnected {
		t.Errorf("expected smtp disconnected on error, got %s", health.SMTP)
	}
}

type dummySTT struct{}

func (d *dummySTT) Transcribe(ctx context.Context, reader io.Reader, filename string, opts STTOptions) (*TranscriptionResult, error) {
	return nil, nil
}

type dummyLLM struct{}

func (d *dummyLLM) GenerateStructured(ctx context.Context, systemPrompt, userPrompt string, schema map[string]interface{}) (*StructuredResponse, error) {
	return nil, nil
}

func (d *dummyLLM) GenerateChatResponse(ctx context.Context, systemPrompt string, messages []ChatMessageInput, opts ChatOptions) (*ChatResponse, error) {
	return nil, nil
}

func (d *dummyLLM) StreamChatResponse(ctx context.Context, systemPrompt string, messages []ChatMessageInput, opts ChatOptions) (<-chan StreamChunk, error) {
	return nil, nil
}

type dummyEmbedding struct{}

func (d *dummyEmbedding) CreateEmbeddings(ctx context.Context, texts []string) ([][]float32, error) {
	return nil, nil
}

type dummyAudioExtractor struct{}

func (d *dummyAudioExtractor) ExtractMonoAudio(ctx context.Context, input io.Reader, filename string) (*AudioExtractionResult, error) {
	return nil, nil
}

func TestService_AIProviders(t *testing.T) {
	var nilSvc *Service
	if nilSvc.STT() != nil || nilSvc.LLM() != nil || nilSvc.Embedding() != nil || nilSvc.AudioExtractor() != nil {
		t.Error("expected nil providers for nil service")
	}

	sttInst := &dummySTT{}
	llmInst := &dummyLLM{}
	embInst := &dummyEmbedding{}
	audioInst := &dummyAudioExtractor{}

	svc := New(config.Config{}, nil, nil).
		WithSTT(sttInst).
		WithLLM(llmInst).
		WithEmbedding(embInst).
		WithAudioExtractor(audioInst)

	if svc.STT() != sttInst {
		t.Errorf("expected stt %v, got %v", sttInst, svc.STT())
	}
	if svc.LLM() != llmInst {
		t.Errorf("expected llm %v, got %v", llmInst, svc.LLM())
	}
	if svc.Embedding() != embInst {
		t.Errorf("expected embedding %v, got %v", embInst, svc.Embedding())
	}
	if svc.AudioExtractor() != audioInst {
		t.Errorf("expected audio extractor %v, got %v", audioInst, svc.AudioExtractor())
	}

	// Test Set methods
	svc.SetSTT(nil)
	svc.SetLLM(nil)
	svc.SetEmbedding(nil)
	svc.SetAudioExtractor(nil)

	if svc.STT() != nil || svc.LLM() != nil || svc.Embedding() != nil || svc.AudioExtractor() != nil {
		t.Error("expected nil providers after Set(nil)")
	}
}

