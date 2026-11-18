package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
	"code-base-golang/internal/queue"
)

// ProcessVisualExtraction coordinates the multi-tier keyframe extraction and dual-engine cascade intelligence.
// If extraction fails or times out, it gracefully degrades to audio-only with a partial warning status.
func (s *Service) ProcessVisualExtraction(ctx context.Context, recording *models.Recording, p queue.RecordingPipelinePayload, segments []models.TranscriptSegment) error {
	if recording == nil {
		return nil
	}

	sourcePath := p.SourcePath
	if sourcePath == "" && recording.AudioURL != nil {
		sourcePath = *recording.AudioURL
	}

	filename := recording.OriginalFilename
	if filename == "" {
		filename = filepath.Base(sourcePath)
	}

	if !isVideoMedia(sourcePath, filename) {
		log.Printf("[PIPELINE INFO] recording %s: source %q is not a video; skipping visual extraction", recording.ID.String(), filename)
		return nil
	}

	if s.visualExtractor == nil {
		log.Printf("[PIPELINE WARN] recording %s: visual extractor not configured; degrading to audio only", recording.ID.String())
		s.recordVisualDegradation(ctx, recording, "visual extractor not configured")
		return nil
	}

	bucket := s.cfg.S3BucketName
	videoReader, err := s.storage.Download(ctx, bucket, sourcePath)
	if err != nil {
		log.Printf("[PIPELINE WARN] recording %s: failed to download source video for visual extraction: %v", recording.ID.String(), err)
		s.recordVisualDegradation(ctx, recording, fmt.Sprintf("download source video failed: %v", err))
		return nil
	}
	defer videoReader.Close()

	// Objective anchor timestamps from transcript boundaries without hardcoded keyword heuristics
	anchors := extractObjectiveSegmentAnchors(segments)

	extractOpts := dtos.KeyframeExtractionOptions{
		SceneThreshold:     0.25,
		MinIntervalSeconds: 5.0,
		MaxFramesPerHour:   60,
		TargetWidth:        1280,
		AudioAnchors:       anchors,
	}

	keyframes, err := s.visualExtractor.ExtractKeyframes(ctx, videoReader, filename, extractOpts)
	if err != nil {
		log.Printf("[PIPELINE WARN] recording %s: keyframe extraction failed: %v; degrading to audio only", recording.ID.String(), err)
		s.recordVisualDegradation(ctx, recording, fmt.Sprintf("keyframe extraction failed: %v", err))
		return nil
	}

	if len(keyframes) == 0 {
		log.Printf("[PIPELINE INFO] recording %s: no visual scene changes detected; degrading to audio only", recording.ID.String())
		s.recordVisualDegradation(ctx, recording, "no visual scene changes detected")
		return nil
	}

	// Clean up any stale keyframes for idempotency
	_ = s.repo.DeleteVideoKeyframesByRecordingID(ctx, recording.ID)

	var modelKeyframes []models.VideoKeyframe
	var validKeyframes []dtos.ExtractedKeyframe
	var validAnalyses []*dtos.KeyframeAnalysisResult

	for idx, kf := range keyframes {
		seqOrder := idx + 1
		destKey := fmt.Sprintf("%s/%s/keyframes/%04d_%.2f.jpg", constants.StoragePrefixRecordings, recording.ID.String(), seqOrder, kf.TimestampSeconds)

		// Upload keyframe thumbnail image to S3
		if err := s.storage.Upload(ctx, bucket, destKey, bytes.NewReader(kf.ImageBytes), int64(len(kf.ImageBytes)), kf.MimeType); err != nil {
			log.Printf("[PIPELINE WARN] recording %s: failed to upload keyframe %d to storage: %v", recording.ID.String(), seqOrder, err)
			continue
		}

		// Dual-Engine Cascade: Fast-path local OCR -> Multimodal Vision LLM fallback
		analysis := s.analyzeKeyframeCascade(ctx, kf)
		if analysis == nil {
			continue
		}

		validKeyframes = append(validKeyframes, kf)
		validAnalyses = append(validAnalyses, analysis)

		metaJSON, _ := json.Marshal(analysis.Metadata)

		modelKeyframes = append(modelKeyframes, models.VideoKeyframe{
			RecordingID:      recording.ID,
			TimestampSeconds: kf.TimestampSeconds,
			S3ImageURL:       destKey,
			CueType:          analysis.CueType,
			Title:            analysis.Title,
			ExtractedText:    analysis.ExtractedText,
			VisualSummary:    analysis.VisualSummary,
			Metadata:         json.RawMessage(metaJSON),
			Confidence:       analysis.Confidence,
			SequenceOrder:    seqOrder,
		})
	}

	if len(modelKeyframes) == 0 {
		s.recordVisualDegradation(ctx, recording, "failed to persist any analyzed keyframes")
		return nil
	}

	if err := s.repo.SaveVideoKeyframes(ctx, modelKeyframes); err != nil {
		log.Printf("[PIPELINE ERROR] recording %s: failed to save video keyframes: %v", recording.ID.String(), err)
		s.recordVisualDegradation(ctx, recording, fmt.Sprintf("database save failed: %v", err))
		return nil
	}

	// Audiovisual Active Speaker Nametag Binding
	if len(segments) > 0 && len(validKeyframes) > 0 {
		speakerMap := correlateVisualSpeakerNames(validKeyframes, validAnalyses, segments)
		if len(speakerMap) > 0 {
			if count, err := s.repo.UpdateTranscriptSpeakerNames(ctx, recording.ID, speakerMap); err != nil {
				log.Printf("[PIPELINE WARN] recording %s: failed to update transcript speaker names from visual detection: %v", recording.ID.String(), err)
			} else {
				log.Printf("[PIPELINE SUCCESS] recording %s: bound %d speaker names from visual nametags (%d segments updated): %v", recording.ID.String(), len(speakerMap), count, speakerMap)
				if recording.AnalyticsData == nil {
					recording.AnalyticsData = make(models.JSONMap)
				}
				recording.AnalyticsData["visual_speakers"] = speakerMap
			}
		}
	}

	// Update recording analytics with visual completion details
	s.recordVisualSuccess(ctx, recording, len(modelKeyframes))
	log.Printf("[PIPELINE SUCCESS] recording %s: extracted and saved %d visual keyframes", recording.ID.String(), len(modelKeyframes))
	return nil
}

