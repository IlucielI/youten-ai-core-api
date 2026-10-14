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
