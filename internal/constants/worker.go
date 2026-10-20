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

	// Administrative domain events
	TopicAdminAuditLog = "admin.audit.created"

	// Domain event alias
	DomainEventRecordingCreated = TopicRecordingUploaded

	// Dead-letter and fallback pipeline queue/topic identifiers
	QueueRecordingDLQ    = "recording.dlq"
	TopicPipelineUnknown = "pipeline.unknown"
	QueuePipelineUnknown = "pipeline.unknown"

	// Meeting voice bot orchestration topics
	TopicBotGoogleMeetDispatch = "bot.google_meet.dispatch"
	TopicBotGoogleMeetStop     = "bot.google_meet.stop"
)
