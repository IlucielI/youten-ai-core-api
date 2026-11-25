package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
	"code-base-golang/internal/pkg/ctxmeta"
	"code-base-golang/internal/pkg/jsonutil"
	"code-base-golang/internal/pkg/timeutil"
	"code-base-golang/internal/templates"
)

// InitiateRecordingChatStream validates access (owner, guest, or share token), retrieves relevant context chunks,
// restores conversational session history, enriches with meeting intelligence metadata, and initiates an LLM response stream.
func (s *Service) InitiateRecordingChatStream(
	ctx context.Context,
	recordingID uuid.UUID,
	ownershipToken string,
	req dtos.RecordingChatRequest,
) (*dtos.RecordingChatStreamResult, error) {
	rec, err := s.repo.FindRecordingByID(ctx, recordingID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrRecordingNotFound
		}
		return nil, fmt.Errorf("failed to lookup recording: %w", err)
	}

	// Access verification: owner, matching ownership token, active guest session, or valid share token
	policy := dtos.RecordingAccessPolicy{AllowShareToken: true}
	if !s.authorizeRecordingAccess(ctx, rec, ownershipToken, policy) {
		if req.ShareToken == "" || !s.authorizeRecordingAccess(ctx, rec, req.ShareToken, policy) {
			return nil, constants.ErrForbidden
		}
	}

	trimmedMsg := strings.TrimSpace(req.Message)

	// Resolve conversation session ID and restore session history
	var sessionID uuid.UUID
	var priorMessages []models.ChatMessage
	if req.SessionID != nil && *req.SessionID != uuid.Nil {
		sessionID = *req.SessionID
		// Prevent share visitors from appending to sessions owned by another user or recording owner
		existingMsgs, err := s.repo.ListChatMessagesBySession(ctx, rec.ID, sessionID)
		if err != nil {
			return nil, fmt.Errorf("failed to check session access: %w", err)
		}
		if !s.isSessionAccessibleByCaller(ctx, rec, ownershipToken, existingMsgs) {
			return nil, constants.ErrForbidden
		}
		priorMessages = existingMsgs
	} else {
		sessionID = uuid.New()
	}

	var userID *uuid.UUID
	if authUserID, ok := ctxmeta.GetAuthUserID(ctx); ok && authUserID != uuid.Nil {
		userID = &authUserID
	}

	// Persist user question turn with session_id
	userMsg := &models.ChatMessage{
		ID:                uuid.New(),
		RecordingID:       rec.ID,
		SessionID:         &sessionID,
		UserID:            userID,
		SenderRole:        constants.ChatRoleUser,
		Content:           trimmedMsg,
		Citations:         json.RawMessage("[]"),
		RetrievedChunkIDs: json.RawMessage("[]"),
		CreatedAt:         time.Now().UTC(),
	}
	if err := s.repo.SaveChatMessage(ctx, userMsg); err != nil {
		return nil, fmt.Errorf("failed to save user chat message: %w", err)
	}

	// Extract high-level meeting metadata from recording and active summary
	meta := extractRecordingMetadata(rec)
	if s.repo != nil {
		if activeSum, sumErr := s.repo.FindActiveSummaryByRecordingID(ctx, rec.ID); sumErr == nil && activeSum != nil {
			populateSummaryMetadata(meta, activeSum)
		}
	}
	if req.SummaryOverview != "" && meta.SummaryOverview == "" {
		meta.SummaryOverview = req.SummaryOverview
	}
	if len(req.ActionItems) > 0 && len(meta.ActionItems) == 0 {
		meta.ActionItems = req.ActionItems
	}

	messages := resolvePriorConversationHistory(req.ConversationHistory, priorMessages)

	// Adaptive retrieval routing: LLM planner selects vector_top_k, full_transcript, or lexical_search
	plan := s.planRecordingChatRetrieval(ctx, trimmedMsg, messages)
	chunkData, retrievedChunkIDs := s.retrieveAdaptiveTranscriptContext(ctx, rec.ID, trimmedMsg, plan, meta)

	// Prompt generation with RAG templates enriched with metadata
	systemPrompt, userPrompt := BuildRAGPrompt(trimmedMsg, chunkData, meta)

	messages = append(messages, dtos.ChatMessageInput{
		Role:    constants.ChatRoleUser,
		Content: userPrompt,
	})

	temp := 0.3
	maxTokens := 2048
	topP := 0.95
	chatOpts := dtos.ChatOptions{
		Temperature: &temp,
		MaxTokens:   &maxTokens,
		TopP:        &topP,
	}

	streamCh, err := s.llm.StreamChatResponse(ctx, systemPrompt, messages, chatOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to stream chat completion: %w", err)
	}

	promptTokens := EstimateChatPromptTokens(systemPrompt, messages)

	return &dtos.RecordingChatStreamResult{
		SessionID:         sessionID,
		StreamChannel:     streamCh,
		RetrievedChunkIDs: retrievedChunkIDs,
		PromptTokens:      promptTokens,
	}, nil
}

