package services_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/pkg/ctxmeta"
)

func TestService_GetRecordingDetail_NotFound(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	_, err := svc.GetRecordingDetail(context.Background(), recID, "")
	if !errors.Is(err, constants.ErrRecordingNotFound) {
		t.Fatalf("expected ErrRecordingNotFound, got %v", err)
	}
}

func TestService_GetRecordingDetail_Forbidden_UnauthenticatedNoToken(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "is_guest"}).
			AddRow(recID, "secret-token", true))

	_, err := svc.GetRecordingDetail(context.Background(), recID, "")
	if !errors.Is(err, constants.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestService_GetRecordingDetail_Forbidden_UnauthenticatedWrongToken(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "is_guest"}).
			AddRow(recID, "secret-token", true))

	_, err := svc.GetRecordingDetail(context.Background(), recID, "invalid-token")
	if !errors.Is(err, constants.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestService_GetRecordingDetail_Forbidden_AuthenticatedDifferentUser(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	ownerID := uuid.New()
	callerID := uuid.New()

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: callerID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "ownership_token", "is_guest"}).
			AddRow(recID, ownerID, "owner-token", false))

	_, err := svc.GetRecordingDetail(ctx, recID, "")
	if !errors.Is(err, constants.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestService_GetRecordingDetail_Success_AuthenticatedOwner(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	ownerID := uuid.New()
	audioPath := "recordings/" + recID.String() + "/speech.mp3"
	now := time.Now()

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: ownerID})

	// 1. FindRecordingByID
	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "title", "original_filename", "audio_url", "source_type",
			"status", "selected_template", "output_language", "is_guest", "created_at", "updated_at",
		}).AddRow(
			recID, ownerID, "Architecture Review", "speech.mp3", audioPath, "UPLOAD",
			"COMPLETED", "MOM", "en", false, now, now,
		))

	// 2. ListTranscriptSegmentsByRecordingID
	segID := uuid.New()
	mock.ExpectQuery(`SELECT \* FROM "transcript_segments" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "recording_id", "speaker_label", "speaker_name", "start_time", "end_time", "text", "sequence_order",
		}).AddRow(segID, recID, "Speaker 0", "Alice", 0.0, 4.5, "Welcome everyone", 0))

	// 3. FindActiveSummaryByRecordingID
	sumID := uuid.New()
	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND is_active = TRUE`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "recording_id", "template_category", "version", "is_active", "markdown_content", "created_at", "updated_at",
		}).AddRow(sumID, recID, "MOM", 1, true, "## Key Decisions", now, now))

	// 4. ListChaptersByRecordingID
	chapID := uuid.New()
	mock.ExpectQuery(`SELECT \* FROM "chapters" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC LIMIT \$2`).
		WithArgs(recID, 100).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "recording_id", "title", "start_time", "end_time", "summary", "sequence_order", "created_at",
		}).AddRow(chapID, recID, "Introduction", 0.0, 60.0, "Session kickoff", 0, now))

	// 5. ListHighlightsByRecordingID
	hlID := uuid.New()
	mock.ExpectQuery(`SELECT \* FROM "highlights" WHERE recording_id = \$1 ORDER BY start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "recording_id", "start_time", "end_time", "source", "created_at",
		}).AddRow(hlID, recID, 10.0, 15.0, "manual", now))

	resp, err := svc.GetRecordingDetail(ctx, recID, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ID != recID.String() {
		t.Errorf("expected ID %s, got %s", recID.String(), resp.ID)
	}
	if resp.Title != "Architecture Review" {
		t.Errorf("expected title Architecture Review, got %s", resp.Title)
	}
	if resp.PlaybackURL == nil || *resp.PlaybackURL != "https://mock.storage/get" {
		t.Errorf("expected presigned playback URL, got %v", resp.PlaybackURL)
	}
	if len(resp.Segments) != 1 || resp.Segments[0].Text != "Welcome everyone" {
		t.Errorf("unexpected segments: %+v", resp.Segments)
	}
	if resp.ActiveSummary == nil || resp.ActiveSummary.MarkdownContent != "## Key Decisions" {
		t.Errorf("unexpected active summary: %+v", resp.ActiveSummary)
	}
	if len(resp.Chapters) != 1 || resp.Chapters[0].Title != "Introduction" {
		t.Errorf("unexpected chapters: %+v", resp.Chapters)
	}
	if len(resp.Highlights) != 1 || resp.Highlights[0].Source != "manual" {
		t.Errorf("unexpected highlights: %+v", resp.Highlights)
	}
}

func TestService_GetRecordingDetail_Success_GuestWithOwnershipToken(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	guestToken := "guest-valid-token-789"
	now := time.Now()

	// 1. FindRecordingByID
	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "title", "original_filename", "audio_url", "source_type",
			"status", "selected_template", "output_language", "is_guest", "ownership_token", "created_at", "updated_at",
		}).AddRow(
			recID, "Guest Meeting", "guest.mp3", "https://external.cdn/guest.mp3", "LINK",
			"COMPLETED", "GENERAL", "id", true, guestToken, now, now,
		))

	// 2. Empty child queries
	mock.ExpectQuery(`SELECT \* FROM "transcript_segments" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND is_active = TRUE`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	mock.ExpectQuery(`SELECT \* FROM "chapters" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC LIMIT \$2`).
		WithArgs(recID, 100).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	mock.ExpectQuery(`SELECT \* FROM "highlights" WHERE recording_id = \$1 ORDER BY start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	resp, err := svc.GetRecordingDetail(context.Background(), recID, guestToken)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ID != recID.String() {
		t.Errorf("expected ID %s, got %s", recID.String(), resp.ID)
	}
	if !resp.IsGuest {
		t.Error("expected IsGuest true")
	}
	if resp.PlaybackURL == nil || *resp.PlaybackURL != "https://external.cdn/guest.mp3" {
		t.Errorf("expected external playback URL retained, got %v", resp.PlaybackURL)
	}
}

