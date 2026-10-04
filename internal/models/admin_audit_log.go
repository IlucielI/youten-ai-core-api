package models

import (
	"time"

	"github.com/google/uuid"
)

// AdminAuditLog records internal staff administrative actions.
type AdminAuditLog struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	AdminID   *uuid.UUID `gorm:"type:uuid;index:idx_admin_audit_logs_admin_id" json:"admin_id,omitempty"`
	Admin     *AdminUser `gorm:"foreignKey:AdminID;constraint:OnDelete:SET NULL" json:"admin,omitempty"`
	Action    string     `gorm:"type:varchar(100);not null;index:idx_admin_audit_logs_action" json:"action"`
	Entity    string     `gorm:"type:varchar(100);not null;index:idx_admin_audit_logs_entity,priority:1" json:"entity"`
	EntityID  *string    `gorm:"type:varchar(100);index:idx_admin_audit_logs_entity,priority:2" json:"entity_id,omitempty"`
	Payload   JSONMap    `gorm:"type:jsonb" json:"payload,omitempty"`
	IPAddress *string    `gorm:"type:varchar(100)" json:"ip_address,omitempty"`
	UserAgent *string    `gorm:"type:text" json:"user_agent,omitempty"`
	CreatedAt time.Time  `gorm:"not null;default:now();index:idx_admin_audit_logs_created_at,sort:desc" json:"created_at"`
}

func (AdminAuditLog) TableName() string {
	return "admin_audit_logs"
}
