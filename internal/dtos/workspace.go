package dtos

import (
	"time"

	"github.com/google/uuid"
)

// SemanticSearchQuery defines query parameters for cross-meeting semantic vector search.
type SemanticSearchQuery struct {
	Q         string  `form:"q" json:"q"`
	Limit     int     `form:"limit" json:"limit"`
	Threshold float64 `form:"threshold" json:"threshold"`
}


// SearchResultItem represents an individual matched transcript chunk snippet from a meeting.
type SearchResultItem struct {
	RecordingID    uuid.UUID `json:"recording_id"`
	RecordingTitle string    `json:"recording_title"`
	ChunkIndex     int       `json:"chunk_index"`
	Snippet        string    `json:"snippet"`
	StartTime      float64   `json:"start_time"`
	EndTime        float64   `json:"end_time"`
	Score          float64   `json:"score"`
}

// SemanticSearchResponse defines the envelope containing all semantic search results.
type SemanticSearchResponse struct {
	Query   string             `json:"query"`
	Count   int                `json:"count"`
	Results []SearchResultItem `json:"results"`
}

// WorkspaceAskRequest defines the input payload for cross-meeting workspace memory chat.
type WorkspaceAskRequest struct {
	Question string             `json:"question"`
	History  []ChatMessageInput `json:"history,omitempty"`
}


// MeetingSourceCitation represents a citation reference to a specific meeting and timestamp segment.
type MeetingSourceCitation struct {
	RecordingID    uuid.UUID `json:"recording_id"`
	RecordingTitle string    `json:"recording_title"`
	ChunkIndex     int       `json:"chunk_index"`
	Snippet        string    `json:"snippet"`
	StartTime      float64   `json:"start_time"`
	EndTime        float64   `json:"end_time"`
}

// WorkspaceAskResponse represents the response containing synthesized answer and meeting citations.
type WorkspaceAskResponse struct {
	Answer  string                  `json:"answer"`
	Sources []MeetingSourceCitation `json:"sources"`
}

// SpeakerSummary represents aggregated participation metrics for a speaker across a user's recordings.
type SpeakerSummary struct {
	Name          string    `json:"name"`
	TotalMeetings int       `json:"total_meetings"`
	TotalTalkTime float64   `json:"total_talk_time"`
	LastActive    time.Time `json:"last_active"`
}

// SpeakerDirectoryResponse defines the envelope returned by the workspace speaker directory endpoint.
type SpeakerDirectoryResponse struct {
	Count    int              `json:"count"`
	Speakers []SpeakerSummary `json:"speakers"`
}

// CreateWorkspaceRequest defines the input payload for creating a new team workspace.
type CreateWorkspaceRequest struct {
	Name    string  `json:"name"`
	LogoURL *string `json:"logo_url,omitempty"`
}

// UpdateWorkspaceRequest defines the input payload for updating workspace details.
type UpdateWorkspaceRequest struct {
	Name    *string `json:"name,omitempty"`
	LogoURL *string `json:"logo_url,omitempty"`
}

// UpdateWorkspaceMemberRoleRequest defines the input payload for modifying a member's role.
type UpdateWorkspaceMemberRoleRequest struct {
	Role string `json:"role"`
}

// WorkspaceResponse represents the public representation of a workspace with the current user's role.
type WorkspaceResponse struct {
	ID           uuid.UUID `json:"id"`
	Name         string    `json:"name"`
	Slug         string    `json:"slug"`
	Type         string    `json:"type"`
	Role         string    `json:"role"`
	OwnerUserID  uuid.UUID `json:"owner_user_id"`
	LogoURL      *string   `json:"logo_url,omitempty"`
	MembersCount int64     `json:"members_count"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// WorkspaceMemberResponse represents a member within a workspace.
type WorkspaceMemberResponse struct {
	ID          uuid.UUID `json:"id"`
	WorkspaceID uuid.UUID `json:"workspace_id"`
	UserID      uuid.UUID `json:"user_id"`
	Email       string    `json:"email"`
	FullName    string    `json:"full_name"`
	Role        string    `json:"role"`
	CreatedAt   time.Time `json:"created_at"`
}



