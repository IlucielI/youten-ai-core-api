package dtos

import (
	"time"

	"github.com/google/uuid"
)

// DispatchBotRequest carries parameters to instruct a voice bot to join a channel or meeting.
type DispatchBotRequest struct {
	Provider    string     `json:"provider"`
	MeetingURL  string     `json:"meeting_url,omitempty"`
	GuildID     string     `json:"guild_id,omitempty"`
	ChannelID   string     `json:"channel_id,omitempty"`
	Title       string     `json:"title,omitempty"`
	Template    string     `json:"template,omitempty"`
	Language    string     `json:"language,omitempty"`
	ScheduledAt *time.Time `json:"scheduled_at,omitempty"`
}

// DispatchBotResponse is returned when a bot session is successfully initialized.
type DispatchBotResponse struct {
	SessionID      uuid.UUID `json:"session_id"`
	RecordingID    uuid.UUID `json:"recording_id"`
	OwnershipToken string    `json:"ownership_token"`
	Provider       string    `json:"provider"`
	Status         string    `json:"status"`
	Message        string    `json:"message"`
}

// BotSessionStatusResponse describes the current live status of a bot session.
type BotSessionStatusResponse struct {
	SessionID    uuid.UUID  `json:"session_id"`
	RecordingID  uuid.UUID  `json:"recording_id"`
	Provider     string     `json:"provider"`
	Status       string     `json:"status"`
	MeetingURL   *string    `json:"meeting_url,omitempty"`
	ChannelID    *string    `json:"channel_id,omitempty"`
	GuildID      *string    `json:"guild_id,omitempty"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	EndedAt      *time.Time `json:"ended_at,omitempty"`
	ErrorMessage *string    `json:"error_message,omitempty"`
}

// StopBotResponse represents the outcome of ordering a bot to leave a channel or meeting.
type StopBotResponse struct {
	SessionID   uuid.UUID `json:"session_id"`
	RecordingID uuid.UUID `json:"recording_id"`
	Status      string    `json:"status"`
	Message     string    `json:"message"`
}

// CapabilitiesResponse describes feature availability across all ingest and bot platforms.
type CapabilitiesResponse struct {
	MeetingBot  map[string]string `json:"meeting_bot"`
	AudioUpload bool              `json:"audio_upload"`
	LinkImport  bool              `json:"link_import"`
}

// GoogleMeetWebhookRequest carries asynchronous status or completion callbacks from a headless bot runner.
type GoogleMeetWebhookRequest struct {
	ExternalSessionID string  `json:"external_session_id"`
	Event             string  `json:"event"`
	AudioURL          *string `json:"audio_url,omitempty"`
	DurationSeconds   *int    `json:"duration_seconds,omitempty"`
	ErrorMessage      *string `json:"error_message,omitempty"`
}

// MSTeamsWebhookRequest carries asynchronous status or completion callbacks from an Azure/MS Teams calling bot worker.
type MSTeamsWebhookRequest struct {
	ExternalSessionID string  `json:"external_session_id"`
	CallConnectionID  string  `json:"call_connection_id,omitempty"`
	Event             string  `json:"event"`
	AudioURL          *string `json:"audio_url,omitempty"`
	DurationSeconds   *int    `json:"duration_seconds,omitempty"`
	ErrorMessage      *string `json:"error_message,omitempty"`
}

// ZoomWebhookRequest carries asynchronous status or completion callbacks from a Zoom bot worker.
type ZoomWebhookRequest struct {
	ExternalSessionID string  `json:"external_session_id"`
	MeetingID         string  `json:"meeting_id,omitempty"`
	Event             string  `json:"event"`
	AudioURL          *string `json:"audio_url,omitempty"`
	DurationSeconds   *int    `json:"duration_seconds,omitempty"`
	ErrorMessage      *string `json:"error_message,omitempty"`
}


