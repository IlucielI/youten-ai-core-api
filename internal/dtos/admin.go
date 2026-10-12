package dtos

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// AdminActionMeta carries administrative actor and request provenance for audit logging across all admin service domains.
type AdminActionMeta struct {
	AdminID   uuid.UUID `json:"admin_id"`
	IPAddress string    `json:"ip_address,omitempty"`
	UserAgent string    `json:"user_agent,omitempty"`
}

// AdminUserListQuery defines query parameters for paginating and filtering registered users.
type AdminUserListQuery struct {
	Search string `form:"search" json:"search"`
	Status string `form:"status" json:"status"`
	Page   int    `form:"page" json:"page"`
	Limit  int    `form:"limit" json:"limit"`
}

// SetDefaults ensures fallback defaults for pagination parameters.
func (q *AdminUserListQuery) SetDefaults() {
	q.Search = strings.TrimSpace(q.Search)
	if len(q.Search) > 100 {
		q.Search = q.Search[:100]
	}
	q.Status = strings.TrimSpace(q.Status)

	if q.Page < 1 {
		q.Page = 1
	}
	if q.Limit < 1 {
		q.Limit = 10
	} else if q.Limit > 100 {
		q.Limit = 100
	}
}

// AdminUserListItem represents a user summary item in the administrative user list.
type AdminUserListItem struct {
	ID             uuid.UUID `json:"id"`
	Email          string    `json:"email"`
	FullName       string    `json:"full_name"`
	Status         string    `json:"status"`
	DailyQuota     int       `json:"daily_quota"`
	QuotaUsedToday int       `json:"quota_used_today"`
	EmailVerified  bool      `json:"email_verified"`
	CreatedAt      time.Time `json:"created_at"`
}

// AdminUserListResponse encapsulates the list of registered users and pagination metadata.
type AdminUserListResponse struct {
	Items      []AdminUserListItem `json:"items"`
	Pagination PaginationMeta      `json:"pagination"`
}

// AdminUserQuotaOverrideRequest defines the payload for setting or resetting a user's daily quota override.
type AdminUserQuotaOverrideRequest struct {
	DailyQuotaOverride *int `json:"daily_quota_override"`
}


// AdminUserQuotaResponse represents the result of a daily quota override mutation.
type AdminUserQuotaResponse struct {
	UserID             uuid.UUID `json:"user_id"`
	DailyQuotaOverride *int      `json:"daily_quota_override"`
	EffectiveQuota     int       `json:"effective_quota"`
}

// AdminRevokeUserSessionsResponse represents the outcome of terminating a user's active sessions.
type AdminRevokeUserSessionsResponse struct {
	UserID  uuid.UUID `json:"user_id"`
	Revoked bool      `json:"revoked"`
	Message string    `json:"message"`
}

// AdminRoleItem represents an administrative RBAC role with decoded permissions.
type AdminRoleItem struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Permissions []string  `json:"permissions"`
	IsSystem    bool      `json:"is_system"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// AdminRoleListQuery defines query parameters for paginating and filtering administrative roles.
type AdminRoleListQuery struct {
	Search string `form:"search" json:"search"`
	Page   int    `form:"page" json:"page"`
	Limit  int    `form:"limit" json:"limit"`
}

// SetDefaults ensures fallback defaults for pagination parameters.
func (q *AdminRoleListQuery) SetDefaults() {
	q.Search = strings.TrimSpace(q.Search)
	if len(q.Search) > 100 {
		q.Search = q.Search[:100]
	}

	if q.Page < 1 {
		q.Page = 1
	}
	if q.Limit < 1 {
		q.Limit = 10
	} else if q.Limit > 100 {
		q.Limit = 100
	}
}

// AdminRoleListResponse encapsulates a list of administrative roles and pagination metadata.
type AdminRoleListResponse struct {
	Items      []AdminRoleItem `json:"items"`
	Pagination PaginationMeta  `json:"pagination"`
}

// AdminCreateRoleRequest defines the payload for creating a new administrative RBAC role.
type AdminCreateRoleRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

// AdminUpdateRoleRequest defines the payload for updating an administrative RBAC role.
type AdminUpdateRoleRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

// AdminUserRoleItem represents a customer user role with permissions and daily quota.
type AdminUserRoleItem struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Code        string    `json:"code"`
	Description string    `json:"description"`
	Permissions []string  `json:"permissions"`
	DailyQuota  int       `json:"daily_quota"`
	IsDefault   bool      `json:"is_default"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// AdminUserRoleListQuery defines query parameters for paginating and filtering customer user roles.
