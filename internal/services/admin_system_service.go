package services

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
	"code-base-golang/internal/repositories"
	"code-base-golang/internal/validations"
)

// AdminListReports queries moderation reports with preloaded target and handler metadata.
func (s *Service) AdminListReports(ctx context.Context, q dtos.AdminReportListQuery) (*dtos.AdminReportListResponse, error) {
	q.SetDefaults()

	offset := (q.Page - 1) * q.Limit
	reports, total, err := s.repo.ListReportsByStatus(ctx, q.Status, q.Limit, offset)
	if err != nil {
		return nil, s.wrapError(ctx, err)
	}

	items := make([]dtos.AdminReportListItem, 0, len(reports))
	for _, r := range reports {
		item := dtos.AdminReportListItem{
			ID:             r.ID,
			RecordingID:    r.RecordingID,
			ReporterType:   r.ReporterType,
			ReporterRef:    r.ReporterRef,
			Reason:         r.Reason,
			Status:         r.Status,
			ResolutionNote: r.ResolutionNote,
			HandledBy:      r.HandledBy,
			CreatedAt:      r.CreatedAt,
			UpdatedAt:      r.UpdatedAt,
		}
		if r.Recording != nil {
			item.RecordingTitle = r.Recording.Title
		}
		if r.Handler != nil {
			handlerName := r.Handler.FullName
			if handlerName == "" {
				handlerName = r.Handler.Username
			}
			item.HandlerName = &handlerName
		}
		items = append(items, item)
	}

	totalPages := int(math.Ceil(float64(total) / float64(q.Limit)))
	if totalPages < 1 {
		totalPages = 1
	}

	return &dtos.AdminReportListResponse{
		Items: items,
		Pagination: dtos.PaginationMeta{
			CurrentPage: q.Page,
			PageSize:    q.Limit,
			TotalItems:  total,
			TotalPages:  totalPages,
		},
	}, nil
}

// AdminResolveReport executes administrative resolution on a reported recording ticket.
func (s *Service) AdminResolveReport(ctx context.Context, meta dtos.AdminActionMeta, reportID uuid.UUID, req dtos.ResolveReportRequest) (*dtos.ResolveReportResponse, error) {
	if err := validations.ValidateResolveReportRequest(&req); err != nil {
		return nil, constants.ErrBadRequest.WithMessage(err.Error())
	}

	report, err := s.repo.FindReportByID(ctx, reportID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrNotFound.WithMessage("report ticket not found")
		}
		return nil, s.wrapError(ctx, err)
	}

	var status string
	switch req.Action {
	case "DISMISS":
		status = "dismissed"
	case "SUSPEND_RECORDING":
		status = "resolved"
		if report.RecordingID != uuid.Nil {
			if err := s.repo.UpdateRecordingShareSettings(ctx, report.RecordingID, false, nil); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, s.wrapError(ctx, err)
			}
		}
	case "BAN_USER":
		status = "resolved"
		if report.Recording != nil && report.Recording.UserID != nil {
			if err := s.repo.UpdateUserStatus(ctx, *report.Recording.UserID, models.UserStatusSuspended); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, s.wrapError(ctx, err)
			}
		}
		if report.RecordingID != uuid.Nil {
			if err := s.repo.UpdateRecordingShareSettings(ctx, report.RecordingID, false, nil); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, s.wrapError(ctx, err)
			}
		}
	}

	resolvedAt := time.Now()
	var notePtr *string
	if req.ResolutionNote != "" {
		notePtr = &req.ResolutionNote
	}

	if err := s.repo.UpdateReportResolution(ctx, reportID, status, notePtr, meta.AdminID); err != nil {
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
		"report_id":       reportID.String(),
		"recording_id":    report.RecordingID.String(),
		"action":          req.Action,
		"resolution_note": req.ResolutionNote,
		"status":          status,
	}

	entityIDStr := reportID.String()
	auditLog := &models.AdminAuditLog{
		AdminID:   &adminIDVal,
		Action:    "report.resolve",
		Entity:    "report",
		EntityID:  &entityIDStr,
		Payload:   payload,
		IPAddress: ip,
		UserAgent: ua,
	}
	if err := s.repo.CreateAdminAuditLog(ctx, auditLog); err != nil {
		return nil, s.wrapError(ctx, err)
	}

	return &dtos.ResolveReportResponse{
		ReportID:       reportID,
		Status:         status,
		Action:         req.Action,
		HandledBy:      meta.AdminID,
		ResolutionNote: req.ResolutionNote,
		ResolvedAt:     resolvedAt,
	}, nil
}

