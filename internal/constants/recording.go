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

// Meeting voice bot waitlist default attributes.
const (
	DefaultBotPlatform    = "google_meet"
	DefaultBotCompanySize = "1-10"
)

// ExportFormat enumerates the supported MOM/transcript export formats.
type ExportFormat string

const (
	ExportFormatMarkdown ExportFormat = "markdown"
	ExportFormatTxt      ExportFormat = "txt"
	ExportFormatJSON     ExportFormat = "json"
	ExportFormatPDF      ExportFormat = "pdf"
)