func TestService_GetRecordingDetail_Success_ShareToken(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	shareToken := "public-share-token-xyz"
	now := time.Now()

	// 1. FindRecordingByID
	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "title", "original_filename", "audio_url", "source_type",
			"status", "selected_template", "output_language", "is_guest", "is_share_enabled", "share_token", "ownership_token", "created_at", "updated_at",
		}).AddRow(
			recID, "Shared Presentation", "pres.mp4", "", "UPLOAD",
			"COMPLETED", "GENERAL", "en", false, true, shareToken, "owner-only-token", now, now,
		))

	// 2. Empty child queries
	mock.ExpectQuery(`SELECT \* FROM "transcript_segments" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND is_active = TRUE`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	mock.ExpectQuery(`SELECT \* FROM "chapters" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC LIMIT \$2`).
		WithArgs(recID, 100).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	mock.ExpectQuery(`SELECT \* FROM "highlights" WHERE recording_id = \$1 ORDER BY start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	resp, err := svc.GetRecordingDetail(context.Background(), recID, shareToken)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ID != recID.String() {
		t.Errorf("expected ID %s, got %s", recID.String(), resp.ID)
	}
}

func TestService_ListRecordings_Unauthorized(t *testing.T) {
	svc, _, _, _ := setupRecordingTestService(t)

	_, err := svc.ListRecordings(context.Background(), dtos.RecordingFilterQuery{})
	if !errors.Is(err, constants.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestService_ListRecordings_Success(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	recID := uuid.New()
	now := time.Now()

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings" WHERE user_id = \$1 AND \(LOWER\(title\) LIKE \$2 OR LOWER\(original_filename\) LIKE \$3\) AND status = \$4 AND selected_template = \$5 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(userID, "%sprint%", "%sprint%", "COMPLETED", "MOM").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE user_id = \$1 AND \(LOWER\(title\) LIKE \$2 OR LOWER\(original_filename\) LIKE \$3\) AND status = \$4 AND selected_template = \$5 AND "recordings"\."deleted_at" IS NULL ORDER BY created_at DESC LIMIT \$6`).
		WithArgs(userID, "%sprint%", "%sprint%", "COMPLETED", "MOM", 10).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "title", "original_filename", "file_size_bytes", "duration_seconds",
			"source_type", "status", "selected_template", "output_language", "is_guest", "created_at", "updated_at",
		}).AddRow(
			recID, userID, "Sprint Planning", "sprint.mp3", 1024, 120.0,
			"UPLOAD", "COMPLETED", "MOM", "en", false, now, now,
		))

	resp, err := svc.ListRecordings(ctx, dtos.RecordingFilterQuery{
		Search:   "sprint",
		Status:   "COMPLETED",
		Template: "MOM",
		Page:     1,
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(resp.Items))
	}
	if resp.Items[0].Title != "Sprint Planning" {
		t.Errorf("expected Title Sprint Planning, got %s", resp.Items[0].Title)
	}
	if resp.Pagination.TotalItems != 1 {
		t.Errorf("expected TotalItems 1, got %d", resp.Pagination.TotalItems)
	}
	if resp.Pagination.TotalPages != 1 {
		t.Errorf("expected TotalPages 1, got %d", resp.Pagination.TotalPages)
	}
}

