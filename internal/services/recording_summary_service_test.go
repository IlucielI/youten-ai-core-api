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

	"code-base-golang/internal/adapters/llm"
	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
	"code-base-golang/internal/pkg/ctxmeta"
)

func TestService_UpdateTranscriptSpeakers_NotFound(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	_, err := svc.UpdateTranscriptSpeakers(ctx, recID, "token", dtos.UpdateSpeakersRequest{
		Speakers: map[string]string{"SPEAKER_00": "Bayu"},
	})
	if !errors.Is(err, constants.ErrRecordingNotFound) {
		t.Fatalf("expected ErrRecordingNotFound, got %v", err)
	}
}

func TestService_UpdateTranscriptSpeakers_Forbidden_Unauthorized(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	ownerID := uuid.New()
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "ownership_token"}).
			AddRow(recID, &ownerID, "secret-token"))

	_, err := svc.UpdateTranscriptSpeakers(ctx, recID, "wrong-token", dtos.UpdateSpeakersRequest{
		Speakers: map[string]string{"SPEAKER_00": "Bayu"},
	})
	if !errors.Is(err, constants.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestService_UpdateTranscriptSpeakers_Forbidden_WrongUser(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	ownerID := uuid.New()
	otherUserID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: otherUserID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "ownership_token"}).
			AddRow(recID, &ownerID, "secret-token"))

	_, err := svc.UpdateTranscriptSpeakers(ctx, recID, "", dtos.UpdateSpeakersRequest{
		Speakers: map[string]string{"SPEAKER_00": "Bayu"},
	})
	if !errors.Is(err, constants.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestService_UpdateTranscriptSpeakers_Success_Guest(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	guestToken := "guest-ownership-token"
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token"}).
			AddRow(recID, guestToken))

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "transcript_segments" SET "speaker_name"=\$1,"updated_at"=\$2 WHERE recording_id = \$3 AND speaker_label = \$4`).
		WithArgs("Bayu", sqlmock.AnyArg(), recID, "SPEAKER_00").
		WillReturnResult(sqlmock.NewResult(0, 4))
	mock.ExpectCommit()

	resp, err := svc.UpdateTranscriptSpeakers(ctx, recID, guestToken, dtos.UpdateSpeakersRequest{
		Speakers: map[string]string{"SPEAKER_00": "Bayu"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.UpdatedCount != 4 {
		t.Errorf("expected UpdatedCount 4, got %d", resp.UpdatedCount)
	}
	if resp.Speakers["SPEAKER_00"] != "Bayu" {
		t.Errorf("expected speaker name 'Bayu', got %s", resp.Speakers["SPEAKER_00"])
	}
}

func TestService_UpdateTranscriptSpeakers_Success_AuthenticatedOwner(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	userID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id"}).
			AddRow(recID, &userID))

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "transcript_segments" SET "speaker_name"=\$1,"updated_at"=\$2 WHERE recording_id = \$3 AND speaker_label = \$4`).
		WithArgs("Alice", sqlmock.AnyArg(), recID, "SPEAKER_01").
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()

	resp, err := svc.UpdateTranscriptSpeakers(ctx, recID, "", dtos.UpdateSpeakersRequest{
		Speakers: map[string]string{"SPEAKER_01": "Alice"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.UpdatedCount != 2 {
		t.Errorf("expected UpdatedCount 2, got %d", resp.UpdatedCount)
	}
	if resp.Speakers["SPEAKER_01"] != "Alice" {
		t.Errorf("expected speaker name 'Alice', got %s", resp.Speakers["SPEAKER_01"])
	}
}

func TestService_RegenerateSummary_NotFound(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	_, err := svc.RegenerateSummary(ctx, recID, "some-token", dtos.RegenerateSummaryRequest{})
	if !errors.Is(err, constants.ErrRecordingNotFound) {
		t.Fatalf("expected ErrRecordingNotFound, got %v", err)
	}
}

func TestService_RegenerateSummary_Forbidden_Unauthorized(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token"}).
			AddRow(recID, "valid-token"))

	_, err := svc.RegenerateSummary(ctx, recID, "wrong-token", dtos.RegenerateSummaryRequest{})
	if !errors.Is(err, constants.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestService_RegenerateSummary_Forbidden_WrongUser(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	ownerID := uuid.New()
	otherUserID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: otherUserID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "ownership_token"}).
			AddRow(recID, &ownerID, "valid-token"))

	_, err := svc.RegenerateSummary(ctx, recID, "", dtos.RegenerateSummaryRequest{})
	if !errors.Is(err, constants.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestService_RegenerateSummary_ConflictProcessing(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	guestToken := "guest-token"
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "status"}).
			AddRow(recID, guestToken, models.RecordingStatusExtracting))

	_, err := svc.RegenerateSummary(ctx, recID, guestToken, dtos.RegenerateSummaryRequest{})
	if !errors.Is(err, constants.ErrConflictProcessing) {
		t.Fatalf("expected ErrConflictProcessing, got %v", err)
	}
}

