package repositories

import (
	"context"
	"sort"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"code-base-golang/internal/models"
)

// SaveTranscriptSegments batch-inserts diarized transcript segments.
func (r *Repositories) SaveTranscriptSegments(ctx context.Context, segments []models.TranscriptSegment) error {
	if len(segments) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Create(&segments).Error
}

// ListTranscriptSegmentsByRecordingID retrieves all transcript segments for a recording in chronological order.
func (r *Repositories) ListTranscriptSegmentsByRecordingID(ctx context.Context, recordingID uuid.UUID) ([]models.TranscriptSegment, error) {
	var segments []models.TranscriptSegment
	err := r.db.WithContext(ctx).
		Where("recording_id = ?", recordingID).
		Order("sequence_order ASC, start_time ASC").
		Find(&segments).Error
	if err != nil {
		return nil, err
	}
	return segments, nil
}

// UpdateTranscriptSpeakerName renames a speaker label across all segments of a recording.
func (r *Repositories) UpdateTranscriptSpeakerName(ctx context.Context, recordingID uuid.UUID, speakerLabel string, newName string) error {
	return r.db.WithContext(ctx).
		Model(&models.TranscriptSegment{}).
		Where("recording_id = ? AND speaker_label = ?", recordingID, speakerLabel).
		Update("speaker_name", newName).Error
}

// UpdateTranscriptSpeakerNames batch-renames speaker labels across matching segments within a transaction.
func (r *Repositories) UpdateTranscriptSpeakerNames(ctx context.Context, recordingID uuid.UUID, speakers map[string]string) (int64, error) {
	if len(speakers) == 0 {
		return 0, nil
	}
	var totalUpdated int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for label, newName := range speakers {
			res := tx.Model(&models.TranscriptSegment{}).
				Where("recording_id = ? AND speaker_label = ?", recordingID, label).
				Update("speaker_name", newName)
			if res.Error != nil {
				return res.Error
			}
			totalUpdated += res.RowsAffected
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return totalUpdated, nil
}

// UpdateTranscriptSegmentText updates the text of a single transcript segment by ID and recording ID.
func (r *Repositories) UpdateTranscriptSegmentText(ctx context.Context, recordingID uuid.UUID, segmentID uuid.UUID, newText string) (*models.TranscriptSegment, error) {
	var segment models.TranscriptSegment
	err := r.db.WithContext(ctx).
		Where("id = ? AND recording_id = ?", segmentID, recordingID).
		First(&segment).Error
	if err != nil {
		return nil, err
	}

	err = r.db.WithContext(ctx).
		Model(&models.TranscriptSegment{}).
		Where("id = ? AND recording_id = ?", segmentID, recordingID).
		Update("text", newText).Error
	if err != nil {
		return nil, err
	}

	segment.Text = newText
	return &segment, nil
}


// SaveTranscriptChunks batch-inserts semantic text chunks with vector embeddings.
func (r *Repositories) SaveTranscriptChunks(ctx context.Context, chunks []models.TranscriptChunk) error {
	if len(chunks) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Create(&chunks).Error
}

// ListTranscriptChunksByRecordingID retrieves all chunks for a recording ordered by chunk index, omitting the large vector embedding.
func (r *Repositories) ListTranscriptChunksByRecordingID(ctx context.Context, recordingID uuid.UUID) ([]models.TranscriptChunk, error) {
	var chunks []models.TranscriptChunk
	err := r.db.WithContext(ctx).
		Omit("embedding").
		Where("recording_id = ?", recordingID).
		Order("chunk_index ASC").
		Find(&chunks).Error
	if err != nil {
		return nil, err
	}
	return chunks, nil
}

// SearchSimilarTranscriptChunks performs pgvector Cosine Distance nearest-neighbor search, omitting raw embeddings.
func (r *Repositories) SearchSimilarTranscriptChunks(ctx context.Context, recordingID uuid.UUID, queryEmbedding []float32, topK int) ([]models.TranscriptChunk, error) {
	vec := models.Vector(queryEmbedding)
	var chunks []models.TranscriptChunk
	err := r.db.WithContext(ctx).
		Omit("embedding").
		Where("recording_id = ?", recordingID).
		Order(clause.OrderBy{
			Expression: clause.Expr{
				SQL:  "embedding <=> ?",
				Vars: []interface{}{vec},
			},
		}).
		Limit(topK).
		Find(&chunks).Error
	if err != nil {
		return nil, err
	}
	return chunks, nil
}

// DeleteTranscriptSegmentsByRecordingID deletes all transcript segments for a recording.
func (r *Repositories) DeleteTranscriptSegmentsByRecordingID(ctx context.Context, recordingID uuid.UUID) error {
	return r.db.WithContext(ctx).Where("recording_id = ?", recordingID).Delete(&models.TranscriptSegment{}).Error
}

// ReplaceTranscriptSegments atomically replaces transcript segments for a recording in a database transaction.
func (r *Repositories) ReplaceTranscriptSegments(ctx context.Context, recordingID uuid.UUID, segments []models.TranscriptSegment) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("recording_id = ?", recordingID).Delete(&models.TranscriptSegment{}).Error; err != nil {
			return err
		}
		if len(segments) == 0 {
			return nil
		}
		return tx.Create(&segments).Error
	})
}