func TestService_ListRecordings_EmptyResults(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings" WHERE user_id = \$1 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE user_id = \$1 AND "recordings"\."deleted_at" IS NULL ORDER BY created_at DESC LIMIT \$2`).
		WithArgs(userID, 10).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	resp, err := svc.ListRecordings(ctx, dtos.RecordingFilterQuery{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Items) != 0 {
		t.Errorf("expected 0 items, got %d", len(resp.Items))
	}
	if resp.Pagination.TotalItems != 0 {
		t.Errorf("expected TotalItems 0, got %d", resp.Pagination.TotalItems)
	}
	if resp.Pagination.TotalPages != 0 {
		t.Errorf("expected TotalPages 0, got %d", resp.Pagination.TotalPages)
	}
}

func TestService_ListRecordings_DBError(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings" WHERE user_id = \$1 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(userID).
		WillReturnError(errors.New("db connection failure"))

	_, err := svc.ListRecordings(ctx, dtos.RecordingFilterQuery{})
	if err == nil || !strings.Contains(err.Error(), "db connection failure") {
		t.Fatalf("expected db connection error, got %v", err)
	}
}

func TestService_GetSharedRecording_EmptyToken(t *testing.T) {
	svc, _, _, _ := setupRecordingTestService(t)
	ctx := context.Background()

	_, err := svc.GetSharedRecording(ctx, "   ")
	if !errors.Is(err, constants.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for empty token, got %v", err)
	}
}

func TestService_GetSharedRecording_NotFound(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE \(share_token = \$1 AND is_share_enabled = TRUE\) AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs("invalid-token", 1).
		WillReturnError(gorm.ErrRecordNotFound)

	_, err := svc.GetSharedRecording(ctx, "invalid-token")
	if !errors.Is(err, constants.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestService_GetSharedRecording_DBError(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE \(share_token = \$1 AND is_share_enabled = TRUE\) AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs("token-123", 1).
		WillReturnError(errors.New("db query error"))

	_, err := svc.GetSharedRecording(ctx, "token-123")
	if err == nil || !strings.Contains(err.Error(), "failed to retrieve shared recording") {
		t.Fatalf("expected error, got %v", err)
	}
}

func TestService_GetSharedRecording_Success(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	ctx := context.Background()
	recID := uuid.New()
	audioKey := "recordings/audio.mp3"

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE \(share_token = \$1 AND is_share_enabled = TRUE\) AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs("valid-token", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "title", "duration_seconds", "audio_url", "is_share_enabled", "share_token"}).
			AddRow(recID, "Public Meeting", 180.0, audioKey, true, "valid-token"))

	// ListTranscriptSegmentsByRecordingID
	mock.ExpectQuery(`SELECT \* FROM "transcript_segments" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "speaker_label", "text"}).
			AddRow(uuid.New(), recID, "spk_0", "Hello public world"))

	// FindActiveSummaryByRecordingID
	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND is_active = TRUE.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "markdown_content", "is_active"}).
			AddRow(uuid.New(), recID, "Public summary markdown", true))

	// ListChaptersByRecordingID
	mock.ExpectQuery(`SELECT \* FROM "chapters" WHERE recording_id = \$1.*LIMIT \$2`).
		WithArgs(recID, 100).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "title"}).
			AddRow(uuid.New(), recID, "Introduction"))

	// ListHighlightsByRecordingID
	mock.ExpectQuery(`SELECT \* FROM "highlights" WHERE recording_id = \$1 ORDER BY start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "source"}).
			AddRow(uuid.New(), recID, "KEY_POINT"))

	resp, err := svc.GetSharedRecording(ctx, "valid-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ID != recID.String() {
		t.Errorf("expected ID %s, got %s", recID.String(), resp.ID)
	}
	if resp.Title != "Public Meeting" {
		t.Errorf("expected Title 'Public Meeting', got %s", resp.Title)
	}
	if len(resp.Segments) != 1 {
		t.Errorf("expected 1 segment, got %d", len(resp.Segments))
	}
	if resp.ActiveSummary == nil || resp.ActiveSummary.MarkdownContent != "Public summary markdown" {
		t.Errorf("expected active summary markdown content, got %v", resp.ActiveSummary)
	}
	if len(resp.Chapters) != 1 {
		t.Errorf("expected 1 chapter, got %d", len(resp.Chapters))
	}
	if len(resp.Highlights) != 1 {
		t.Errorf("expected 1 highlight, got %d", len(resp.Highlights))
	}
}

