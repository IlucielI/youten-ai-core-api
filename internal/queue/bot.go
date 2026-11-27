package queue

import (
	"time"

	"github.com/google/uuid"
)

// BotDispatchPayload represents the strongly-typed message contract dispatched to meeting bot runners.
type BotDispatchPayload struct {
	SessionID         uuid.UUID  `json:"session_id"`
	RecordingID       uuid.UUID  `json:"recording_id"`
	Provider          string     `json:"provider"`
	MeetingURL        string     `json:"meeting_url"`
	ExternalSessionID string     `json:"external_session_id"`
	DispatchedAt      time.Time  `json:"dispatched_at"`
	ScheduledAt       *time.Time `json:"scheduled_at,omitempty"`
}

// BotStopPayload represents the command message dispatched to signal a bot to leave the meeting.
type BotStopPayload struct {
	SessionID         uuid.UUID `json:"session_id,omitempty"`
	ExternalSessionID string    `json:"external_session_id"`
	Reason            string    `json:"reason,omitempty"`
	StoppedAt         time.Time `json:"stopped_at"`
}
