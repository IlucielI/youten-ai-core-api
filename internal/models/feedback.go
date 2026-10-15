package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// FeedbackAttachment represents an attached file or screenshot metadata in user feedback.
type FeedbackAttachment struct {
	URL           string `json:"url"`
	Filename      string `json:"filename"`
	ContentType   string `json:"content_type"`
	FileSizeBytes int64  `json:"file_size_bytes"`
}

// FeedbackAttachments is a custom GORM data type for handling a slice of FeedbackAttachment in JSONB.
type FeedbackAttachments []FeedbackAttachment

// Value serializes the FeedbackAttachments into JSON driver.Value for SQL storage.
func (a FeedbackAttachments) Value() (driver.Value, error) {
	if a == nil {
		return "[]", nil
	}
	bytes, err := json.Marshal(a)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal FeedbackAttachments: %w", err)
	}
	return string(bytes), nil
}

// Scan deserializes a database driver value into FeedbackAttachments.
func (a *FeedbackAttachments) Scan(value interface{}) error {
	if value == nil {
		*a = make(FeedbackAttachments, 0)
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return fmt.Errorf("unsupported type for FeedbackAttachments: %T", value)
	}

	if len(bytes) == 0 {
		*a = make(FeedbackAttachments, 0)
		return nil
	}

	var res []FeedbackAttachment
	if err := json.Unmarshal(bytes, &res); err != nil {
		return fmt.Errorf("failed to unmarshal FeedbackAttachments: %w", err)
	}

	*a = res
	return nil
}

// Feedback represents a user feedback, bug report, feature request, or suggestion.
type Feedback struct {
	ID             uuid.UUID            `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	UserID         *uuid.UUID           `gorm:"type:uuid;index:idx_feedbacks_user_id" json:"user_id,omitempty"`
	User           *User                `gorm:"foreignKey:UserID;constraint:OnDelete:SET NULL" json:"user,omitempty"`
	UserName       *string              `gorm:"type:varchar(150)" json:"user_name,omitempty"`
	UserEmail      *string              `gorm:"type:varchar(255)" json:"user_email,omitempty"`
	Title          string               `gorm:"type:varchar(255);not null" json:"title"`
	Category       string               `gorm:"type:varchar(50);not null;default:'general';index:idx_feedbacks_category" json:"category"`
	Rating         int                  `gorm:"type:int;not null;default:5" json:"rating"`
	Message        string               `gorm:"type:text;not null" json:"message"`
	Status         string               `gorm:"type:varchar(50);not null;default:'new'" json:"status"`
	ClientIP       *string              `gorm:"type:varchar(45)" json:"client_ip,omitempty"`
	UserAgent      *string              `gorm:"type:text" json:"user_agent,omitempty"`
	IdempotencyKey *string              `gorm:"type:varchar(100);uniqueIndex:uidx_feedbacks_idempotency_key" json:"idempotency_key,omitempty"`
	Attachments    FeedbackAttachments  `gorm:"type:jsonb;default:'[]'" json:"attachments,omitempty"`
	Metadata       JSONMap              `gorm:"type:jsonb;default:'{}'" json:"metadata,omitempty"`
	CreatedAt      time.Time            `gorm:"not null;default:now();index:idx_feedbacks_created_at,sort:desc" json:"created_at"`
	UpdatedAt      time.Time            `gorm:"not null;default:now()" json:"updated_at"`
}

func (Feedback) TableName() string {
	return "feedbacks"
}