type AdminUserRoleListQuery struct {
	Search string `form:"search" json:"search"`
	Page   int    `form:"page" json:"page"`
	Limit  int    `form:"limit" json:"limit"`
}

// SetDefaults ensures fallback defaults for pagination parameters.
func (q *AdminUserRoleListQuery) SetDefaults() {
	q.Search = strings.TrimSpace(q.Search)
	if len(q.Search) > 100 {
		q.Search = q.Search[:100]
	}

	if q.Page < 1 {
		q.Page = 1
	}
	if q.Limit < 1 {
		q.Limit = 10
	} else if q.Limit > 100 {
		q.Limit = 100
	}
}

// AdminUserRoleListResponse encapsulates the list of customer user roles and pagination metadata.
type AdminUserRoleListResponse struct {
	Items      []AdminUserRoleItem `json:"items"`
	Pagination PaginationMeta      `json:"pagination"`
}

// AdminCreateUserRoleRequest defines the payload for creating a customer user role.
type AdminCreateUserRoleRequest struct {
	Name        string   `json:"name"`
	Code        string   `json:"code"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
	DailyQuota  int      `json:"daily_quota"`
	IsDefault   bool     `json:"is_default"`
}

// AdminUpdateUserRoleRequest defines the payload for updating a customer user role.
type AdminUpdateUserRoleRequest struct {
	Name        *string   `json:"name,omitempty"`
	Description *string   `json:"description,omitempty"`
	Permissions *[]string `json:"permissions,omitempty"`
	DailyQuota  *int      `json:"daily_quota,omitempty"`
	IsDefault   *bool     `json:"is_default,omitempty"`
}

// AdminAssignUserRoleRequest defines the payload for assigning a role to a user.
type AdminAssignUserRoleRequest struct {
	RoleID uuid.UUID `json:"role_id"`
}

// AdminTemplateItem represents a prompt template in administrative views.
type AdminTemplateItem struct {
	ID           uuid.UUID              `json:"id"`
	CategoryKey  string                 `json:"category_key"`
	Name         string                 `json:"name"`
	Description  string                 `json:"description"`
	Prompt       string                 `json:"prompt"`
	OutputSchema map[string]interface{} `json:"output_schema"`
	Version      int                    `json:"version"`
	IsActive     bool                   `json:"is_active"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

// AdminTemplateListQuery defines query parameters for paginating and filtering prompt templates.
type AdminTemplateListQuery struct {
	Search string `form:"search" json:"search"`
	Page   int    `form:"page" json:"page"`
	Limit  int    `form:"limit" json:"limit"`
}

// SetDefaults ensures fallback defaults for pagination parameters.
func (q *AdminTemplateListQuery) SetDefaults() {
	q.Search = strings.TrimSpace(q.Search)
	if len(q.Search) > 100 {
		q.Search = q.Search[:100]
	}

	if q.Page < 1 {
		q.Page = 1
	}
	if q.Limit < 1 {
		q.Limit = 10
	} else if q.Limit > 100 {
		q.Limit = 100
	}
}

// AdminTemplateListResponse encapsulates a list of prompt templates and pagination metadata.
type AdminTemplateListResponse struct {
	Items      []AdminTemplateItem `json:"items"`
	Pagination PaginationMeta      `json:"pagination"`
}

// AdminCreateTemplateRequest defines the payload for creating a new prompt template.
type AdminCreateTemplateRequest struct {
	CategoryKey  string                 `json:"category_key"`
	Name         string                 `json:"name"`
	Description  string                 `json:"description"`
	Prompt       string                 `json:"prompt"`
	OutputSchema map[string]interface{} `json:"output_schema"`
	IsActive     *bool                  `json:"is_active"`
}

// AdminUpdateTemplateRequest defines the payload for updating an existing prompt template.
type AdminUpdateTemplateRequest struct {
	Name         string                 `json:"name"`
	Description  string                 `json:"description"`
	Prompt       string                 `json:"prompt"`
	OutputSchema map[string]interface{} `json:"output_schema"`
	IsActive     *bool                  `json:"is_active"`
}

// AdminTestTemplateRequest defines the payload for testing prompt templates in sandbox.
type AdminTestTemplateRequest struct {
	Prompt           string                 `json:"prompt"`
	OutputSchema     map[string]interface{} `json:"output_schema"`
	SampleTranscript string                 `json:"sample_transcript"`
}

// AdminTestTemplateResponse represents the dry run sandbox output and metrics.
type AdminTestTemplateResponse struct {
	RawOutput       string      `json:"raw_output"`
	ParsedJSON      interface{} `json:"parsed_json"`
	ExecutionTimeMs int64       `json:"execution_time_ms"`
	TokensUsed      int         `json:"tokens_used"`
}

// DLQMessageItem represents a single dead-letter or stuck pipeline item.
type DLQMessageItem struct {
	RecordingID  uuid.UUID `json:"recording_id"`
	Queue        string    `json:"queue"`
	RoutingKey   string    `json:"routing_key"`
	Stage        string    `json:"stage"`
	Status       string    `json:"status"`
	ErrorCode    string    `json:"error_code,omitempty"`
	ErrorMessage string    `json:"error_message,omitempty"`
	RetryCount   int       `json:"retry_count"`
	FailedAt     time.Time `json:"failed_at"`
	Title        string    `json:"title"`
	UserName     string    `json:"user_name,omitempty"`
	UserEmail    string    `json:"user_email,omitempty"`
}

// AdminDLQListQuery defines query parameters for paginating and monitoring DLQ messages.
type AdminDLQListQuery struct {
	Page  int `form:"page" json:"page"`
	Limit int `form:"limit" json:"limit"`
}

// SetDefaults ensures fallback defaults for DLQ pagination parameters.
func (q *AdminDLQListQuery) SetDefaults() {
	if q.Page < 1 {
		q.Page = 1
	}
	if q.Limit < 1 {
		q.Limit = 20
	} else if q.Limit > 100 {
		q.Limit = 100
	}
}

// DLQMessagesResponse represents the monitor output for DLQ pipeline messages and stuck recordings.
type DLQMessagesResponse struct {
	Total       int64            `json:"total"`
	FailedCount int64            `json:"failed_count"`
	StuckCount  int64            `json:"stuck_count"`
	Page        int              `json:"page"`
	Limit       int              `json:"limit"`
	Items       []DLQMessageItem `json:"items"`
}

// DLQRetryRequest defines the payload for an admin to reprocess a dead-lettered or stuck pipeline job.
type DLQRetryRequest struct {
	RecordingID uuid.UUID `json:"recording_id"`
	Stage       string    `json:"stage,omitempty"`
}


// DLQRetryResponse returns details of the re-initiated pipeline job.
type DLQRetryResponse struct {
	RecordingID uuid.UUID `json:"recording_id"`
	Status      string    `json:"status"`
	Stage       string    `json:"stage"`
	Message     string    `json:"message"`
	RetriedAt   time.Time `json:"retried_at"`
}

// SystemConfigResponse represents active system feature flags and maintenance settings.
type SystemConfigResponse struct {
	MaintenanceMode    bool `json:"maintenance_mode"`
	AllowGuestUploads  bool `json:"allow_guest_uploads"`
	BotWaitlistEnabled bool `json:"bot_waitlist_enabled"`
}

// UpdateSystemConfigRequest defines payload for updating system flags.
type UpdateSystemConfigRequest struct {
	MaintenanceMode    *bool `json:"maintenance_mode,omitempty"`
	AllowGuestUploads  *bool `json:"allow_guest_uploads,omitempty"`
	BotWaitlistEnabled *bool `json:"bot_waitlist_enabled,omitempty"`
}


// AdminReportListQuery defines filter and pagination parameters for the abuse reports queue.
type AdminReportListQuery struct {
	Status string `form:"status"`
	Page   int    `form:"page"`
	Limit  int    `form:"limit"`
}

// SetDefaults sanitizes pagination values and status filter.
func (q *AdminReportListQuery) SetDefaults() {
	if q.Page < 1 {
		q.Page = 1
	}
	if q.Limit < 1 {
		q.Limit = 10
	}
	if q.Limit > 100 {
		q.Limit = 100
	}
	q.Status = strings.ToLower(strings.TrimSpace(q.Status))
}

// AdminReportListItem contains summarized abuse report and target metadata.
type AdminReportListItem struct {
	ID             uuid.UUID  `json:"id"`
	RecordingID    uuid.UUID  `json:"recording_id"`
	RecordingTitle string     `json:"recording_title,omitempty"`
	ReporterType   string     `json:"reporter_type"`
	ReporterRef    *string    `json:"reporter_ref,omitempty"`
	Reason         string     `json:"reason"`
	Status         string     `json:"status"`
	ResolutionNote *string    `json:"resolution_note,omitempty"`
	HandledBy      *uuid.UUID `json:"handled_by,omitempty"`
	HandlerName    *string    `json:"handler_name,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// AdminReportListResponse represents a paginated list of moderation reports.
type AdminReportListResponse struct {
	Items      []AdminReportListItem `json:"items"`
	Pagination PaginationMeta        `json:"pagination"`
}

// ResolveReportRequest defines moderation action on an abuse ticket.
type ResolveReportRequest struct {
	Action         string `json:"action"` // DISMISS | SUSPEND_RECORDING | BAN_USER
	ResolutionNote string `json:"resolution_note"`
}


// ResolveReportResponse represents outcome of report resolution.
type ResolveReportResponse struct {
	ReportID       uuid.UUID `json:"report_id"`
	Status         string    `json:"status"` // resolved | dismissed
	Action         string    `json:"action"`
	HandledBy      uuid.UUID `json:"handled_by"`
	ResolutionNote string    `json:"resolution_note,omitempty"`
	ResolvedAt     time.Time `json:"resolved_at"`
}

// AdminAuditLogListQuery defines filter criteria for querying staff audit logs.
type AdminAuditLogListQuery struct {
	Action  string     `form:"action"`
	Entity  string     `form:"entity"`
	AdminID *uuid.UUID `form:"admin_id"`
	Page    int        `form:"page"`
	Limit   int        `form:"limit"`
}

// SetDefaults sanitizes pagination values and filters.
func (q *AdminAuditLogListQuery) SetDefaults() {
	if q.Page < 1 {
		q.Page = 1
	}
	if q.Limit < 1 {
		q.Limit = 20
	}
	if q.Limit > 100 {
		q.Limit = 100
	}
	q.Action = strings.TrimSpace(q.Action)
	q.Entity = strings.TrimSpace(q.Entity)
}

// AdminAuditLogListItem represents a single administrative audit event.
type AdminAuditLogListItem struct {
	ID            uuid.UUID              `json:"id"`
	AdminID       *uuid.UUID             `json:"admin_id,omitempty"`
	AdminUsername *string                `json:"admin_username,omitempty"`
	AdminFullName *string                `json:"admin_full_name,omitempty"`
	Action        string                 `json:"action"`
	Entity        string                 `json:"entity"`
	EntityID      *string                `json:"entity_id,omitempty"`
	Payload       map[string]interface{} `json:"payload,omitempty"`
	IPAddress     *string                `json:"ip_address,omitempty"`
	UserAgent     *string                `json:"user_agent,omitempty"`
	CreatedAt     time.Time              `json:"created_at"`
}

// AdminAuditLogListResponse represents a paginated audit trail response.
type AdminAuditLogListResponse struct {
	Items      []AdminAuditLogListItem `json:"items"`
	Pagination PaginationMeta          `json:"pagination"`
}

// SystemOverviewStatsResponse provides system-wide operational metrics.
type SystemOverviewStatsResponse struct {
	TotalUsers           int64   `json:"total_users"`
	ActiveUsers          int64   `json:"active_users"`
	TotalRecordings      int64   `json:"total_recordings"`
	CompletedRecordings  int64   `json:"completed_recordings"`
	FailedRecordings     int64   `json:"failed_recordings"`
	TotalStorageBytes    int64   `json:"total_storage_bytes"`
	TotalDurationSeconds float64 `json:"total_duration_seconds"`
}

// CostOversightResponse provides cost estimation and consumption metrics.
type CostOversightResponse struct {
	TotalAudioMinutes     float64 `json:"total_audio_minutes"`
	EstimatedSTTCostUSD   float64 `json:"estimated_stt_cost_usd"`
	TotalLLMTokens        int64   `json:"total_llm_tokens"`
	EstimatedLLMCostUSD   float64 `json:"estimated_llm_cost_usd"`
	TotalEstimatedCostUSD float64 `json:"total_estimated_cost_usd"`
	Currency              string  `json:"currency"`
}











