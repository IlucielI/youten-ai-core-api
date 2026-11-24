package repositories

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"code-base-golang/internal/models"
)

func TestRepositories_CreateAIUsageLog(t *testing.T) {
	repo, mock, cleanup := setupMockChatDB(t)
	defer cleanup()

	ctx := context.Background()
	logID := uuid.New()
	userID := uuid.New()
	recID := uuid.New()

	log := &models.AIUsageLog{
		ID:               logID,
		UserID:           &userID,
		RecordingID:      &recID,
		ServiceType:      "llm_summary",
		Provider:         "openai",
		ModelName:        "gpt-4o-mini",
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
		AudioSeconds:     0,
		LatencyMs:        450,
		CreatedAt:        time.Now(),
	}

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "ai_usage_logs"`)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(logID, time.Now()))
	mock.ExpectCommit()

	err := repo.CreateAIUsageLog(ctx, log)
	assert.NoError(t, err)

	// nil safety check
	assert.NoError(t, repo.CreateAIUsageLog(ctx, nil))
}

func TestRepositories_IncrementRecordingTokenUsage(t *testing.T) {
	repo, mock, cleanup := setupMockChatDB(t)
	defer cleanup()

	ctx := context.Background()
	recID := uuid.New()

	// Guard clauses check
	assert.NoError(t, repo.IncrementRecordingTokenUsage(ctx, uuid.Nil, 100))
	assert.NoError(t, repo.IncrementRecordingTokenUsage(ctx, recID, 0))
	assert.NoError(t, repo.IncrementRecordingTokenUsage(ctx, recID, -10))

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings" SET "total_tokens_used"=total_tokens_used \+ \$1`).
		WithArgs(250, sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := repo.IncrementRecordingTokenUsage(ctx, recID, 250)
	assert.NoError(t, err)
}

func TestRepositories_ListAIUsageLogsFiltered(t *testing.T) {
	repo, mock, cleanup := setupMockChatDB(t)
	defer cleanup()

	ctx := context.Background()
	userID := uuid.New()
	now := time.Now()

	filter := AIUsageFilter{
		UserID:      &userID,
		ServiceType: "llm_summary",
		Provider:    "openai",
		ModelName:   "gpt-4o-mini",
		StartDate:   &now,
		EndDate:     &now,
		Limit:       10,
		Offset:      0,
	}

	// 1. Count query
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "ai_usage_logs"`)).
		WithArgs(userID, "llm_summary", "openai", "gpt-4o-mini", now, now).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// 2. Aggregate query
	mock.ExpectQuery(`SELECT COALESCE\(SUM\(prompt_tokens\), 0\) AS total_prompt_tokens.*FROM "ai_usage_logs"`).
		WithArgs(userID, "llm_summary", "openai", "gpt-4o-mini", now, now).
		WillReturnRows(sqlmock.NewRows([]string{"total_prompt_tokens", "total_completion_tokens", "total_tokens", "total_audio_seconds"}).
			AddRow(100, 50, 150, 0.0))

	// 3. Find logs query
	logID := uuid.New()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "ai_usage_logs" WHERE user_id = $1 AND service_type = $2 AND provider = $3 AND model_name = $4 AND created_at >= $5 AND created_at <= $6 ORDER BY created_at DESC LIMIT $7`)).
		WithArgs(userID, "llm_summary", "openai", "gpt-4o-mini", now, now, 10).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "recording_id", "service_type", "provider", "model_name", "total_tokens", "created_at"}).
			AddRow(logID, userID, nil, "llm_summary", "openai", "gpt-4o-mini", 150, now))

	// 4. Preload User
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE "users"."id" = $1 AND "users"."deleted_at" IS NULL`)).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email"}).AddRow(userID, "test@example.com"))

	logs, total, summary, err := repo.ListAIUsageLogsFiltered(ctx, filter)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, int64(150), summary.TotalTokens)
	assert.Equal(t, int64(100), summary.TotalPromptTokens)
	assert.Equal(t, int64(50), summary.TotalCompletionTokens)
	assert.Len(t, logs, 1)
	assert.Equal(t, logID, logs[0].ID)
	assert.NotNil(t, logs[0].User)
	assert.Equal(t, "test@example.com", logs[0].User.Email)
}