func extractRecordingMetadata(rec *models.Recording) *dtos.RecordingContextMetadata {
	if rec == nil {
		return nil
	}

	meta := &dtos.RecordingContextMetadata{
		Title:             rec.Title,
		DurationSeconds:   rec.DurationSeconds,
		DurationFormatted: timeutil.FormatTimestamp(rec.DurationSeconds),
		SelectedTemplate:  rec.SelectedTemplate,
		OutputLanguage:    rec.OutputLanguage,
	}
	if rec.DetectedLanguage != nil {
		meta.DetectedLanguage = *rec.DetectedLanguage
	}

	if rec.AnalyticsData != nil {
		if spkRaw, ok := rec.AnalyticsData["speakers"]; ok {
			meta.Speakers = parseSpeakersFromAnalytics(spkRaw)
		}
	}

	return meta
}

func parseSpeakersFromAnalytics(raw interface{}) []string {
	if raw == nil {
		return nil
	}

	var speakers []string
	seen := make(map[string]bool)

	switch list := raw.(type) {
	case []dtos.SpeakerAnalytics:
		for _, spk := range list {
			trimmed := strings.TrimSpace(spk.Name)
			if trimmed != "" && !seen[trimmed] {
				seen[trimmed] = true
				speakers = append(speakers, trimmed)
			}
		}
	case []interface{}:
		for _, item := range list {
			if m, ok := item.(map[string]interface{}); ok {
				if name, ok := m["name"].(string); ok {
					trimmed := strings.TrimSpace(name)
					if trimmed != "" && !seen[trimmed] {
						seen[trimmed] = true
						speakers = append(speakers, trimmed)
					}
				}
			}
		}
	}

	return speakers
}

func populateSummaryMetadata(meta *dtos.RecordingContextMetadata, summary *models.Summary) {
	if meta == nil || summary == nil {
		return
	}

	formatted := formatSummaryContent(summary)
	if formatted.Overview != "" {
		meta.SummaryOverview = formatted.Overview
	}
	if len(formatted.ActionItems) > 0 {
		meta.ActionItems = formatted.ActionItems
	}
	for _, sec := range formatted.Sections {
		if heading := strings.TrimSpace(sec.Heading); heading != "" {
			meta.KeyTopics = append(meta.KeyTopics, heading)
		}
	}
}

