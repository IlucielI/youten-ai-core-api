package services_test

import (
	"bytes"
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
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
	"code-base-golang/internal/queue"
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
func (m *mockPipelineStorage) StatObject(ctx context.Context, bucketName, objectName string) (int64, error) {
	return 1024, nil
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
		WithArgs("recordings/"+recID.String()+"/audio.mp3", 120.0, 17, sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	p := queue.RecordingPipelinePayload{
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
		TranscribeFunc: func(ctx context.Context, reader io.Reader, filename string, opts dtos.STTOptions) (*dtos.TranscriptionResult, error) {
			return &dtos.TranscriptionResult{
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

	p := queue.RecordingPipelinePayload{
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
	if !errors.Is(err, constants.ErrInvalidRetryState) {
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

type mockPipelineLinkExtractor struct {
	supportsFunc      func(rawURL string) bool
	extractIDFunc     func(rawURL string) (string, error)
	normalizeURLFunc  func(rawURL string) (string, error)
	fetchMetadataFunc func(ctx context.Context, rawURL string) (*dtos.MediaMetadata, error)
	extractAudioFunc  func(ctx context.Context, rawURL string) (*dtos.ExtractedAudio, error)
}

func (m *mockPipelineLinkExtractor) Supports(rawURL string) bool {
	if m.supportsFunc != nil {
		return m.supportsFunc(rawURL)
	}
	return true
}

func (m *mockPipelineLinkExtractor) ExtractID(rawURL string) (string, error) {
	if m.extractIDFunc != nil {
		return m.extractIDFunc(rawURL)
	}
	return "dQw4w9WgXcQ", nil
}

func (m *mockPipelineLinkExtractor) NormalizeURL(rawURL string) (string, error) {
	if m.normalizeURLFunc != nil {
		return m.normalizeURLFunc(rawURL)
	}
	return rawURL, nil
}

func (m *mockPipelineLinkExtractor) FetchMetadata(ctx context.Context, rawURL string) (*dtos.MediaMetadata, error) {
	if m.fetchMetadataFunc != nil {
		return m.fetchMetadataFunc(ctx, rawURL)
	}
	return &dtos.MediaMetadata{ID: "dQw4w9WgXcQ", Title: "Extracted Video Title"}, nil
}

func (m *mockPipelineLinkExtractor) ExtractAudio(ctx context.Context, rawURL string) (*dtos.ExtractedAudio, error) {
	if m.extractAudioFunc != nil {
		return m.extractAudioFunc(ctx, rawURL)
	}
	return &dtos.ExtractedAudio{
		Title:           "Extracted Video Title",
		DurationSeconds: 180.0,
		SizeBytes:       512,
		Stream:          io.NopCloser(strings.NewReader("dummy audio stream")),
		ContentType:     "audio/mpeg",
	}, nil
}

func TestPipelineService_ProcessImport_Success(t *testing.T) {
	svc, mock, pub, cleanup := setupTestPipelineService(t)
	defer cleanup()

	mockExt := &mockPipelineLinkExtractor{}
	svc.WithMediaLinkExtractor(mockExt)

	recID := uuid.New()
	targetURL := "https://www.youtube.com/watch?v=dQw4w9WgXcQ"

	// 1. FindRecordingByID
	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "title", "original_filename", "audio_url", "status"}).
			AddRow(recID, "YouTube (dQw4w9WgXcQ)", "youtube_dQw4w9WgXcQ.m4a", "", models.RecordingStatusQueued))

	// 2. UpdateRecordingStatus -> EXTRACTING
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings" SET .* WHERE id = .*`).
		WithArgs(models.RecordingStatusExtracting, sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	// 3. UpdateRecordingAudioURL
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings" SET .* WHERE id = .*`).
		WithArgs("recordings/"+recID.String()+"/audio.mp3", 180.0, 512, sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	// 4. UpdateRecording (title updated)
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings" SET .* WHERE .*`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	p := queue.RecordingPipelinePayload{
		RecordingID: recID,
		URL:         targetURL,
		Template:    "GENERAL",
		Language:    "en",
	}

	err := svc.ProcessImport(context.Background(), p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(pub.publishedTopics) == 0 || pub.publishedTopics[len(pub.publishedTopics)-1] != constants.TopicRecordingTranscribe {
		t.Errorf("expected TopicRecordingTranscribe published, got %v", pub.publishedTopics)
	}
}

func TestPipelineService_ProcessImport_ExtractorFailure(t *testing.T) {
	svc, mock, pub, cleanup := setupTestPipelineService(t)
	defer cleanup()

	mockExt := &mockPipelineLinkExtractor{
		extractAudioFunc: func(ctx context.Context, rawURL string) (*dtos.ExtractedAudio, error) {
			return nil, errors.New("yt-dlp binary crash")
		},
	}
	svc.WithMediaLinkExtractor(mockExt)

	recID := uuid.New()
	targetURL := "https://www.youtube.com/watch?v=bad"

	// 1. FindRecordingByID
	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "title", "original_filename", "audio_url", "status"}).
			AddRow(recID, "YouTube Video", "youtube_bad.m4a", "", models.RecordingStatusQueued))

	// 2. UpdateRecordingStatus -> EXTRACTING
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings" SET .* WHERE id = .*`).
		WithArgs(models.RecordingStatusExtracting, sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	// 3. FailRecording -> FAILED
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings" SET .* WHERE id = .*`).
		WithArgs(models.ErrCodeImportFetchFailed, sqlmock.AnyArg(), models.RecordingStatusFailed, sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	p := queue.RecordingPipelinePayload{
		RecordingID: recID,
		URL:         targetURL,
	}

	err := svc.ProcessImport(context.Background(), p)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	foundFailed := false
	for _, topic := range pub.publishedTopics {
		if topic == constants.TopicRecordingFailed {
			foundFailed = true
		}
	}
	if !foundFailed {
		t.Errorf("expected TopicRecordingFailed published, got %v", pub.publishedTopics)
	}
}

func TestPipelineService_ProcessImport_GoogleDrive_Success(t *testing.T) {
	svc, mock, pub, cleanup := setupTestPipelineService(t)
	defer cleanup()

	mockExt := &mockPipelineLinkExtractor{
		extractAudioFunc: func(ctx context.Context, rawURL string) (*dtos.ExtractedAudio, error) {
			return &dtos.ExtractedAudio{
				Stream:          io.NopCloser(bytes.NewReader([]byte("gdrive-audio-data"))),
				SizeBytes:       17,
				DurationSeconds: 120.0,
				ContentType:     "audio/mpeg",
				Filename:        "quarterly_sync.mp3",
				Title:           "Quarterly Sync Meeting",
			}, nil
		},
	}
	svc.WithMediaLinkExtractor(mockExt)

	recID := uuid.New()
	targetURL := "https://drive.google.com/file/d/1BxiMVs0XRA5nFMdKvBdBZjgmUUqptlbs74OgvE2upms/view"

	// 1. FindRecordingByID
	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "title", "original_filename", "audio_url", "status"}).
			AddRow(recID, "Google Drive (1BxiMVs0XRA5nFMdKvBdBZjgmUUqptlbs74OgvE2upms)", "import_1BxiMVs0XRA5nFMdKvBdBZjgmUUqptlbs74OgvE2upms.m4a", "", models.RecordingStatusQueued))

	// 2. UpdateRecordingStatus -> EXTRACTING
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings" SET .* WHERE id = .*`).
		WithArgs(models.RecordingStatusExtracting, sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	// 3. UpdateRecordingAudioURL
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings" SET .* WHERE id = .*`).
		WithArgs("recordings/"+recID.String()+"/audio.mp3", 120.0, 17, sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	// 4. UpdateRecording (title updated to "Quarterly Sync Meeting")
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings" SET .* WHERE .*`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	p := queue.RecordingPipelinePayload{
		RecordingID: recID,
		URL:         targetURL,
		Template:    "GENERAL",
		Language:    "en",
	}

	err := svc.ProcessImport(context.Background(), p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(pub.publishedTopics) == 0 || pub.publishedTopics[len(pub.publishedTopics)-1] != constants.TopicRecordingTranscribe {
		t.Errorf("expected TopicRecordingTranscribe published, got %v", pub.publishedTopics)
	}
}

func TestPipelineService_ProcessImport_GoogleDrive_PermissionDenied(t *testing.T) {
	svc, mock, pub, cleanup := setupTestPipelineService(t)
	defer cleanup()

	mockExt := &mockPipelineLinkExtractor{
		extractAudioFunc: func(ctx context.Context, rawURL string) (*dtos.ExtractedAudio, error) {
			return nil, errors.New("access denied: file is private or restricted")
		},
	}
	svc.WithMediaLinkExtractor(mockExt)

	recID := uuid.New()
	targetURL := "https://drive.google.com/file/d/1BxiMVs0XRA5nFMdKvBdBZjgmUUqptlbs74OgvE2upms/view"

	// 1. FindRecordingByID
	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "title", "original_filename", "audio_url", "status"}).
			AddRow(recID, "Media Import", "import_gdrive.m4a", "", models.RecordingStatusQueued))

	// 2. UpdateRecordingStatus -> EXTRACTING
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings" SET .* WHERE id = .*`).
		WithArgs(models.RecordingStatusExtracting, sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	// 3. FailRecording -> FAILED with ErrCodeGdriveAccessDenied
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings" SET .* WHERE id = .*`).
		WithArgs(models.ErrCodeGDriveAccessDenied, sqlmock.AnyArg(), models.RecordingStatusFailed, sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	p := queue.RecordingPipelinePayload{
		RecordingID: recID,
		URL:         targetURL,
	}

	err := svc.ProcessImport(context.Background(), p)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	foundFailed := false
	for _, topic := range pub.publishedTopics {
		if topic == constants.TopicRecordingFailed {
			foundFailed = true
		}
	}
	if !foundFailed {
		t.Errorf("expected TopicRecordingFailed published, got %v", pub.publishedTopics)
	}
}

func TestPipelineService_ProcessImport_UnsupportedMediaType(t *testing.T) {
	svc, mock, pub, cleanup := setupTestPipelineService(t)
	defer cleanup()

	mockExt := &mockPipelineLinkExtractor{
		extractAudioFunc: func(ctx context.Context, rawURL string) (*dtos.ExtractedAudio, error) {
			return nil, errors.New("unsupported media format")
		},
	}
	svc.WithMediaLinkExtractor(mockExt)

	recID := uuid.New()
	targetURL := "https://drive.google.com/file/d/1BxiMVs0XRA5nFMdKvBdBZjgmUUqptlbs74OgvE2upms/view"

	// 1. FindRecordingByID
	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "title", "original_filename", "audio_url", "status"}).
			AddRow(recID, "Media Import", "import_gdrive.m4a", "", models.RecordingStatusQueued))

	// 2. UpdateRecordingStatus -> EXTRACTING
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings" SET .* WHERE id = .*`).
		WithArgs(models.RecordingStatusExtracting, sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	// 3. FailRecording -> FAILED with ErrCodeUnsupportedMediaType
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings" SET .* WHERE id = .*`).
		WithArgs(models.ErrCodeUnsupportedMediaType, sqlmock.AnyArg(), models.RecordingStatusFailed, sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	p := queue.RecordingPipelinePayload{
		RecordingID: recID,
		URL:         targetURL,
	}

	err := svc.ProcessImport(context.Background(), p)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	foundFailed := false
	for _, topic := range pub.publishedTopics {
		if topic == constants.TopicRecordingFailed {
			foundFailed = true
		}
	}
	if !foundFailed {
		t.Errorf("expected TopicRecordingFailed published, got %v", pub.publishedTopics)
	}
}

func TestPipelineService_HasNativeSpeakerLabels(t *testing.T) {
	// Case 1: Empty segments -> false
	if services.HasNativeSpeakerLabels(nil) {
		t.Error("expected false for nil segments")
	}

	// Case 2: Segments with default "Speaker 0" -> false
	defaultSegs := []dtos.SegmentResult{
		{Text: "Halo", SpeakerLabel: "Speaker 0"},
		{Text: "Dunia", SpeakerLabel: "Speaker 0"},
	}
	if services.HasNativeSpeakerLabels(defaultSegs) {
		t.Error("expected false for default Speaker 0 segments")
	}

	// Case 3: Segments with distinct native acoustic labels -> true
	nativeSegs := []dtos.SegmentResult{
		{Text: "Halo", SpeakerLabel: "SPEAKER_00"},
		{Text: "Dunia", SpeakerLabel: "SPEAKER_01"},
	}
	if !services.HasNativeSpeakerLabels(nativeSegs) {
		t.Error("expected true for native acoustic segments")
	}

	// Case 4: Segments with AcousticSpeaker field populated -> true
	acousticSegs := []dtos.SegmentResult{
		{Text: "Halo", AcousticSpeaker: "Speaker A"},
		{Text: "Dunia", AcousticSpeaker: "Speaker B"},
	}
	if !services.HasNativeSpeakerLabels(acousticSegs) {
		t.Error("expected true for AcousticSpeaker segments")
	}
}

func TestPipelineService_TranscribeAudioAdaptive(t *testing.T) {
	svc, _, _, cleanup := setupTestPipelineService(t)
	defer cleanup()

	recID := uuid.New()
	opts := dtos.STTOptions{Language: "id"}

	// 1. Short duration (< 600s) -> delegates directly to Transcribe
	mockSTT := stt.NewMock()
	mockSTT.TranscribeFunc = func(ctx context.Context, reader io.Reader, filename string, opts dtos.STTOptions) (*dtos.TranscriptionResult, error) {
		return &dtos.TranscriptionResult{Text: "single pass short"}, nil
	}
	svc.WithSTT(mockSTT)

	res, err := svc.TranscribeAudioAdaptive(context.Background(), strings.NewReader("audio"), 300.0, opts, recID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Text != "single pass short" {
		t.Errorf("expected single pass short, got %s", res.Text)
	}

	// 2. Long duration (>= 600s) with AudioSlicer returning multiple chunks
	mockExtractor := audio.NewMock()
	mockExtractor.SliceFn = func(ctx context.Context, input io.Reader, filename string, totalDurationSec float64) ([]dtos.AudioChunk, error) {
		return []dtos.AudioChunk{
			{Index: 0, StartOffsetSec: 0, DurationSeconds: 360, Data: []byte("c1"), Format: "mp3"},
			{Index: 1, StartOffsetSec: 360, DurationSeconds: 360, Data: []byte("c2"), Format: "mp3"},
		}, nil
	}
	svc.WithAudioExtractor(mockExtractor)

	parallelCalled := false
	mockSTT.TranscribeChunksFunc = func(ctx context.Context, chunks []dtos.AudioChunk, opts dtos.STTOptions) (*dtos.TranscriptionResult, error) {
		parallelCalled = true
		return &dtos.TranscriptionResult{Text: "parallel chunks transcribed"}, nil
	}

	resLong, err := svc.TranscribeAudioAdaptive(context.Background(), strings.NewReader("long audio"), 720.0, opts, recID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !parallelCalled {
		t.Error("expected parallel chunk transcription to be called")
	}
	if resLong.Text != "parallel chunks transcribed" {
		t.Errorf("expected parallel chunks transcribed, got %s", resLong.Text)
	}

	// 3. Nil reader -> error
	_, err = svc.TranscribeAudioAdaptive(context.Background(), nil, 300.0, opts, recID)
	if err == nil {
		t.Error("expected error for nil reader")
	}

	// 4. Slicing error falls back cleanly to single-pass STT without empty audio
	mockExtractor.SliceFn = func(ctx context.Context, input io.Reader, filename string, totalDurationSec float64) ([]dtos.AudioChunk, error) {
		return nil, errors.New("slice failed")
	}
	var receivedAudio []byte
	mockSTT.TranscribeFunc = func(ctx context.Context, reader io.Reader, filename string, opts dtos.STTOptions) (*dtos.TranscriptionResult, error) {
		receivedAudio, _ = io.ReadAll(reader)
		return &dtos.TranscriptionResult{Text: "fallback success"}, nil
	}

	resFallback, err := svc.TranscribeAudioAdaptive(context.Background(), strings.NewReader("buffered audio stream"), 720.0, opts, recID)
	if err != nil {
		t.Fatalf("unexpected fallback error: %v", err)
	}
	if resFallback.Text != "fallback success" {
		t.Errorf("expected fallback success, got %s", resFallback.Text)
	}
	if string(receivedAudio) != "buffered audio stream" {
		t.Errorf("expected 'buffered audio stream', got '%s'", string(receivedAudio))
	}
}

