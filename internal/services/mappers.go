package services

import (
	"github.com/google/uuid"

	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
)

// uuidPtrToString renders an optional UUID as an optional string.
func uuidPtrToString(id *uuid.UUID) *string {
	if id == nil {
		return nil
	}
	s := id.String()
	return &s
}

// toTranscriptSegmentDTOs maps transcript segment models into their DTO representation.
func toTranscriptSegmentDTOs(segments []models.TranscriptSegment) []dtos.TranscriptSegmentDTO {
	out := make([]dtos.TranscriptSegmentDTO, len(segments))
	for i, seg := range segments {
		out[i] = dtos.TranscriptSegmentDTO{
			ID:            seg.ID.String(),
			SpeakerLabel:  seg.SpeakerLabel,
			SpeakerName:   seg.SpeakerName,
			StartTime:     seg.StartTime,
			EndTime:       seg.EndTime,
			Text:          seg.Text,
			WordsData:     seg.WordsData,
			SequenceOrder: seg.SequenceOrder,
		}
	}
	return out
}

// toSummaryDTO maps the active summary model into its DTO representation.
func toSummaryDTO(summary *models.Summary) *dtos.SummaryDTO {
	if summary == nil {
		return nil
	}
	return &dtos.SummaryDTO{
		ID:               summary.ID.String(),
		TemplateCategory: summary.TemplateCategory,
		CustomAngle:      summary.CustomAngle,
		Version:          summary.Version,
		IsActive:         summary.IsActive,
		StructuredData:   summary.StructuredData,
		MarkdownContent:  summary.MarkdownContent,
		CreatedAt:        summary.CreatedAt,
		UpdatedAt:        summary.UpdatedAt,
	}
}

// toChapterDTOs maps chapter models into their DTO representation.
func toChapterDTOs(chapters []models.Chapter) []dtos.ChapterDTO {
	out := make([]dtos.ChapterDTO, len(chapters))
	for i, chap := range chapters {
		out[i] = dtos.ChapterDTO{
			ID:            chap.ID.String(),
			Title:         chap.Title,
			StartTime:     chap.StartTime,
			EndTime:       chap.EndTime,
			Summary:       chap.Summary,
			SequenceOrder: chap.SequenceOrder,
			CreatedAt:     chap.CreatedAt,
		}
	}
	return out
}

// toHighlightDTOs maps highlight models into their DTO representation.
func toHighlightDTOs(highlights []models.Highlight) []dtos.HighlightDTO {
	out := make([]dtos.HighlightDTO, len(highlights))
	for i, hl := range highlights {
		out[i] = dtos.HighlightDTO{
			ID:        hl.ID.String(),
			StartTime: hl.StartTime,
			EndTime:   hl.EndTime,
			Title:     hl.Title,
			Note:      hl.Note,
			Source:    hl.Source,
			ClipURL:   hl.ClipURL,
			CreatedAt: hl.CreatedAt,
		}
	}
	return out
}

// toSummaryVersionResponse maps a summary model into its version response representation.
func toSummaryVersionResponse(summary models.Summary) dtos.SummaryVersionResponse {
	return dtos.SummaryVersionResponse{
		ID:               summary.ID.String(),
		Version:          summary.Version,
		TemplateCategory: summary.TemplateCategory,
		CustomAngle:      summary.CustomAngle,
		StructuredData:   summary.StructuredData,
		MarkdownContent:  summary.MarkdownContent,
		IsActive:         summary.IsActive,
		CreatedAt:        summary.CreatedAt,
	}
}

// toCommentResponse maps an inline comment (and its nested replies) into its response representation.
func toCommentResponse(comment models.InlineComment) dtos.CommentResponse {
	resp := dtos.CommentResponse{
		ID:           comment.ID.String(),
		RecordingID:  comment.RecordingID.String(),
		UserID:       uuidPtrToString(comment.UserID),
		SegmentID:    uuidPtrToString(comment.SegmentID),
		TimestampSec: comment.TimestampSec,
		SelectedText: comment.SelectedText,
		AuthorName:   comment.AuthorName,
		CommentText:  comment.CommentText,
		ParentID:     uuidPtrToString(comment.ParentID),
		CreatedAt:    comment.CreatedAt,
		UpdatedAt:    comment.UpdatedAt,
	}
	if len(comment.Replies) > 0 {
		resp.Replies = make([]dtos.CommentResponse, 0, len(comment.Replies))
		for _, reply := range comment.Replies {
			resp.Replies = append(resp.Replies, toCommentResponse(reply))
		}
	}
	return resp
}
