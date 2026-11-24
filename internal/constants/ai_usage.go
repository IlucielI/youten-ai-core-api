package constants

// AIServiceType defines domain types of AI pipeline consumption.
type AIServiceType string

const (
	AIServiceTypeLLMSummary    AIServiceType = "llm_summary"
	AIServiceTypeRecordingChat AIServiceType = "recording_chat"
	AIServiceTypeWorkspaceChat AIServiceType = "workspace_chat"
	AIServiceTypeEmbedding     AIServiceType = "embedding"
	AIServiceTypeSTT           AIServiceType = "stt"
	AIServiceTypeDiarization   AIServiceType = "diarization"
	AIServiceTypeVision        AIServiceType = "vision"
)

func (s AIServiceType) String() string {
	return string(s)
}

// ScopeAdminAIUsageRead defines permission to view AI token and model consumption audit logs.
const ScopeAdminAIUsageRead Scope = "audit:read"
