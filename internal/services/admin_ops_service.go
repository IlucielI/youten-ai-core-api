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
	"code-base-golang/internal/validations"
)

// AdminGetDLQMessages retrieves failed and stuck pipeline items for DLQ monitoring.
func (s *Service) AdminGetDLQMessages(ctx context.Context, meta dtos.AdminActionMeta, page, limit int) (*dtos.DLQMessagesResponse, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
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
	query.SetDefaults()
	offset := (query.Page - 1) * query.Limit

	recs, total, err := s.repo.ListAllRecordingsForAdmin(ctx, query.Status, query.Search, query.Limit, offset)
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

	totalPages := 0
	if query.Limit > 0 {
		totalPages = int((total + int64(query.Limit) - 1) / int64(query.Limit))
	}

	return &dtos.AdminJobListResponse{
		Items:      items,
		Total:      total,
		Page:       query.Page,
		Limit:      query.Limit,
		TotalPages: totalPages,
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
		return "recording.dlq", constants.TopicRecordingFailed
	default:
		return "pipeline.unknown", "pipeline.unknown"
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

	// Persist admin audit log
	adminIDVal := meta.AdminID
	entityIDStr := req.RecordingID.String()
	var ip *string
	var ua *string
	if meta.IPAddress != "" {
		ip = &meta.IPAddress
	}
	if meta.UserAgent != "" {
		ua = &meta.UserAgent
	}

	auditLog := &models.AdminAuditLog{
		AdminID:  &adminIDVal,
		Action:   "pipeline.dlq_retry",
		Entity:   "recording",
		EntityID: &entityIDStr,
		Payload: models.JSONMap{
			"previous_status": previousStatus,
			"requested_stage": previousStage,
			"resumed_status":  status,
			"resumed_stage":   stage,
		},
		IPAddress: ip,
		UserAgent: ua,
	}
	if err := s.repo.CreateAdminAuditLog(ctx, auditLog); err != nil {
		return nil, s.wrapError(ctx, err)
	}

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

	// Persist admin audit log
	adminIDVal := meta.AdminID
	var ip *string
	var ua *string
	if meta.IPAddress != "" {
		ip = &meta.IPAddress
	}
	if meta.UserAgent != "" {
		ua = &meta.UserAgent
	}

	payload := models.JSONMap{
		"maintenance_mode":     cfg.MaintenanceMode,
		"allow_guest_uploads":  cfg.AllowGuestUploads,
		"bot_waitlist_enabled": cfg.BotWaitlistEnabled,
	}

	auditLog := &models.AdminAuditLog{
		AdminID:   &adminIDVal,
		Action:    "config.update_flags",
		Entity:    "system_config",
		Payload:   payload,
		IPAddress: ip,
		UserAgent: ua,
	}
	if err := s.repo.CreateAdminAuditLog(ctx, auditLog); err != nil {
		return nil, s.wrapError(ctx, err)
	}

	return &dtos.SystemConfigResponse{
		MaintenanceMode:    cfg.MaintenanceMode,
		AllowGuestUploads:  cfg.AllowGuestUploads,
		BotWaitlistEnabled: cfg.BotWaitlistEnabled,
	}, nil
}