// SaveAssistantChatMessage parses citations, maps the model entity, and persists the assistant response turn for the session.
func (s *Service) SaveAssistantChatMessage(
	ctx context.Context,
	recordingID uuid.UUID,
	sessionID uuid.UUID,
	content string,
	chunkIDs []string,
) (*models.ChatMessage, []string, error) {
	citations := ExtractCitations(content)
	if citations == nil {
		citations = []string{}
	}

	var userID *uuid.UUID
	if authUserID, ok := ctxmeta.GetAuthUserID(ctx); ok && authUserID != uuid.Nil {
		userID = &authUserID
	}

	var sessionIDPtr *uuid.UUID
	if sessionID != uuid.Nil {
		sessionIDPtr = &sessionID
	}

	asstMsg := &models.ChatMessage{
		ID:                uuid.New(),
		RecordingID:       recordingID,
		SessionID:         sessionIDPtr,
		UserID:            userID,
		SenderRole:        constants.ChatRoleAssistant,
		Content:           content,
		Citations:         jsonutil.RawStringSlice(citations),
		RetrievedChunkIDs: jsonutil.RawStringSlice(chunkIDs),
		CreatedAt:         time.Now().UTC(),
	}

	if err := s.repo.SaveChatMessage(ctx, asstMsg); err != nil {
		return nil, nil, fmt.Errorf("failed to persist assistant chat message: %w", err)
	}

	compTokens := EstimateTokens(content)
	promptTokens := 500
	var wsID *uuid.UUID
	if rec, rErr := s.repo.FindRecordingByID(ctx, recordingID); rErr == nil && rec != nil {
		wsID = rec.WorkspaceID
		if userID == nil {
			userID = rec.UserID
		}
	}
	s.RecordAIUsageAsync(dtos.AIUsageRecordInput{
		UserID:           userID,
		WorkspaceID:      wsID,
		RecordingID:      &recordingID,
		ServiceType:      constants.AIServiceTypeRecordingChat.String(),
		Provider:         s.cfg.LLMProvider,
		ModelName:        s.cfg.LLMModel,
		PromptTokens:     promptTokens,
		CompletionTokens: compTokens,
		TotalTokens:      promptTokens + compTokens,
	})

	return asstMsg, citations, nil
}

// ListRecordingChatHistory retrieves chat messages for a specific conversation session on a recording.
func (s *Service) ListRecordingChatHistory(
	ctx context.Context,
	recordingID uuid.UUID,
	sessionID uuid.UUID,
	ownershipToken string,
	shareToken string,
) ([]models.ChatMessage, error) {
	rec, err := s.repo.FindRecordingByID(ctx, recordingID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrRecordingNotFound
		}
		return nil, fmt.Errorf("failed to lookup recording: %w", err)
	}

	policy := dtos.RecordingAccessPolicy{AllowShareToken: true}
	if !s.authorizeRecordingAccess(ctx, rec, ownershipToken, policy) {
		if shareToken == "" || !s.authorizeRecordingAccess(ctx, rec, shareToken, policy) {
			return nil, constants.ErrForbidden
		}
	}

	messages, err := s.repo.ListChatMessagesBySession(ctx, recordingID, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to list chat messages: %w", err)
	}
	if !s.isSessionAccessibleByCaller(ctx, rec, ownershipToken, messages) {
		return nil, constants.ErrForbidden
	}
	if messages == nil {
		messages = []models.ChatMessage{}
	}
	return messages, nil
}

// isSessionAccessibleByCaller enforces strict cross-user and owner-share boundary isolation on chat sessions.
func (s *Service) isSessionAccessibleByCaller(ctx context.Context, rec *models.Recording, ownershipToken string, messages []models.ChatMessage) bool {
	if len(messages) == 0 {
		return true
	}

	// Owner (authenticated user or matching guest ownership token) has full access
	if s.authorizeRecordingAccess(ctx, rec, ownershipToken, dtos.RecordingAccessPolicy{AllowShareToken: false}) {
		return true
	}

	callerUserID, hasCallerUser := ctxmeta.GetAuthUserID(ctx)
	for _, msg := range messages {
		if msg.UserID == nil {
			continue
		}
		// Prevent share visitors from accessing sessions created by the recording owner
		if rec.UserID != nil && *msg.UserID == *rec.UserID {
			return false
		}
		// Prevent cross-user session access among logged-in share viewers
		if !hasCallerUser || *msg.UserID != callerUserID {
			return false
		}
	}

	return true
}

var chatRouterJSONSchema = map[string]interface{}{
	"type": "object",
	"properties": map[string]interface{}{
		"strategy": map[string]interface{}{
			"type": "string",
			"enum": []string{
				constants.ChatRetrievalStrategyVectorTopK,
				constants.ChatRetrievalStrategyFullTranscript,
				constants.ChatRetrievalStrategyLexicalSearch,
			},
		},
		"target_terms": map[string]interface{}{
			"type": "array",
			"items": map[string]interface{}{
				"type": "string",
			},
		},
	},
	"required": []string{"strategy"},
}

