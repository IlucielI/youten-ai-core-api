package services

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
	"code-base-golang/internal/payload"
	"code-base-golang/internal/repositories"
	"code-base-golang/internal/validations"
)

// AdminListTemplates retrieves paginated prompt templates for administrative management.
func (s *Service) AdminListTemplates(ctx context.Context, q dtos.AdminTemplateListQuery) (*dtos.AdminTemplateListResponse, error) {
	q.SetDefaults()

	offset := (q.Page - 1) * q.Limit
	repoQuery := repositories.ListTemplatesQuery{
		Search: q.Search,
		Offset: offset,
		Limit:  q.Limit,
	}

	templates, total, err := s.repo.ListTemplatesPaginated(ctx, repoQuery)
	if err != nil {
		return nil, s.wrapError(ctx, err)
	}

	items := make([]dtos.AdminTemplateItem, 0, len(templates))
	for _, t := range templates {
		items = append(items, dtos.AdminTemplateItem{
			ID:           t.ID,
			CategoryKey:  t.CategoryKey,
			Name:         t.Name,
			Description:  t.Description,
			Prompt:       t.Prompt,
			OutputSchema: t.OutputSchema,
			Version:      t.Version,
			IsActive:     t.IsActive,
			CreatedAt:    t.CreatedAt,
			UpdatedAt:    t.UpdatedAt,
		})
	}

	totalPages := int(math.Ceil(float64(total) / float64(q.Limit)))
	if totalPages == 0 && total == 0 {
		totalPages = 0
	} else if totalPages == 0 {
		totalPages = 1
	}

	return &dtos.AdminTemplateListResponse{
		Items: items,
		Pagination: dtos.PaginationMeta{
			CurrentPage: q.Page,
			PageSize:    q.Limit,
			TotalItems:  total,
			TotalPages:  totalPages,
		},
	}, nil
}