// DeleteTranscriptChunksByRecordingID deletes all transcript vector chunks for a recording.
func (r *Repositories) DeleteTranscriptChunksByRecordingID(ctx context.Context, recordingID uuid.UUID) error {
	return r.db.WithContext(ctx).Where("recording_id = ?", recordingID).Delete(&models.TranscriptChunk{}).Error
}

// WorkspaceChunkMatch holds the search result for cross-meeting vector similarity matches.
type WorkspaceChunkMatch struct {
	ID             uuid.UUID  `gorm:"column:id"`
	RecordingID    uuid.UUID  `gorm:"column:recording_id"`
	RecordingTitle string     `gorm:"column:recording_title"`
	FolderID       *uuid.UUID `gorm:"column:folder_id"`
	FolderName     string     `gorm:"column:folder_name"`
	ChunkIndex     int        `gorm:"column:chunk_index"`
	Content        string     `gorm:"column:content"`
	StartTime      float64    `gorm:"column:start_time"`
	EndTime        float64    `gorm:"column:end_time"`
	Distance       float64    `gorm:"column:distance"`
}

// SearchWorkspaceTranscriptChunks performs cross-meeting semantic vector search using pgvector cosine distance.
func (r *Repositories) SearchWorkspaceTranscriptChunks(ctx context.Context, userID uuid.UUID, queryEmbedding []float32, topK int, maxDistance float64) ([]WorkspaceChunkMatch, error) {
	if topK <= 0 {
		topK = 10
	}
	vec := models.Vector(queryEmbedding)
	var results []WorkspaceChunkMatch

	query := r.db.WithContext(ctx).
		Table("transcript_chunks tc").
		Select("tc.id, tc.recording_id, r.title as recording_title, tc.chunk_index, tc.content, tc.start_time, tc.end_time, (tc.embedding <=> ?) as distance", vec).
		Joins("JOIN recordings r ON tc.recording_id = r.id").
		Where("r.user_id = ? AND r.deleted_at IS NULL", userID)

	if maxDistance > 0 {
		query = query.Where("(tc.embedding <=> ?) <= ?", vec, maxDistance)
	}

	err := query.
		Order(clause.OrderBy{
			Expression: clause.Expr{
				SQL:  "tc.embedding <=> ?",
				Vars: []interface{}{vec},
			},
		}).
		Limit(topK).
		Scan(&results).Error

	if err != nil {
		return nil, err
	}
	return results, nil
}

// SearchWorkspaceTranscriptChunksByWorkspace performs cross-meeting semantic vector search scoped to a workspace.
// Uses round-robin partitioned ranking per recording so diverse meetings are represented fairly in the context window.
func (r *Repositories) SearchWorkspaceTranscriptChunksByWorkspace(ctx context.Context, workspaceID uuid.UUID, queryEmbedding []float32, topK int, maxDistance float64) ([]WorkspaceChunkMatch, error) {
	if topK <= 0 {
		topK = 10
	}
	vec := models.Vector(queryEmbedding)
	var results []WorkspaceChunkMatch

	rawQuery := `
		WITH ranked_chunks AS (
			SELECT 
				tc.id, 
				tc.recording_id, 
				r.title as recording_title, 
				r.folder_id, 
				f.name as folder_name, 
				tc.chunk_index, 
				tc.content, 
				tc.start_time, 
				tc.end_time, 
				(tc.embedding <=> ?) as distance,
				ROW_NUMBER() OVER (
					PARTITION BY tc.recording_id 
					ORDER BY (tc.embedding <=> ?) ASC
				) as rank_per_recording
			FROM transcript_chunks tc
			JOIN recordings r ON tc.recording_id = r.id
			LEFT JOIN folders f ON r.folder_id = f.id
			WHERE r.workspace_id = ? AND r.deleted_at IS NULL
	`
	args := []interface{}{vec, vec, workspaceID}

	if maxDistance > 0 {
		rawQuery += " AND (tc.embedding <=> ?) <= ?"
		args = append(args, vec, maxDistance)
	}

	rawQuery += `
		)
		SELECT 
			id, 
			recording_id, 
			recording_title, 
			folder_id, 
			folder_name, 
			chunk_index, 
			content, 
			start_time, 
			end_time, 
			distance
		FROM ranked_chunks
		ORDER BY rank_per_recording ASC, distance ASC
		LIMIT ?
	`
	args = append(args, topK)

	err := r.db.WithContext(ctx).Raw(rawQuery, args...).Scan(&results).Error
	if err != nil {
		return nil, err
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Distance < results[j].Distance
	})

	return results, nil
}

