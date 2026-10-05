package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
	"code-base-golang/internal/payload"
	"code-base-golang/internal/sse"
	"code-base-golang/internal/templates"
)

// publishEvent dispatches a pipeline domain event, wrapping any broker failure with
// the topic for context. A nil publisher is a no-op so the pipeline can run without a
// broker (e.g. in tests).
func (s *Service) publishEvent(ctx context.Context, topic string, payload any) error {
	if s.publisher == nil {
		return nil
	}
	if err := s.publisher.Publish(ctx, topic, payload); err != nil {
		return fmt.Errorf("failed to publish %s event: %w", topic, err)
	}
	return nil
}

// failAndLog marks a recording as failed and logs if the failure update encounters an error.
func (s *Service) failAndLog(ctx context.Context, recordingID uuid.UUID, errCode, errMsg string) {
	if failErr := s.FailRecording(ctx, recordingID, errCode, errMsg); failErr != nil {
		log.Printf("[PIPELINE WARN] recording %s: FailRecording failed: %v", recordingID.String(), failErr)
	}
}

// ProcessExtraction handles the audio extraction stage of the recording pipeline.
func (s *Service) ProcessExtraction(ctx context.Context, p payload.RecordingPipelinePayload) error {
	recording, err := s.repo.FindRecordingByID(ctx, p.RecordingID)
	if err != nil {
		return fmt.Errorf("failed to find recording: %w", err)
	}

	// Update status to EXTRACTING
	if err := s.repo.UpdateRecordingStatus(ctx, recording.ID, models.RecordingStatusExtracting, nil, nil); err != nil {
		return fmt.Errorf("failed to update recording status: %w", err)
	}
	s.publishProgress(recording.ID, models.RecordingStatusExtracting, nil, nil)

	bucket := s.cfg.S3BucketName

	sourceKey := p.SourcePath
	if sourceKey == "" && recording.AudioURL != nil {
		sourceKey = *recording.AudioURL
	}
	if sourceKey == "" {
		errMsg := "no source file path available for extraction"
		s.failAndLog(ctx, recording.ID, models.ErrCodeExtractionFailed, errMsg)
		return errors.New(errMsg)
	}

	reader, err := s.storage.Download(ctx, bucket, sourceKey)
	if err != nil {
		s.failAndLog(ctx, recording.ID, models.ErrCodeExtractionFailed, fmt.Sprintf("failed to download source media: %v", err))
		return fmt.Errorf("failed to download source media: %w", err)
	}
	defer reader.Close()

	if s.audioExtractor == nil {
		errMsg := "audio extractor is not configured"
		s.failAndLog(ctx, recording.ID, models.ErrCodeExtractionFailed, errMsg)
		return errors.New(errMsg)
	}

	extracted, err := s.audioExtractor.ExtractMonoAudio(ctx, reader, recording.OriginalFilename)
	if err != nil {
		s.failAndLog(ctx, recording.ID, models.ErrCodeExtractionFailed, fmt.Sprintf("audio extraction failed: %v", err))
		return fmt.Errorf("audio extraction failed: %w", err)
	}
	defer extracted.Reader.Close()

	destAudioKey := fmt.Sprintf("%s/%s/audio.mp3", constants.StoragePrefixRecordings, recording.ID.String())
	if err := s.storage.Upload(ctx, bucket, destAudioKey, extracted.Reader, extracted.SizeBytes, "audio/mpeg"); err != nil {
		s.failAndLog(ctx, recording.ID, models.ErrCodeExtractionFailed, fmt.Sprintf("failed to upload extracted audio: %v", err))
		return fmt.Errorf("failed to upload extracted audio: %w", err)
	}

	if err := s.repo.UpdateRecordingAudioURL(ctx, recording.ID, destAudioKey, extracted.DurationSeconds); err != nil {
		s.failAndLog(ctx, recording.ID, models.ErrCodeExtractionFailed, fmt.Sprintf("failed to update audio url: %v", err))
		return fmt.Errorf("failed to update audio url: %w", err)
	}

	// Dispatch next stage: TRANSCRIBE
	nextPayload := p
	nextPayload.AudioPath = destAudioKey
	nextPayload.Stage = models.RecordingStatusTranscribing
	// Dispatching the next stage gates pipeline progression; surface a failure so the
	// worker nacks and retries extraction (which is local/cheap and idempotent).
	return s.publishEvent(ctx, constants.TopicRecordingTranscribe, nextPayload)
}

