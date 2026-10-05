// recording_summary_controller.go — transcript speakers, summary versions, export, and workspace speakers endpoints.
package controllers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/validations"
)

// UpdateTranscriptSpeakers handles batch updating speaker names on a recording's transcript segments.
func (c *Controllers) UpdateTranscriptSpeakers(ctx *gin.Context) {

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

	if err := validations.ValidateUpdateSpeakersRequest(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, dtos.BaseResponse{
			Status:    constants.ResponseStatusFail,
			Code:      constants.ResponseCodeBadRequest,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
		return
	}

	ownershipToken := c.extractOwnershipToken(ctx, req.OwnershipToken)

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

	if err := validations.ValidateRegenerateSummaryRequest(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, dtos.BaseResponse{
			Status:    constants.ResponseStatusFail,
			Code:      constants.ResponseCodeBadRequest,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
		return
	}

	ownershipToken := c.extractOwnershipToken(ctx, req.OwnershipToken)

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

	idParam := ctx.Param("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.Wrap(err))
		return
	}

	ownershipToken := c.extractOwnershipToken(ctx)

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

	ownershipToken := c.extractOwnershipToken(ctx)

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

// ExportRecording handles downloading meeting MOM and transcript in multiple formats (markdown, txt, json, pdf).
func (c *Controllers) ExportRecording(ctx *gin.Context) {
	if c == nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": constants.ErrInternalServerError.Error()})
		return
	}

	idParam := ctx.Param("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.Wrap(err))
		return
	}

	format := strings.TrimSpace(ctx.DefaultQuery("format", "markdown"))

	ownershipToken := c.extractOwnershipToken(ctx)

	result, err := c.svc.ExportRecordingMOM(ctx.Request.Context(), id, ownershipToken, format)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	ctx.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", result.Filename))
	ctx.Data(http.StatusOK, result.ContentType, result.Data)
}

// GetWorkspaceSpeakers handles retrieving aggregated speaker directory metrics across user meetings.
func (c *Controllers) GetWorkspaceSpeakers(ctx *gin.Context) {

	res, err := c.svc.GetWorkspaceSpeakers(ctx.Request.Context())
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, dtos.APIResponse[dtos.SpeakerDirectoryResponse]{
		Status:    constants.ResponseStatusSuccess,
		Code:      constants.ResponseCodeSuccess,
		Message:   "workspace speakers retrieved successfully",
		Data:      *res,
		Timestamp: time.Now(),
	})
}