func resolvePriorConversationHistory(reqHistory []dtos.ChatMessageInput, priorMessages []models.ChatMessage) []dtos.ChatMessageInput {
	if len(reqHistory) > 0 {
		out := make([]dtos.ChatMessageInput, len(reqHistory))
		copy(out, reqHistory)
		return out
	}
	if len(priorMessages) == 0 {
		return nil
	}

	startIdx := 0
	if len(priorMessages) > 8 {
		startIdx = len(priorMessages) - 8
	}
	var messages []dtos.ChatMessageInput
	for _, pm := range priorMessages[startIdx:] {
		if strings.TrimSpace(pm.Content) == "" {
			continue
		}
		role := pm.SenderRole
		if role != constants.ChatRoleUser && role != constants.ChatRoleAssistant {
			role = constants.ChatRoleUser
		}
		messages = append(messages, dtos.ChatMessageInput{
			Role:    role,
			Content: pm.Content,
		})
	}
	return messages
}

func (s *Service) planRecordingChatRetrieval(ctx context.Context, query string, history []dtos.ChatMessageInput) dtos.ChatRetrievalPlan {
	defaultPlan := dtos.ChatRetrievalPlan{Strategy: constants.ChatRetrievalStrategyVectorTopK}
	if s.llm == nil || strings.TrimSpace(query) == "" {
		return defaultPlan
	}

	sysPrompt, err := templates.DefaultChatRouterSystemPrompt()
	if err != nil || sysPrompt == "" {
		return defaultPlan
	}

	routerPrompt := buildChatRouterUserPrompt(query, history)
	routerCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()

	res, err := s.llm.GenerateStructured(routerCtx, sysPrompt, routerPrompt, chatRouterJSONSchema)
	if err != nil || res == nil || strings.TrimSpace(res.RawJSON) == "" {
		return defaultPlan
	}

	cleaned := jsonutil.CleanMarkdownJSON(res.RawJSON)
	var plan dtos.ChatRetrievalPlan
	if err := json.Unmarshal([]byte(cleaned), &plan); err != nil {
		return defaultPlan
	}

	return normalizeChatRetrievalPlan(plan)
}

func buildChatRouterUserPrompt(query string, history []dtos.ChatMessageInput) string {
	var sb strings.Builder
	if len(history) > 0 {
		start := 0
		if len(history) > 4 {
			start = len(history) - 4
		}
		sb.WriteString("Recent Conversation History:\n")
		for _, h := range history[start:] {
			sb.WriteString(fmt.Sprintf("- %s: %s\n", h.Role, strings.TrimSpace(h.Content)))
		}
		sb.WriteString("\n")
	}
	sb.WriteString("Current User Question: ")
	sb.WriteString(strings.TrimSpace(query))
	return sb.String()
}

func normalizeChatRetrievalPlan(plan dtos.ChatRetrievalPlan) dtos.ChatRetrievalPlan {
	strategy := strings.ToLower(strings.TrimSpace(plan.Strategy))
	terms := deduplicateCleanTerms(plan.TargetTerms)

	switch strategy {
	case constants.ChatRetrievalStrategyLexicalSearch:
		if len(terms) == 0 {
			return dtos.ChatRetrievalPlan{Strategy: constants.ChatRetrievalStrategyVectorTopK}
		}
		return dtos.ChatRetrievalPlan{
			Strategy:    constants.ChatRetrievalStrategyLexicalSearch,
			TargetTerms: terms,
		}
	case constants.ChatRetrievalStrategyFullTranscript:
		return dtos.ChatRetrievalPlan{Strategy: constants.ChatRetrievalStrategyFullTranscript}
	default:
		return dtos.ChatRetrievalPlan{Strategy: constants.ChatRetrievalStrategyVectorTopK}
	}
}

func deduplicateCleanTerms(raw []string) []string {
	seen := make(map[string]bool)
	var clean []string
	for _, t := range raw {
		trimmed := strings.Trim(strings.TrimSpace(t), "\"'`")
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		if seen[lower] {
			continue
		}
		seen[lower] = true
		clean = append(clean, trimmed)
	}
	return clean
}

