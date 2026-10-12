package services

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
	"code-base-golang/internal/payload"
	"code-base-golang/internal/validations"
)

// AdminGetDLQMessages retrieves failed and stuck pipeline items for DLQ monitoring.
func (s *Service) AdminGetDLQMessages(ctx context.Context, meta dtos.AdminActionMeta, page, limit int) (*dtos.DLQMessagesResponse, error) {
	offset := (page - 1) * limit

	// Recordings stuck in intermediate states for > 15 minutes are considered stuck
	stuckThreshold := 15 * time.Minute

	recs, total, failedCount, stuckCount, err := s.repo.ListDLQRecordings(ctx, stuckThreshold, limit, offset)
	if err != nil {
		return nil, s.wrapError(ctx, err)
	}

	items := make([]dtos.DLQMessageItem, 0, len(recs))
	for _, rec := range recs {
		queueName, routingKey := resolveDLQQueueAndTopic(rec.Status, rec.SourceType)

		item := dtos.DLQMessageItem{
			RecordingID: rec.ID,
			Queue:       queueName,
			RoutingKey:  routingKey,
			Stage:       rec.Status,
			Status:      rec.Status,
			RetryCount:  0,
			FailedAt:    rec.UpdatedAt,
			Title:       rec.Title,
		}

		if rec.ErrorCode != nil {
			item.ErrorCode = *rec.ErrorCode
		}
		if rec.ErrorMessage != nil {
			item.ErrorMessage = *rec.ErrorMessage
		}
		if rec.User != nil {
			item.UserName = rec.User.FullName
			item.UserEmail = rec.User.Email
		}

		items = append(items, item)
	}

	return &dtos.DLQMessagesResponse{
		Total:       total,
		FailedCount: failedCount,
		StuckCount:  stuckCount,
		Page:        page,
		Limit:       limit,
		Items:       items,
	}, nil
}

// AdminListJobs retrieves paginated generation and pipeline recordings across all users and statuses.
func (s *Service) AdminListJobs(ctx context.Context, meta dtos.AdminActionMeta, query dtos.AdminJobListQuery) (*dtos.AdminJobListResponse, error) {
	recs, total, err := s.repo.ListAllRecordingsForAdmin(ctx, query.Status, query.Search, query.Limit, query.Offset())
	if err != nil {
		return nil, s.wrapError(ctx, err)
	}

	items := make([]dtos.AdminJobItem, 0, len(recs))
	for _, rec := range recs {
		item := dtos.AdminJobItem{
			ID:               rec.ID,
			Title:            rec.Title,
			OriginalFilename: rec.OriginalFilename,
			FileSizeBytes:    rec.FileSizeBytes,
			DurationSeconds:  rec.DurationSeconds,
			SourceType:       rec.SourceType,
			Status:           rec.Status,
			SelectedTemplate: rec.SelectedTemplate,
			DetectedLanguage: rec.DetectedLanguage,
			OutputLanguage:   rec.OutputLanguage,
			ErrorMessage:     rec.ErrorMessage,
			ErrorCode:        rec.ErrorCode,
			IsGuest:          rec.IsGuest,
			CreatedAt:        rec.CreatedAt,
			UpdatedAt:        rec.UpdatedAt,
		}
		if rec.User != nil {
			item.UserName = &rec.User.FullName
			item.UserEmail = &rec.User.Email
		}
		items = append(items, item)
	}

	return &dtos.AdminJobListResponse{
		Items:      items,
		Total:      total,
		Page:       query.Page,
		Limit:      query.Limit,
		TotalPages: dtos.CalculateTotalPages(total, query.Limit),
	}, nil
}

func resolveDLQQueueAndTopic(status, sourceType string) (string, string) {
	switch status {
	case models.RecordingStatusQueued, models.RecordingStatusExtracting:
		if sourceType == constants.RecordingSourceTypeLink {
			return constants.TopicRecordingImport, constants.TopicRecordingImport
		}
		return constants.TopicRecordingExtract, constants.TopicRecordingExtract
	case models.RecordingStatusTranscribing:
		return constants.TopicRecordingTranscribe, constants.TopicRecordingTranscribe
	case models.RecordingStatusSummarizing:
		return constants.TopicRecordingSummarize, constants.TopicRecordingSummarize
	case models.RecordingStatusIndexing:
		return constants.TopicRecordingIndex, constants.TopicRecordingIndex
	case models.RecordingStatusFailed:
		return constants.QueueRecordingDLQ, constants.TopicRecordingFailed
	default:
		return constants.QueuePipelineUnknown, constants.TopicPipelineUnknown
	}
}

