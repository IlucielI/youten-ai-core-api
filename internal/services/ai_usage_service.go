package services

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
	"code-base-golang/internal/repositories"
)

// RecordAIUsageAsync dispatches AI execution audit logging in a non-blocking background goroutine.
// Panics within the goroutine are safely recovered to prevent application interruption.
func (s *Service) RecordAIUsageAsync(input dtos.AIUsageRecordInput) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[AI USAGE PANIC RECOVERED] failed to record AI usage: %v", r)
			}
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := s.RecordAIUsageSync(ctx, input); err != nil {
			log.Printf("[AI USAGE WARN] failed to record AI usage log: %v", err)
		}
	}()
}

// RecordAIUsageSync persists an AI operation ledger record and updates recording total token caches.
func (s *Service) RecordAIUsageSync(ctx context.Context, input dtos.AIUsageRecordInput) error {
	if s.repo == nil {
		return nil
	}

	totalTokens := input.TotalTokens
	if totalTokens == 0 && (input.PromptTokens > 0 || input.CompletionTokens > 0) {
		totalTokens = input.PromptTokens + input.CompletionTokens
	}

	modelName := input.ModelName
	if modelName == "" {
		modelName = s.cfg.LLMModel
		if modelName == "" {
			modelName = "unknown"
		}
	}

	provider := input.Provider
	if provider == "" {
		provider = ResolveAIProvider(modelName, s.cfg.LLMProvider)
	}

	serviceType := input.ServiceType
	if serviceType == "" {
		serviceType = constants.AIServiceTypeLLMSummary.String()
	}

	var metadata models.JSONMap
	if input.Metadata != nil {
		metadata = models.JSONMap(input.Metadata)
	}

	s.enrichAIUsageInputFromRecording(ctx, &input)

	usageLog := &models.AIUsageLog{
		UserID:           input.UserID,
		WorkspaceID:      input.WorkspaceID,
		RecordingID:      input.RecordingID,
		ServiceType:      serviceType,
		Provider:         provider,
		ModelName:        modelName,
		PromptTokens:     input.PromptTokens,
		CompletionTokens: input.CompletionTokens,
		TotalTokens:      totalTokens,
		AudioSeconds:     input.AudioSeconds,
		LatencyMs:        input.LatencyMs,
		Metadata:         metadata,
		CreatedAt:        time.Now(),
	}

	if err := s.repo.CreateAIUsageLog(ctx, usageLog); err != nil {
		return err
	}

	if input.RecordingID != nil && *input.RecordingID != uuid.Nil && totalTokens > 0 {
		return s.repo.IncrementRecordingTokenUsage(ctx, *input.RecordingID, totalTokens)
	}

	return nil
}

// enrichAIUsageInputFromRecording populates missing UserID and WorkspaceID from the parent recording when neither is set.
func (s *Service) enrichAIUsageInputFromRecording(ctx context.Context, input *dtos.AIUsageRecordInput) {
	if input == nil || input.RecordingID == nil || *input.RecordingID == uuid.Nil {
		return
	}
	if input.UserID != nil || input.WorkspaceID != nil {
		return
	}
	rec, err := s.repo.FindRecordingByID(ctx, *input.RecordingID)
	if err != nil || rec == nil {
		return
	}
	input.UserID = rec.UserID
	input.WorkspaceID = rec.WorkspaceID
}