func (s *Service) retrieveAdaptiveTranscriptContext(
	ctx context.Context,
	recID uuid.UUID,
	query string,
	plan dtos.ChatRetrievalPlan,
	meta *dtos.RecordingContextMetadata,
) ([]dtos.TranscriptChunkData, []string) {
	if meta != nil {
		meta.RetrievalStrategy = plan.Strategy
	}

	switch plan.Strategy {
	case constants.ChatRetrievalStrategyLexicalSearch:
		return s.retrieveLexicalTranscriptContext(ctx, recID, query, plan.TargetTerms, meta)
	case constants.ChatRetrievalStrategyFullTranscript:
		return s.retrieveFullTranscriptContext(ctx, recID, query)
	default:
		return s.retrieveVectorTopKChunks(ctx, recID, query, constants.DefaultRecordingChatTopK)
	}
}

func (s *Service) retrieveVectorTopKChunks(
	ctx context.Context,
	recID uuid.UUID,
	query string,
	topK int,
) ([]dtos.TranscriptChunkData, []string) {
	if s.embedding == nil || s.repo == nil {
		return nil, nil
	}
	embeddings, err := s.embedding.CreateEmbeddings(ctx, []string{query})
	if err != nil || len(embeddings) == 0 || len(embeddings[0]) == 0 {
		return nil, nil
	}
	chunks, err := s.repo.SearchSimilarTranscriptChunks(ctx, recID, embeddings[0], topK)
	if err != nil || len(chunks) == 0 {
		return nil, nil
	}

	chunkData := make([]dtos.TranscriptChunkData, 0, len(chunks))
	retrievedIDs := make([]string, 0, len(chunks))
	for _, c := range chunks {
		chunkData = append(chunkData, dtos.TranscriptChunkData{
			ChunkIndex: c.ChunkIndex,
			Content:    c.Content,
			StartTime:  c.StartTime,
			EndTime:    c.EndTime,
		})
		retrievedIDs = append(retrievedIDs, c.ID.String())
	}
	return chunkData, retrievedIDs
}

func (s *Service) retrieveLexicalTranscriptContext(
	ctx context.Context,
	recID uuid.UUID,
	query string,
	terms []string,
	meta *dtos.RecordingContextMetadata,
) ([]dtos.TranscriptChunkData, []string) {
	if s.repo == nil {
		return nil, nil
	}
	segments, err := s.repo.ListTranscriptSegmentsByRecordingID(ctx, recID)
	if err != nil || len(segments) == 0 {
		return s.retrieveVectorTopKChunks(ctx, recID, query, constants.DefaultRecordingChatTopK)
	}

	report, matchedChunks := ComputeLexicalAnalysis(segments, terms)
	if meta != nil {
		meta.LexicalAnalysisReport = report
	}
	if len(matchedChunks) > 0 {
		return matchedChunks, nil
	}
	return s.retrieveVectorTopKChunks(ctx, recID, query, constants.DefaultRecordingChatTopK)
}

func (s *Service) retrieveFullTranscriptContext(
	ctx context.Context,
	recID uuid.UUID,
	query string,
) ([]dtos.TranscriptChunkData, []string) {
	if s.repo == nil {
		return nil, nil
	}
	segments, err := s.repo.ListTranscriptSegmentsByRecordingID(ctx, recID)
	if err != nil || len(segments) == 0 {
		return s.retrieveVectorTopKChunks(ctx, recID, query, constants.DefaultRecordingChatTopK)
	}

	allChunks := ChunkModelSegments(segments, dtos.ChunkerConfig{
		MaxTokens:     constants.FullTranscriptChunkTokens,
		OverlapTokens: 0,
	})
	if len(allChunks) == 0 {
		return s.retrieveVectorTopKChunks(ctx, recID, query, constants.DefaultRecordingChatTopK)
	}

	totalTokens := 0
	for _, c := range allChunks {
		totalTokens += EstimateTokens(c.Content)
	}
	if totalTokens <= constants.FullTranscriptMaxBudgetTokens {
		return allChunks, nil
	}

	sampled := sampleChronologicalChunks(allChunks, constants.FullTranscriptSampledChronologicalCap)
	vectorChunks, vectorIDs := s.retrieveVectorTopKChunks(ctx, recID, query, constants.DefaultRecordingChatTopK)
	return mergeChronologicalAndVectorChunks(sampled, vectorChunks), vectorIDs
}

