package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	gormPostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"

	"code-base-golang/internal/models"
)

func setupKeyframeRepoTest(t *testing.T) (*Repositories, sqlmock.Sqlmock) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}

	gormDB, err := gorm.Open(gormPostgres.New(gormPostgres.Config{
		Conn: sqlDB,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to initialize gorm: %v", err)
	}

	repo := New(gormDB)
	return repo, mock
}

func TestRepositories_SaveVideoKeyframes_Empty(t *testing.T) {
	repo, _ := setupKeyframeRepoTest(t)
	err := repo.SaveVideoKeyframes(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected nil error on empty keyframes, got: %v", err)
	}
}

func TestRepositories_SaveVideoKeyframes_Success(t *testing.T) {
	repo, mock := setupKeyframeRepoTest(t)
	recID := uuid.New()

	frames := []models.VideoKeyframe{
		{
			RecordingID:      recID,
			TimestampSeconds: 10.0,
			S3ImageURL:       "recordings/test/keyframes/0001.jpg",
			CueType:          "SLIDE",
			Title:            "Slide 1",
			ExtractedText:    "Hello World",
			VisualSummary:    "Intro slide",
			Confidence:       0.95,
			SequenceOrder:    1,
		},
	}

	mock.ExpectBegin()
	rows := sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).
		AddRow(uuid.New(), time.Now(), time.Now())
	mock.ExpectQuery(`INSERT INTO "video_keyframes"`).WillReturnRows(rows)
	mock.ExpectCommit()

	err := repo.SaveVideoKeyframes(context.Background(), frames)
	if err != nil {
		t.Fatalf("unexpected error saving keyframes: %v", err)
	}
}

func TestRepositories_ListVideoKeyframesByRecordingID(t *testing.T) {
	repo, mock := setupKeyframeRepoTest(t)
	recID := uuid.New()

	rows := sqlmock.NewRows([]string{"id", "recording_id", "timestamp_seconds", "s3_image_url", "cue_type", "title", "extracted_text", "visual_summary", "confidence", "sequence_order"}).
		AddRow(uuid.New(), recID, 12.0, "s3://frame1.jpg", "SLIDE", "Overview", "Point A", "Summary A", 0.9, 1).
		AddRow(uuid.New(), recID, 45.0, "s3://frame2.jpg", "DEMO", "Terminal", "git status", "Summary B", 0.85, 2)

	mock.ExpectQuery(`SELECT \* FROM "video_keyframes" WHERE recording_id = \$1 ORDER BY sequence_order ASC, timestamp_seconds ASC`).
		WithArgs(recID).
		WillReturnRows(rows)

	results, err := repo.ListVideoKeyframesByRecordingID(context.Background(), recID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 keyframes, got %d", len(results))
	}
	if results[0].Title != "Overview" || results[1].Title != "Terminal" {
		t.Errorf("unexpected keyframe titles: %s, %s", results[0].Title, results[1].Title)
	}
}

func TestRepositories_DeleteVideoKeyframesByRecordingID(t *testing.T) {
	repo, mock := setupKeyframeRepoTest(t)
	recID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM "video_keyframes" WHERE recording_id = \$1`).
		WithArgs(recID).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()

	err := repo.DeleteVideoKeyframesByRecordingID(context.Background(), recID)
	if err != nil {
		t.Fatalf("unexpected error deleting keyframes: %v", err)
	}
}