// GetAdminAIUsageLogs returns paginated AI model usage logs with aggregate totals for CMS admin staff.
func (s *Service) GetAdminAIUsageLogs(ctx context.Context, f dtos.AdminAIUsageFilter) (*dtos.AdminAIUsageResponse, error) {
	f.SetDefaults()
	if err := f.Validate(); err != nil {
		return nil, err
	}

	repoFilter := repositories.AIUsageFilter{
		ServiceType: f.ServiceType,
		Provider:    f.Provider,
		ModelName:   f.ModelName,
		Limit:       f.Limit,
		Offset:      (f.Page - 1) * f.Limit,
	}

	if f.UserID != nil && *f.UserID != "" {
		if uid, err := uuid.Parse(*f.UserID); err == nil {
			repoFilter.UserID = &uid
		}
	}
	if f.WorkspaceID != nil && *f.WorkspaceID != "" {
		if wid, err := uuid.Parse(*f.WorkspaceID); err == nil {
			repoFilter.WorkspaceID = &wid
		}
	}
	if f.RecordingID != nil && *f.RecordingID != "" {
		if rid, err := uuid.Parse(*f.RecordingID); err == nil {
			repoFilter.RecordingID = &rid
		}
	}
	if f.StartDate != "" {
		if st, err := time.Parse(time.RFC3339, f.StartDate); err == nil {
			repoFilter.StartDate = &st
		}
	}
	if f.EndDate != "" {
		if et, err := time.Parse(time.RFC3339, f.EndDate); err == nil {
			repoFilter.EndDate = &et
		}
	}

	logs, total, summary, err := s.repo.ListAIUsageLogsFiltered(ctx, repoFilter)
	if err != nil {
		return nil, err
	}

	items := make([]dtos.AdminAIUsageItemDTO, len(logs))
	for i, l := range logs {
		var userEmail *string
		if l.User != nil && l.User.Email != "" {
			userEmail = &l.User.Email
		}
		var recTitle *string
		if l.Recording != nil && l.Recording.Title != "" {
			recTitle = &l.Recording.Title
		}

		var metaMap map[string]interface{}
		if l.Metadata != nil {
			metaMap = map[string]interface{}(l.Metadata)
		}

		items[i] = dtos.AdminAIUsageItemDTO{
			ID:               l.ID,
			UserID:           l.UserID,
			UserEmail:        userEmail,
			WorkspaceID:      l.WorkspaceID,
			RecordingID:      l.RecordingID,
			RecordingTitle:   recTitle,
			ServiceType:      l.ServiceType,
			Provider:         l.Provider,
			ModelName:        l.ModelName,
			PromptTokens:     l.PromptTokens,
			CompletionTokens: l.CompletionTokens,
			TotalTokens:      l.TotalTokens,
			AudioSeconds:     l.AudioSeconds,
			LatencyMs:        l.LatencyMs,
			Metadata:         metaMap,
			CreatedAt:        l.CreatedAt,
		}
	}

	return &dtos.AdminAIUsageResponse{
		Items:      items,
		Summary:    summary,
		Pagination: dtos.NewPaginationMeta(f.Page, f.Limit, total),
	}, nil
}

// ResolveAIProvider extracts or infers provider identity from model names or fallback configs.
func ResolveAIProvider(modelName string, fallbackProvider string) string {
	lower := strings.ToLower(modelName)
	if strings.Contains(lower, "deepgram") {
		return "deepgram"
	}
	if strings.Contains(lower, "whisper") {
		return "whisper"
	}
	if strings.Contains(lower, "pyannote") {
		return "pyannote"
	}
	if strings.Contains(lower, "cf/") || strings.Contains(lower, "bge") {
		return "cloudflare"
	}
	if strings.Contains(lower, "gpt") || strings.Contains(lower, "openai") {
		return "openai"
	}
	if strings.Contains(lower, "gemini") {
		return "gemini"
	}
	if strings.Contains(lower, "claude") || strings.Contains(lower, "anthropic") {
		return "anthropic"
	}
	if fallbackProvider != "" {
		return fallbackProvider
	}
	return "omniroute"
}

// EstimateChatPromptTokens calculates estimated tokens across system prompt and conversation messages.
func EstimateChatPromptTokens(systemPrompt string, messages []dtos.ChatMessageInput) int {
	total := EstimateTokens(systemPrompt)
	for _, m := range messages {
		total += EstimateTokens(m.Content) + 4
	}
	return total
}

