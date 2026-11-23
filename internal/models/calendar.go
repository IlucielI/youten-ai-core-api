package models

import (
	"time"

	"github.com/google/uuid"
)

// UserGoogleCalendarConnection stores a user's Google Calendar OAuth integration credentials and sync configuration.
type UserGoogleCalendarConnection struct {
	BaseModel
	UserID                 uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:uidx_user_google_calendar_user_id" json:"user_id"`
	User                   *User      `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"user,omitempty"`
	GoogleUserID           string     `gorm:"type:varchar(255);not null" json:"google_user_id"`
	GoogleEmail            string     `gorm:"type:varchar(255);not null" json:"google_email"`
	EncryptedAccessToken   string     `gorm:"type:text" json:"-"`
	EncryptedRefreshToken  string     `gorm:"type:text;not null" json:"-"`
	TokenExpiresAt         time.Time  `gorm:"not null" json:"token_expires_at"`
	SyncToken              *string    `gorm:"type:text" json:"sync_token,omitempty"`
	IsAutoRecordEnabled    bool       `gorm:"not null;default:true" json:"is_auto_record_enabled"`
	LeadTimeMinutes        int        `gorm:"not null;default:2" json:"lead_time_minutes"`
	LastSyncedAt           *time.Time `json:"last_synced_at,omitempty"`
	Status                 string     `gorm:"type:varchar(50);not null;default:'active';index:idx_user_google_calendar_status" json:"status"`
}

// TableName returns the table name for UserGoogleCalendarConnection.
func (UserGoogleCalendarConnection) TableName() string {
	return "user_google_calendar_connections"
}

// ScheduledCalendarEvent stores synchronized calendar events, detected meeting links, and bot dispatch lifecycle state.
type ScheduledCalendarEvent struct {
	BaseModel
	UserID              uuid.UUID   `gorm:"type:uuid;not null;index:idx_sched_cal_events_user_time,priority:1" json:"user_id"`
	User                *User       `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"user,omitempty"`
	GoogleEventID       string      `gorm:"type:varchar(255);not null;uniqueIndex:uidx_user_scheduled_google_event,priority:2" json:"google_event_id"`
	Title               string      `gorm:"type:varchar(500);not null" json:"title"`
	Description         *string     `gorm:"type:text" json:"description,omitempty"`
	StartTime           time.Time   `gorm:"not null;index:idx_sched_cal_events_user_time,priority:2" json:"start_time"`
	EndTime             time.Time   `gorm:"not null" json:"end_time"`
	ScheduledDispatchAt time.Time   `gorm:"not null;index:idx_sched_cal_events_dispatch,priority:1" json:"scheduled_dispatch_at"`
	MeetingURL          string      `gorm:"type:text;not null" json:"meeting_url"`
	MeetingPlatform     string      `gorm:"type:varchar(50);not null" json:"meeting_platform"`
	OrganizerEmail      *string     `gorm:"type:varchar(255)" json:"organizer_email,omitempty"`
	UserResponseStatus  string      `gorm:"type:varchar(50);not null;default:'accepted'" json:"user_response_status"`
	AutoRecordEnabled   bool        `gorm:"not null;default:true;index:idx_sched_cal_events_dispatch,priority:3" json:"auto_record_enabled"`
	DispatchStatus      string      `gorm:"type:varchar(50);not null;default:'PENDING';index:idx_sched_cal_events_dispatch,priority:2" json:"dispatch_status"`
	BotSessionID        *uuid.UUID  `gorm:"type:uuid" json:"bot_session_id,omitempty"`
	BotSession          *BotSession `gorm:"foreignKey:BotSessionID;constraint:OnDelete:SET NULL" json:"bot_session,omitempty"`
	RecordingID         *uuid.UUID  `gorm:"type:uuid" json:"recording_id,omitempty"`
	Recording           *Recording  `gorm:"foreignKey:RecordingID;constraint:OnDelete:SET NULL" json:"recording,omitempty"`
	ErrorMessage        *string     `gorm:"type:text" json:"error_message,omitempty"`
}

// TableName returns the table name for ScheduledCalendarEvent.
func (ScheduledCalendarEvent) TableName() string {
	return "scheduled_calendar_events"
}

