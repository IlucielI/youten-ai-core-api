package repositories

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
)

// AIUsageFilter specifies filtering and pagination parameters for AI usage audit logs.
type AIUsageFilter struct {
	UserID      *uuid.UUID
	WorkspaceID *uuid.UUID
	RecordingID *uuid.UUID
	ServiceType string
	Provider    string
	ModelName   string
	StartDate   *time.Time
	EndDate     *time.Time
	Limit       int
	Offset      int
}

// scope applies active filter conditions to the GORM query.
func (f AIUsageFilter) scope(db *gorm.DB) *gorm.DB {
	if f.UserID != nil && *f.UserID != uuid.Nil {
		db = db.Where("user_id = ?", *f.UserID)
	}
	if f.WorkspaceID != nil && *f.WorkspaceID != uuid.Nil {
		db = db.Where("workspace_id = ?", *f.WorkspaceID)
	}
	if f.RecordingID != nil && *f.RecordingID != uuid.Nil {
		db = db.Where("recording_id = ?", *f.RecordingID)
	}
	if f.ServiceType != "" {
		db = db.Where("service_type = ?", f.ServiceType)
	}
	if f.Provider != "" {
		db = db.Where("provider = ?", f.Provider)
	}
	if f.ModelName != "" {
		db = db.Where("model_name = ?", f.ModelName)
	}
	if f.StartDate != nil {
		db = db.Where("created_at >= ?", *f.StartDate)
	}
	if f.EndDate != nil {
		db = db.Where("created_at <= ?", *f.EndDate)
	}
	return db
}

// CreateAIUsageLog records an immutable audit log entry for an AI operation.
func (r *Repositories) CreateAIUsageLog(ctx context.Context, log *models.AIUsageLog) error {
	if log == nil {
		return nil
	}
	return r.db.WithContext(ctx).Create(log).Error
}

// IncrementRecordingTokenUsage increments total_tokens_used in recordings atomically.
func (r *Repositories) IncrementRecordingTokenUsage(ctx context.Context, recordingID uuid.UUID, tokens int) error {
	if recordingID == uuid.Nil || tokens <= 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Model(&models.Recording{}).
		Where("id = ?", recordingID).
		Update("total_tokens_used", gorm.Expr("total_tokens_used + ?", tokens)).
		Error
}

type aiUsageAggregate struct {
	TotalPromptTokens     int64   `gorm:"column:total_prompt_tokens"`
	TotalCompletionTokens int64   `gorm:"column:total_completion_tokens"`
	TotalTokens           int64   `gorm:"column:total_tokens"`
	TotalAudioSeconds     float64 `gorm:"column:total_audio_seconds"`
}

// ListAIUsageLogsFiltered fetches paginated AI usage logs and aggregates summary totals across the filtered dataset.
func (r *Repositories) ListAIUsageLogsFiltered(ctx context.Context, f AIUsageFilter) ([]models.AIUsageLog, int64, dtos.AIUsageSummaryDTO, error) {
	var logs []models.AIUsageLog
	var total int64
	var summary dtos.AIUsageSummaryDTO

	baseQuery := r.db.WithContext(ctx).Model(&models.AIUsageLog{}).Scopes(f.scope)

	if err := baseQuery.Count(&total).Error; err != nil {
		return nil, 0, summary, err
	}

	var agg aiUsageAggregate
	err := r.db.WithContext(ctx).
		Model(&models.AIUsageLog{}).
		Scopes(f.scope).
		Select(
			"COALESCE(SUM(prompt_tokens), 0) AS total_prompt_tokens",
			"COALESCE(SUM(completion_tokens), 0) AS total_completion_tokens",
			"COALESCE(SUM(total_tokens), 0) AS total_tokens",
			"COALESCE(SUM(audio_seconds), 0) AS total_audio_seconds",
		).
		Scan(&agg).Error
	if err != nil {
		return nil, 0, summary, err
	}

	summary = dtos.AIUsageSummaryDTO{
		TotalPromptTokens:     agg.TotalPromptTokens,
		TotalCompletionTokens: agg.TotalCompletionTokens,
		TotalTokens:           agg.TotalTokens,
		TotalAudioSeconds:     agg.TotalAudioSeconds,
	}

	if f.Limit <= 0 {
		f.Limit = 20
	}

	err = r.db.WithContext(ctx).
		Model(&models.AIUsageLog{}).
		Scopes(f.scope).
		Preload("User").
		Preload("Recording").
		Preload("Workspace").
		Order("created_at DESC").
		Limit(f.Limit).
		Offset(f.Offset).
		Find(&logs).Error
	if err != nil {
		return nil, 0, summary, err
	}

	return logs, total, summary, nil
}
