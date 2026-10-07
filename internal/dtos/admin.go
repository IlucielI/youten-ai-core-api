package dtos

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

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

// Validate ensures daily quota override is non-negative and bounded.
func (r *AdminUserQuotaOverrideRequest) Validate() error {
	if r.DailyQuotaOverride != nil {
		if *r.DailyQuotaOverride < 0 {
			return errors.New("daily_quota_override must be non-negative or null")
		}
		if *r.DailyQuotaOverride > 10000 {
			return errors.New("daily_quota_override cannot exceed 10000")
		}
	}
	return nil
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

// AdminRoleListResponse encapsulates a list of administrative roles.
type AdminRoleListResponse struct {
	Items []AdminRoleItem `json:"items"`
}

// AdminCreateRoleRequest defines the payload for creating a new administrative RBAC role.
type AdminCreateRoleRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

// Validate ensures role creation payload meets domain invariants.
func (r *AdminCreateRoleRequest) Validate() error {
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		return errors.New("role name is required")
	}
	if len(r.Name) > 100 {
		return errors.New("role name cannot exceed 100 characters")
	}
	r.Description = strings.TrimSpace(r.Description)
	if len(r.Description) > 500 {
		return errors.New("role description cannot exceed 500 characters")
	}
	if len(r.Permissions) == 0 {
		return errors.New("at least one permission is required")
	}
	for i, p := range r.Permissions {
		r.Permissions[i] = strings.TrimSpace(p)
		if r.Permissions[i] == "" {
			return errors.New("empty permission item is not allowed")
		}
	}
	return nil
}

// AdminUpdateRoleRequest defines the payload for updating an administrative RBAC role.
type AdminUpdateRoleRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

// Validate ensures role update payload meets domain invariants.
func (r *AdminUpdateRoleRequest) Validate() error {
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		return errors.New("role name is required")
	}
	if len(r.Name) > 100 {
		return errors.New("role name cannot exceed 100 characters")
	}
	r.Description = strings.TrimSpace(r.Description)
	if len(r.Description) > 500 {
		return errors.New("role description cannot exceed 500 characters")
	}
	if len(r.Permissions) == 0 {
		return errors.New("at least one permission is required")
	}
	for i, p := range r.Permissions {
		r.Permissions[i] = strings.TrimSpace(p)
		if r.Permissions[i] == "" {
			return errors.New("empty permission item is not allowed")
		}
	}
	return nil
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

// AdminTemplateListResponse encapsulates a list of prompt templates.
type AdminTemplateListResponse struct {
	Items []AdminTemplateItem `json:"items"`
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

// Validate ensures template creation payload meets domain invariants and Draft-07 JSON schema rules.
func (r *AdminCreateTemplateRequest) Validate() error {
	r.CategoryKey = strings.TrimSpace(r.CategoryKey)
	if r.CategoryKey == "" {
		return errors.New("category_key is required")
	}
	if len(r.CategoryKey) > 100 {
		return errors.New("category_key cannot exceed 100 characters")
	}
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		return errors.New("template name is required")
	}
	if len(r.Name) > 255 {
		return errors.New("template name cannot exceed 255 characters")
	}
	r.Description = strings.TrimSpace(r.Description)
	r.Prompt = strings.TrimSpace(r.Prompt)
	if r.Prompt == "" {
		return errors.New("prompt is required")
	}

	if len(r.OutputSchema) == 0 {
		return errors.New("output_schema is required")
	}

	// Validate Draft-07 JSON schema structure
	schemaType, ok := r.OutputSchema["type"].(string)
	if !ok || schemaType != "object" {
		return errors.New("output_schema must have type 'object'")
	}
	props, ok := r.OutputSchema["properties"].(map[string]interface{})
	if !ok || len(props) == 0 {
		return errors.New("output_schema must define non-empty 'properties'")
	}

	return nil
}

// AdminUpdateTemplateRequest defines the payload for updating an existing prompt template.
type AdminUpdateTemplateRequest struct {
	Name         string                 `json:"name"`
	Description  string                 `json:"description"`
	Prompt       string                 `json:"prompt"`
	OutputSchema map[string]interface{} `json:"output_schema"`
	IsActive     *bool                  `json:"is_active"`
}

// Validate ensures update payload meets domain invariants and Draft-07 JSON schema rules.
func (r *AdminUpdateTemplateRequest) Validate() error {
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		return errors.New("template name is required")
	}
	if len(r.Name) > 255 {
		return errors.New("template name cannot exceed 255 characters")
	}
	r.Description = strings.TrimSpace(r.Description)
	r.Prompt = strings.TrimSpace(r.Prompt)
	if r.Prompt == "" {
		return errors.New("prompt is required")
	}

	if len(r.OutputSchema) == 0 {
		return errors.New("output_schema is required")
	}

	// Validate Draft-07 JSON schema structure
	schemaType, ok := r.OutputSchema["type"].(string)
	if !ok || schemaType != "object" {
		return errors.New("output_schema must have type 'object'")
	}
	props, ok := r.OutputSchema["properties"].(map[string]interface{})
	if !ok || len(props) == 0 {
		return errors.New("output_schema must define non-empty 'properties'")
	}

	return nil
}

// AdminTestTemplateRequest defines the payload for testing prompt templates in sandbox.
type AdminTestTemplateRequest struct {
	Prompt           string                 `json:"prompt"`
	OutputSchema     map[string]interface{} `json:"output_schema"`
	SampleTranscript string                 `json:"sample_transcript"`
}

// Validate ensures sandbox test payload has necessary inputs.
func (r *AdminTestTemplateRequest) Validate() error {
	r.Prompt = strings.TrimSpace(r.Prompt)
	if r.Prompt == "" {
		return errors.New("prompt is required")
	}
	if len(r.Prompt) > 20000 {
		return errors.New("prompt exceeds maximum length of 20000 characters")
	}
	r.SampleTranscript = strings.TrimSpace(r.SampleTranscript)
	if r.SampleTranscript == "" {
		return errors.New("sample_transcript is required")
	}
	if len(r.SampleTranscript) > 100000 {
		return errors.New("sample_transcript exceeds maximum length of 100000 characters")
	}
	if len(r.OutputSchema) == 0 {
		return errors.New("output_schema is required")
	}

	schemaType, ok := r.OutputSchema["type"].(string)
	if !ok || schemaType != "object" {
		return errors.New("output_schema must have type 'object'")
	}
	props, ok := r.OutputSchema["properties"].(map[string]interface{})
	if !ok || len(props) == 0 {
		return errors.New("output_schema must define non-empty 'properties'")
	}

	return nil
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

// Validate ensures recording_id is provided and optional stage is valid.
func (r *DLQRetryRequest) Validate() error {
	if r.RecordingID == uuid.Nil {
		return errors.New("recording_id is required")
	}
	r.Stage = strings.ToUpper(strings.TrimSpace(r.Stage))
	if r.Stage != "" {
		switch r.Stage {
		case "QUEUED", "EXTRACTING", "TRANSCRIBING", "SUMMARIZING", "INDEXING":
			// valid
		default:
			return errors.New("invalid stage, allowed values: QUEUED, EXTRACTING, TRANSCRIBING, SUMMARIZING, INDEXING")
		}
	}
	return nil
}

// DLQRetryResponse returns details of the re-initiated pipeline job.
type DLQRetryResponse struct {
	RecordingID uuid.UUID `json:"recording_id"`
	Status      string    `json:"status"`
	Stage       string    `json:"stage"`
	Message     string    `json:"message"`
	RetriedAt   time.Time `json:"retried_at"`
}






