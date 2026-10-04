package services_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	gormPostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"

	"code-base-golang/internal/adapters/audio"
	"code-base-golang/internal/adapters/embedding"
	"code-base-golang/internal/adapters/llm"
	"code-base-golang/internal/adapters/stt"
	"code-base-golang/internal/config"
	"code-base-golang/internal/constants"
	"code-base-golang/internal/models"
	"code-base-golang/internal/payload"
	"code-base-golang/internal/repositories"
	"code-base-golang/internal/services"
)

type mockPublisher struct {
	publishedTopics []string
}

func (m *mockPublisher) Ping(ctx context.Context) error { return nil }
func (m *mockPublisher) Publish(ctx context.Context, topic string, payload any) error {
	m.publishedTopics = append(m.publishedTopics, topic)
	return nil
}
func (m *mockPublisher) MustPublish(ctx context.Context, topic string, payload any) {
	_ = m.Publish(ctx, topic, payload)
}

type mockPipelineStorage struct{}

func (m *mockPipelineStorage) Ping(ctx context.Context) error { return nil }
func (m *mockPipelineStorage) Upload(ctx context.Context, bucketName, objectName string, reader io.Reader, size int64, contentType string) error {
	return nil
}
func (m *mockPipelineStorage) Download(ctx context.Context, bucketName, objectName string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("dummy audio bytes")), nil
}
func (m *mockPipelineStorage) Delete(ctx context.Context, bucketName, objectName string) error {
	return nil
}
func (m *mockPipelineStorage) PresignGetObject(ctx context.Context, bucketName, objectName string, expiry time.Duration) (string, error) {
	return "https://mock.storage/get", nil
}
func (m *mockPipelineStorage) PresignPutObject(ctx context.Context, bucketName, objectName string, expiry time.Duration) (string, error) {
	return "https://mock.storage/put", nil
}