// ProcessTranscription handles the speech-to-text diarization stage.
func (s *Service) ProcessTranscription(ctx context.Context, p payload.RecordingPipelinePayload) error {
	recording, err := s.repo.FindRecordingByID(ctx, p.RecordingID)
	if err != nil {
		return fmt.Errorf("failed to find recording: %w", err)
	}

	if err := s.repo.UpdateRecordingStatus(ctx, recording.ID, models.RecordingStatusTranscribing, nil, nil); err != nil {
		return fmt.Errorf("failed to update recording status: %w", err)
	}
	s.publishProgress(recording.ID, models.RecordingStatusTranscribing, nil, nil)

	bucket := s.cfg.S3BucketName

	audioKey := p.AudioPath
	if audioKey == "" && recording.AudioURL != nil {
		audioKey = *recording.AudioURL
	}
	if audioKey == "" {
		errMsg := "no audio file available for transcription"
		s.failAndLog(ctx, recording.ID, models.ErrCodeTranscriptionFail, errMsg)
		return errors.New(errMsg)
	}

	audioReader, err := s.storage.Download(ctx, bucket, audioKey)
	if err != nil {
		s.failAndLog(ctx, recording.ID, models.ErrCodeTranscriptionFail, fmt.Sprintf("failed to download audio: %v", err))
		return fmt.Errorf("failed to download audio: %w", err)
	}
	defer audioReader.Close()

	if s.stt == nil {
		errMsg := "stt provider is not configured"
		s.failAndLog(ctx, recording.ID, models.ErrCodeTranscriptionFail, errMsg)
		return errors.New(errMsg)
	}

	sttResult, err := s.stt.Transcribe(ctx, audioReader, "audio.mp3", dtos.STTOptions{
		Language: recording.OutputLanguage,
	})
	if err != nil {
		s.failAndLog(ctx, recording.ID, models.ErrCodeTranscriptionFail, fmt.Sprintf("stt transcription failed: %v", err))
		return fmt.Errorf("stt transcription failed: %w", err)
	}

	if sttResult == nil || (len(sttResult.Segments) == 0 && strings.TrimSpace(sttResult.Text) == "") {
		errMsg := "No speech detected in audio recording"
		s.failAndLog(ctx, recording.ID, models.ErrCodeNoSpeechDetected, errMsg)
		return errors.New(errMsg)
	}

	// Idempotent cleanup before saving segments
	if err := s.repo.DeleteTranscriptSegmentsByRecordingID(ctx, recording.ID); err != nil {
		errMsg := fmt.Sprintf("failed to clean up transcript segments: %v", err)
		s.failAndLog(ctx, recording.ID, models.ErrCodeTranscriptionFail, errMsg)
		return fmt.Errorf("failed to clean up transcript segments: %w", err)
	}

	var modelSegments []models.TranscriptSegment
	for i, seg := range sttResult.Segments {
		wordsJSON, _ := json.Marshal(seg.Words)
		speaker := seg.SpeakerLabel
		if speaker == "" {
			speaker = constants.DefaultSpeakerLabel
		}
		modelSegments = append(modelSegments, models.TranscriptSegment{
			RecordingID:   recording.ID,
			SpeakerLabel:  speaker,
			SpeakerName:   speaker,
			StartTime:     seg.Start,
			EndTime:       seg.End,
			Text:          seg.Text,
			WordsData:     json.RawMessage(wordsJSON),
			SequenceOrder: i + 1,
		})
	}

	// If no segments provided by STT, fallback to a single segment from full text
	if len(modelSegments) == 0 && strings.TrimSpace(sttResult.Text) != "" {
		modelSegments = append(modelSegments, models.TranscriptSegment{
			RecordingID:   recording.ID,
			SpeakerLabel:  constants.DefaultSpeakerLabel,
			SpeakerName:   constants.DefaultSpeakerLabel,
			StartTime:     0,
			EndTime:       sttResult.Duration,
			Text:          sttResult.Text,
			WordsData:     json.RawMessage("[]"),
			SequenceOrder: 1,
		})
	}

	if err := s.repo.SaveTranscriptSegments(ctx, modelSegments); err != nil {
		s.failAndLog(ctx, recording.ID, models.ErrCodeTranscriptionFail, fmt.Sprintf("failed to save transcript segments: %v", err))
		return fmt.Errorf("failed to save transcript segments: %w", err)
	}

	if err := s.repo.UpdateRecordingDurationAndLanguage(ctx, recording.ID, sttResult.Duration, sttResult.Language); err != nil {
		log.Printf("[PIPELINE WARN] recording %s: failed to update duration and language: %v", recording.ID.String(), err)
	}

	// Analysis fan-out: dispatch independent downstream stages. These are best-effort
	// because transcription (paid STT) has already succeeded here; a failed dispatch is
	// logged rather than returned so the worker does not re-run STT on retry.
	for _, topic := range []string{
		constants.TopicRecordingSummarize,
		constants.TopicRecordingIndex,
		constants.TopicRecordingAnalytics,
		constants.TopicRecordingChapterize,
	} {
		if err := s.publishEvent(ctx, topic, p); err != nil {
			log.Printf("[PIPELINE ERROR] recording %s: %v", p.RecordingID.String(), err)
		}
	}

	return nil
}