func TestService_GetRecordingProgress_NotFound(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	_, _, _, err := svc.GetRecordingProgress(ctx, recID, "")
	if !errors.Is(err, constants.ErrRecordingNotFound) {
		t.Fatalf("expected ErrRecordingNotFound, got %v", err)
	}
}

func TestService_GetRecordingProgress_Forbidden_Unauthorized(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	userID := uuid.New()
	ctx := context.Background() // Not authenticated, no token

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "ownership_token", "is_share_enabled"}).
			AddRow(recID, &userID, "token-1", false))

	_, _, _, err := svc.GetRecordingProgress(ctx, recID, "")
	if !errors.Is(err, constants.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestService_GetRecordingProgress_Forbidden_NotOwner(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	ownerID := uuid.New()
	callerID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: callerID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "ownership_token", "is_share_enabled"}).
			AddRow(recID, &ownerID, "token-1", false))

	_, _, _, err := svc.GetRecordingProgress(ctx, recID, "")
	if !errors.Is(err, constants.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestService_GetRecordingProgress_Success_AuthUser(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	userID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "status", "ownership_token"}).
			AddRow(recID, &userID, "TRANSCRIBING", "token-1"))

	initial, subCh, unsub, err := svc.GetRecordingProgress(ctx, recID, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer unsub()

	if initial.RecordingID != recID.String() {
		t.Errorf("expected recording ID %s, got %s", recID.String(), initial.RecordingID)
	}
	if initial.Status != "TRANSCRIBING" || initial.Progress != 55 {
		t.Errorf("expected status TRANSCRIBING and progress 55, got %s / %d", initial.Status, initial.Progress)
	}
	if subCh == nil {
		t.Error("expected non-nil subscription channel")
	}
}

func TestService_GetRecordingProgress_Success_GuestToken(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "is_guest", "status", "ownership_token"}).
			AddRow(recID, true, "EXTRACTING", "guest-token-123"))

	initial, subCh, unsub, err := svc.GetRecordingProgress(ctx, recID, "guest-token-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer unsub()

	if initial.Status != "EXTRACTING" || initial.Progress != 30 {
		t.Errorf("expected EXTRACTING / 30, got %s / %d", initial.Status, initial.Progress)
	}
	if subCh == nil {
		t.Error("expected non-nil sub channel")
	}
}

func TestService_GetRecordingProgress_Success_ShareToken(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	shareToken := "share-token-xyz"
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "is_share_enabled", "share_token", "status"}).
			AddRow(recID, true, &shareToken, "COMPLETED"))

	initial, subCh, unsub, err := svc.GetRecordingProgress(ctx, recID, "share-token-xyz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer unsub()

	if initial.Status != "COMPLETED" || initial.Progress != 100 {
		t.Errorf("expected COMPLETED / 100, got %s / %d", initial.Status, initial.Progress)
	}
	if subCh == nil {
		t.Error("expected non-nil sub channel")
	}
}

func TestService_PollRecordingProgress_Success(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).
			AddRow(recID, "SUMMARIZING"))

	event, err := svc.PollRecordingProgress(ctx, recID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.Status != "SUMMARIZING" || event.Progress != 75 {
		t.Errorf("expected SUMMARIZING / 75, got %s / %d", event.Status, event.Progress)
	}
}

func TestService_PollRecordingProgress_NotFound(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	_, err := svc.PollRecordingProgress(ctx, recID)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected ErrRecordNotFound, got %v", err)
	}
}
