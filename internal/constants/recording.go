package constants

// Template category keys for structured summarization output.
const (
	TemplateKeyMOM            = "MOM"
	TemplateKeyOneOnOne       = "1_ON_1"
	TemplateKeyInterview      = "INTERVIEW"
	TemplateKeyTechReview     = "TECH_REVIEW"
	TemplateKeySalesDiscovery = "SALES_DISCOVERY"
	TemplateKeyDailyStandup   = "DAILY_STANDUP"
	TemplateKeyGeneral        = "GENERAL"
)

// DefaultTemplateKey is the fallback template applied when none is specified.
const DefaultTemplateKey = TemplateKeyGeneral

// DefaultSpeakerLabel is the placeholder label assigned to a speaker before diarization renaming.
const DefaultSpeakerLabel = "Speaker 0"

// Chat message sender roles.
const (
	ChatRoleUser      = "user"
	ChatRoleAssistant = "assistant"
)

// Meeting voice bot waitlist default attributes and status.
const (
	DefaultBotPlatform    = "google_meet"
	DefaultBotCompanySize = "1-10"

	WaitlistStatusPending  = "PENDING"
	WaitlistStatusApproved = "APPROVED"
	WaitlistStatusRejected = "REJECTED"
)

// Recording media source types.
const (
	RecordingSourceTypeUpload = "UPLOAD"
	RecordingSourceTypeLink   = "LINK"
)

// Highlight origin sources.
const (
	HighlightSourceManual      = "manual"
	HighlightSourceAI          = "ai"
	HighlightSourceAISuggested = "AI_SUGGESTED"
)

// Notification types for in-app milestone alerts and system notifications.
const (
	NotificationTypeRecordingCompleted = "RECORDING_COMPLETED"
	NotificationTypeRecordingFailed    = "RECORDING_FAILED"
)

// ExportFormat enumerates the supported MOM/transcript export formats.
type ExportFormat string

const (
	ExportFormatMarkdown ExportFormat = "markdown"
	ExportFormatTxt      ExportFormat = "txt"
	ExportFormatJSON     ExportFormat = "json"
	ExportFormatPDF      ExportFormat = "pdf"
)
