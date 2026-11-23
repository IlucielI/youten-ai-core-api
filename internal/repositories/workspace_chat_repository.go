package repositories

import (
	"context"
	"time"

	"github.com/google/uuid"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/models"
)

// CreateWorkspaceChatSession persists a new workspace or folder chat session.
func (r *Repositories) CreateWorkspaceChatSession(ctx context.Context, session *models.WorkspaceChatSession) error {
	return r.db.WithContext(ctx).Create(session).Error
}

// FindWorkspaceChatSessionByID retrieves a session by ID including Folder information.
func (r *Repositories) FindWorkspaceChatSessionByID(ctx context.Context, sessionID uuid.UUID) (*models.WorkspaceChatSession, error) {
	var session models.WorkspaceChatSession
	err := r.db.WithContext(ctx).
		Preload("Folder").
		Where("id = ?", sessionID).
		First(&session).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// FindWorkspaceChatSessions retrieves sessions for a user within a workspace, optionally filtered by folder.
// When folderID is nil, it retrieves workspace-level sessions (folder_id IS NULL) to isolate workspace chat from folder chat.
func (r *Repositories) FindWorkspaceChatSessions(
	ctx context.Context,
	workspaceID uuid.UUID,
	userID uuid.UUID,
	folderID *uuid.UUID,
) ([]models.WorkspaceChatSession, error) {
	var sessions []models.WorkspaceChatSession
	query := r.db.WithContext(ctx).
		Preload("Folder").
		Where("workspace_id = ? AND user_id = ?", workspaceID, userID)

	if folderID != nil && *folderID != uuid.Nil {
		query = query.Where("folder_id = ?", *folderID)
	} else {
		query = query.Where("folder_id IS NULL")
	}

	err := query.Order("updated_at DESC").Find(&sessions).Error
	if err != nil {
		return nil, err
	}
	return sessions, nil
}

// UpdateWorkspaceChatSessionTitle updates the title and timestamp of a chat session.
func (r *Repositories) UpdateWorkspaceChatSessionTitle(ctx context.Context, sessionID uuid.UUID, title string) error {
	updates := map[string]interface{}{}
	if title != "" {
		updates["title"] = title
	}
	return r.db.WithContext(ctx).
		Model(&models.WorkspaceChatSession{}).
		Where("id = ?", sessionID).
		Updates(updates).Error
}

// DeleteWorkspaceChatSession soft-deletes a session and its associated messages.
func (r *Repositories) DeleteWorkspaceChatSession(ctx context.Context, sessionID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("id = ?", sessionID).
		Delete(&models.WorkspaceChatSession{}).Error
}

// CreateWorkspaceChatMessage persists a chat dialogue turn with citations.
func (r *Repositories) CreateWorkspaceChatMessage(ctx context.Context, msg *models.WorkspaceChatMessage) error {
	return r.db.WithContext(ctx).Create(msg).Error
}

// FindWorkspaceChatMessagesBySession retrieves all messages in chronological order for a session.
func (r *Repositories) FindWorkspaceChatMessagesBySession(ctx context.Context, sessionID uuid.UUID) ([]models.WorkspaceChatMessage, error) {
	var messages []models.WorkspaceChatMessage
	err := r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("created_at ASC").
		Find(&messages).Error
	if err != nil {
		return nil, err
	}
	return messages, nil
}

// CountWorkspaceChatMessagesBySession returns the total message count in a session.
func (r *Repositories) CountWorkspaceChatMessagesBySession(ctx context.Context, sessionID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&models.WorkspaceChatMessage{}).
		Where("session_id = ?", sessionID).
		Count(&count).Error
	return count, err
}

// CountWorkspaceChatMessagesBySessions batch counts messages for multiple sessions in a single query.
func (r *Repositories) CountWorkspaceChatMessagesBySessions(ctx context.Context, sessionIDs []uuid.UUID) (map[uuid.UUID]int64, error) {
	result := make(map[uuid.UUID]int64)
	if len(sessionIDs) == 0 {
		return result, nil
	}
	type countRow struct {
		SessionID uuid.UUID `gorm:"column:session_id"`
		Count     int64     `gorm:"column:count"`
	}
	var rows []countRow
	err := r.db.WithContext(ctx).
		Model(&models.WorkspaceChatMessage{}).
		Select("session_id, count(*) as count").
		Where("session_id IN ?", sessionIDs).
		Group("session_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		result[row.SessionID] = row.Count
	}
	return result, nil
}

