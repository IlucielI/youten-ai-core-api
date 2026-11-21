package models

import (
	"encoding/json"

	"github.com/google/uuid"
)

// WorkspaceChatSession represents a persistent multi-turn conversation session
// scoped to either an entire workspace or a specific folder tree within it.
type WorkspaceChatSession struct {
	BaseModel
	WorkspaceID uuid.UUID              `gorm:"type:uuid;not null;index:idx_ws_chat_sessions_ws_user,priority:1" json:"workspace_id"`
	Workspace   *Workspace             `gorm:"foreignKey:WorkspaceID;constraint:OnDelete:CASCADE" json:"workspace,omitempty"`
	FolderID    *uuid.UUID             `gorm:"type:uuid;index:idx_ws_chat_sessions_folder" json:"folder_id,omitempty"`
	Folder      *Folder                `gorm:"foreignKey:FolderID;constraint:OnDelete:CASCADE" json:"folder,omitempty"`
	UserID      uuid.UUID              `gorm:"type:uuid;not null;index:idx_ws_chat_sessions_ws_user,priority:2" json:"user_id"`
	User        *User                  `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"user,omitempty"`
	Title       string                 `gorm:"type:varchar(255);not null;default:'New Chat'" json:"title"`
	Messages    []WorkspaceChatMessage `gorm:"foreignKey:SessionID;constraint:OnDelete:CASCADE" json:"messages,omitempty"`
}

// TableName returns the table name for WorkspaceChatSession.
func (WorkspaceChatSession) TableName() string {
	return "workspace_chat_sessions"
}

// WorkspaceChatMessage represents a single dialogue turn in a workspace/folder chat session.
type WorkspaceChatMessage struct {
	BaseModel
	SessionID         uuid.UUID             `gorm:"type:uuid;not null;index:idx_ws_chat_messages_session,priority:1" json:"session_id"`
	Session           *WorkspaceChatSession `gorm:"foreignKey:SessionID;constraint:OnDelete:CASCADE" json:"session,omitempty"`
	SenderRole        string                `gorm:"type:varchar(20);not null" json:"sender_role"` // 'user' | 'assistant'
	Content           string                `gorm:"type:text;not null" json:"content"`
	Citations         json.RawMessage       `gorm:"type:jsonb;not null;default:'[]'" json:"citations"`
	RetrievedChunkIDs json.RawMessage       `gorm:"type:jsonb;not null;default:'[]'" json:"retrieved_chunk_ids"`
}

// TableName returns the table name for WorkspaceChatMessage.
func (WorkspaceChatMessage) TableName() string {
	return "workspace_chat_messages"
}
