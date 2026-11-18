package services_test

import (
	"bytes"
	"context"
	"errors"
	"io"
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
	"code-base-golang/internal/queue"
	"code-base-golang/internal/repositories"
	"code-base-golang/internal/services"
)

// mockStorage implements services.FileStorage for visual tests.
type mockVisualStorage struct {
	downloadData []byte
	downloadErr  error
	uploadCalls  int
	uploadErr    error
}

func (m *mockVisualStorage) Ping(ctx context.Context) error {
	return nil
}

func (m *mockVisualStorage) Download(ctx context.Context, bucket, objectKey string) (io.ReadCloser, error) {
	if m.downloadErr != nil {
		return nil, m.downloadErr
	}
	return io.NopCloser(bytes.NewReader(m.downloadData)), nil
}

func (m *mockVisualStorage) Upload(ctx context.Context, bucket, objectKey string, body io.Reader, size int64, contentType string) error {
	m.uploadCalls++
	return m.uploadErr
}

func (m *mockVisualStorage) Delete(ctx context.Context, bucket, objectKey string) error {
	return nil
}

func (m *mockVisualStorage) PresignGetObject(ctx context.Context, bucket, objectKey string, expires time.Duration) (string, error) {
	return "https://s3.example.com/" + objectKey, nil
}

func (m *mockVisualStorage) PresignPutObject(ctx context.Context, bucket, objectKey string, expires time.Duration) (string, error) {
	return "https://s3.example.com/" + objectKey, nil
}

func (m *mockVisualStorage) StatObject(ctx context.Context, bucket, objectKey string) (int64, error) {
	return int64(len(m.downloadData)), nil
}

// mockVisualExtractor implements services.VisualExtractor.
type mockVisualExtractor struct {
	extractFn func(ctx context.Context, input io.Reader, filename string, opts dtos.KeyframeExtractionOptions) ([]dtos.ExtractedKeyframe, error)
}

func (m *mockVisualExtractor) ExtractKeyframes(ctx context.Context, input io.Reader, filename string, opts dtos.KeyframeExtractionOptions) ([]dtos.ExtractedKeyframe, error) {
	if m.extractFn != nil {
		return m.extractFn(ctx, input, filename, opts)
	}
	return nil, nil
}

// mockVisionAnalyzer implements services.VisionAnalyzer.
type mockVisionAnalyzer struct {
	analyzeFn func(ctx context.Context, imgBytes []byte, mimeType string, prompt string) (*dtos.KeyframeAnalysisResult, error)
}

func (m *mockVisionAnalyzer) AnalyzeKeyframe(ctx context.Context, imgBytes []byte, mimeType string, prompt string) (*dtos.KeyframeAnalysisResult, error) {
	if m.analyzeFn != nil {
		return m.analyzeFn(ctx, imgBytes, mimeType, prompt)
	}
	return &dtos.KeyframeAnalysisResult{
		CueType:       constants.VisualCueTypeSlide,
		Title:         "Sample Slide Title",
		ExtractedText: "Sample extracted text",
		VisualSummary: "Sample visual summary",
		Confidence:    0.90,
	}, nil
}

func (m *mockVisionAnalyzer) AnalyzeKeyframesBatch(ctx context.Context, frames []dtos.KeyframeBatchInput) ([]dtos.KeyframeAnalysisResult, error) {
	var results []dtos.KeyframeAnalysisResult
	for _, f := range frames {
		res, err := m.AnalyzeKeyframe(ctx, f.ImageBytes, f.MimeType, "")
		if err != nil {
			return nil, err
		}
		res.SequenceOrder = f.SequenceOrder
		res.TimestampSeconds = f.TimestampSeconds
		results = append(results, *res)
	}
	return results, nil
}

// mockLocalOCR implements services.LocalOCRExtractor.
type mockLocalOCR struct {
	available  bool
	extractFn  func(ctx context.Context, imgBytes []byte) (string, float64, bool, error)
}

func (m *mockLocalOCR) IsAvailable() bool {
	return m.available
}

func (m *mockLocalOCR) ExtractText(ctx context.Context, imgBytes []byte) (string, float64, bool, error) {
	if m.extractFn != nil {
		return m.extractFn(ctx, imgBytes)
	}
	return "", 0.0, false, nil
}

func setupVisualTestService(t *testing.T) (*services.Service, sqlmock.Sqlmock, *mockVisualStorage) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}

	dialector := postgres.New(postgres.Config{
		Conn:       db,
		DriverName: "postgres",
	})
	gormDB, err := gorm.Open(dialector, &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open gorm db: %v", err)
	}

	repo := repositories.New(gormDB, nil)
	storage := &mockVisualStorage{downloadData: []byte("fake-video-content")}
	cfg := config.Config{S3BucketName: "test-bucket"}

	svc := services.New(cfg, repo, storage)
	return svc, mock, storage
}

