package constants

// Domain event topics for background worker orchestration
const (
	TopicRecordingUploaded   = "recording.uploaded"
	TopicRecordingExtract    = "recording.extract"
	TopicRecordingTranscribe = "recording.transcribe"
	TopicRecordingSummarize  = "recording.summarize"
	TopicRecordingIndex      = "recording.index"
	TopicRecordingAnalytics  = "recording.analytics"
	TopicRecordingChapterize = "recording.chapterize"
	TopicRecordingImport     = "recording.import"
	TopicRecordingCompleted  = "recording.completed"
	TopicRecordingFailed     = "recording.failed"

	// Domain event alias
	DomainEventRecordingCreated = TopicRecordingUploaded
)
