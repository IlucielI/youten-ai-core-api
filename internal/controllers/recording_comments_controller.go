// recording_comments_controller.go — inline comments, recording chat, and workspace memory endpoints.
package controllers

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/sse"
	"code-base-golang/internal/validations"
)

// CreateInlineComment handles adding an inline comment or reply to a recording.
func (c *Controllers) CreateInlineComment(ctx *gin.Context) {

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

	if err := validations.ValidateCreateCommentRequest(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, dtos.BaseResponse{
			Status:    constants.ResponseStatusFail,
			Code:      constants.ResponseCodeBadRequest,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
		return
	}

	ownershipToken := c.extractOwnershipToken(ctx, req.OwnershipToken)

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

	idParam := ctx.Param("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.Wrap(err))
		return
	}

	ownershipToken := c.extractOwnershipToken(ctx)

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

	ownershipToken := c.extractOwnershipToken(ctx)

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

// StreamRecordingChat handles interactive RAG chat streaming via Server-Sent Events (SSE).
func (c *Controllers) StreamRecordingChat(ctx *gin.Context) {

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

	if err := validations.ValidateRecordingChatRequest(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, dtos.BaseResponse{
			Status:    constants.ResponseStatusFail,
			Code:      constants.ResponseCodeBadRequest,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
		return
	}

	ownershipToken := c.extractOwnershipToken(ctx, req.OwnershipToken)

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
	streaming := true

	for streaming {
		select {
		case <-ctx.Request.Context().Done():
			return
		case <-pingTicker.C:
			if err := sse.WritePing(ctx.Writer); err != nil {
				return
			}
		case chunk, ok := <-result.StreamChannel:
			if !ok {
				streaming = false
				break
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

// AskWorkspaceMemory handles cross-meeting AI chat questions with meeting citations.
func (c *Controllers) AskWorkspaceMemory(ctx *gin.Context) {

	var req dtos.WorkspaceAskRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.Wrap(err))
		return
	}

	if err := validations.ValidateWorkspaceAskRequest(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.Wrap(err))
		return
	}

	res, err := c.svc.AskWorkspaceMemory(ctx.Request.Context(), req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	if res == nil {
		c.wrapError(ctx, constants.ErrInternalServerError)
		return
	}

	ctx.JSON(http.StatusOK, dtos.APIResponse[dtos.WorkspaceAskResponse]{
		Status:    constants.ResponseStatusSuccess,
		Code:      constants.ResponseCodeSuccess,
		Message:   "workspace memory response generated successfully",
		Data:      *res,
		Timestamp: time.Now(),
	})
}
