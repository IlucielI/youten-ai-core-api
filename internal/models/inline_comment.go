package models

import (
	"time"

	"github.com/google/uuid"
)

// InlineComment represents code-review style timestamped comments and threaded discussions.
type InlineComment struct {
	ID           uuid.UUID            `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	RecordingID  uuid.UUID            `gorm:"type:uuid;not null;index:idx_comments_recording_time,priority:1" json:"recording_id"`
	Recording    *Recording           `gorm:"foreignKey:RecordingID;constraint:OnDelete:CASCADE" json:"recording,omitempty"`
	SegmentID    *uuid.UUID           `gorm:"type:uuid" json:"segment_id,omitempty"`
	Segment      *TranscriptSegment   `gorm:"foreignKey:SegmentID;constraint:OnDelete:SET NULL" json:"segment,omitempty"`
	TimestampSec float64              `gorm:"not null;default:0;index:idx_comments_recording_time,priority:2" json:"timestamp_sec"`
	SelectedText *string              `gorm:"type:text" json:"selected_text,omitempty"`
	AuthorName   string               `gorm:"type:varchar(100);not null" json:"author_name"`
	CommentText  string               `gorm:"type:text;not null" json:"comment_text"`
	ParentID     *uuid.UUID           `gorm:"type:uuid;index:idx_comments_parent_id" json:"parent_id,omitempty"`
	Replies      []InlineComment      `gorm:"foreignKey:ParentID;constraint:OnDelete:CASCADE" json:"replies,omitempty"`
	CreatedAt    time.Time            `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt    time.Time            `gorm:"not null;default:now()" json:"updated_at"`
}

func (InlineComment) TableName() string {
	return "inline_comments"
}