// ProcessSummarization executes the active template prompt against transcript segments to generate structured notes.
func (s *Service) ProcessSummarization(ctx context.Context, p payload.RecordingPipelinePayload) error {
	recording, err := s.repo.FindRecordingByID(ctx, p.RecordingID)
	if err != nil {
		return fmt.Errorf("failed to find recording: %w", err)
	}

	if err := s.repo.UpdateRecordingStatus(ctx, recording.ID, models.RecordingStatusSummarizing, nil, nil); err != nil {
		return fmt.Errorf("failed to update recording status: %w", err)
	}
	s.publishProgress(recording.ID, models.RecordingStatusSummarizing, nil, nil)

	// Retrieve template
	templateKey := recording.SelectedTemplate
	if templateKey == "" {
		templateKey = constants.TemplateKeyGeneral
	}
	template, err := s.repo.FindTemplateByCategoryKey(ctx, templateKey)
	if err != nil || template == nil {
		var fallbackErr error
		template, fallbackErr = s.repo.FindTemplateByCategoryKey(ctx, constants.TemplateKeyGeneral)
		if fallbackErr != nil && !errors.Is(fallbackErr, gorm.ErrRecordNotFound) {
			log.Printf("[PIPELINE WARN] recording %s: failed to load general template fallback: %v", recording.ID.String(), fallbackErr)
		}
	}

	segments, err := s.repo.ListTranscriptSegmentsByRecordingID(ctx, recording.ID)
	if err != nil {
		return fmt.Errorf("failed to list transcript segments: %w", err)
	}
	if len(segments) == 0 {
		errMsg := "cannot summarize recording with empty transcript"
		s.failAndLog(ctx, recording.ID, models.ErrCodeSummarizationFail, errMsg)
		return errors.New(errMsg)
	}

	// Build prompt from segments
	var sb strings.Builder
	for _, seg := range segments {
		startStr := FormatTimestamp(seg.StartTime)
		endStr := FormatTimestamp(seg.EndTime)
		sb.WriteString(fmt.Sprintf("[%s - %s] %s: %s\n", startStr, endStr, seg.SpeakerName, seg.Text))
	}
	transcriptBody := sb.String()

	systemPrompt, err := templates.DefaultSummarySystemPrompt()
	if err != nil {
		systemPrompt = "You are a professional executive meeting secretary and structured summarizer."
	}
	if template != nil && template.Prompt != "" {
		systemPrompt = template.Prompt
	}

	targetLang := recording.OutputLanguage
	if targetLang == "" {
		targetLang = p.Language
	}

	userPrompt, err := templates.RenderSummaryUserPrompt(transcriptBody, targetLang)
	if err != nil {
		userPrompt = fmt.Sprintf("Please summarize the following meeting transcript:\n\n%s", transcriptBody)
	}

	var schema map[string]interface{}
	if template != nil && len(template.OutputSchema) > 0 {
		schema = map[string]interface{}(template.OutputSchema)
	}

	if s.llm == nil {
		errMsg := "llm provider is not configured"
		s.failAndLog(ctx, recording.ID, models.ErrCodeSummarizationFail, errMsg)
		return errors.New(errMsg)
	}

	structuredRes, err := s.llm.GenerateStructured(ctx, systemPrompt, userPrompt, schema)
	if err != nil {
		s.failAndLog(ctx, recording.ID, models.ErrCodeSummarizationFail, fmt.Sprintf("llm generation failed: %v", err))
		return fmt.Errorf("llm generation failed: %w", err)
	}

	// Deactivate older summaries for this recording
	if err := s.repo.DeactivatePreviousSummaries(ctx, recording.ID); err != nil {
		log.Printf("[PIPELINE WARN] recording %s: failed to deactivate previous summaries: %v", recording.ID.String(), err)
	}

	var structMap map[string]interface{}
	if err := json.Unmarshal([]byte(structuredRes.RawJSON), &structMap); err != nil {
		log.Printf("[PIPELINE WARN] recording %s: failed to unmarshal structured summary JSON: %v", recording.ID.String(), err)
	}

	markdownContent := ""
	if md, ok := structMap["markdown_content"].(string); ok && md != "" {
		markdownContent = md
	} else if md, ok := structMap["executive_summary"].(string); ok && md != "" {
		markdownContent = md
	} else {
		markdownContent = structuredRes.RawJSON
	}

	summary := models.Summary{
		RecordingID:      recording.ID,
		TemplateCategory: templateKey,
		Version:          1,
		IsActive:         true,
		StructuredData:   models.JSONMap(structMap),
		MarkdownContent:  markdownContent,
	}

	if err := s.repo.CreateSummary(ctx, &summary); err != nil {
		s.failAndLog(ctx, recording.ID, models.ErrCodeSummarizationFail, fmt.Sprintf("failed to save summary: %v", err))
		return fmt.Errorf("failed to save summary: %w", err)
	}

	// Check if all core tasks are finished to mark COMPLETED
	if _, err := s.CheckAndCompleteRecording(ctx, recording.ID); err != nil {
		log.Printf("[PIPELINE ERROR] recording %s: failed to complete recording after summary: %v", recording.ID.String(), err)
		return fmt.Errorf("failed to check and complete recording: %w", err)
	}
	return nil
}