// analyzeKeyframeCascade executes the Dual-Engine Cascade: local OCR fast-path -> Multimodal Vision LLM.
func (s *Service) analyzeKeyframeCascade(ctx context.Context, kf dtos.ExtractedKeyframe) *dtos.KeyframeAnalysisResult {
	// Stage 1: Fast-Path Local OCR (if configured and available)
	if s.localOCR != nil && s.localOCR.IsAvailable() {
		text, conf, usable, err := s.localOCR.ExtractText(ctx, kf.ImageBytes)
		if err == nil && usable && conf >= 0.75 {
			firstLine := extractFirstLine(text)
			return &dtos.KeyframeAnalysisResult{
				SequenceOrder:    kf.SequenceOrder,
				TimestampSeconds: kf.TimestampSeconds,
				CueType:          constants.VisualCueTypeSlide,
				Title:            firstLine,
				ExtractedText:    text,
				VisualSummary:    "Presentation slide containing textual content",
				Confidence:       conf,
				Metadata: map[string]interface{}{
					"engine": "local_ocr",
				},
			}
		}
	}

	// Stage 2: Multimodal Vision LLM Escalation
	if s.visionAnalyzer != nil {
		result, err := s.visionAnalyzer.AnalyzeKeyframe(ctx, kf.ImageBytes, kf.MimeType, "")
		if err == nil && result != nil {
			result.SequenceOrder = kf.SequenceOrder
			result.TimestampSeconds = kf.TimestampSeconds
			if result.Metadata == nil {
				result.Metadata = make(map[string]interface{})
			}
			result.Metadata["engine"] = "vision_llm"
			return result
		}
		log.Printf("[PIPELINE WARN] vision analyzer error for frame at %.2fs: %v", kf.TimestampSeconds, err)
	}

	// Fallback placeholder if both engines are unavailable or failed
	return &dtos.KeyframeAnalysisResult{
		SequenceOrder:    kf.SequenceOrder,
		TimestampSeconds: kf.TimestampSeconds,
		CueType:          constants.VisualCueTypeSlide,
		Title:            "",
		ExtractedText:    "",
		VisualSummary:    "Visual keyframe captured",
		Confidence:       0.40,
		Metadata: map[string]interface{}{
			"engine": "fallback",
		},
	}
}

// recordVisualDegradation stores partial warning status in recording analytics without failing the job.
func (s *Service) recordVisualDegradation(ctx context.Context, recording *models.Recording, reason string) {
	if recording.AnalyticsData == nil {
		recording.AnalyticsData = make(models.JSONMap)
	}
	recording.AnalyticsData["visual_status"] = models.RecordingStatusCompletedAudioOnly
	recording.AnalyticsData["visual_warning"] = reason

	_ = s.repo.UpdateRecordingAnalytics(ctx, recording.ID, recording.AnalyticsData)
}

