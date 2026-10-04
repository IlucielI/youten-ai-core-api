package models

import (
	"time"

	"github.com/google/uuid"
)

// Report represents an abuse moderation report ticket for administrative review.
type Report struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	RecordingID    uuid.UUID  `gorm:"type:uuid;not null;index:idx_reports_recording_id" json:"recording_id"`
	Recording      *Recording `gorm:"foreignKey:RecordingID;constraint:OnDelete:CASCADE" json:"recording,omitempty"`
	ReporterType   string     `gorm:"type:varchar(50);not null;default:'guest'" json:"reporter_type"` // 'guest' | 'user'
	ReporterRef    *string    `gorm:"type:varchar(255)" json:"reporter_ref,omitempty"`
	Reason         string     `gorm:"type:text;not null" json:"reason"`
	Status         string     `gorm:"type:varchar(50);not null;default:'open';index:idx_reports_status" json:"status"`
	ResolutionNote *string    `gorm:"type:text" json:"resolution_note,omitempty"`
	HandledBy      *uuid.UUID `gorm:"type:uuid" json:"handled_by,omitempty"`
	Handler        *AdminUser `gorm:"foreignKey:HandledBy;constraint:OnDelete:SET NULL" json:"handler,omitempty"`
	CreatedAt      time.Time  `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt      time.Time  `gorm:"not null;default:now()" json:"updated_at"`
}

func (Report) TableName() string {
	return "reports"
}
