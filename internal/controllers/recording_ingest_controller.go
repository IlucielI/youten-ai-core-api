// recording_ingest_controller.go — media ingestion and pipeline retry endpoints.
package controllers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/validations"
)

// PresignUpload generates a pre-signed S3 PUT URL so that the frontend
// can upload the media file directly to object storage without streaming through the backend.
func (c *Controllers) PresignUpload(ctx *gin.Context) {
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

	if err := validations.ValidatePresignUploadRequest(&req); err != nil {
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

	c.respondOK(ctx, "presigned upload url generated", resp)
}

// UploadRecording confirms media file upload after the frontend uploaded directly to S3.
// The frontend only submits the filename and metadata, preventing file streams through the backend.
func (c *Controllers) UploadRecording(ctx *gin.Context) {
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

	if err := validations.ValidateUploadRecordingRequest(&req); err != nil {
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

	c.respondCreated(ctx, "recording uploaded successfully", resp)
}

// ImportURL handles media ingestion from a remote URL with Anti-SSRF protection.
func (c *Controllers) ImportURL(ctx *gin.Context) {
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

	if err := validations.ValidateImportURLRequest(&req); err != nil {
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

	c.respondCreated(ctx, "recording imported successfully", resp)
}

// RetryRecording handles POST /v1/recordings/:id/retry to initiate a smart retry of a failed pipeline.
func (c *Controllers) RetryRecording(ctx *gin.Context) {

	idParam := ctx.Param("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.Wrap(err))
		return
	}

	var bodyToken string
	if ctx.Request.Body != nil && ctx.Request.ContentLength > 0 {
		var req dtos.RetryRecordingRequest
		if err := ctx.ShouldBindJSON(&req); err == nil {
			bodyToken = req.OwnershipToken
		}
	}
	ownershipToken := c.extractOwnershipToken(ctx, bodyToken)

	resp, err := c.svc.RetryRecordingPipeline(ctx.Request.Context(), id, ownershipToken)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondOK(ctx, "pipeline retry initiated successfully", resp)
}