// AdminListAuditLogs retrieves a paginated and filtered audit trail of administrative actions.
func (s *Service) AdminListAuditLogs(ctx context.Context, q dtos.AdminAuditLogListQuery) (*dtos.AdminAuditLogListResponse, error) {
	q.SetDefaults()

	offset := (q.Page - 1) * q.Limit
	filter := repositories.AdminAuditLogFilter{
		Action:  q.Action,
		Entity:  q.Entity,
		AdminID: q.AdminID,
		Limit:   q.Limit,
		Offset:  offset,
	}

	logs, total, err := s.repo.ListAdminAuditLogsFiltered(ctx, filter)
	if err != nil {
		return nil, s.wrapError(ctx, err)
	}

	items := make([]dtos.AdminAuditLogListItem, 0, len(logs))
	for _, l := range logs {
		item := dtos.AdminAuditLogListItem{
			ID:        l.ID,
			AdminID:   l.AdminID,
			Action:    l.Action,
			Entity:    l.Entity,
			EntityID:  l.EntityID,
			Payload:   map[string]interface{}(l.Payload),
			IPAddress: l.IPAddress,
			UserAgent: l.UserAgent,
			CreatedAt: l.CreatedAt,
		}
		if l.Admin != nil {
			u := l.Admin.Username
			item.AdminUsername = &u
			fn := l.Admin.FullName
			item.AdminFullName = &fn
		}
		items = append(items, item)
	}

	totalPages := int(math.Ceil(float64(total) / float64(q.Limit)))
	if totalPages < 1 {
		totalPages = 1
	}

	return &dtos.AdminAuditLogListResponse{
		Items: items,
		Pagination: dtos.PaginationMeta{
			CurrentPage: q.Page,
			PageSize:    q.Limit,
			TotalItems:  total,
			TotalPages:  totalPages,
		},
	}, nil
}

// AdminGetOverviewStats returns system-wide metrics across users, recordings, and storage.
func (s *Service) AdminGetOverviewStats(ctx context.Context) (*dtos.SystemOverviewStatsResponse, error) {
	stats, err := s.repo.GetSystemOverviewStats(ctx)
	if err != nil {
		return nil, s.wrapError(ctx, err)
	}

	return &dtos.SystemOverviewStatsResponse{
		TotalUsers:           stats.TotalUsers,
		ActiveUsers:          stats.ActiveUsers,
		TotalRecordings:      stats.TotalRecordings,
		CompletedRecordings:  stats.CompletedRecordings,
		FailedRecordings:     stats.FailedRecordings,
		TotalStorageBytes:    stats.TotalStorageBytes,
		TotalDurationSeconds: stats.TotalDurationSeconds,
	}, nil
}

// AdminGetCostOversight calculates estimates for transcription (STT) and intelligence (LLM) expenses.
func (s *Service) AdminGetCostOversight(ctx context.Context) (*dtos.CostOversightResponse, error) {
	stats, err := s.repo.GetSystemOverviewStats(ctx)
	if err != nil {
		return nil, s.wrapError(ctx, err)
	}

	totalMinutes := stats.TotalDurationSeconds / 60.0

	// Whisper/Deepgram STT cost benchmark: ~$0.006 per audio minute
	sttRatePerMinute := 0.006
	sttCost := math.Round(totalMinutes*sttRatePerMinute*1000) / 1000

	// LLM benchmark estimate: ~150 words/min = ~200 tokens/min input + ~100 tokens/min output = ~300 tokens/min
	// GPT-4o-mini / Gemini Flash blended cost: ~$0.0003 per 1K tokens
	estimatedTokens := int64(math.Round(totalMinutes * 300))
	llmRatePer1kTokens := 0.0003
	llmCost := math.Round((float64(estimatedTokens)/1000.0)*llmRatePer1kTokens*1000) / 1000

	totalCost := math.Round((sttCost+llmCost)*1000) / 1000

	return &dtos.CostOversightResponse{
		TotalAudioMinutes:     math.Round(totalMinutes*100) / 100,
		EstimatedSTTCostUSD:   sttCost,
		TotalLLMTokens:        estimatedTokens,
		EstimatedLLMCostUSD:   llmCost,
		TotalEstimatedCostUSD: totalCost,
		Currency:              "USD",
	}, nil
}
