package models

import (
	"time"

	"github.com/google/uuid"
)

// AuditLog tracks sensitive operations performed by guests, registered users, and system processes (Section 13.6).
type AuditLog struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	ActorUserID *uuid.UUID `gorm:"type:uuid;index:idx_audit_logs_actor,priority:2" json:"actor_user_id,omitempty"`
	ActorUser   *User      `gorm:"foreignKey:ActorUserID;constraint:OnDelete:SET NULL" json:"actor_user,omitempty"`
	ActorType   string     `gorm:"type:varchar(50);not null;default:'guest';index:idx_audit_logs_actor,priority:1" json:"actor_type"` // guest | user | admin | system
	Action      string     `gorm:"type:varchar(100);not null;index:idx_audit_logs_action" json:"action"`                               // upload | view | share_enable | claim | delete | etc.
	EntityType  string     `gorm:"type:varchar(100);not null;index:idx_audit_logs_entity,priority:1" json:"entity_type"`              // recording | user | config
	EntityID    *uuid.UUID `gorm:"type:uuid;index:idx_audit_logs_entity,priority:2" json:"entity_id,omitempty"`
	IPAddress   *string    `gorm:"type:varchar(100)" json:"ip_address,omitempty"`
	Metadata    JSONMap    `gorm:"type:jsonb;not null;default:'{}'" json:"metadata"`
	CreatedAt   time.Time  `gorm:"not null;default:now();index:idx_audit_logs_created_at,sort:desc" json:"created_at"`
}

func (AuditLog) TableName() string {
	return "audit_logs"
}