// WorkspaceCatalogRecord holds high-level metadata and executive summary for a meeting recording.
type WorkspaceCatalogRecord struct {
	RecordingID     uuid.UUID  `gorm:"column:recording_id"`
	Title           string     `gorm:"column:title"`
	FolderID        *uuid.UUID `gorm:"column:folder_id"`
	FolderName      string     `gorm:"column:folder_name"`
	CreatedAt       time.Time  `gorm:"column:created_at"`
	DurationSeconds float64    `gorm:"column:duration_seconds"`
	Overview        string     `gorm:"column:overview"`
}

// GetWorkspaceMeetingCatalog retrieves the catalog of completed recordings with their summaries
// in a workspace, optionally filtered to a specific folder (and its subfolders recursively).
func (r *Repositories) GetWorkspaceMeetingCatalog(
	ctx context.Context,
	workspaceID uuid.UUID,
	folderID *uuid.UUID,
	limit int,
) ([]WorkspaceCatalogRecord, error) {
	if limit <= 0 {
		limit = constants.DefaultWorkspaceMeetingCatalogLimit
	}

	var results []WorkspaceCatalogRecord

	if folderID != nil && *folderID != uuid.Nil {
		rawQuery := `
			WITH RECURSIVE folder_tree AS (
				SELECT id FROM folders WHERE id = ? AND workspace_id = ? AND deleted_at IS NULL
				UNION ALL
				SELECT f.id FROM folders f
				INNER JOIN folder_tree ft ON f.parent_id = ft.id
				WHERE f.deleted_at IS NULL AND f.workspace_id = ?
			)
			SELECT 
				r.id AS recording_id,
				r.title,
				r.folder_id,
				f.name AS folder_name,
				r.created_at,
				r.duration_seconds,
				COALESCE(
					NULLIF(s.structured_data->>'executive_overview', ''),
					NULLIF(s.structured_data->>'context', ''),
					NULLIF(s.structured_data->>'summary_text', ''),
					NULLIF(s.structured_data->>'justification', ''),
					SUBSTRING(s.markdown_content, 1, 350),
					''
				) AS overview
			FROM recordings r
			LEFT JOIN folders f ON r.folder_id = f.id
			LEFT JOIN summaries s ON r.id = s.recording_id AND s.is_active = true
			WHERE r.workspace_id = ?
			  AND r.folder_id IN (SELECT id FROM folder_tree)
			  AND r.status = 'COMPLETED'
			  AND r.deleted_at IS NULL
			ORDER BY r.created_at DESC
			LIMIT ?
		`
		err := r.db.WithContext(ctx).Raw(rawQuery, *folderID, workspaceID, workspaceID, workspaceID, limit).Scan(&results).Error
		return results, err
	}

	rawQuery := `
		SELECT 
			r.id AS recording_id,
			r.title,
			r.folder_id,
			f.name AS folder_name,
			r.created_at,
			r.duration_seconds,
			COALESCE(
				NULLIF(s.structured_data->>'executive_overview', ''),
				NULLIF(s.structured_data->>'context', ''),
				NULLIF(s.structured_data->>'summary_text', ''),
				NULLIF(s.structured_data->>'justification', ''),
				SUBSTRING(s.markdown_content, 1, 350),
				''
			) AS overview
		FROM recordings r
		LEFT JOIN folders f ON r.folder_id = f.id
		LEFT JOIN summaries s ON r.id = s.recording_id AND s.is_active = true
		WHERE r.workspace_id = ?
		  AND r.status = 'COMPLETED'
		  AND r.deleted_at IS NULL
		ORDER BY r.created_at DESC
		LIMIT ?
	`
	err := r.db.WithContext(ctx).Raw(rawQuery, workspaceID, limit).Scan(&results).Error
	return results, err
}