// ProcessIndexing chunks transcript segments and generates pgvector embeddings for RAG retrieval.
func (s *Service) ProcessIndexing(ctx context.Context, p payload.RecordingPipelinePayload) error {
	recording, err := s.repo.FindRecordingByID(ctx, p.RecordingID)
	if err != nil {
		return fmt.Errorf("failed to find recording: %w", err)
	}

	if err := s.repo.UpdateRecordingStatus(ctx, recording.ID, models.RecordingStatusIndexing, nil, nil); err != nil {
		return fmt.Errorf("failed to update recording status: %w", err)
	}
	s.publishProgress(recording.ID, models.RecordingStatusIndexing, nil, nil)

	segments, err := s.repo.ListTranscriptSegmentsByRecordingID(ctx, recording.ID)
	if err != nil {
		return fmt.Errorf("failed to list transcript segments: %w", err)
	}
	if len(segments) == 0 {
		errMsg := "cannot index recording with empty transcript"
		s.failAndLog(ctx, recording.ID, models.ErrCodeIndexingFail, errMsg)
		return errors.New(errMsg)
	}

	chunkCfg := DefaultChunkerConfig()
	if s.cfg.RAGChunkTokens > 0 {
		chunkCfg.MaxTokens = s.cfg.RAGChunkTokens
	}
	if s.cfg.RAGChunkOverlapTokens > 0 {
		chunkCfg.OverlapTokens = s.cfg.RAGChunkOverlapTokens
	}

	chunks := ChunkModelSegments(segments, chunkCfg)
	if len(chunks) == 0 {
		errMsg := "chunking produced 0 chunks"
		s.failAndLog(ctx, recording.ID, models.ErrCodeIndexingFail, errMsg)
		return errors.New(errMsg)
	}

	chunkTexts := make([]string, len(chunks))
	for i, c := range chunks {
		chunkTexts[i] = c.Content
	}

	if s.embedding == nil {
		errMsg := "embedding provider is not configured"
		s.failAndLog(ctx, recording.ID, models.ErrCodeIndexingFail, errMsg)
		return errors.New(errMsg)
	}

	embeddings, err := s.embedding.CreateEmbeddings(ctx, chunkTexts)
	if err != nil {
		s.failAndLog(ctx, recording.ID, models.ErrCodeIndexingFail, fmt.Sprintf("embedding generation failed: %v", err))
		return fmt.Errorf("embedding generation failed: %w", err)
	}

	// Idempotent cleanup before inserting
	if err := s.repo.DeleteTranscriptChunksByRecordingID(ctx, recording.ID); err != nil {
		errMsg := fmt.Sprintf("failed to clean up transcript chunks: %v", err)
		s.failAndLog(ctx, recording.ID, models.ErrCodeIndexingFail, errMsg)
		return fmt.Errorf("failed to clean up transcript chunks: %w", err)
	}

	var modelChunks []models.TranscriptChunk
	for i, c := range chunks {
		var vec models.Vector
		if i < len(embeddings) {
			vec = models.Vector(embeddings[i])
		}
		modelChunks = append(modelChunks, models.TranscriptChunk{
			RecordingID: recording.ID,
			ChunkIndex:  c.ChunkIndex,
			Content:     c.Content,
			StartTime:   c.StartTime,
			EndTime:     c.EndTime,
			Embedding:   vec,
		})
	}

	if err := s.repo.SaveTranscriptChunks(ctx, modelChunks); err != nil {
		s.failAndLog(ctx, recording.ID, models.ErrCodeIndexingFail, fmt.Sprintf("failed to save transcript chunks: %v", err))
		return fmt.Errorf("failed to save transcript chunks: %w", err)
	}

	// Check if all core tasks are finished to mark COMPLETED
	if _, err := s.CheckAndCompleteRecording(ctx, recording.ID); err != nil {
		log.Printf("[PIPELINE ERROR] recording %s: failed to complete recording after indexing: %v", recording.ID.String(), err)
		return fmt.Errorf("failed to check and complete recording: %w", err)
	}
	return nil
}