func TestService_RegenerateSummary_VersionLimitReached(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	guestToken := "guest-token"
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "status"}).
			AddRow(recID, guestToken, models.RecordingStatusCompleted))

	mock.ExpectQuery(`SELECT count\(\*\) FROM "summaries" WHERE recording_id = \$1`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(5))

	_, err := svc.RegenerateSummary(ctx, recID, guestToken, dtos.RegenerateSummaryRequest{})
	if !errors.Is(err, constants.ErrSummaryVersionLimit) {
		t.Fatalf("expected ErrSummaryVersionLimit, got %v", err)
	}
}

func TestService_RegenerateSummary_EmptyTranscript(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	guestToken := "guest-token"
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "status"}).
			AddRow(recID, guestToken, models.RecordingStatusCompleted))

	mock.ExpectQuery(`SELECT count\(\*\) FROM "summaries" WHERE recording_id = \$1`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	mock.ExpectQuery(`SELECT \* FROM "transcript_segments" WHERE recording_id = \$1 ORDER BY sequence_order ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, err := svc.RegenerateSummary(ctx, recID, guestToken, dtos.RegenerateSummaryRequest{})
	if err == nil {
		t.Fatal("expected error on empty transcript, got nil")
	}
	if !strings.Contains(err.Error(), "cannot regenerate summary without transcript") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestService_RegenerateSummary_LLMFailure(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	guestToken := "guest-token"
	ctx := context.Background()

	mockLLM := llm.NewMock()
	mockLLM.GenerateStructuredFunc = func(ctx context.Context, systemPrompt, userPrompt string, schema map[string]interface{}) (*dtos.StructuredResponse, error) {
		return nil, errors.New("llm provider timeout")
	}
	svc.SetLLM(mockLLM)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "status", "detected_language", "output_language", "selected_template"}).
			AddRow(recID, guestToken, models.RecordingStatusCompleted, "en", "en", "GENERAL"))

	mock.ExpectQuery(`SELECT count\(\*\) FROM "summaries" WHERE recording_id = \$1`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	mock.ExpectQuery(`SELECT \* FROM "transcript_segments" WHERE recording_id = \$1 ORDER BY sequence_order ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "speaker_name", "text", "start_time", "end_time"}).
			AddRow(uuid.New(), recID, "Speaker 1", "Hello world", 0.0, 5.0))

	mock.ExpectQuery(`SELECT \* FROM "templates" WHERE category_key = \$1 AND is_active = TRUE.*LIMIT \$2`).
		WithArgs("GENERAL", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "category_key", "prompt", "output_schema"}).
			AddRow(uuid.New(), "GENERAL", "Default summary prompt", "{}"))

	_, err := svc.RegenerateSummary(ctx, recID, guestToken, dtos.RegenerateSummaryRequest{})
	if err == nil {
		t.Fatal("expected error on LLM failure, got nil")
	}
	if !strings.Contains(err.Error(), "llm generation failed") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestService_RegenerateSummary_MalformedLLMJSON(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	guestToken := "guest-token"
	ctx := context.Background()

	mockLLM := llm.NewMock()
	mockLLM.GenerateStructuredFunc = func(ctx context.Context, systemPrompt, userPrompt string, schema map[string]interface{}) (*dtos.StructuredResponse, error) {
		return &dtos.StructuredResponse{
			RawJSON: `{malformed json`,
		}, nil
	}
	svc.SetLLM(mockLLM)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "status", "detected_language", "output_language", "selected_template"}).
			AddRow(recID, guestToken, models.RecordingStatusCompleted, "en", "en", "GENERAL"))

	mock.ExpectQuery(`SELECT count\(\*\) FROM "summaries" WHERE recording_id = \$1`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	mock.ExpectQuery(`SELECT \* FROM "transcript_segments" WHERE recording_id = \$1 ORDER BY sequence_order ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "speaker_name", "text", "start_time", "end_time"}).
			AddRow(uuid.New(), recID, "Speaker 1", "Hello world", 0.0, 5.0))

	mock.ExpectQuery(`SELECT \* FROM "templates" WHERE category_key = \$1 AND is_active = TRUE.*LIMIT \$2`).
		WithArgs("GENERAL", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "category_key", "prompt", "output_schema"}).
			AddRow(uuid.New(), "GENERAL", "Default summary prompt", "{}"))

	_, err := svc.RegenerateSummary(ctx, recID, guestToken, dtos.RegenerateSummaryRequest{})
	if err == nil {
		t.Fatal("expected error on malformed LLM JSON, got nil")
	}
	if !strings.Contains(err.Error(), "invalid structured response JSON") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestService_RegenerateSummary_Success_Guest(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	guestToken := "guest-token"
	ctx := context.Background()

	customAngle := "Focus on technical decisions and blockers"
	mockLLM := llm.NewMock()
	mockLLM.GenerateStructuredFunc = func(ctx context.Context, systemPrompt, userPrompt string, schema map[string]interface{}) (*dtos.StructuredResponse, error) {
		if !strings.Contains(userPrompt, "Focus Angle: Focus on technical decisions and blockers") {
			t.Errorf("expected user prompt to contain custom angle, got: %s", userPrompt)
		}
		return &dtos.StructuredResponse{
			RawJSON: `{"title": "Technical Debrief", "markdown_content": "# Technical Debrief\nKey decisions made.", "action_items": []}`,
		}, nil
	}
	svc.SetLLM(mockLLM)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "status", "detected_language", "output_language", "selected_template"}).
			AddRow(recID, guestToken, models.RecordingStatusCompleted, "en", "en", "MOM"))

	mock.ExpectQuery(`SELECT count\(\*\) FROM "summaries" WHERE recording_id = \$1`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	mock.ExpectQuery(`SELECT \* FROM "transcript_segments" WHERE recording_id = \$1 ORDER BY sequence_order ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "speaker_name", "text", "start_time", "end_time"}).
			AddRow(uuid.New(), recID, "Bayu", "We need to optimize queries.", 0.0, 10.0))

	mock.ExpectQuery(`SELECT \* FROM "templates" WHERE category_key = \$1 AND is_active = TRUE.*LIMIT \$2`).
		WithArgs("MOM", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "category_key", "prompt", "output_schema"}).
			AddRow(uuid.New(), "MOM", "MOM prompt", "{}"))

	// SaveNewSummaryVersion expectations
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT count\(\*\) FROM "summaries" WHERE recording_id = \$1`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectQuery(`SELECT COALESCE\(MAX\(version\), 0\) FROM "summaries" WHERE recording_id = \$1`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"coalesce"}).AddRow(2))
	mock.ExpectExec(`UPDATE "summaries" SET "is_active"=\$1,"updated_at"=\$2 WHERE recording_id = \$3`).
		WithArgs(false, sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 2))
	mock.ExpectQuery(`INSERT INTO "summaries"`).
		WithArgs(recID, "MOM", &customAngle, 3, true, sqlmock.AnyArg(), "# Technical Debrief\nKey decisions made.", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(uuid.New(), time.Now(), time.Now()))
	mock.ExpectCommit()

	resp, err := svc.RegenerateSummary(ctx, recID, guestToken, dtos.RegenerateSummaryRequest{
		TemplateCategory: "MOM",
		CustomAngle:      &customAngle,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Version != 3 {
		t.Errorf("expected version 3, got %d", resp.Version)
	}
	if resp.TemplateCategory != "MOM" {
		t.Errorf("expected template MOM, got %s", resp.TemplateCategory)
	}
	if resp.CustomAngle == nil || *resp.CustomAngle != customAngle {
		t.Errorf("expected CustomAngle %s, got %v", customAngle, resp.CustomAngle)
	}
	if resp.MarkdownContent != "# Technical Debrief\nKey decisions made." {
		t.Errorf("unexpected MarkdownContent: %s", resp.MarkdownContent)
	}
	if !resp.IsActive {
		t.Errorf("expected is_active true, got %v", resp.IsActive)
	}
}

func TestService_RegenerateSummary_Success_AuthenticatedOwner(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	userID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mockLLM := llm.NewMock()
	mockLLM.GenerateStructuredFunc = func(ctx context.Context, systemPrompt, userPrompt string, schema map[string]interface{}) (*dtos.StructuredResponse, error) {
		return &dtos.StructuredResponse{
			RawJSON: `{"executive_summary": "High level recap", "action_items": []}`,
		}, nil
	}
	svc.SetLLM(mockLLM)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "status", "detected_language", "output_language", "selected_template"}).
			AddRow(recID, &userID, models.RecordingStatusCompleted, "id", "id", ""))

	mock.ExpectQuery(`SELECT count\(\*\) FROM "summaries" WHERE recording_id = \$1`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	mock.ExpectQuery(`SELECT \* FROM "transcript_segments" WHERE recording_id = \$1 ORDER BY sequence_order ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "speaker_name", "text", "start_time", "end_time"}).
			AddRow(uuid.New(), recID, "Alice", "Pembahasan sistem baru.", 0.0, 15.0))

	mock.ExpectQuery(`SELECT \* FROM "templates" WHERE category_key = \$1 AND is_active = TRUE.*LIMIT \$2`).
		WithArgs("GENERAL", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "category_key", "prompt", "output_schema"}).
			AddRow(uuid.New(), "GENERAL", "General system prompt", "{}"))

	// SaveNewSummaryVersion expectations
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT count\(\*\) FROM "summaries" WHERE recording_id = \$1`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`SELECT COALESCE\(MAX\(version\), 0\) FROM "summaries" WHERE recording_id = \$1`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"coalesce"}).AddRow(1))
	mock.ExpectExec(`UPDATE "summaries" SET "is_active"=\$1,"updated_at"=\$2 WHERE recording_id = \$3`).
		WithArgs(false, sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(`INSERT INTO "summaries"`).
		WithArgs(recID, "GENERAL", nil, 2, true, sqlmock.AnyArg(), "High level recap", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(uuid.New(), time.Now(), time.Now()))
	mock.ExpectCommit()

	resp, err := svc.RegenerateSummary(ctx, recID, "", dtos.RegenerateSummaryRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Version != 2 {
		t.Errorf("expected version 2, got %d", resp.Version)
	}
	if resp.TemplateCategory != "GENERAL" {
		t.Errorf("expected template GENERAL, got %s", resp.TemplateCategory)
	}
	if resp.MarkdownContent != "High level recap" {
		t.Errorf("unexpected MarkdownContent: %s", resp.MarkdownContent)
	}
	if !resp.IsActive {
		t.Errorf("expected is_active true, got %v", resp.IsActive)
	}
}

func TestService_ListSummaryVersions_NotFound(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	_, err := svc.ListSummaryVersions(ctx, recID, "some-token")
	if !errors.Is(err, constants.ErrRecordingNotFound) {
		t.Fatalf("expected ErrRecordingNotFound, got %v", err)
	}
}

func TestService_ListSummaryVersions_Forbidden_Unauthorized(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id"}).
			AddRow(recID, "valid-token-123", nil))

	_, err := svc.ListSummaryVersions(ctx, recID, "wrong-token")
	if !errors.Is(err, constants.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestService_ListSummaryVersions_Forbidden_WrongUser(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	ownerID := uuid.New()
	differentUserID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: differentUserID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id"}).
			AddRow(recID, "valid-token", &ownerID))

	_, err := svc.ListSummaryVersions(ctx, recID, "")
	if !errors.Is(err, constants.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestService_ListSummaryVersions_Success_Guest(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	guestToken := "valid-guest-secret-token"
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id"}).
			AddRow(recID, guestToken, nil))

	sum1ID := uuid.New()
	sum2ID := uuid.New()
	now := time.Now()
	angle := "Focus on action items"

	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 ORDER BY version ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "version", "template_category", "custom_angle", "structured_data", "markdown_content", "is_active", "created_at"}).
			AddRow(sum1ID, recID, 1, "GENERAL", nil, models.JSONMap{"summary": "v1"}, "# Summary v1", false, now.Add(-10*time.Minute)).
			AddRow(sum2ID, recID, 2, "EXECUTIVE", &angle, models.JSONMap{"summary": "v2"}, "# Summary v2", true, now))

	resps, err := svc.ListSummaryVersions(ctx, recID, guestToken)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resps) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(resps))
	}

	// Verify ordering and field values
	if resps[0].Version != 1 || resps[0].IsActive || resps[0].TemplateCategory != "GENERAL" || resps[0].CustomAngle != nil {
		t.Errorf("version 1 response mismatch: %+v", resps[0])
	}
	if resps[1].Version != 2 || !resps[1].IsActive || resps[1].TemplateCategory != "EXECUTIVE" || resps[1].CustomAngle == nil || *resps[1].CustomAngle != angle {
		t.Errorf("version 2 response mismatch: %+v", resps[1])
	}
}

func TestService_ListSummaryVersions_Success_AuthenticatedOwner(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	userID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id"}).
			AddRow(recID, "token", &userID))

	sumID := uuid.New()
	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 ORDER BY version ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "version", "template_category", "structured_data", "markdown_content", "is_active", "created_at"}).
			AddRow(sumID, recID, 1, "GENERAL", models.JSONMap{"summary": "v1"}, "# Summary v1", true, time.Now()))

	resps, err := svc.ListSummaryVersions(ctx, recID, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resps) != 1 {
		t.Fatalf("expected 1 version, got %d", len(resps))
	}
	if resps[0].ID != sumID.String() || resps[0].Version != 1 || !resps[0].IsActive {
		t.Errorf("unexpected version response: %+v", resps[0])
	}
}

func TestService_ListSummaryVersions_EmptyList(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	userID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id"}).
			AddRow(recID, "token", &userID))

	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 ORDER BY version ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "version", "template_category", "structured_data", "markdown_content", "is_active", "created_at"}))

	resps, err := svc.ListSummaryVersions(ctx, recID, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resps == nil {
		t.Fatal("expected non-nil empty slice, got nil")
	}
	if len(resps) != 0 {
		t.Fatalf("expected 0 versions, got %d", len(resps))
	}
}

func TestService_ActivateSummaryVersion_NotFound_Recording(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	_, err := svc.ActivateSummaryVersion(ctx, recID, uuid.New().String(), "token")
	if !errors.Is(err, constants.ErrRecordingNotFound) {
		t.Fatalf("expected ErrRecordingNotFound, got %v", err)
	}
}

func TestService_ActivateSummaryVersion_Forbidden_Unauthorized(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id"}).
			AddRow(recID, "valid-token", nil))

	_, err := svc.ActivateSummaryVersion(ctx, recID, uuid.New().String(), "wrong-token")
	if !errors.Is(err, constants.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestService_ActivateSummaryVersion_Forbidden_WrongUser(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	ownerID := uuid.New()
	otherUserID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: otherUserID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id"}).
			AddRow(recID, "token", &ownerID))

	_, err := svc.ActivateSummaryVersion(ctx, recID, uuid.New().String(), "")
	if !errors.Is(err, constants.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestService_ActivateSummaryVersion_NotFound_Summary(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	guestToken := "guest-token"
	missingSumID := uuid.New()
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id"}).
			AddRow(recID, guestToken, nil))

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND id = \$2.*LIMIT \$3`).
		WithArgs(recID, missingSumID, 1).
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectRollback()

	_, err := svc.ActivateSummaryVersion(ctx, recID, missingSumID.String(), guestToken)
	if !errors.Is(err, constants.ErrSummaryNotFound) {
		t.Fatalf("expected ErrSummaryNotFound, got %v", err)
	}
}

