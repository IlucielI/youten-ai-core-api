package services_test

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/pkg/ctxmeta"
)

func TestService_GetWorkspaceSpeakers_Unauthorized(t *testing.T) {
	svc, _, _, _ := setupRecordingTestService(t)

	// Missing auth context
	res, err := svc.GetWorkspaceSpeakers(context.Background())
	if !errors.Is(err, constants.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
	if res != nil {
		t.Fatalf("expected nil response, got %+v", res)
	}

	// Nil UserID
	ctxNil := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: uuid.Nil})
	res, err = svc.GetWorkspaceSpeakers(ctxNil)
	if !errors.Is(err, constants.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized for nil user, got %v", err)
	}
	if res != nil {
		t.Fatalf("expected nil response, got %+v", res)
	}
}

func TestService_GetWorkspaceSpeakers_DBError(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	expectedSQL := regexp.QuoteMeta(`SELECT ts.speaker_name AS name, COUNT(DISTINCT ts.recording_id) AS total_meetings, COALESCE(SUM(GREATEST(0, ts.end_time - ts.start_time)), 0) AS total_talk_time, MAX(r.created_at) AS last_active FROM transcript_segments ts JOIN recordings r ON ts.recording_id = r.id WHERE r.user_id = $1 AND r.deleted_at IS NULL AND TRIM(ts.speaker_name) != '' GROUP BY "ts"."speaker_name" ORDER BY total_meetings DESC, total_talk_time DESC, name ASC`)

	mock.ExpectQuery(expectedSQL).
		WithArgs(userID).
		WillReturnError(errors.New("connection reset by peer"))

	res, err := svc.GetWorkspaceSpeakers(ctx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if res != nil {
		t.Fatalf("expected nil response on error, got: %+v", res)
	}
}

func TestService_GetWorkspaceSpeakers_Empty(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	expectedSQL := regexp.QuoteMeta(`SELECT ts.speaker_name AS name, COUNT(DISTINCT ts.recording_id) AS total_meetings, COALESCE(SUM(GREATEST(0, ts.end_time - ts.start_time)), 0) AS total_talk_time, MAX(r.created_at) AS last_active FROM transcript_segments ts JOIN recordings r ON ts.recording_id = r.id WHERE r.user_id = $1 AND r.deleted_at IS NULL AND TRIM(ts.speaker_name) != '' GROUP BY "ts"."speaker_name" ORDER BY total_meetings DESC, total_talk_time DESC, name ASC`)

	rows := sqlmock.NewRows([]string{"name", "total_meetings", "total_talk_time", "last_active"})
	mock.ExpectQuery(expectedSQL).
		WithArgs(userID).
		WillReturnRows(rows)

	res, err := svc.GetWorkspaceSpeakers(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Count != 0 {
		t.Errorf("expected count 0, got %d", res.Count)
	}
	if len(res.Speakers) != 0 {
		t.Errorf("expected empty speakers slice, got %d items", len(res.Speakers))
	}
}

func TestService_GetWorkspaceSpeakers_Success(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})
	now := time.Now().Truncate(time.Second)

	expectedSQL := regexp.QuoteMeta(`SELECT ts.speaker_name AS name, COUNT(DISTINCT ts.recording_id) AS total_meetings, COALESCE(SUM(GREATEST(0, ts.end_time - ts.start_time)), 0) AS total_talk_time, MAX(r.created_at) AS last_active FROM transcript_segments ts JOIN recordings r ON ts.recording_id = r.id WHERE r.user_id = $1 AND r.deleted_at IS NULL AND TRIM(ts.speaker_name) != '' GROUP BY "ts"."speaker_name" ORDER BY total_meetings DESC, total_talk_time DESC, name ASC`)

	rows := sqlmock.NewRows([]string{"name", "total_meetings", "total_talk_time", "last_active"}).
		AddRow("Alice", 5, 840.456, now).
		AddRow("Bob", 2, 210.123, now.Add(-24*time.Hour))

	mock.ExpectQuery(expectedSQL).
		WithArgs(userID).
		WillReturnRows(rows)

	res, err := svc.GetWorkspaceSpeakers(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Count != 2 {
		t.Fatalf("expected count 2, got %d", res.Count)
	}
	if len(res.Speakers) != 2 {
		t.Fatalf("expected 2 speakers, got %d", len(res.Speakers))
	}

	// Verify speaker 0
	if res.Speakers[0].Name != "Alice" || res.Speakers[0].TotalMeetings != 5 || res.Speakers[0].TotalTalkTime != 840.46 {
		t.Errorf("unexpected Alice metrics: %+v", res.Speakers[0])
	}
	// Verify speaker 1
	if res.Speakers[1].Name != "Bob" || res.Speakers[1].TotalMeetings != 2 || res.Speakers[1].TotalTalkTime != 210.12 {
		t.Errorf("unexpected Bob metrics: %+v", res.Speakers[1])
	}
}
