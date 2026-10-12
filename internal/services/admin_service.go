package services

import (
	"github.com/google/uuid"
)

// AdminActionMeta carries administrative actor and request provenance for audit logging across all admin service domains.
type AdminActionMeta struct {
	AdminID   uuid.UUID
	IPAddress string
	UserAgent string
}