// AdminRetryDLQJob reprocesses a dead-lettered or stuck pipeline job, recovers state, and logs the administrative action.
func (s *Service) AdminRetryDLQJob(ctx context.Context, meta dtos.AdminActionMeta, req dtos.DLQRetryRequest) (*dtos.DLQRetryResponse, error) {
	if err := validations.ValidateDLQRetryRequest(&req); err != nil {
		return nil, constants.ErrBadRequest.WithMessage(err.Error())
	}

	rec, err := s.repo.FindRecordingByID(ctx, req.RecordingID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrRecordingNotFound
		}
		return nil, s.wrapError(ctx, err)
	}

	if strings.ToUpper(strings.TrimSpace(rec.Status)) == models.RecordingStatusCompleted {
		return nil, constants.ErrRecordingAlreadyCompleted
	}

	previousStatus := rec.Status
	previousStage := req.Stage

	// If job was stuck in non-terminal processing state, reset status to FAILED so resume can proceed
	if rec.Status != models.RecordingStatusFailed {
		rec.Status = models.RecordingStatusFailed
	}

	// Execute smart pipeline resume
	p, err := s.ResumeRecordingPipeline(ctx, rec)
	if err != nil {
		return nil, s.wrapError(ctx, err)
	}

	status := p.Status
	if status == "" {
		status = models.RecordingStatusQueued
	}
	stage := p.Stage
	if stage == "" {
		stage = models.RecordingStatusExtracting
	}

	entityIDStr := req.RecordingID.String()
	auditLog := payload.AdminAuditPayload{
		AdminID:  meta.AdminIDPtr(),
		Action:   constants.AuditActionPipelineDLQRetry,
		Entity:   constants.AuditEntityRecording,
		EntityID: &entityIDStr,
		Payload: map[string]any{
			"previous_status": previousStatus,
			"requested_stage": previousStage,
			"resumed_status":  status,
			"resumed_stage":   stage,
		},
		IPAddress: meta.IPPtr(),
		UserAgent: meta.UAPtr(),
	}
	s.emitAdminAuditLog(ctx, auditLog)

	return &dtos.DLQRetryResponse{
		RecordingID: rec.ID,
		Status:      status,
		Stage:       stage,
		Message:     "pipeline job retry initiated successfully",
		RetriedAt:   time.Now().UTC(),
	}, nil
}

// IsMaintenanceMode checks if maintenance mode is enabled in Redis/in-memory config.
func (s *Service) IsMaintenanceMode(ctx context.Context) bool {
	if s == nil || s.repo == nil {
		return false
	}
	return s.repo.IsMaintenanceMode(ctx)
}

// AdminGetSystemConfig retrieves current system flags and maintenance status.
func (s *Service) AdminGetSystemConfig(ctx context.Context, meta dtos.AdminActionMeta) (*dtos.SystemConfigResponse, error) {
	cfg, err := s.repo.GetSystemConfig(ctx)
	if err != nil {
		return nil, s.wrapError(ctx, err)
	}

	return &dtos.SystemConfigResponse{
		MaintenanceMode:    cfg.MaintenanceMode,
		AllowGuestUploads:  cfg.AllowGuestUploads,
		BotWaitlistEnabled: cfg.BotWaitlistEnabled,
	}, nil
}

// AdminUpdateSystemConfig updates system flags and persists an audit log.
func (s *Service) AdminUpdateSystemConfig(ctx context.Context, meta dtos.AdminActionMeta, req dtos.UpdateSystemConfigRequest) (*dtos.SystemConfigResponse, error) {
	cfg, err := s.repo.UpdateSystemConfig(ctx, req.MaintenanceMode, req.AllowGuestUploads, req.BotWaitlistEnabled)
	if err != nil {
		return nil, s.wrapError(ctx, err)
	}

	cfgPayload := map[string]any{
		"maintenance_mode":     cfg.MaintenanceMode,
		"allow_guest_uploads":  cfg.AllowGuestUploads,
		"bot_waitlist_enabled": cfg.BotWaitlistEnabled,
	}

	auditLog := payload.AdminAuditPayload{
		AdminID:   meta.AdminIDPtr(),
		Action:    constants.AuditActionConfigUpdateFlags,
		Entity:    constants.AuditEntitySystemConfig,
		Payload:   cfgPayload,
		IPAddress: meta.IPPtr(),
		UserAgent: meta.UAPtr(),
	}
	s.emitAdminAuditLog(ctx, auditLog)

	return &dtos.SystemConfigResponse{
		MaintenanceMode:    cfg.MaintenanceMode,
		AllowGuestUploads:  cfg.AllowGuestUploads,
		BotWaitlistEnabled: cfg.BotWaitlistEnabled,
	}, nil
}