// ProcessAnalytics calculates speaker talk-time distribution and speaking share metrics.
func (s *Service) ProcessAnalytics(ctx context.Context, p payload.RecordingPipelinePayload) error {
	recording, err := s.repo.FindRecordingByID(ctx, p.RecordingID)
	if err != nil {
		return fmt.Errorf("failed to find recording: %w", err)
	}

	segments, err := s.repo.ListTranscriptSegmentsByRecordingID(ctx, recording.ID)
	if err != nil || len(segments) == 0 {
		return nil
	}

	speakerMap := make(map[string]*dtos.SpeakerAnalytics)
	var totalDuration float64
	var totalWords int

	for _, seg := range segments {
		dur := seg.EndTime - seg.StartTime
		if dur < 0 {
			dur = 0
		}
		words := len(strings.Fields(seg.Text))

		totalDuration += dur
		totalWords += words

		stat, ok := speakerMap[seg.SpeakerName]
		if !ok {
			stat = &dtos.SpeakerAnalytics{Name: seg.SpeakerName}
			speakerMap[seg.SpeakerName] = stat
		}
		stat.TotalSeconds += dur
		stat.WordCount += words
	}

	var speakers []dtos.SpeakerAnalytics
	for _, stat := range speakerMap {
		if totalDuration > 0 {
			stat.SharePercent = (stat.TotalSeconds / totalDuration) * 100.0
		}
		speakers = append(speakers, *stat)
	}

	analyticsData := models.JSONMap{
		"total_duration_seconds": totalDuration,
		"total_words":            totalWords,
		"speakers":               speakers,
	}

	if err := s.repo.UpdateRecordingAnalytics(ctx, recording.ID, analyticsData); err != nil {
		log.Printf("[PIPELINE ERROR] recording %s: failed to update analytics: %v", recording.ID.String(), err)
		return fmt.Errorf("failed to update recording analytics: %w", err)
	}
	if _, err := s.CheckAndCompleteRecording(ctx, recording.ID); err != nil {
		log.Printf("[PIPELINE WARN] recording %s: failed to complete recording after analytics: %v", recording.ID.String(), err)
	}
	return nil
}

