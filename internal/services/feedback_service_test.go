package services_test

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"

	"code-base-golang/internal/dtos"
	"code-base-golang/internal/pkg/ctxmeta"
)

func TestService_SubmitFeedback_SuccessGuest(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)

	feedbackID := uuid.New()
	now := time.Now()
	authorName := "Guest User"
	authorEmail := "guest@example.com"
	key := "idem-guest-001"

	// 1. Check idempotency key: returns record not found
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "feedbacks" WHERE idempotency_key = $1 ORDER BY "feedbacks"."id" LIMIT $2`)).
		WithArgs(key, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	// 2. Insert new feedback
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "feedbacks"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(feedbackID, now, now))
	mock.ExpectCommit()

	ctx := ctxmeta.WithClientMeta(context.Background(), "192.168.1.100", "Mozilla/5.0")

	req := dtos.FeedbackRequest{
		Title:          "Improve summary export",
		Category:       "feature",
		Rating:         4,
		Message:        "Please add option to export summaries directly to Notion.",
		UserName:       &authorName,
		UserEmail:      &authorEmail,
		IdempotencyKey: &key,
	}

	resp, err := svc.SubmitFeedback(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	if resp.ID != feedbackID {
		t.Errorf("expected feedback ID %s, got %s", feedbackID, resp.ID)
	}
	if resp.Title != req.Title {
		t.Errorf("expected title %s, got %s", req.Title, resp.Title)
	}
	if resp.Category != "feature" {
		t.Errorf("expected category feature, got %s", resp.Category)
	}
	if resp.Rating != 4 {
		t.Errorf("expected rating 4, got %d", resp.Rating)
	}
}

func TestService_SubmitFeedback_IdempotentDuplicate(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)

	existingID := uuid.New()
	existingCreatedAt := time.Now().Add(-10 * time.Minute)
	key := "idem-existing-key"
	userName := "Bob"
	userEmail := "bob@example.com"

	// When idempotency key already exists, query returns the existing record
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "feedbacks" WHERE idempotency_key = $1 ORDER BY "feedbacks"."id" LIMIT $2`)).
		WithArgs(key, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "title", "category", "rating", "message", "status", "user_name", "user_email", "idempotency_key", "created_at", "updated_at",
		}).AddRow(
			existingID, "Original Title", "general", 5, "Original Message", "new", userName, userEmail, key, existingCreatedAt, existingCreatedAt,
		))

	req := dtos.FeedbackRequest{
		Title:          "Duplicate Title",
		Category:       "bug",
		Rating:         1,
		Message:        "This is a duplicate submission that should be short-circuited.",
		IdempotencyKey: &key,
	}

	resp, err := svc.SubmitFeedback(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ID != existingID {
		t.Errorf("expected existing ID %s, got %s", existingID, resp.ID)
	}
	if resp.Title != "Original Title" {
		t.Errorf("expected original title to be preserved, got %s", resp.Title)
	}
}

func TestService_SubmitFeedback_AuthenticatedUser(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)

	feedbackID := uuid.New()
	userID := uuid.New()
	now := time.Now()

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "feedbacks"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(feedbackID, now, now))
	mock.ExpectCommit()

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{
		UserID: userID,
		Email:  "authuser@example.com",
	})

	req := dtos.FeedbackRequest{
		Title:   "Audio player waveform glitch",
		Message: "Waveform flickers when seeking rapidly in long audio recordings.",
	}

	resp, err := svc.SubmitFeedback(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp == nil || resp.ID != feedbackID {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if resp.Category != "general" {
		t.Errorf("expected default category general, got %s", resp.Category)
	}
	if resp.Rating != 5 {
		t.Errorf("expected default rating 5, got %d", resp.Rating)
	}
}

func TestService_SubmitFeedback_DatabaseError(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "feedbacks"`).
		WillReturnError(errors.New("db connection lost"))
	mock.ExpectRollback()

	req := dtos.FeedbackRequest{
		Title:   "Crash report",
		Message: "Something went wrong on the dashboard.",
	}

	resp, err := svc.SubmitFeedback(context.Background(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if resp != nil {
		t.Errorf("expected nil response on error, got %+v", resp)
	}
}

func TestService_SubmitFeedback_WithAttachments(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)

	feedbackID := uuid.New()
	now := time.Now()

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "feedbacks"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(feedbackID, now, now))
	mock.ExpectCommit()

	req := dtos.FeedbackRequest{
		Title:   "Visual bug on waveform",
		Message: "See attached screenshot of visual glitch.",
		Attachments: []dtos.FeedbackAttachment{
			{
				URL:           "https://mock.storage/feedbacks/uuid/screen.png",
				Filename:      "screen.png",
				ContentType:   "image/png",
				FileSizeBytes: 54321,
			},
		},
	}

	resp, err := svc.SubmitFeedback(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if len(resp.Attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(resp.Attachments))
	}
	if resp.Attachments[0].Filename != "screen.png" {
		t.Errorf("expected attachment filename screen.png, got %s", resp.Attachments[0].Filename)
	}
}

func TestService_GenerateFeedbackPresignUpload(t *testing.T) {
	svc, _, storage, _ := setupRecordingTestService(t)

	// 1. Success case
	req := dtos.FeedbackPresignRequest{
		Filename:      "bug_screenshot.png",
		ContentType:   "image/png",
		FileSizeBytes: 102400,
	}

	resp, err := svc.GenerateFeedbackPresignUpload(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if resp.UploadURL == "" {
		t.Error("expected non-empty upload url")
	}
	if resp.ObjectKey == "" || !strings.Contains(resp.ObjectKey, "feedbacks/") {
		t.Errorf("expected feedbacks object key, got %s", resp.ObjectKey)
	}
	if resp.Filename != "bug_screenshot.png" {
		t.Errorf("expected filename bug_screenshot.png, got %s", resp.Filename)
	}

	// 2. Invalid filename
	badReq := dtos.FeedbackPresignRequest{
		Filename:    "",
		ContentType: "image/png",
	}
	_, err = svc.GenerateFeedbackPresignUpload(context.Background(), badReq)
	if err == nil {
		t.Error("expected error for empty filename, got nil")
	}

	// 3. Storage error
	storage.presignErr = errors.New("s3 connection timeout")
	_, err = svc.GenerateFeedbackPresignUpload(context.Background(), req)
	if err == nil {
		t.Error("expected error when storage fails, got nil")
	}
}
