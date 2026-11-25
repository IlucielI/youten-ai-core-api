package services

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	gormPostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"

	"code-base-golang/internal/config"
	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/repositories"
)

func setupTestAIUsageService(t *testing.T) (*Service, sqlmock.Sqlmock, func()) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}

	gormDB, err := gorm.Open(gormPostgres.New(gormPostgres.Config{
		Conn: sqlDB,
	}), &gorm.Config{})
	if err != nil {
		sqlDB.Close()
		t.Fatalf("failed to open gorm db: %v", err)
	}

	repo := repositories.New(gormDB)
	cfg := config.Config{
		LLMProvider: "omniroute",
		LLMModel:    "gpt-4o-mini",
	}

	svc := New(cfg, repo, nil)

	cleanup := func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("there were unfulfilled expectations: %s", err)
		}
		sqlDB.Close()
	}

	return svc, mock, cleanup
}

func TestService_RecordAIUsageSync(t *testing.T) {
	svc, mock, cleanup := setupTestAIUsageService(t)
	defer cleanup()

	ctx := context.Background()
	recID := uuid.New()
	userID := uuid.New()

	input := dtos.AIUsageRecordInput{
		UserID:           &userID,
		RecordingID:      &recID,
		ServiceType:      constants.AIServiceTypeLLMSummary.String(),
		PromptTokens:     120,
		CompletionTokens: 80,
		TotalTokens:      0, // should be auto-summed to 200
		AudioSeconds:     0,
		LatencyMs:        600,
	}

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "ai_usage_logs"`)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(uuid.New(), time.Now()))
	mock.ExpectCommit()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings" SET "total_tokens_used"=total_tokens_used \+ \$1`).
		WithArgs(200, sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := svc.RecordAIUsageSync(ctx, input)
	assert.NoError(t, err)
}

func TestService_RecordAIUsageSync_EnrichesFromRecording(t *testing.T) {
	svc, mock, cleanup := setupTestAIUsageService(t)
	defer cleanup()

	ctx := context.Background()
	recID := uuid.New()
	userID := uuid.New()
	wsID := uuid.New()

	input := dtos.AIUsageRecordInput{
		RecordingID:      &recID,
		ServiceType:      constants.AIServiceTypeLLMSummary.String(),
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
	}

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "recordings" WHERE id = $1 AND "recordings"."deleted_at" IS NULL ORDER BY "recordings"."id" LIMIT $2`)).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "workspace_id"}).AddRow(recID, userID, wsID))

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "ai_usage_logs" ("user_id","workspace_id","recording_id","service_type","provider","model_name","prompt_tokens","completion_tokens","total_tokens","audio_seconds","latency_ms","metadata","created_at")`)).
		WithArgs(
			userID,
			wsID,
			recID,
			constants.AIServiceTypeLLMSummary.String(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			100,
			50,
			150,
			0.0,
			0,
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
		).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(uuid.New(), time.Now()))
	mock.ExpectCommit()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings" SET "total_tokens_used"=total_tokens_used \+ \$1`).
		WithArgs(150, sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := svc.RecordAIUsageSync(ctx, input)
	assert.NoError(t, err)
}

func TestService_GetAdminAIUsageLogs(t *testing.T) {
	svc, mock, cleanup := setupTestAIUsageService(t)
	defer cleanup()

	ctx := context.Background()
	userID := uuid.New()
	recID := uuid.New()
	now := time.Now()
	filter := dtos.AdminAIUsageFilter{
		Page:        1,
		Limit:       10,
		ServiceType: "llm_summary",
	}

	// 1. Count
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "ai_usage_logs"`)).
		WithArgs("llm_summary").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// 2. Aggregate
	mock.ExpectQuery(`SELECT COALESCE\(SUM\(prompt_tokens\), 0\) AS total_prompt_tokens.*FROM "ai_usage_logs"`).
		WithArgs("llm_summary").
		WillReturnRows(sqlmock.NewRows([]string{"total_prompt_tokens", "total_completion_tokens", "total_tokens", "total_audio_seconds"}).
			AddRow(120, 80, 200, 0.0))

	// 3. Find logs
	logID := uuid.New()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "ai_usage_logs" WHERE service_type = $1 ORDER BY created_at DESC LIMIT $2`)).
		WithArgs("llm_summary", 10).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "recording_id", "service_type", "provider", "model_name", "total_tokens", "created_at"}).
			AddRow(logID, userID, recID, "llm_summary", "omniroute", "gpt-4o-mini", 200, now))

	// 4. Preload Recording
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "recordings" WHERE "recordings"."id" = $1 AND "recordings"."deleted_at" IS NULL`)).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "title"}).AddRow(recID, "Team Standup"))

	// 5. Preload User
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE "users"."id" = $1 AND "users"."deleted_at" IS NULL`)).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email"}).AddRow(userID, "user@example.com"))

	resp, err := svc.GetAdminAIUsageLogs(ctx, filter)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, int64(1), resp.Pagination.TotalItems)
	assert.Equal(t, int64(200), resp.Summary.TotalTokens)
	assert.Len(t, resp.Items, 1)
	assert.Equal(t, "user@example.com", *resp.Items[0].UserEmail)
	assert.Equal(t, "Team Standup", *resp.Items[0].RecordingTitle)
}