// ProcessChapterization generates navigational chapters and key moment highlights.
func (s *Service) ProcessChapterization(ctx context.Context, p payload.RecordingPipelinePayload) error {
	recording, err := s.repo.FindRecordingByID(ctx, p.RecordingID)
	if err != nil {
		return fmt.Errorf("failed to find recording: %w", err)
	}

	segments, err := s.repo.ListTranscriptSegmentsByRecordingID(ctx, recording.ID)
	if err != nil || len(segments) == 0 {
		return nil
	}

	// Group segments into logical chapters (e.g. per 5-10 minutes or significant topic boundaries)
	var chapters []models.Chapter
	const chapterInterval = 300.0 // 5 minutes

	var currentChapter *models.Chapter
	var chapterIndex int

	for _, seg := range segments {
		if currentChapter == nil || seg.StartTime >= currentChapter.EndTime {
			chapterIndex++
			start := seg.StartTime
			end := start + chapterInterval
			if currentChapter != nil && end <= currentChapter.EndTime {
				end = currentChapter.EndTime + chapterInterval
			}

			titlePreview := seg.Text
			if len(titlePreview) > 40 {
				titlePreview = titlePreview[:40] + "..."
			}

			chap := models.Chapter{
				RecordingID:   recording.ID,
				Title:         fmt.Sprintf("Chapter %d: %s", chapterIndex, strings.TrimSpace(titlePreview)),
				StartTime:     start,
				EndTime:       end,
				Summary:       seg.Text,
				SequenceOrder: chapterIndex,
			}
			chapters = append(chapters, chap)
			currentChapter = &chapters[len(chapters)-1]
		} else {
			if seg.EndTime > currentChapter.EndTime {
				currentChapter.EndTime = seg.EndTime
			}
			if len(currentChapter.Summary) < 300 {
				currentChapter.Summary += " " + seg.Text
			}
		}
	}

	if len(chapters) > 0 {
		if err := s.repo.DeleteChaptersByRecordingID(ctx, recording.ID); err != nil {
			log.Printf("[PIPELINE WARN] recording %s: failed to clean up chapters: %v", recording.ID.String(), err)
		}
		if err := s.repo.SaveChapters(ctx, chapters); err != nil {
			log.Printf("[PIPELINE ERROR] recording %s: failed to save chapters: %v", recording.ID.String(), err)
			return fmt.Errorf("failed to save chapters: %w", err)
		}
	}

	// Generate candidate highlights from first and key segments
	var highlights []models.Highlight
	for i, seg := range segments {
		if i == 0 || (i%5 == 0 && len(highlights) < 5) {
			title := seg.Text
			if len(title) > 50 {
				title = title[:50] + "..."
			}
			highlights = append(highlights, models.Highlight{
				RecordingID: recording.ID,
				StartTime:   seg.StartTime,
				EndTime:     seg.EndTime,
				Title:       &title,
				Source:      constants.HighlightSourceAISuggested,
			})
		}
	}

	if len(highlights) > 0 {
		if err := s.repo.DeleteHighlightsByRecordingID(ctx, recording.ID); err != nil {
			log.Printf("[PIPELINE WARN] recording %s: failed to clean up highlights: %v", recording.ID.String(), err)
		}
		if err := s.repo.SaveHighlights(ctx, highlights); err != nil {
			log.Printf("[PIPELINE ERROR] recording %s: failed to save highlights: %v", recording.ID.String(), err)
			return fmt.Errorf("failed to save highlights: %w", err)
		}
	}

	if _, err := s.CheckAndCompleteRecording(ctx, recording.ID); err != nil {
		log.Printf("[PIPELINE WARN] recording %s: failed to complete recording after chapterization: %v", recording.ID.String(), err)
	}
	return nil
}

