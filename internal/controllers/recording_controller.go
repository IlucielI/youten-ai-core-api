package controllers

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/sse"
)


// PresignUpload generates a pre-signed S3 PUT URL so that the frontend
// can upload the media file directly to object storage without streaming through the backend.
func (c *Controllers) PresignUpload(ctx *gin.Context) {
	if c == nil || c.svc == nil {
		c.wrapError(ctx, constants.ErrInternalServerError)
		return
	}

	// Restrict payload size for metadata JSON (64KB)
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, 64*1024)

	var req dtos.PresignUploadRequest
	if err := ctx.ShouldBind(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, dtos.BaseResponse{
			Status:    constants.ResponseStatusFail,
			Code:      constants.ResponseCodeBadRequest,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
		return
	}

	if err := req.Validate(); err != nil {
		ctx.JSON(http.StatusBadRequest, dtos.BaseResponse{
			Status:    constants.ResponseStatusFail,
			Code:      constants.ResponseCodeBadRequest,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
		return
	}

	if !dtos.IsValidMediaMIME(req.ContentType, req.Filename) {
		c.wrapError(ctx, constants.ErrUnsupportedMediaType)
		return
	}

	resp, err := c.svc.GeneratePresignUpload(ctx.Request.Context(), req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	if resp == nil {
		c.wrapError(ctx, constants.ErrInternalServerError)
		return
	}

	ctx.JSON(http.StatusOK, dtos.APIResponse[dtos.PresignUploadResponse]{
		Status:    constants.ResponseStatusSuccess,
		Code:      constants.ResponseCodeSuccess,
		Message:   "presigned upload url generated",
		Data:      *resp,
		Timestamp: time.Now(),
	})
}

// UploadRecording confirms media file upload after the frontend uploaded directly to S3.
// The frontend only submits the filename and metadata, preventing file streams through the backend.
func (c *Controllers) UploadRecording(ctx *gin.Context) {
	if c == nil || c.svc == nil {
		c.wrapError(ctx, constants.ErrInternalServerError)
		return
	}

	// Restrict metadata payload size (64KB)
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, 64*1024)

	var req dtos.UploadRecordingRequest
	if err := ctx.ShouldBind(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, dtos.BaseResponse{
			Status:    constants.ResponseStatusFail,
			Code:      constants.ResponseCodeBadRequest,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
		return
	}

	if err := req.Validate(); err != nil {
		ctx.JSON(http.StatusBadRequest, dtos.BaseResponse{
			Status:    constants.ResponseStatusFail,
			Code:      constants.ResponseCodeBadRequest,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
		return
	}

	resp, err := c.svc.UploadRecording(ctx.Request.Context(), req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	if resp == nil {
		c.wrapError(ctx, constants.ErrInternalServerError)
		return
	}

	ctx.JSON(http.StatusCreated, dtos.APIResponse[dtos.RecordingUploadResponse]{
		Status:    constants.ResponseStatusSuccess,
		Code:      constants.ResponseCodeSuccess,
		Message:   "recording uploaded successfully",
		Data:      *resp,
		Timestamp: time.Now(),
	})
}

// ImportURL handles media ingestion from a remote URL with Anti-SSRF protection.
func (c *Controllers) ImportURL(ctx *gin.Context) {
	if c == nil || c.svc == nil {
		c.wrapError(ctx, constants.ErrInternalServerError)
		return
	}

	// Restrict JSON request body size (64KB)
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, 64*1024)

	var req dtos.ImportURLRequest
	if err := ctx.ShouldBind(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, dtos.BaseResponse{
			Status:    constants.ResponseStatusFail,
			Code:      constants.ResponseCodeBadRequest,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
		return
	}

	if err := req.Validate(); err != nil {
		ctx.JSON(http.StatusBadRequest, dtos.BaseResponse{
			Status:    constants.ResponseStatusFail,
			Code:      constants.ResponseCodeBadRequest,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
		return
	}

	resp, err := c.svc.ImportRecordingFromURL(ctx.Request.Context(), req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	if resp == nil {
		c.wrapError(ctx, constants.ErrInternalServerError)
		return
	}

	ctx.JSON(http.StatusCreated, dtos.APIResponse[dtos.RecordingUploadResponse]{
		Status:    constants.ResponseStatusSuccess,
		Code:      constants.ResponseCodeSuccess,
		Message:   "recording imported successfully",
		Data:      *resp,
		Timestamp: time.Now(),
	})
}

// GetRecordingDetail handles retrieving recording metadata, segments, active summary, chapters,
// and presigned playback audio URL.
func (c *Controllers) GetRecordingDetail(ctx *gin.Context) {
	if c == nil || c.svc == nil {
		c.wrapError(ctx, constants.ErrInternalServerError)
		return
	}

	idParam := ctx.Param("id")
	recID, err := uuid.Parse(idParam)
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.Wrap(err))
		return
	}

	// Extract ownership token from header or query param
	token := ctx.GetHeader("X-Ownership-Token")
	if token == "" {
		token = ctx.Query("token")
	}
	if token == "" {
		token = ctx.Query("ownership_token")
	}

	resp, err := c.svc.GetRecordingDetail(ctx.Request.Context(), recID, token)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	if resp == nil {
		c.wrapError(ctx, constants.ErrInternalServerError)
		return
	}

	ctx.JSON(http.StatusOK, dtos.APIResponse[dtos.RecordingDetailResponse]{
		Status:    constants.ResponseStatusSuccess,
		Code:      constants.ResponseCodeSuccess,
		Message:   "recording detail retrieved successfully",
		Data:      *resp,
		Timestamp: time.Now(),
	})
}

// ListRecordings handles retrieving paginated and filtered recordings owned by the authenticated user.
func (c *Controllers) ListRecordings(ctx *gin.Context) {
	if c == nil {
		ctx.JSON(http.StatusInternalServerError, dtos.BaseResponse{
			Status:    constants.ResponseStatusError,
			Code:      constants.ResponseCodeInternalError,
			Message:   constants.ErrInternalServerError.Message,
			Timestamp: time.Now(),
		})
		return
	}
	if c.svc == nil {
		c.wrapError(ctx, constants.ErrInternalServerError)
		return
	}

	var query dtos.RecordingFilterQuery
	if err := ctx.ShouldBindQuery(&query); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.Wrap(err))
		return
	}

	resp, err := c.svc.ListRecordings(ctx.Request.Context(), query)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	if resp == nil {
		c.wrapError(ctx, constants.ErrInternalServerError)
		return
	}

	ctx.JSON(http.StatusOK, dtos.APIResponse[dtos.RecordingListResponse]{
		Status:    constants.ResponseStatusSuccess,
		Code:      constants.ResponseCodeSuccess,
		Message:   "recordings retrieved successfully",
		Data:      *resp,
		Timestamp: time.Now(),
	})
}

// DeleteRecording handles soft-deleting a recording owned by the authenticated user.
func (c *Controllers) DeleteRecording(ctx *gin.Context) {
	if c == nil {
		ctx.JSON(http.StatusInternalServerError, dtos.BaseResponse{
			Status:    constants.ResponseStatusError,
			Code:      constants.ResponseCodeInternalError,
			Message:   constants.ErrInternalServerError.Message,
			Timestamp: time.Now(),
		})
		return
	}
	if c.svc == nil {
		c.wrapError(ctx, constants.ErrInternalServerError)
		return
	}

	idParam := ctx.Param("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.Wrap(err))
		return
	}

	if err := c.svc.DeleteRecording(ctx.Request.Context(), id); err != nil {
		c.wrapError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, dtos.BaseResponse{
		Status:    constants.ResponseStatusSuccess,
		Code:      constants.ResponseCodeSuccess,
		Message:   "recording deleted successfully",
		Timestamp: time.Now(),
	})
}

// ClaimRecording handles claiming a guest recording session to the authenticated user.
func (c *Controllers) ClaimRecording(ctx *gin.Context) {
	if c == nil {
		ctx.JSON(http.StatusInternalServerError, dtos.BaseResponse{
			Status:    constants.ResponseStatusError,
			Code:      constants.ResponseCodeInternalError,
			Message:   constants.ErrInternalServerError.Message,
			Timestamp: time.Now(),
		})
		return
	}
	if c.svc == nil {
		c.wrapError(ctx, constants.ErrInternalServerError)
		return
	}

	idParam := ctx.Param("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.Wrap(err))
		return
	}

	var req dtos.ClaimRecordingRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, dtos.BaseResponse{
			Status:    constants.ResponseStatusFail,
			Code:      constants.ResponseCodeBadRequest,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
		return
	}

	if err := req.Validate(); err != nil {
		ctx.JSON(http.StatusBadRequest, dtos.BaseResponse{
			Status:    constants.ResponseStatusFail,
			Code:      constants.ResponseCodeBadRequest,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
		return
	}

	if err := c.svc.ClaimRecording(ctx.Request.Context(), id, req); err != nil {
		c.wrapError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, dtos.BaseResponse{
		Status:    constants.ResponseStatusSuccess,
		Code:      constants.ResponseCodeSuccess,
		Message:   "recording claimed successfully",
		Timestamp: time.Now(),
	})
}

// ClaimBulkRecordings handles claiming multiple guest recording sessions to the authenticated user.
func (c *Controllers) ClaimBulkRecordings(ctx *gin.Context) {
	if c == nil {
		ctx.JSON(http.StatusInternalServerError, dtos.BaseResponse{
			Status:    constants.ResponseStatusError,
			Code:      constants.ResponseCodeInternalError,
			Message:   constants.ErrInternalServerError.Message,
			Timestamp: time.Now(),
		})
		return
	}
	if c.svc == nil {
		c.wrapError(ctx, constants.ErrInternalServerError)
		return
	}

	var req dtos.BulkClaimRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, dtos.BaseResponse{
			Status:    constants.ResponseStatusFail,
			Code:      constants.ResponseCodeBadRequest,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
		return
	}

	if err := req.Validate(); err != nil {
		ctx.JSON(http.StatusBadRequest, dtos.BaseResponse{
			Status:    constants.ResponseStatusFail,
			Code:      constants.ResponseCodeBadRequest,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
		return
	}

	resp, err := c.svc.ClaimBulkRecordings(ctx.Request.Context(), req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, dtos.APIResponse[*dtos.BulkClaimResponse]{
		Status:    constants.ResponseStatusSuccess,
		Code:      constants.ResponseCodeSuccess,
		Message:   "recordings claimed successfully",
		Data:      resp,
		Timestamp: time.Now(),
	})
}

// ToggleRecordingShare handles toggling public sharing on or off for a recording.
func (c *Controllers) ToggleRecordingShare(ctx *gin.Context) {
	if c == nil {
		ctx.JSON(http.StatusInternalServerError, dtos.BaseResponse{
			Status:    constants.ResponseStatusError,
			Code:      constants.ResponseCodeInternalError,
			Message:   constants.ErrInternalServerError.Message,
			Timestamp: time.Now(),
		})
		return
	}
	if c.svc == nil {
		c.wrapError(ctx, constants.ErrInternalServerError)
		return
	}

	idParam := ctx.Param("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.Wrap(err))
		return
	}

	var req dtos.ShareToggleRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, dtos.BaseResponse{
			Status:    constants.ResponseStatusFail,
			Code:      constants.ResponseCodeBadRequest,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
		return
	}

	if err := req.Validate(); err != nil {
		ctx.JSON(http.StatusBadRequest, dtos.BaseResponse{
			Status:    constants.ResponseStatusFail,
			Code:      constants.ResponseCodeBadRequest,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
		return
	}

	resp, err := c.svc.ToggleRecordingShare(ctx.Request.Context(), id, req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, dtos.APIResponse[*dtos.ShareToggleResponse]{
		Status:    constants.ResponseStatusSuccess,
		Code:      constants.ResponseCodeSuccess,
		Message:   "recording share settings updated successfully",
		Data:      resp,
		Timestamp: time.Now(),
	})
}

// GetSharedRecording handles public retrieval of a shared recording by its share token.
func (c *Controllers) GetSharedRecording(ctx *gin.Context) {
	if c == nil {
		ctx.JSON(http.StatusInternalServerError, dtos.BaseResponse{
			Status:    constants.ResponseStatusError,
			Code:      constants.ResponseCodeInternalError,
			Message:   constants.ErrInternalServerError.Message,
			Timestamp: time.Now(),
		})
		return
	}
	if c.svc == nil {
		c.wrapError(ctx, constants.ErrInternalServerError)
		return
	}

	token := strings.TrimSpace(ctx.Param("token"))
	if token == "" {
		c.wrapError(ctx, constants.ErrNotFound)
		return
	}

	resp, err := c.svc.GetSharedRecording(ctx.Request.Context(), token)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, dtos.APIResponse[*dtos.SharedRecordingResponse]{
		Status:    constants.ResponseStatusSuccess,
		Code:      constants.ResponseCodeSuccess,
		Message:   "shared recording retrieved successfully",
		Data:      resp,
		Timestamp: time.Now(),
	})
}

// StreamRecordingProgress handles real-time SSE progress streaming for a recording pipeline.
func (c *Controllers) StreamRecordingProgress(ctx *gin.Context) {
	if c == nil {
		ctx.JSON(http.StatusInternalServerError, dtos.BaseResponse{
			Status:    constants.ResponseStatusError,
			Code:      constants.ResponseCodeInternalError,
			Message:   constants.ErrInternalServerError.Message,
			Timestamp: time.Now(),
		})
		return
	}
	if c.svc == nil {
		c.wrapError(ctx, constants.ErrInternalServerError)
		return
	}

	idParam := ctx.Param("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.Wrap(err))
		return
	}

	ownershipToken := strings.TrimSpace(ctx.Query("token"))
	if ownershipToken == "" {
		ownershipToken = strings.TrimSpace(ctx.Query("ownership_token"))
	}
	if ownershipToken == "" {
		ownershipToken = strings.TrimSpace(ctx.GetHeader("X-Ownership-Token"))
	}

	initial, subCh, unsub, err := c.svc.GetRecordingProgress(ctx.Request.Context(), id, ownershipToken)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}
	defer unsub()

	// Write SSE Response Headers
	ctx.Writer.Header().Set("Content-Type", "text/event-stream")
	ctx.Writer.Header().Set("Cache-Control", "no-cache")
	ctx.Writer.Header().Set("Connection", "keep-alive")
	ctx.Writer.Header().Set("Transfer-Encoding", "chunked")
	ctx.Writer.Header().Set("X-Accel-Buffering", "no")
	ctx.Writer.WriteHeader(http.StatusOK)
	ctx.Writer.Flush()

	// Stream initial event
	if err := sse.WriteProgress(ctx.Writer, *initial); err != nil {
		return
	}
	ctx.Writer.Flush()

	if sse.IsTerminalStatus(initial.Status) {
		return
	}

	pingTicker := time.NewTicker(15 * time.Second)
	defer pingTicker.Stop()

	pollTicker := time.NewTicker(2 * time.Second)
	defer pollTicker.Stop()

	lastStatus := initial.Status

	for {
		select {
		case <-ctx.Request.Context().Done():
			return
		case event, ok := <-subCh:
			if !ok {
				return
			}
			lastStatus = event.Status
			if err := sse.WriteProgress(ctx.Writer, event); err != nil {
				return
			}
			ctx.Writer.Flush()
			if sse.IsTerminalStatus(event.Status) {
				return
			}
		case <-pollTicker.C:
			current, err := c.svc.PollRecordingProgress(ctx.Request.Context(), id)
			if err == nil && current != nil && current.Status != lastStatus {
				lastStatus = current.Status
				if err := sse.WriteProgress(ctx.Writer, *current); err != nil {
					return
				}
				ctx.Writer.Flush()
				if sse.IsTerminalStatus(current.Status) {
					return
				}
			}
		case <-pingTicker.C:
			if err := sse.WritePing(ctx.Writer); err != nil {
				return
			}
			ctx.Writer.Flush()
		}
	}
}

// RetryRecording handles POST /v1/recordings/:id/retry to initiate a smart retry of a failed pipeline.
func (c *Controllers) RetryRecording(ctx *gin.Context) {
	if c == nil {
		ctx.JSON(http.StatusInternalServerError, dtos.BaseResponse{
			Status:    constants.ResponseStatusError,
			Code:      constants.ResponseCodeInternalError,
			Message:   constants.ErrInternalServerError.Message,
			Timestamp: time.Now(),
		})
		return
	}
	if c.svc == nil {
		c.wrapError(ctx, constants.ErrInternalServerError)
		return
	}

	idParam := ctx.Param("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.Wrap(err))
		return
	}

	ownershipToken := strings.TrimSpace(ctx.GetHeader("X-Ownership-Token"))
	if ownershipToken == "" && ctx.Request.Body != nil && ctx.Request.ContentLength > 0 {
		var req dtos.RetryRecordingRequest
		if err := ctx.ShouldBindJSON(&req); err == nil && req.OwnershipToken != "" {
			ownershipToken = strings.TrimSpace(req.OwnershipToken)
		}
	}

	resp, err := c.svc.RetryRecordingPipeline(ctx.Request.Context(), id, ownershipToken)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	if resp == nil {
		c.wrapError(ctx, constants.ErrInternalServerError)
		return
	}

	ctx.JSON(http.StatusOK, dtos.APIResponse[dtos.RetryRecordingResponse]{
		Status:    constants.ResponseStatusSuccess,
		Code:      constants.ResponseCodeSuccess,
		Message:   "pipeline retry initiated successfully",
		Data:      *resp,
		Timestamp: time.Now(),
	})
}

// StreamRecordingChat handles interactive RAG chat streaming via Server-Sent Events (SSE).
func (c *Controllers) StreamRecordingChat(ctx *gin.Context) {
	if c == nil {
		ctx.JSON(http.StatusInternalServerError, dtos.BaseResponse{
			Status:    constants.ResponseStatusError,
			Code:      constants.ResponseCodeInternalError,
			Message:   constants.ErrInternalServerError.Message,
			Timestamp: time.Now(),
		})
		return
	}
	if c.svc == nil {
		c.wrapError(ctx, constants.ErrInternalServerError)
		return
	}

	idParam := ctx.Param("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.Wrap(err))
		return
	}

	// Limit body size for chat prompt payload (64KB)
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, 64*1024)

	var req dtos.RecordingChatRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, dtos.BaseResponse{
			Status:    constants.ResponseStatusFail,
			Code:      constants.ResponseCodeBadRequest,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
		return
	}

	if err := req.Validate(); err != nil {
		ctx.JSON(http.StatusBadRequest, dtos.BaseResponse{
			Status:    constants.ResponseStatusFail,
			Code:      constants.ResponseCodeBadRequest,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
		return
	}

	ownershipToken := strings.TrimSpace(req.OwnershipToken)
	if ownershipToken == "" {
		ownershipToken = strings.TrimSpace(ctx.GetHeader("X-Ownership-Token"))
	}
	if ownershipToken == "" {
		ownershipToken = strings.TrimSpace(ctx.Query("token"))
	}
	if ownershipToken == "" {
		ownershipToken = strings.TrimSpace(ctx.Query("ownership_token"))
	}

	result, err := c.svc.InitiateRecordingChatStream(ctx.Request.Context(), id, ownershipToken, req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	// Write SSE Response Headers
	ctx.Writer.Header().Set("Content-Type", "text/event-stream")
	ctx.Writer.Header().Set("Cache-Control", "no-cache")
	ctx.Writer.Header().Set("Connection", "keep-alive")
	ctx.Writer.Header().Set("Transfer-Encoding", "chunked")
	ctx.Writer.Header().Set("X-Accel-Buffering", "no")
	ctx.Writer.WriteHeader(http.StatusOK)
	ctx.Writer.Flush()

	pingTicker := time.NewTicker(15 * time.Second)
	defer pingTicker.Stop()

	var fullContent strings.Builder

streamLoop:
	for {
		select {
		case <-ctx.Request.Context().Done():
			return
		case <-pingTicker.C:
			if err := sse.WritePing(ctx.Writer); err != nil {
				return
			}
		case chunk, ok := <-result.StreamChannel:
			if !ok {
				break streamLoop
			}
			if chunk.Err != nil {
				_ = sse.WriteChatError(ctx.Writer, chunk.Err.Error())
				return
			}
			if chunk.Content != "" {
				fullContent.WriteString(chunk.Content)
				if err := sse.WriteChatToken(ctx.Writer, chunk.Content); err != nil {
					return
				}
			}
		}
	}

	saveCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	asstMsg, citations, err := result.SaveAssistantMsg(saveCtx, fullContent.String())
	if err != nil {
		_ = sse.WriteChatError(ctx.Writer, "failed to persist assistant message")
		return
	}

	chunkIDs := result.RetrievedChunkIDs
	if chunkIDs == nil {
		chunkIDs = []string{}
	}

	_ = sse.WriteChatDone(ctx.Writer, sse.ChatDoneEvent{
		MessageID:         asstMsg.ID.String(),
		Content:           fullContent.String(),
		Citations:         citations,
		RetrievedChunkIDs: chunkIDs,
	})
}

// UpdateTranscriptSpeakers handles batch updating speaker names on a recording's transcript segments.
func (c *Controllers) UpdateTranscriptSpeakers(ctx *gin.Context) {
	if c == nil {
		ctx.JSON(http.StatusInternalServerError, dtos.BaseResponse{
			Status:    constants.ResponseStatusError,
			Code:      constants.ResponseCodeInternalError,
			Message:   constants.ErrInternalServerError.Message,
			Timestamp: time.Now(),
		})
		return
	}
	if c.svc == nil {
		c.wrapError(ctx, constants.ErrInternalServerError)
		return
	}

	idParam := ctx.Param("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.Wrap(err))
		return
	}

	var req dtos.UpdateSpeakersRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, dtos.BaseResponse{
			Status:    constants.ResponseStatusFail,
			Code:      constants.ResponseCodeBadRequest,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
		return
	}

	if err := req.Validate(); err != nil {
		ctx.JSON(http.StatusBadRequest, dtos.BaseResponse{
			Status:    constants.ResponseStatusFail,
			Code:      constants.ResponseCodeBadRequest,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
		return
	}

	ownershipToken := strings.TrimSpace(req.OwnershipToken)
	if ownershipToken == "" {
		ownershipToken = strings.TrimSpace(ctx.GetHeader("X-Ownership-Token"))
	}
	if ownershipToken == "" {
		ownershipToken = strings.TrimSpace(ctx.Query("token"))
	}
	if ownershipToken == "" {
		ownershipToken = strings.TrimSpace(ctx.Query("ownership_token"))
	}

	resp, err := c.svc.UpdateTranscriptSpeakers(ctx.Request.Context(), id, ownershipToken, req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, dtos.APIResponse[dtos.UpdateSpeakersResponse]{
		Status:    constants.ResponseStatusSuccess,
		Code:      constants.ResponseCodeSuccess,
		Message:   "transcript speaker labels updated successfully",
		Data:      *resp,
		Timestamp: time.Now(),
	})
}

// RegenerateSummary handles generating a new summary version with optional template and custom angle.
func (c *Controllers) RegenerateSummary(ctx *gin.Context) {
	if c == nil {
		ctx.JSON(http.StatusInternalServerError, dtos.BaseResponse{
			Status:    constants.ResponseStatusError,
			Code:      constants.ResponseCodeInternalError,
			Message:   constants.ErrInternalServerError.Message,
			Timestamp: time.Now(),
		})
		return
	}
	if c.svc == nil {
		c.wrapError(ctx, constants.ErrInternalServerError)
		return
	}

	idParam := ctx.Param("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.Wrap(err))
		return
	}

	var req dtos.RegenerateSummaryRequest
	if ctx.Request.ContentLength > 0 {
		if err := ctx.ShouldBindJSON(&req); err != nil {
			ctx.JSON(http.StatusBadRequest, dtos.BaseResponse{
				Status:    constants.ResponseStatusFail,
				Code:      constants.ResponseCodeBadRequest,
				Message:   err.Error(),
				Timestamp: time.Now(),
			})
			return
		}
	}

	if err := req.Validate(); err != nil {
		ctx.JSON(http.StatusBadRequest, dtos.BaseResponse{
			Status:    constants.ResponseStatusFail,
			Code:      constants.ResponseCodeBadRequest,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
		return
	}

	ownershipToken := strings.TrimSpace(req.OwnershipToken)
	if ownershipToken == "" {
		ownershipToken = strings.TrimSpace(ctx.GetHeader("X-Ownership-Token"))
	}
	if ownershipToken == "" {
		ownershipToken = strings.TrimSpace(ctx.Query("token"))
	}
	if ownershipToken == "" {
		ownershipToken = strings.TrimSpace(ctx.Query("ownership_token"))
	}

	resp, err := c.svc.RegenerateSummary(ctx.Request.Context(), id, ownershipToken, req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, dtos.APIResponse[dtos.SummaryVersionResponse]{
		Status:    constants.ResponseStatusSuccess,
		Code:      constants.ResponseCodeSuccess,
		Message:   "summary regenerated successfully",
		Data:      *resp,
		Timestamp: time.Now(),
	})
}

// ListSummaryVersions handles listing all summary versions for a recording.
func (c *Controllers) ListSummaryVersions(ctx *gin.Context) {
	if c == nil {
		ctx.JSON(http.StatusInternalServerError, dtos.BaseResponse{
			Status:    constants.ResponseStatusError,
			Code:      constants.ResponseCodeInternalError,
			Message:   constants.ErrInternalServerError.Message,
			Timestamp: time.Now(),
		})
		return
	}
	if c.svc == nil {
		c.wrapError(ctx, constants.ErrInternalServerError)
		return
	}

	idParam := ctx.Param("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.Wrap(err))
		return
	}

	ownershipToken := strings.TrimSpace(ctx.GetHeader("X-Ownership-Token"))
	if ownershipToken == "" {
		ownershipToken = strings.TrimSpace(ctx.Query("token"))
	}
	if ownershipToken == "" {
		ownershipToken = strings.TrimSpace(ctx.Query("ownership_token"))
	}

	resp, err := c.svc.ListSummaryVersions(ctx.Request.Context(), id, ownershipToken)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, dtos.APIResponse[[]dtos.SummaryVersionResponse]{
		Status:    constants.ResponseStatusSuccess,
		Code:      constants.ResponseCodeSuccess,
		Message:   "summary versions retrieved successfully",
		Data:      resp,
		Timestamp: time.Now(),
	})
}

// ActivateSummaryVersion handles switching the active summary version of a recording.
func (c *Controllers) ActivateSummaryVersion(ctx *gin.Context) {
	if c == nil {
		ctx.JSON(http.StatusInternalServerError, dtos.BaseResponse{
			Status:    constants.ResponseStatusError,
			Code:      constants.ResponseCodeInternalError,
			Message:   constants.ErrInternalServerError.Message,
			Timestamp: time.Now(),
		})
		return
	}
	if c.svc == nil {
		c.wrapError(ctx, constants.ErrInternalServerError)
		return
	}

	idParam := ctx.Param("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.Wrap(err))
		return
	}

	versionID := strings.TrimSpace(ctx.Param("versionId"))
	if versionID == "" {
		c.wrapError(ctx, constants.ErrBadRequest)
		return
	}

	ownershipToken := strings.TrimSpace(ctx.GetHeader("X-Ownership-Token"))
	if ownershipToken == "" {
		ownershipToken = strings.TrimSpace(ctx.Query("token"))
	}
	if ownershipToken == "" {
		ownershipToken = strings.TrimSpace(ctx.Query("ownership_token"))
	}

	resp, err := c.svc.ActivateSummaryVersion(ctx.Request.Context(), id, versionID, ownershipToken)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, dtos.APIResponse[dtos.SummaryVersionResponse]{
		Status:    constants.ResponseStatusSuccess,
		Code:      constants.ResponseCodeSuccess,
		Message:   "summary version activated successfully",
		Data:      *resp,
		Timestamp: time.Now(),
	})
}

// CreateInlineComment handles adding an inline comment or reply to a recording.
func (c *Controllers) CreateInlineComment(ctx *gin.Context) {
	if c == nil {
		ctx.JSON(http.StatusInternalServerError, dtos.BaseResponse{
			Status:    constants.ResponseStatusError,
			Code:      constants.ResponseCodeInternalError,
			Message:   constants.ErrInternalServerError.Message,
			Timestamp: time.Now(),
		})
		return
	}
	if c.svc == nil {
		c.wrapError(ctx, constants.ErrInternalServerError)
		return
	}

	idParam := ctx.Param("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.Wrap(err))
		return
	}

	var req dtos.CreateCommentRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, dtos.BaseResponse{
			Status:    constants.ResponseStatusFail,
			Code:      constants.ResponseCodeBadRequest,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
		return
	}

	if err := req.Validate(); err != nil {
		ctx.JSON(http.StatusBadRequest, dtos.BaseResponse{
			Status:    constants.ResponseStatusFail,
			Code:      constants.ResponseCodeBadRequest,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
		return
	}

	ownershipToken := strings.TrimSpace(req.OwnershipToken)
	if ownershipToken == "" {
		ownershipToken = strings.TrimSpace(ctx.GetHeader("X-Ownership-Token"))
	}
	if ownershipToken == "" {
		ownershipToken = strings.TrimSpace(ctx.Query("token"))
	}
	if ownershipToken == "" {
		ownershipToken = strings.TrimSpace(ctx.Query("ownership_token"))
	}

	resp, err := c.svc.CreateInlineComment(ctx.Request.Context(), id, ownershipToken, req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	ctx.JSON(http.StatusCreated, dtos.APIResponse[dtos.CommentResponse]{
		Status:    constants.ResponseStatusSuccess,
		Code:      constants.ResponseCodeSuccess,
		Message:   "comment created successfully",
		Data:      *resp,
		Timestamp: time.Now(),
	})
}

// ListInlineComments handles retrieving all timestamped inline comments for a recording.
func (c *Controllers) ListInlineComments(ctx *gin.Context) {
	if c == nil {
		ctx.JSON(http.StatusInternalServerError, dtos.BaseResponse{
			Status:    constants.ResponseStatusError,
			Code:      constants.ResponseCodeInternalError,
			Message:   constants.ErrInternalServerError.Message,
			Timestamp: time.Now(),
		})
		return
	}
	if c.svc == nil {
		c.wrapError(ctx, constants.ErrInternalServerError)
		return
	}

	idParam := ctx.Param("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.Wrap(err))
		return
	}

	ownershipToken := strings.TrimSpace(ctx.GetHeader("X-Ownership-Token"))
	if ownershipToken == "" {
		ownershipToken = strings.TrimSpace(ctx.Query("token"))
	}
	if ownershipToken == "" {
		ownershipToken = strings.TrimSpace(ctx.Query("ownership_token"))
	}

	resp, err := c.svc.ListInlineComments(ctx.Request.Context(), id, ownershipToken)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, dtos.APIResponse[[]dtos.CommentResponse]{
		Status:    constants.ResponseStatusSuccess,
		Code:      constants.ResponseCodeSuccess,
		Message:   "comments retrieved successfully",
		Data:      resp,
		Timestamp: time.Now(),
	})
}

// DeleteInlineComment handles deleting an inline comment for a recording.
func (c *Controllers) DeleteInlineComment(ctx *gin.Context) {
	if c == nil {
		ctx.JSON(http.StatusInternalServerError, dtos.BaseResponse{
			Status:    constants.ResponseStatusError,
			Code:      constants.ResponseCodeInternalError,
			Message:   constants.ErrInternalServerError.Message,
			Timestamp: time.Now(),
		})
		return
	}
	if c.svc == nil {
		c.wrapError(ctx, constants.ErrInternalServerError)
		return
	}

	idParam := ctx.Param("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.Wrap(err))
		return
	}

	commentIDParam := ctx.Param("commentId")
	commentID, err := uuid.Parse(commentIDParam)
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.Wrap(err))
		return
	}

	ownershipToken := strings.TrimSpace(ctx.GetHeader("X-Ownership-Token"))
	if ownershipToken == "" {
		ownershipToken = strings.TrimSpace(ctx.Query("token"))
	}
	if ownershipToken == "" {
		ownershipToken = strings.TrimSpace(ctx.Query("ownership_token"))
	}

	if err := c.svc.DeleteInlineComment(ctx.Request.Context(), id, commentID, ownershipToken); err != nil {
		c.wrapError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, dtos.BaseResponse{
		Status:    constants.ResponseStatusSuccess,
		Code:      constants.ResponseCodeSuccess,
		Message:   "comment deleted successfully",
		Timestamp: time.Now(),
	})
}

// ExportRecording handles downloading meeting MOM and transcript in multiple formats (markdown, txt, json, pdf).
func (c *Controllers) ExportRecording(ctx *gin.Context) {
	if c == nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": constants.ErrInternalServerError.Error()})
		return
	}
	if c.svc == nil {
		c.wrapError(ctx, constants.ErrInternalServerError)
		return
	}

	idParam := ctx.Param("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.Wrap(err))
		return
	}

	format := strings.TrimSpace(ctx.DefaultQuery("format", "markdown"))

	ownershipToken := strings.TrimSpace(ctx.GetHeader("X-Ownership-Token"))
	if ownershipToken == "" {
		ownershipToken = strings.TrimSpace(ctx.Query("token"))
	}
	if ownershipToken == "" {
		ownershipToken = strings.TrimSpace(ctx.Query("ownership_token"))
	}

	result, err := c.svc.ExportRecordingMOM(ctx.Request.Context(), id, ownershipToken, format)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	ctx.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", result.Filename))
	ctx.Data(http.StatusOK, result.ContentType, result.Data)
}

// SearchRecordings handles cross-meeting semantic vector search across user recordings.
func (c *Controllers) SearchRecordings(ctx *gin.Context) {
	if c == nil {
		ctx.JSON(http.StatusInternalServerError, dtos.BaseResponse{
			Status:    constants.ResponseStatusError,
			Code:      constants.ResponseCodeInternalError,
			Message:   constants.ErrInternalServerError.Message,
			Timestamp: time.Now(),
		})
		return
	}
	if c.svc == nil {
		c.wrapError(ctx, constants.ErrInternalServerError)
		return
	}

	var query dtos.SemanticSearchQuery
	if err := ctx.ShouldBindQuery(&query); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.Wrap(err))
		return
	}

	if err := query.Validate(); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.Wrap(err))
		return
	}

	res, err := c.svc.SearchWorkspaceSemantic(ctx.Request.Context(), query)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	if res == nil {
		c.wrapError(ctx, constants.ErrInternalServerError)
		return
	}

	ctx.JSON(http.StatusOK, dtos.APIResponse[dtos.SemanticSearchResponse]{
		Status:    constants.ResponseStatusSuccess,
		Code:      constants.ResponseCodeSuccess,
		Message:   "search results retrieved successfully",
		Data:      *res,
		Timestamp: time.Now(),
	})
}

