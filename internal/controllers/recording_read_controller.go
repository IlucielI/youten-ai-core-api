// recording_read_controller.go — recording detail, listing, progress stream, and search endpoints.
package controllers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/sse"
	"code-base-golang/internal/validations"
)

// GetRecordingDetail handles retrieving recording metadata, segments, active summary, chapters,
// and presigned playback audio URL.
func (c *Controllers) GetRecordingDetail(ctx *gin.Context) {

	idParam := ctx.Param("id")
	recID, err := uuid.Parse(idParam)
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.Wrap(err))
		return
	}

	// Extract ownership token from header or query param
	token := c.extractOwnershipToken(ctx)

	resp, err := c.svc.GetRecordingDetail(ctx.Request.Context(), recID, token)
	if err != nil {
		c.wrapError(ctx, err)
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

	ctx.JSON(http.StatusOK, dtos.APIResponse[dtos.RecordingListResponse]{
		Status:    constants.ResponseStatusSuccess,
		Code:      constants.ResponseCodeSuccess,
		Message:   "recordings retrieved successfully",
		Data:      *resp,
		Timestamp: time.Now(),
	})
}

// StreamRecordingProgress handles real-time SSE progress streaming for a recording pipeline.
func (c *Controllers) StreamRecordingProgress(ctx *gin.Context) {

	idParam := ctx.Param("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.Wrap(err))
		return
	}

	ownershipToken := c.extractOwnershipToken(ctx)

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

// SearchRecordings handles cross-meeting semantic vector search across user recordings.
func (c *Controllers) SearchRecordings(ctx *gin.Context) {

	var query dtos.SemanticSearchQuery
	if err := ctx.ShouldBindQuery(&query); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.Wrap(err))
		return
	}

	if err := validations.ValidateSemanticSearchQuery(&query); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.Wrap(err))
		return
	}

	res, err := c.svc.SearchWorkspaceSemantic(ctx.Request.Context(), query)
	if err != nil {
		c.wrapError(ctx, err)
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