// AdminCreateTemplate creates a new prompt template and writes an administrative audit log.
func (s *Service) AdminCreateTemplate(ctx context.Context, meta dtos.AdminActionMeta, req dtos.AdminCreateTemplateRequest) (*dtos.AdminTemplateItem, error) {
	if err := validations.ValidateAdminCreateTemplateRequest(&req); err != nil {
		return nil, constants.ErrBadRequest.WithMessage(err.Error())
	}

	existing, err := s.repo.FindAnyTemplateByCategoryKey(ctx, req.CategoryKey)
	if err == nil && existing != nil {
		return nil, constants.ErrConflict.WithMessage("template with this category key already exists")
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	tmpl := &models.Template{
		CategoryKey:  req.CategoryKey,
		Name:         req.Name,
		Description:  req.Description,
		Prompt:       req.Prompt,
		OutputSchema: models.JSONMap(req.OutputSchema),
		Version:      1,
		IsActive:     isActive,
	}

	if err := s.repo.CreateTemplate(ctx, tmpl); err != nil {
		return nil, s.wrapError(ctx, err)
	}

	adminIDVal := meta.AdminID
	tmplIDStr := tmpl.ID.String()
	var ip *string
	var ua *string
	if meta.IPAddress != "" {
		ip = &meta.IPAddress
	}
	if meta.UserAgent != "" {
		ua = &meta.UserAgent
	}

	auditLog := payload.AdminAuditPayload{
		AdminID:  &adminIDVal,
		Action:   constants.AuditActionTemplateCreate,
		Entity:   constants.AuditEntityTemplate,
		EntityID: &tmplIDStr,
		Payload: map[string]any{
			"category_key": tmpl.CategoryKey,
			"name":         tmpl.Name,
			"version":      tmpl.Version,
		},
		IPAddress: ip,
		UserAgent: ua,
	}
	s.emitAdminAuditLog(ctx, auditLog)

	return &dtos.AdminTemplateItem{
		ID:           tmpl.ID,
		CategoryKey:  tmpl.CategoryKey,
		Name:         tmpl.Name,
		Description:  tmpl.Description,
		Prompt:       tmpl.Prompt,
		OutputSchema: map[string]interface{}(tmpl.OutputSchema),
		Version:      tmpl.Version,
		IsActive:     tmpl.IsActive,
		CreatedAt:    tmpl.CreatedAt,
		UpdatedAt:    tmpl.UpdatedAt,
	}, nil
}

// AdminUpdateTemplate modifies an existing prompt template and increments version when prompt or schema changes.
func (s *Service) AdminUpdateTemplate(ctx context.Context, meta dtos.AdminActionMeta, templateID uuid.UUID, req dtos.AdminUpdateTemplateRequest) (*dtos.AdminTemplateItem, error) {
	if err := validations.ValidateAdminUpdateTemplateRequest(&req); err != nil {
		return nil, constants.ErrBadRequest.WithMessage(err.Error())
	}

	tmpl, err := s.repo.FindTemplateByID(ctx, templateID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrNotFound.WithMessage("template not found")
		}
		return nil, s.wrapError(ctx, err)
	}

	promptChanged := tmpl.Prompt != req.Prompt
	schemaChanged := !reflect.DeepEqual(map[string]interface{}(tmpl.OutputSchema), req.OutputSchema)

	if promptChanged || schemaChanged {
		tmpl.Version = tmpl.Version + 1
	}

	tmpl.Name = req.Name
	tmpl.Description = req.Description
	tmpl.Prompt = req.Prompt
	tmpl.OutputSchema = models.JSONMap(req.OutputSchema)
	if req.IsActive != nil {
		tmpl.IsActive = *req.IsActive
	}

	if err := s.repo.UpdateTemplate(ctx, tmpl); err != nil {
		return nil, s.wrapError(ctx, err)
	}

	adminIDVal := meta.AdminID
	tmplIDStr := tmpl.ID.String()
	var ip *string
	var ua *string
	if meta.IPAddress != "" {
		ip = &meta.IPAddress
	}
	if meta.UserAgent != "" {
		ua = &meta.UserAgent
	}

	auditLog := payload.AdminAuditPayload{
		AdminID:  &adminIDVal,
		Action:   constants.AuditActionTemplateUpdate,
		Entity:   constants.AuditEntityTemplate,
		EntityID: &tmplIDStr,
		Payload: map[string]any{
			"category_key":   tmpl.CategoryKey,
			"name":           tmpl.Name,
			"version":        tmpl.Version,
			"prompt_changed": promptChanged,
			"schema_changed": schemaChanged,
		},
		IPAddress: ip,
		UserAgent: ua,
	}
	s.emitAdminAuditLog(ctx, auditLog)

	return &dtos.AdminTemplateItem{
		ID:           tmpl.ID,
		CategoryKey:  tmpl.CategoryKey,
		Name:         tmpl.Name,
		Description:  tmpl.Description,
		Prompt:       tmpl.Prompt,
		OutputSchema: map[string]interface{}(tmpl.OutputSchema),
		Version:      tmpl.Version,
		IsActive:     tmpl.IsActive,
		CreatedAt:    tmpl.CreatedAt,
		UpdatedAt:    tmpl.UpdatedAt,
	}, nil
}

// AdminTestTemplate executes a sandbox dry run of a prompt and schema against a sample transcript.
func (s *Service) AdminTestTemplate(ctx context.Context, meta dtos.AdminActionMeta, req dtos.AdminTestTemplateRequest) (*dtos.AdminTestTemplateResponse, error) {
	if err := validations.ValidateAdminTestTemplateRequest(&req); err != nil {
		return nil, constants.ErrBadRequest.WithMessage(err.Error())
	}

	if s.llm == nil {
		return nil, constants.ErrInternalServerError.WithMessage("llm provider is not configured")
	}

	startTime := time.Now()
	structuredRes, err := s.llm.GenerateStructured(ctx, req.Prompt, req.SampleTranscript, req.OutputSchema)
	executionTimeMs := time.Since(startTime).Milliseconds()
	if err != nil {
		return nil, s.wrapError(ctx, err)
	}

	var parsedJSON interface{}
	if structuredRes.RawJSON != "" {
		_ = json.Unmarshal([]byte(structuredRes.RawJSON), &parsedJSON)
	}

	return &dtos.AdminTestTemplateResponse{
		RawOutput:       structuredRes.RawJSON,
		ParsedJSON:      parsedJSON,
		ExecutionTimeMs: executionTimeMs,
		TokensUsed:      structuredRes.Usage.TotalTokens,
	}, nil
}
