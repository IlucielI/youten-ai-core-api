package repositories

import (
	"context"

	"github.com/google/uuid"
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