func TestService_ActivateSummaryVersion_Success_GuestWithUUID(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	guestToken := "guest-token"
	targetSumID := uuid.New()
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id"}).
			AddRow(recID, guestToken, nil))

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND id = \$2.*LIMIT \$3`).
		WithArgs(recID, targetSumID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "version", "template_category", "structured_data", "markdown_content", "is_active", "created_at"}).
			AddRow(targetSumID, recID, 2, "GENERAL", models.JSONMap{"summary": "test"}, "# Test", false, time.Now()))
	mock.ExpectExec(`UPDATE "summaries" SET "is_active"=\$1,"updated_at"=\$2 WHERE recording_id = \$3`).
		WithArgs(false, sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 2))
	mock.ExpectExec(`UPDATE "summaries" SET "is_active"=\$1,"updated_at"=\$2 WHERE id = \$3`).
		WithArgs(true, sqlmock.AnyArg(), targetSumID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	resp, err := svc.ActivateSummaryVersion(ctx, recID, targetSumID.String(), guestToken)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ID != targetSumID.String() {
		t.Errorf("expected summary ID %s, got %s", targetSumID.String(), resp.ID)
	}
	if !resp.IsActive {
		t.Errorf("expected is_active true, got %v", resp.IsActive)
	}
	if resp.Version != 2 {
		t.Errorf("expected version 2, got %d", resp.Version)
	}
}

func TestService_ActivateSummaryVersion_Success_AuthenticatedOwnerWithVersionInt(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	userID := uuid.New()
	targetSumID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id"}).
			AddRow(recID, "token", &userID))

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND version = \$2.*LIMIT \$3`).
		WithArgs(recID, 3, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "version", "template_category", "structured_data", "markdown_content", "is_active", "created_at"}).
			AddRow(targetSumID, recID, 3, "EXECUTIVE", models.JSONMap{"summary": "v3"}, "# V3", false, time.Now()))
	mock.ExpectExec(`UPDATE "summaries" SET "is_active"=\$1,"updated_at"=\$2 WHERE recording_id = \$3`).
		WithArgs(false, sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 3))
	mock.ExpectExec(`UPDATE "summaries" SET "is_active"=\$1,"updated_at"=\$2 WHERE id = \$3`).
		WithArgs(true, sqlmock.AnyArg(), targetSumID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	resp, err := svc.ActivateSummaryVersion(ctx, recID, "3", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ID != targetSumID.String() || resp.Version != 3 || !resp.IsActive {
		t.Errorf("unexpected activated response: %+v", resp)
	}
}
