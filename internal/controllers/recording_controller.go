package controllers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
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