func TestProcessVisualExtraction_NonVideo(t *testing.T) {
	svc, _, _ := setupVisualTestService(t)

	rec := &models.Recording{
		BaseModel:        models.BaseModel{ID: uuid.New()},
		OriginalFilename: "audio_only.mp3",
	}
	p := queue.RecordingPipelinePayload{
		RecordingID: rec.ID,
		SourcePath:  "recordings/test/audio.mp3",
	}

	err := svc.ProcessVisualExtraction(context.Background(), rec, p, nil)
	if err != nil {
		t.Fatalf("expected nil error for non-video, got: %v", err)
	}
}

func TestProcessVisualExtraction_ExtractorNotConfigured_Degrades(t *testing.T) {
	svc, mock, _ := setupVisualTestService(t)

	recID := uuid.New()
	rec := &models.Recording{
		BaseModel:        models.BaseModel{ID: recID},
		OriginalFilename: "presentation.mp4",
	}
	p := queue.RecordingPipelinePayload{
		RecordingID: recID,
		SourcePath:  "recordings/test/presentation.mp4",
	}

	// Expect analytics update recording degradation
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings"`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := svc.ProcessVisualExtraction(context.Background(), rec, p, nil)
	if err != nil {
		t.Fatalf("expected nil error on degradation, got: %v", err)
	}
	if rec.AnalyticsData["visual_status"] != models.RecordingStatusCompletedAudioOnly {
		t.Errorf("expected visual_status %s, got %v", models.RecordingStatusCompletedAudioOnly, rec.AnalyticsData["visual_status"])
	}
}

func TestProcessVisualExtraction_ClearsStaleWarningOnSuccess(t *testing.T) {
	svc, mock, _ := setupVisualTestService(t)

	recID := uuid.New()
	rec := &models.Recording{
		BaseModel:        models.BaseModel{ID: recID},
		OriginalFilename: "lecture_demo.mp4",
		AnalyticsData: models.JSONMap{
			"visual_warning": "previous failure",
			"visual_status":  "FAILED",
		},
	}
	p := queue.RecordingPipelinePayload{
		RecordingID: recID,
		SourcePath:  "recordings/test/lecture_demo.mp4",
	}

	extractor := &mockVisualExtractor{
		extractFn: func(ctx context.Context, input io.Reader, filename string, opts dtos.KeyframeExtractionOptions) ([]dtos.ExtractedKeyframe, error) {
			return []dtos.ExtractedKeyframe{
				{
					TimestampSeconds: 5.0,
					SequenceOrder:    0,
					ImageBytes:       []byte("fake-jpeg-data"),
					MimeType:         "image/jpeg",
				},
			}, nil
		},
	}
	svc.SetVisualExtractor(extractor)

	// Expect DeleteVideoKeyframesByRecordingID
	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM "video_keyframes" WHERE recording_id = \$1`).
		WithArgs(recID).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	// GORM query expectation for batch insert
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "video_keyframes"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "metadata", "created_at", "updated_at"}).
			AddRow(uuid.New(), []byte(`{}`), time.Now(), time.Now()))
	mock.ExpectCommit()

	// Update recording analytics expectation
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings"`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := svc.ProcessVisualExtraction(context.Background(), rec, p, nil)
	if err != nil {
		t.Fatalf("expected nil error on success, got: %v", err)
	}

	if _, exists := rec.AnalyticsData["visual_warning"]; exists {
		t.Errorf("expected visual_warning to be deleted from analytics_data, but it still exists: %v", rec.AnalyticsData["visual_warning"])
	}
	if rec.AnalyticsData["visual_status"] != models.RecordingStatusCompleted {
		t.Errorf("expected visual_status COMPLETED, got %v", rec.AnalyticsData["visual_status"])
	}
}


