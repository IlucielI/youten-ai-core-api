package payload

import (
	"time"

	"github.com/google/uuid"
)

// AdminAuditPayload represents an administrative audit log event dispatched via message broker.
type AdminAuditPayload struct {
	AdminID   *uuid.UUID     `json:"admin_id,omitempty"`
	Action    string         `json:"action"`
	Entity    string         `json:"entity"`
	EntityID  *string        `json:"entity_id,omitempty"`
	Payload   map[string]any `json:"payload,omitempty"`
	IPAddress *string        `json:"ip_address,omitempty"`
	UserAgent *string        `json:"user_agent,omitempty"`
	CreatedAt time.Time      `json:"created_at,omitempty"`
}