func setupTestPipelineService(t *testing.T) (*services.Service, sqlmock.Sqlmock, *mockPublisher, func()) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}

	gormDB, err := gorm.Open(gormPostgres.New(gormPostgres.Config{
		Conn: sqlDB,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to init gorm: %v", err)
	}

	repo := repositories.New(gormDB)
	storage := &mockPipelineStorage{}
	pub := &mockPublisher{}

	cfg := config.Config{
		AppName:               "youten-test",
		S3BucketName:          "youten-test-bucket",
		RAGChunkTokens:        300,
		RAGChunkOverlapTokens: 50,
	}

	svc := services.New(cfg, repo, storage, pub).
		WithAudioExtractor(audio.NewMock()).
		WithSTT(stt.NewMock()).
		WithLLM(llm.NewMock()).
		WithEmbedding(embedding.NewMock())

	cleanup := func() {
		sqlDB.Close()
	}

	return svc, mock, pub, cleanup
}

func TestPipelineService_NilChecks(t *testing.T) {
	nilSvc := services.New(config.Config{}, nil, nil)
	p := payload.RecordingPipelinePayload{RecordingID: uuid.New()}

	if err := nilSvc.ProcessExtraction(context.Background(), p); !errors.Is(err, services.ErrNilRepositories) {
		t.Errorf("expected ErrNilRepositories, got %v", err)
	}
	if err := nilSvc.ProcessTranscription(context.Background(), p); !errors.Is(err, services.ErrNilRepositories) {
		t.Errorf("expected ErrNilRepositories, got %v", err)
	}
	if err := nilSvc.ProcessSummarization(context.Background(), p); !errors.Is(err, services.ErrNilRepositories) {
		t.Errorf("expected ErrNilRepositories, got %v", err)
	}
	if err := nilSvc.ProcessIndexing(context.Background(), p); !errors.Is(err, services.ErrNilRepositories) {
		t.Errorf("expected ErrNilRepositories, got %v", err)
	}
	if err := nilSvc.ProcessAnalytics(context.Background(), p); !errors.Is(err, services.ErrNilRepositories) {
		t.Errorf("expected ErrNilRepositories, got %v", err)
	}
	if err := nilSvc.ProcessChapterization(context.Background(), p); !errors.Is(err, services.ErrNilRepositories) {
		t.Errorf("expected ErrNilRepositories, got %v", err)
	}
	if _, err := nilSvc.CheckAndCompleteRecording(context.Background(), p.RecordingID); !errors.Is(err, services.ErrNilRepositories) {
		t.Errorf("expected ErrNilRepositories, got %v", err)
	}
	if _, err := nilSvc.RetryRecording(context.Background(), p.RecordingID); !errors.Is(err, services.ErrNilRepositories) {
		t.Errorf("expected ErrNilRepositories, got %v", err)
	}
	if err := nilSvc.FailRecording(context.Background(), p.RecordingID, "ERR", "msg"); !errors.Is(err, services.ErrNilRepositories) {
		t.Errorf("expected ErrNilRepositories, got %v", err)
	}
}

func TestPipelineService_ProcessExtraction_Success(t *testing.T) {
	svc, mock, pub, cleanup := setupTestPipelineService(t)
	defer cleanup()

	recID := uuid.New()
	audioPath := "recordings/" + recID.String() + "/original.mp4"

	// 1. FindRecordingByID
	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "title", "original_filename", "audio_url", "status"}).
			AddRow(recID, "Team Standup", "standup.mp4", audioPath, models.RecordingStatusQueued))

	// 2. UpdateRecordingStatus -> EXTRACTING
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings" SET .* WHERE id = .*`).
		WithArgs(models.RecordingStatusExtracting, sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	// 3. UpdateRecordingAudioURL
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings" SET .* WHERE id = .*`).
		WithArgs("recordings/"+recID.String()+"/audio.mp3", 120.0, sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	p := payload.RecordingPipelinePayload{
		RecordingID: recID,
		SourcePath:  audioPath,
	}

	err := svc.ProcessExtraction(context.Background(), p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(pub.publishedTopics) == 0 || pub.publishedTopics[len(pub.publishedTopics)-1] != constants.TopicRecordingTranscribe {
		t.Errorf("expected TopicRecordingTranscribe published, got %v", pub.publishedTopics)
	}
}

func TestPipelineService_ProcessTranscription_NoSpeech(t *testing.T) {
	svc, mock, _, cleanup := setupTestPipelineService(t)
	defer cleanup()

	recID := uuid.New()
	audioPath := "recordings/" + recID.String() + "/audio.mp3"

	// Mock STT to return empty transcript
	emptySTT := &stt.MockSTT{
		TranscribeFunc: func(ctx context.Context, reader io.Reader, filename string, opts services.STTOptions) (*services.TranscriptionResult, error) {
			return &services.TranscriptionResult{
				Text:     "",
				Segments: nil,
			}, nil
		},
	}
	svc.WithSTT(emptySTT)

	// 1. FindRecordingByID
	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "title", "audio_url", "status", "output_language"}).
			AddRow(recID, "Silent Video", audioPath, models.RecordingStatusExtracting, "id"))

	// 2. UpdateRecordingStatus -> TRANSCRIBING
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings" SET .* WHERE id = .*`).
		WithArgs(models.RecordingStatusTranscribing, sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	// 3. FailRecording -> FAILED with ERR_NO_SPEECH_DETECTED
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings" SET .* WHERE id = .*`).
		WithArgs(models.ErrCodeNoSpeechDetected, "No speech detected in audio recording", models.RecordingStatusFailed, sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	p := payload.RecordingPipelinePayload{
		RecordingID: recID,
		AudioPath:   audioPath,
	}

	err := svc.ProcessTranscription(context.Background(), p)
	if err == nil || !strings.Contains(err.Error(), "No speech detected") {
		t.Errorf("expected No speech detected error, got %v", err)
	}
}

func TestPipelineService_RetryRecording_SmartStateRecovery(t *testing.T) {
	svc, mock, pub, cleanup := setupTestPipelineService(t)
	defer cleanup()

	recID := uuid.New()

	// Scenario A: Recording is not FAILED -> ErrInvalidRetryState
	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).
			AddRow(recID, models.RecordingStatusCompleted))

	_, err := svc.RetryRecording(context.Background(), recID)
	if !errors.Is(err, services.ErrInvalidRetryState) {
		t.Errorf("expected ErrInvalidRetryState, got %v", err)
	}

	// Scenario B: Recording is FAILED and has no audioURL -> Dispatches TopicRecordingUploaded
	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "audio_url", "status", "selected_template", "output_language"}).
			AddRow(recID, nil, models.RecordingStatusFailed, "GENERAL", "id"))

	// Segments check
	mock.ExpectQuery(`SELECT \* FROM "transcript_segments" WHERE recording_id = \$1`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	// Summary check
	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND is_active = TRUE.*`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	// Chunks check
	mock.ExpectQuery(`SELECT .* FROM "transcript_chunks" WHERE recording_id = \$1.*`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	// Reset status to QUEUED
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings" SET .* WHERE id = .*`).
		WithArgs(models.RecordingStatusQueued, sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	pub.publishedTopics = nil
	p, err := svc.RetryRecording(context.Background(), recID)
	if err != nil {
		t.Fatalf("unexpected error on retry: %v", err)
	}
	if p.Stage != models.RecordingStatusExtracting {
		t.Errorf("expected stage EXTRACTING, got %s", p.Stage)
	}
	if len(pub.publishedTopics) == 0 || pub.publishedTopics[0] != constants.TopicRecordingUploaded {
		t.Errorf("expected TopicRecordingUploaded, got %v", pub.publishedTopics)
	}
}

func TestPipelineService_CheckAndCompleteRecording(t *testing.T) {
	svc, mock, pub, cleanup := setupTestPipelineService(t)
	defer cleanup()

	recID := uuid.New()
	userID := uuid.New()

	// 1. FindActiveSummaryByRecordingID
	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND is_active = TRUE.*`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "is_active"}).
			AddRow(uuid.New(), recID, true))

	// 2. ListTranscriptChunksByRecordingID
	mock.ExpectQuery(`SELECT .* FROM "transcript_chunks" WHERE recording_id = \$1.*`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "chunk_index"}).
			AddRow(uuid.New(), recID, 1))

	// 3. FindRecordingByID
	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "title", "status", "user_id"}).
			AddRow(recID, "All Hands", models.RecordingStatusIndexing, userID))

	// 4. UpdateRecordingStatus -> COMPLETED
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings" SET .* WHERE id = .*`).
		WithArgs(models.RecordingStatusCompleted, sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	// 5. CreateNotification
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "notifications".*`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).
			AddRow(uuid.New(), time.Now()))
	mock.ExpectCommit()

	completed, err := svc.CheckAndCompleteRecording(context.Background(), recID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !completed {
		t.Errorf("expected completed to be true")
	}

	if len(pub.publishedTopics) == 0 || pub.publishedTopics[len(pub.publishedTopics)-1] != constants.TopicRecordingCompleted {
		t.Errorf("expected TopicRecordingCompleted, got %v", pub.publishedTopics)
	}
}
