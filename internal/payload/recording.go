package payload

import "github.com/google/uuid"

// RecordingPipelinePayload represents the standard message body dispatched across pipeline worker stages.
type RecordingPipelinePayload struct {
	RecordingID uuid.UUID `json:"recording_id"`
	SourcePath  string    `json:"source_path,omitempty"` // Uploaded media file path (S3 object key)
	AudioPath   string    `json:"audio_path,omitempty"`  // Extracted mono audio path (S3 object key)
	Template    string    `json:"template,omitempty"`    // Selected template key (e.g. MOM, GENERAL)
	Language    string    `json:"language,omitempty"`    // Output language code
	Stage       string    `json:"stage,omitempty"`       // Pipeline stage
	Attempt     int       `json:"attempt,omitempty"`     // Retry attempt number
}
