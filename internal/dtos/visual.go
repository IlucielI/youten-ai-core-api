package dtos

import (
	"time"

	"github.com/google/uuid"
)

// KeyframeExtractionOptions specifies configuration parameters for video keyframe extraction.
type KeyframeExtractionOptions struct {
	SceneThreshold     float64   // e.g. 0.25 (FFmpeg scene change score)
	MinIntervalSeconds float64   // e.g. 5.0s minimum gap between consecutive frames
	MaxFramesPerHour   int       // e.g. 60 frames max per hour
	TargetWidth        int       // e.g. 1280 (720p width)
	Quality            int       // e.g. 80
	AudioAnchors       []float64 // specific timestamp anchors from audio transcript
}

// ExtractedKeyframe represents an individual raw video frame extracted from media stream.
type ExtractedKeyframe struct {
	TimestampSeconds float64
	ImageBytes       []byte
	MimeType         string // image/webp or image/jpeg
	Width            int
	Height           int
	DHash            uint64
	SequenceOrder    int
}

// KeyframeBatchInput represents input to a batch multimodal vision analysis call.
type KeyframeBatchInput struct {
	TimestampSeconds float64
	ImageBytes       []byte
	MimeType         string
	SequenceOrder    int
}

// KeyframeAnalysisResult represents the AI extraction output for a single keyframe.
type KeyframeAnalysisResult struct {
	SequenceOrder   int                    `json:"sequence_order"`
	TimestampSeconds float64               `json:"timestamp_seconds"`
	CueType         string                 `json:"cue_type"`
	Title           string                 `json:"title"`
	ExtractedText   string                 `json:"extracted_text"`
	VisualSummary   string                 `json:"visual_summary"`
	VisibleSpeakers []string               `json:"visible_speakers,omitempty"`
	ActiveSpeaker   string                 `json:"active_speaker,omitempty"`
	MeetingPlatform string                 `json:"meeting_platform,omitempty"`
	Confidence      float64                `json:"confidence"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// VideoKeyframeResponse represents client-facing keyframe representation.
type VideoKeyframeResponse struct {
	ID                 uuid.UUID              `json:"id"`
	RecordingID        uuid.UUID              `json:"recording_id"`
	TimestampSeconds   float64                `json:"timestamp_seconds"`
	TimestampFormatted string                 `json:"timestamp_formatted"`
	S3ImageURL         string                 `json:"s3_image_url"`
	CueType            string                 `json:"cue_type"`
	Title              string                 `json:"title"`
	ExtractedText      string                 `json:"extracted_text"`
	VisualSummary      string                 `json:"visual_summary"`
	Metadata           map[string]interface{} `json:"metadata,omitempty"`
	Confidence         float64                `json:"confidence"`
	SequenceOrder      int                    `json:"sequence_order"`
	CreatedAt          time.Time              `json:"created_at"`
}

