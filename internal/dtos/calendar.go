package dtos

import (
	"time"

	"github.com/google/uuid"
)

// GoogleCalendarAuthURLResponse returns the OAuth authorization URL.
type GoogleCalendarAuthURLResponse struct {
	AuthURL string `json:"auth_url"`
	State   string `json:"state"`
}

// GoogleCalendarCallbackRequest carries authorization code and state from OAuth redirect.
type GoogleCalendarCallbackRequest struct {
	Code  string `json:"code"`
	State string `json:"state"`
}

// GoogleCalendarConnectionResponse summarizes integration status for the authenticated user.
type GoogleCalendarConnectionResponse struct {
	Connected           bool       `json:"connected"`
	GoogleEmail         string     `json:"google_email,omitempty"`
	IsAutoRecordEnabled bool       `json:"is_auto_record_enabled"`
	LeadTimeMinutes     int        `json:"lead_time_minutes"`
	LastSyncedAt        *time.Time `json:"last_synced_at,omitempty"`
	Status              string     `json:"status"`
}

// UpdateCalendarSettingsRequest carries settings updates for calendar auto-recording.
type UpdateCalendarSettingsRequest struct {
	IsAutoRecordEnabled *bool `json:"is_auto_record_enabled,omitempty"`
	LeadTimeMinutes     *int  `json:"lead_time_minutes,omitempty"`
}

// ToggleEventAutoRecordRequest enables or disables auto-recording for an individual meeting event.
type ToggleEventAutoRecordRequest struct {
	AutoRecordEnabled bool `json:"auto_record_enabled"`
}

// CalendarEventDTO represents a scheduled calendar event response for UI presentation.
type CalendarEventDTO struct {
	ID                  uuid.UUID  `json:"id"`
	GoogleEventID       string     `json:"google_event_id"`
	Title               string     `json:"title"`
	Description         *string    `json:"description,omitempty"`
	StartTime           time.Time  `json:"start_time"`
	EndTime             time.Time  `json:"end_time"`
	ScheduledDispatchAt time.Time  `json:"scheduled_dispatch_at"`
	MeetingURL          string     `json:"meeting_url"`
	MeetingPlatform     string     `json:"meeting_platform"`
	OrganizerEmail      *string    `json:"organizer_email,omitempty"`
	UserResponseStatus  string     `json:"user_response_status"`
	AutoRecordEnabled   bool       `json:"auto_record_enabled"`
	DispatchStatus      string     `json:"dispatch_status"`
	BotSessionID        *uuid.UUID `json:"bot_session_id,omitempty"`
	RecordingID         *uuid.UUID `json:"recording_id,omitempty"`
	ErrorMessage        *string    `json:"error_message,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
}

// ListCalendarEventsQuery carries filtering and pagination parameters for calendar events.
type ListCalendarEventsQuery struct {
	StartDate      *time.Time `form:"start_date" json:"start_date,omitempty"`
	EndDate        *time.Time `form:"end_date" json:"end_date,omitempty"`
	Platform       string     `form:"platform" json:"platform,omitempty"`
	DispatchStatus string     `form:"dispatch_status" json:"dispatch_status,omitempty"`
	Page           int        `form:"page" json:"page"`
	Limit          int        `form:"limit" json:"limit"`
}

// SetDefaults assigns sensible pagination boundaries.
func (q *ListCalendarEventsQuery) SetDefaults() {
	if q.Page < 1 {
		q.Page = 1
	}
	if q.Limit < 1 {
		q.Limit = 20
	} else if q.Limit > 100 {
		q.Limit = 100
	}
}