// CheckAndCompleteRecording verifies if all mandatory core tasks are done and transitions state to COMPLETED.
func (s *Service) CheckAndCompleteRecording(ctx context.Context, recordingID uuid.UUID) (bool, error) {
	summary, err := s.repo.FindActiveSummaryByRecordingID(ctx, recordingID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, fmt.Errorf("failed to check active summary: %w", err)
	}
	chunks, err := s.repo.ListTranscriptChunksByRecordingID(ctx, recordingID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, fmt.Errorf("failed to check transcript chunks: %w", err)
	}

	// Both summary and vector indexing are mandatory for completion
	if summary == nil || len(chunks) == 0 {
		return false, nil
	}

	recording, err := s.repo.FindRecordingByID(ctx, recordingID)
	if err != nil {
		return false, err
	}

	if recording.Status == models.RecordingStatusCompleted {
		return true, nil
	}

	if err := s.repo.UpdateRecordingStatus(ctx, recordingID, models.RecordingStatusCompleted, nil, nil); err != nil {
		return false, err
	}
	s.publishProgress(recordingID, models.RecordingStatusCompleted, nil, nil)

	// Create user notification if tied to registered user
	if recording.UserID != nil {
		notif := models.Notification{
			UserID:      recording.UserID,
			RecordingID: &recordingID,
			Title:       "Recording Processed",
			Message:     fmt.Sprintf("Your recording '%s' has been transcribed and summarized successfully.", recording.Title),
			Type:        constants.NotificationTypeRecordingCompleted,
			IsRead:      false,
		}
		if err := s.repo.CreateNotification(ctx, &notif); err != nil {
			log.Printf("[PIPELINE WARN] recording %s: failed to create completion notification: %v", recordingID.String(), err)
		}
	}

	// Publish completed domain event (best-effort terminal notification).
	if err := s.publishEvent(ctx, constants.TopicRecordingCompleted, payload.RecordingPipelinePayload{
		RecordingID: recordingID,
		Stage:       models.RecordingStatusCompleted,
	}); err != nil {
		log.Printf("[PIPELINE ERROR] recording %s: %v", recordingID.String(), err)
	}

	log.Printf("[PIPELINE] Recording %s marked as COMPLETED", recordingID.String())
	return true, nil
}

// RetryRecording executes Smart State Recovery: resumes pipeline from failed stage without re-uploading file.
func (s *Service) RetryRecording(ctx context.Context, recordingID uuid.UUID) (*payload.RecordingPipelinePayload, error) {
	recording, err := s.repo.FindRecordingByID(ctx, recordingID)
	if err != nil {
		return nil, fmt.Errorf("failed to find recording: %w", err)
	}

	return s.ResumeRecordingPipeline(ctx, recording)
}

