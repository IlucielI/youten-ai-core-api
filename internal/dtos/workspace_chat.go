package dtos

import (
	"time"

	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/google/uuid"
)

// CreateWorkspaceChatSessionRequest defines the payload to initiate a new chat session.
type CreateWorkspaceChatSessionRequest struct {
	FolderID *uuid.UUID `json:"folder_id,omitempty"`
	Title    string     `json:"title,omitempty"`
}

func (r CreateWorkspaceChatSessionRequest) Validate() error {
	return validation.ValidateStruct(&r,
		validation.Field(&r.Title, validation.Length(0, 255)),
	)
}

// WorkspaceChatSessionResponse represents a session in list or detail views.
type WorkspaceChatSessionResponse struct {
	ID           uuid.UUID  `json:"id"`
	WorkspaceID  uuid.UUID  `json:"workspace_id"`
	FolderID     *uuid.UUID `json:"folder_id,omitempty"`
	FolderName   string     `json:"folder_name,omitempty"`
	UserID       uuid.UUID  `json:"user_id"`
	Title        string     `json:"title"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	MessageCount int        `json:"message_count"`
}

// WorkspaceChatMessageResponse represents a conversation turn.
type WorkspaceChatMessageResponse struct {
	ID        uuid.UUID                `json:"id"`
	SessionID uuid.UUID                `json:"session_id"`
	Role      string                   `json:"role"`
	Content   string                   `json:"content"`
	Citations []MeetingSourceCitation `json:"citations,omitempty"`
	CreatedAt time.Time                `json:"created_at"`
}

// StreamWorkspaceChatRequest represents input for the SSE typewriter streaming endpoint.
type StreamWorkspaceChatRequest struct {
	SessionID *uuid.UUID         `json:"session_id,omitempty"`
	FolderID  *uuid.UUID         `json:"folder_id,omitempty"`
	Question  string             `json:"question"`
	Query     string             `json:"query,omitempty"`
	History   []ChatMessageInput `json:"history,omitempty"`
}

func (r *StreamWorkspaceChatRequest) SetDefaults() {
	if r.Question == "" && r.Query != "" {
		r.Question = r.Query
	}
}

func (r StreamWorkspaceChatRequest) Validate() error {
	return validation.ValidateStruct(&r,
		validation.Field(&r.Question, validation.Required, validation.Length(1, 2000)),
	)
}

// WorkspaceChatStreamResult contains artifacts and streaming channel for workspace/folder chat.
type WorkspaceChatStreamResult struct {
	SessionID         uuid.UUID
	StreamChannel     <-chan StreamChunk
	Sources           []MeetingSourceCitation
	RetrievedChunkIDs []uuid.UUID
}
