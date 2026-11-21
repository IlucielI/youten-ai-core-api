package models

import (
	"time"

	"github.com/google/uuid"
)

// CommentMention represents a user tagged with @mention in an inline comment.
type CommentMention struct {
	ID              uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CommentID       uuid.UUID      `gorm:"type:uuid;not null;index:idx_comment_mentions_comment_id" json:"comment_id"`
	Comment         *InlineComment `gorm:"foreignKey:CommentID" json:"comment,omitempty"`
	MentionedUserID uuid.UUID      `gorm:"type:uuid;not null;index:idx_comment_mentions_user" json:"mentioned_user_id"`
	MentionedUser   *User          `gorm:"foreignKey:MentionedUserID" json:"mentioned_user,omitempty"`
	CreatedAt       time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
}

// TableName returns the table name for GORM.
func (CommentMention) TableName() string {
	return "comment_mentions"
}