// SearchWorkspaceTranscriptChunksByFolderRecursive performs cross-meeting semantic vector search
// scoped to a specific folder and all of its sub-folders recursively, using round-robin partitioned ranking.
func (r *Repositories) SearchWorkspaceTranscriptChunksByFolderRecursive(
	ctx context.Context,
	workspaceID uuid.UUID,
	folderID uuid.UUID,
	queryEmbedding []float32,
	topK int,
	maxDistance float64,
) ([]WorkspaceChunkMatch, error) {
	if topK <= 0 {
		topK = 10
	}
	vec := models.Vector(queryEmbedding)
	var results []WorkspaceChunkMatch

	rawQuery := `
		WITH RECURSIVE folder_tree AS (
			SELECT id FROM folders WHERE id = ? AND workspace_id = ? AND deleted_at IS NULL
			UNION ALL
			SELECT f.id FROM folders f
			INNER JOIN folder_tree ft ON f.parent_id = ft.id
			WHERE f.deleted_at IS NULL AND f.workspace_id = ?
		),
		ranked_chunks AS (
			SELECT 
				tc.id,
				tc.recording_id,
				r.title AS recording_title,
				r.folder_id,
				f.name AS folder_name,
				tc.chunk_index,
				tc.content,
				tc.start_time,
				tc.end_time,
				(tc.embedding <=> ?) AS distance,
				ROW_NUMBER() OVER (
					PARTITION BY tc.recording_id 
					ORDER BY (tc.embedding <=> ?) ASC
				) as rank_per_recording
			FROM transcript_chunks tc
			JOIN recordings r ON tc.recording_id = r.id
			LEFT JOIN folders f ON r.folder_id = f.id
			WHERE r.workspace_id = ?
			  AND r.folder_id IN (SELECT id FROM folder_tree)
			  AND r.deleted_at IS NULL
	`
	args := []interface{}{folderID, workspaceID, workspaceID, vec, vec, workspaceID}

	if maxDistance > 0 {
		rawQuery += " AND (tc.embedding <=> ?) <= ?"
		args = append(args, vec, maxDistance)
	}

	rawQuery += `
		)
		SELECT 
			id, 
			recording_id, 
			recording_title, 
			folder_id, 
			folder_name, 
			chunk_index, 
			content, 
			start_time, 
			end_time, 
			distance
		FROM ranked_chunks
		ORDER BY rank_per_recording ASC, distance ASC
		LIMIT ?
	`
	args = append(args, topK)

	err := r.db.WithContext(ctx).Raw(rawQuery, args...).Scan(&results).Error
	if err != nil {
		return nil, err
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Distance < results[j].Distance
	})

	return results, nil
}

// WorkspaceSpeakerStat represents aggregated speaker participation data returned from database queries.
type WorkspaceSpeakerStat struct {
	Name          string    `gorm:"column:name"`
	TotalMeetings int       `gorm:"column:total_meetings"`
	TotalTalkTime float64   `gorm:"column:total_talk_time"`
	LastActive    time.Time `gorm:"column:last_active"`
}

// AggregateWorkspaceSpeakers aggregates distinct speakers across all recordings owned by the user.
func (r *Repositories) AggregateWorkspaceSpeakers(ctx context.Context, userID uuid.UUID) ([]WorkspaceSpeakerStat, error) {
	var stats []WorkspaceSpeakerStat
	err := r.db.WithContext(ctx).
		Table("transcript_segments ts").
		Select("ts.speaker_name AS name, COUNT(DISTINCT ts.recording_id) AS total_meetings, COALESCE(SUM(GREATEST(0, ts.end_time - ts.start_time)), 0) AS total_talk_time, MAX(r.created_at) AS last_active").
		Joins("JOIN recordings r ON ts.recording_id = r.id").
		Where("r.user_id = ? AND r.deleted_at IS NULL AND TRIM(ts.speaker_name) != ''", userID).
		Group("ts.speaker_name").
		Order("total_meetings DESC, total_talk_time DESC, name ASC").
		Scan(&stats).Error
	if err != nil {
		return nil, err
	}
	return stats, nil
}