func sampleChronologicalChunks(chunks []dtos.TranscriptChunkData, maxCount int) []dtos.TranscriptChunkData {
	if len(chunks) <= maxCount || maxCount <= 0 {
		return chunks
	}
	sampled := make([]dtos.TranscriptChunkData, 0, maxCount)
	lastIdx := len(chunks) - 1
	for i := 0; i < maxCount; i++ {
		idx := (i * lastIdx) / (maxCount - 1)
		sampled = append(sampled, chunks[idx])
	}
	return sampled
}

func mergeChronologicalAndVectorChunks(primary []dtos.TranscriptChunkData, secondary []dtos.TranscriptChunkData) []dtos.TranscriptChunkData {
	merged := make([]dtos.TranscriptChunkData, 0, len(primary)+len(secondary))
	merged = append(merged, primary...)
	for _, vc := range secondary {
		if !isTimeRangeCovered(merged, vc.StartTime, vc.EndTime) {
			merged = append(merged, vc)
		}
	}
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].StartTime < merged[j].StartTime
	})
	return merged
}

func isTimeRangeCovered(existing []dtos.TranscriptChunkData, start, end float64) bool {
	for _, c := range existing {
		if start >= c.StartTime && end <= c.EndTime {
			return true
		}
	}
	return false
}

type speakerTermCount struct {
	Speaker      string
	ExactCount   int
	VariantCount int
}

// ComputeLexicalAnalysis performs a deterministic, language-agnostic full-transcript scan for target words/phrases.
// It separates Exact Word/Phrase Matches from Derived/Affixed Substring Matches and produces per-speaker breakdowns
// and timestamped matching segments.
func ComputeLexicalAnalysis(segments []models.TranscriptSegment, rawTerms []string) (string, []dtos.TranscriptChunkData) {
	terms := deduplicateCleanTerms(rawTerms)
	if len(segments) == 0 || len(terms) == 0 {
		return "", nil
	}

	var reportBuilder strings.Builder
	matchedSegIndices := make(map[int]bool)
	var orderedMatchedIndices []int

	for tIdx, term := range terms {
		if tIdx > 0 {
			reportBuilder.WriteString("\n")
		}
		termReport, segIndices := analyzeSingleTermInSegments(segments, term)
		reportBuilder.WriteString(termReport)
		for _, idx := range segIndices {
			if !matchedSegIndices[idx] {
				matchedSegIndices[idx] = true
				orderedMatchedIndices = append(orderedMatchedIndices, idx)
			}
		}
	}

	sort.Ints(orderedMatchedIndices)
	if len(orderedMatchedIndices) > constants.LexicalSearchMaxMatchingSegmentsPerTerm {
		orderedMatchedIndices = orderedMatchedIndices[:constants.LexicalSearchMaxMatchingSegmentsPerTerm]
	}

	chunks := make([]dtos.TranscriptChunkData, 0, len(orderedMatchedIndices))
	for chunkIdx, segIdx := range orderedMatchedIndices {
		seg := segments[segIdx]
		speaker := resolveSegmentSpeaker(seg)
		chunks = append(chunks, dtos.TranscriptChunkData{
			ChunkIndex: chunkIdx,
			Content:    fmt.Sprintf("[%s]: %s", speaker, strings.TrimSpace(seg.Text)),
			StartTime:  seg.StartTime,
			EndTime:    seg.EndTime,
		})
	}

	return strings.TrimSpace(reportBuilder.String()), chunks
}