func TestProcessVisualExtraction_Success_WithCascade(t *testing.T) {
	svc, mock, storage := setupVisualTestService(t)

	recID := uuid.New()
	rec := &models.Recording{
		BaseModel:        models.BaseModel{ID: recID},
		OriginalFilename: "lecture_demo.mp4",
	}
	p := queue.RecordingPipelinePayload{
		RecordingID: recID,
		SourcePath:  "recordings/test/lecture_demo.mp4",
	}

	segments := []models.TranscriptSegment{
		{StartTime: 0.0, EndTime: 10.0, SpeakerLabel: "Speaker 1"},
		{StartTime: 12.5, EndTime: 30.0, SpeakerLabel: "Speaker 2"},
	}

	// Mock Visual Extractor returning 2 frames
	extractor := &mockVisualExtractor{
		extractFn: func(ctx context.Context, input io.Reader, filename string, opts dtos.KeyframeExtractionOptions) ([]dtos.ExtractedKeyframe, error) {
			return []dtos.ExtractedKeyframe{
				{TimestampSeconds: 5.0, ImageBytes: []byte("frame1-bytes"), MimeType: "image/jpeg", SequenceOrder: 1},
				{TimestampSeconds: 15.0, ImageBytes: []byte("frame2-bytes"), MimeType: "image/jpeg", SequenceOrder: 2},
			}, nil
		},
	}
	svc.SetVisualExtractor(extractor)

	// Mock Local OCR: First frame has good OCR
	localOCR := &mockLocalOCR{
		available: true,
		extractFn: func(ctx context.Context, imgBytes []byte) (string, float64, bool, error) {
			if string(imgBytes) == "frame1-bytes" {
				return "Microservices Architecture\n1. API Gateway\n2. Database", 0.90, true, nil
			}
			return "", 0.20, false, nil // second frame fails OCR -> escalates to Vision LLM
		},
	}
	svc.SetLocalOCR(localOCR)

	// Mock Vision Analyzer: Second frame escalated here
	visionAnalyzer := &mockVisionAnalyzer{
		analyzeFn: func(ctx context.Context, imgBytes []byte, mimeType string, prompt string) (*dtos.KeyframeAnalysisResult, error) {
			return &dtos.KeyframeAnalysisResult{
				CueType:       constants.VisualCueTypeScreenShare,
				Title:         "Terminal Screencast",
				ExtractedText: "docker compose up -d",
				VisualSummary: "User is deploying services via CLI",
				Confidence:    0.95,
			}, nil
		},
	}
	svc.SetVisionAnalyzer(visionAnalyzer)

	// DB Expectations:
	// 1. DeleteVideoKeyframesByRecordingID
	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM "video_keyframes" WHERE recording_id = \$1`).
		WithArgs(recID).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	// 2. SaveVideoKeyframes (batch insert 2 rows with RETURNING)
	mock.ExpectBegin()
	rows := sqlmock.NewRows([]string{"id", "metadata", "created_at", "updated_at"}).
		AddRow(uuid.New(), []byte("{}"), time.Now(), time.Now()).
		AddRow(uuid.New(), []byte("{}"), time.Now(), time.Now())
	mock.ExpectQuery(`INSERT INTO "video_keyframes"`).WillReturnRows(rows)
	mock.ExpectCommit()

	// 3. UpdateRecordingAnalytics
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings"`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := svc.ProcessVisualExtraction(context.Background(), rec, p, segments)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if storage.uploadCalls != 2 {
		t.Errorf("expected 2 thumbnail uploads to storage, got %d", storage.uploadCalls)
	}

	if rec.AnalyticsData["visual_status"] != models.RecordingStatusCompleted {
		t.Errorf("expected visual_status COMPLETED, got %v", rec.AnalyticsData["visual_status"])
	}
	if rec.AnalyticsData["keyframes_count"] != 2 {
		t.Errorf("expected keyframes_count 2, got %v", rec.AnalyticsData["keyframes_count"])
	}
}

func TestProcessVisualExtraction_KeyframeExtractionFails_Degrades(t *testing.T) {
	svc, mock, _ := setupVisualTestService(t)

	recID := uuid.New()
	rec := &models.Recording{
		BaseModel:        models.BaseModel{ID: recID},
		OriginalFilename: "corrupt.mp4",
	}
	p := queue.RecordingPipelinePayload{
		RecordingID: recID,
		SourcePath:  "recordings/test/corrupt.mp4",
	}

	extractor := &mockVisualExtractor{
		extractFn: func(ctx context.Context, input io.Reader, filename string, opts dtos.KeyframeExtractionOptions) ([]dtos.ExtractedKeyframe, error) {
			return nil, errors.New("ffmpeg exit status 1")
		},
	}
	svc.SetVisualExtractor(extractor)

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings"`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := svc.ProcessVisualExtraction(context.Background(), rec, p, nil)
	if err != nil {
		t.Fatalf("expected nil error on degradation, got: %v", err)
	}
	if rec.AnalyticsData["visual_status"] != models.RecordingStatusCompletedAudioOnly {
		t.Errorf("expected visual_status %s, got %v", models.RecordingStatusCompletedAudioOnly, rec.AnalyticsData["visual_status"])
	}
}