// recordVisualSuccess stores successful visual extraction metadata in recording analytics.
func (s *Service) recordVisualSuccess(ctx context.Context, recording *models.Recording, keyframeCount int) {
	if recording.AnalyticsData == nil {
		recording.AnalyticsData = make(models.JSONMap)
	}
	delete(recording.AnalyticsData, "visual_warning")
	recording.AnalyticsData["visual_status"] = models.RecordingStatusCompleted
	recording.AnalyticsData["keyframes_count"] = keyframeCount

	_ = s.repo.UpdateRecordingAnalytics(ctx, recording.ID, recording.AnalyticsData)
}

// isVideoMedia determines whether a media path or filename corresponds to a supported video format.
func isVideoMedia(path, filename string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		ext = strings.ToLower(filepath.Ext(filename))
	}

	switch ext {
	case ".mp4", ".webm", ".mov", ".mkv", ".avi", ".m4v":
		return true
	default:
		return false
	}
}

// extractObjectiveSegmentAnchors gathers timestamp anchors from segment boundaries and speaker turns
// without hardcoded language dictionaries. It targets speech midpoints where meeting video UI
// active speaker indicators (green/blue/purple borders) are fully rendered and stable.
func extractObjectiveSegmentAnchors(segments []models.TranscriptSegment) []float64 {
	if len(segments) == 0 {
		return nil
	}

	var anchors []float64
	speakerSampleCount := make(map[string]int)

	for i, seg := range segments {
		duration := seg.EndTime - seg.StartTime
		// Midpoint sample timestamp during active utterance (stable tile highlight)
		sampleTs := seg.StartTime + (duration / 2.0)
		if duration >= 2.0 {
			sampleTs = seg.StartTime + 1.0
		}

		label := strings.TrimSpace(seg.SpeakerLabel)
		if label == "" {
			label = constants.DefaultSpeakerLabel
		}

		// Ensure up to 2 dedicated speech samples per speaker to capture active nametags
		if speakerSampleCount[label] < 2 && duration >= 0.8 {
			anchors = append(anchors, sampleTs)
			speakerSampleCount[label]++
		}

		// Also sample scene boundaries or long speech pauses (>= 2.0s)
		if i == 0 {
			anchors = append(anchors, seg.StartTime)
			continue
		}
		prev := segments[i-1]
		if seg.SpeakerLabel != prev.SpeakerLabel || (seg.StartTime-prev.EndTime) >= 2.0 {
			anchors = append(anchors, seg.StartTime)
		}
	}

	return anchors
}

// extractFirstLine safely returns the first non-empty line of text for title fallback.
func extractFirstLine(text string) string {
	lines := strings.Split(text, "\n")
	for _, l := range lines {
		trim := strings.TrimSpace(l)
		if len(trim) > 0 {
			runes := []rune(trim)
			if len(runes) > 80 {
				return string(runes[:80]) + "..."
			}
			return trim
		}
	}
	return ""
}

// correlateVisualSpeakerNames associates meeting video active speaker nametags with transcript speaker labels.
func correlateVisualSpeakerNames(keyframes []dtos.ExtractedKeyframe, analyses []*dtos.KeyframeAnalysisResult, segments []models.TranscriptSegment) map[string]string {
	if len(keyframes) == 0 || len(analyses) == 0 || len(segments) == 0 {
		return nil
	}

	speakerMap := make(map[string]string)
	for i, kf := range keyframes {
		if i >= len(analyses) || analyses[i] == nil {
			continue
		}
		analysis := analyses[i]

		detectedName := strings.TrimSpace(analysis.ActiveSpeaker)
		if detectedName == "" && len(analysis.VisibleSpeakers) == 1 && analysis.CueType == constants.VisualCueTypeActiveSpeaker {
			detectedName = strings.TrimSpace(analysis.VisibleSpeakers[0])
		}

		if detectedName == "" || isGenericSpeakerLabel(detectedName) {
			continue
		}

		seg := findSegmentAtTimestamp(kf.TimestampSeconds, segments)
		if seg == nil {
			continue
		}

		label := strings.TrimSpace(seg.SpeakerLabel)
		if label == "" {
			label = constants.DefaultSpeakerLabel
		}

		if _, exists := speakerMap[label]; !exists {
			speakerMap[label] = detectedName
		}
	}

	return speakerMap
}

// findSegmentAtTimestamp returns the transcript segment that covers the given timestamp.
func findSegmentAtTimestamp(ts float64, segments []models.TranscriptSegment) *models.TranscriptSegment {
	for i := range segments {
		if ts >= segments[i].StartTime && ts <= segments[i].EndTime {
			return &segments[i]
		}
	}
	return nil
}