// ResumeRecordingPipeline executes Smart State Recovery using an already-loaded recording model.
func (s *Service) ResumeRecordingPipeline(ctx context.Context, recording *models.Recording) (*payload.RecordingPipelinePayload, error) {
	if recording.Status != models.RecordingStatusFailed {
		return nil, constants.ErrInvalidRetryState
	}

	// Check existing assets to resume at exact failing point
	segments, err := s.repo.ListTranscriptSegmentsByRecordingID(ctx, recording.ID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to check existing transcript segments: %w", err)
	}
	summary, err := s.repo.FindActiveSummaryByRecordingID(ctx, recording.ID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to check existing active summary: %w", err)
	}
	chunks, err := s.repo.ListTranscriptChunksByRecordingID(ctx, recording.ID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to check existing transcript chunks: %w", err)
	}

	p := payload.RecordingPipelinePayload{
		RecordingID: recording.ID,
		Template:    recording.SelectedTemplate,
		Language:    recording.OutputLanguage,
	}

	// 1. If audio not yet extracted
	if recording.AudioURL == nil || *recording.AudioURL == "" {
		p.Stage = models.RecordingStatusExtracting
		p.Status = models.RecordingStatusQueued
		if err := s.repo.UpdateRecordingStatus(ctx, recording.ID, models.RecordingStatusQueued, nil, nil); err != nil {
			return nil, fmt.Errorf("failed to update recording status to queued: %w", err)
		}
		s.publishProgress(recording.ID, models.RecordingStatusQueued, nil, nil)
		if err := s.publishEvent(ctx, constants.TopicRecordingUploaded, p); err != nil {
			log.Printf("[PIPELINE ERROR] recording %s: %v", recording.ID.String(), err)
		}
		return &p, nil
	}

	p.AudioPath = *recording.AudioURL

	// 2. If audio exists but no transcripts
	if len(segments) == 0 {
		p.Stage = models.RecordingStatusTranscribing
		p.Status = models.RecordingStatusQueued
		if err := s.repo.UpdateRecordingStatus(ctx, recording.ID, models.RecordingStatusQueued, nil, nil); err != nil {
			return nil, fmt.Errorf("failed to update recording status to queued: %w", err)
		}
		s.publishProgress(recording.ID, models.RecordingStatusQueued, nil, nil)
		if err := s.publishEvent(ctx, constants.TopicRecordingTranscribe, p); err != nil {
			log.Printf("[PIPELINE ERROR] recording %s: %v", recording.ID.String(), err)
		}
		return &p, nil
	}

	// 3. If transcripts exist, run fan-out for missing summary or indexing
	p.Stage = models.RecordingStatusTranscribing
	p.Status = models.RecordingStatusTranscribing
	if err := s.repo.UpdateRecordingStatus(ctx, recording.ID, models.RecordingStatusTranscribing, nil, nil); err != nil {
		return nil, fmt.Errorf("failed to update recording status to transcribing: %w", err)
	}
	s.publishProgress(recording.ID, models.RecordingStatusTranscribing, nil, nil)

	if summary == nil {
		if err := s.publishEvent(ctx, constants.TopicRecordingSummarize, p); err != nil {
			log.Printf("[PIPELINE ERROR] recording %s: %v", recording.ID.String(), err)
		}
	}
	if len(chunks) == 0 {
		if err := s.publishEvent(ctx, constants.TopicRecordingIndex, p); err != nil {
			log.Printf("[PIPELINE ERROR] recording %s: %v", recording.ID.String(), err)
		}
	}

	if summary != nil && len(chunks) > 0 {
		if _, err := s.CheckAndCompleteRecording(ctx, recording.ID); err != nil {
			log.Printf("[PIPELINE WARN] recording %s: failed to complete recording on resume: %v", recording.ID.String(), err)
		}
	}

	return &p, nil
}

// FailRecording marks recording state as FAILED with error code and description.
func (s *Service) FailRecording(ctx context.Context, recordingID uuid.UUID, errCode, errMsg string) error {
	log.Printf("[PIPELINE ERROR] Recording %s FAILED: [%s] %s", recordingID.String(), errCode, errMsg)

	if err := s.repo.UpdateRecordingStatus(ctx, recordingID, models.RecordingStatusFailed, &errCode, &errMsg); err != nil {
		log.Printf("[PIPELINE ERROR] recording %s: failed to update status to FAILED in DB: %v", recordingID.String(), err)
		return err
	}
	s.publishProgress(recordingID, models.RecordingStatusFailed, &errCode, &errMsg)

	// Best-effort terminal failure notification.
	if err := s.publishEvent(ctx, constants.TopicRecordingFailed, payload.RecordingPipelinePayload{
		RecordingID: recordingID,
		Stage:       models.RecordingStatusFailed,
	}); err != nil {
		log.Printf("[PIPELINE ERROR] recording %s: %v", recordingID.String(), err)
	}

	return nil
}

func (s *Service) publishProgress(recordingID uuid.UUID, status string, errCode, errMsg *string) {
	if s.sseHub == nil {
		return
	}
	stage, prog := sse.MapStatusToProgress(status)
	s.sseHub.Publish(recordingID, sse.ProgressEvent{
		RecordingID:  recordingID.String(),
		Status:       status,
		Stage:        stage,
		Progress:     prog,
		ErrorCode:    errCode,
		ErrorMessage: errMsg,
		UpdatedAt:    time.Now().UTC(),
	})
}