func analyzeSingleTermInSegments(segments []models.TranscriptSegment, term string) (string, []int) {
	lowerTerm := strings.ToLower(strings.TrimSpace(term))
	termTokens := tokenizeUnicodeWords(lowerTerm)
	if len(termTokens) == 0 {
		return "", nil
	}

	totalExact := 0
	totalVariant := 0
	matchingSegmentsCount := 0
	var matchedIndices []int

	speakerStats := make(map[string]*speakerTermCount)
	var speakerOrder []string
	variantWordCounts := make(map[string]int)

	for idx, seg := range segments {
		exactInSeg, variantInSeg, variants := countTermInText(seg.Text, lowerTerm, termTokens)
		if exactInSeg == 0 && variantInSeg == 0 {
			continue
		}
		matchingSegmentsCount++
		matchedIndices = append(matchedIndices, idx)
		totalExact += exactInSeg
		totalVariant += variantInSeg

		speaker := resolveSegmentSpeaker(seg)
		stat, exists := speakerStats[speaker]
		if !exists {
			stat = &speakerTermCount{Speaker: speaker}
			speakerStats[speaker] = stat
			speakerOrder = append(speakerOrder, speaker)
		}
		stat.ExactCount += exactInSeg
		stat.VariantCount += variantInSeg

		for vw, c := range variants {
			variantWordCounts[vw] += c
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("- Target Term: %q\n", term))
	sb.WriteString(fmt.Sprintf("  * Exact Word/Phrase Occurrences: %d times\n", totalExact))
	sb.WriteString(fmt.Sprintf("  * Derived / Affixed / Substring Variant Occurrences: %d times\n", totalVariant))
	sb.WriteString(fmt.Sprintf("  * Total Combined Occurrences: %d times (across %d dialogue segments out of %d total segments)\n",
		totalExact+totalVariant, matchingSegmentsCount, len(segments)))

	if len(variantWordCounts) > 0 {
		sb.WriteString("  * Variant Words Detected: ")
		sb.WriteString(formatVariantWordCounts(variantWordCounts))
		sb.WriteString("\n")
	}

	if len(speakerOrder) > 0 {
		sb.WriteString("  * Per-Speaker Breakdown:\n")
		for _, spk := range speakerOrder {
			st := speakerStats[spk]
			sb.WriteString(fmt.Sprintf("    - %s: %d exact, %d variant (total: %d)\n",
				st.Speaker, st.ExactCount, st.VariantCount, st.ExactCount+st.VariantCount))
		}
	}

	return sb.String(), matchedIndices
}

func countTermInText(text string, lowerTerm string, termTokens []string) (exactCount int, variantCount int, variants map[string]int) {
	words := tokenizeUnicodeWords(strings.ToLower(text))
	if len(words) == 0 {
		return 0, 0, nil
	}

	if len(termTokens) == 1 {
		targetWord := termTokens[0]
		variants = make(map[string]int)
		for _, w := range words {
			if w == targetWord {
				exactCount++
				continue
			}
			if strings.Contains(w, targetWord) {
				variantCount++
				variants[w]++
			}
		}
		return exactCount, variantCount, variants
	}

	// Multi-word phrase matching
	phraseLen := len(termTokens)
	for i := 0; i <= len(words)-phraseLen; i++ {
		if matchesTokenSequence(words[i:i+phraseLen], termTokens) {
			exactCount++
		}
	}
	totalSub := strings.Count(strings.ToLower(text), lowerTerm)
	if totalSub > exactCount {
		variantCount = totalSub - exactCount
	}
	return exactCount, variantCount, nil
}

func matchesTokenSequence(window []string, target []string) bool {
	if len(window) != len(target) {
		return false
	}
	for i := range target {
		if window[i] != target[i] {
			return false
		}
	}
	return true
}

func tokenizeUnicodeWords(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
}

func resolveSegmentSpeaker(seg models.TranscriptSegment) string {
	if strings.TrimSpace(seg.SpeakerName) != "" {
		return strings.TrimSpace(seg.SpeakerName)
	}
	if strings.TrimSpace(seg.SpeakerLabel) != "" {
		return strings.TrimSpace(seg.SpeakerLabel)
	}
	return constants.DefaultSpeakerLabel
}

func formatVariantWordCounts(counts map[string]int) string {
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s (%dx)", k, counts[k]))
	}
	return strings.Join(parts, ", ")
}

