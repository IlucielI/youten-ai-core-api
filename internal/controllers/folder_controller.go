package controllers

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/validations"
)

// CreateFolder handles creating a folder within a workspace.
func (c *Controllers) CreateFolder(ctx *gin.Context) {
	workspaceID, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid workspace id"))
		return
	}

	var req dtos.CreateFolderRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	if err := validations.ValidateCreateFolderRequest(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	resp, err := c.svc.CreateFolder(ctx.Request.Context(), workspaceID, req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}
	c.respondCreated(ctx, "folder created successfully", resp)
}

// ListWorkspaceFolders handles retrieving the hierarchical folder tree for a workspace.
func (c *Controllers) ListWorkspaceFolders(ctx *gin.Context) {
	workspaceID, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid workspace id"))
		return
	}

	resp, err := c.svc.ListWorkspaceFolders(ctx.Request.Context(), workspaceID)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}
	c.respondOK(ctx, "folders retrieved successfully", resp)
}

// UpdateFolder handles updating folder metadata or moving it in the hierarchy.
func (c *Controllers) UpdateFolder(ctx *gin.Context) {
	workspaceID, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid workspace id"))
		return
	}

	folderID, err := uuid.Parse(ctx.Param("folderId"))
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid folder id"))
		return
	}

	var req dtos.UpdateFolderRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	if err := validations.ValidateUpdateFolderRequest(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	resp, err := c.svc.UpdateFolder(ctx.Request.Context(), workspaceID, folderID, req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}
	c.respondOK(ctx, "folder updated successfully", resp)
}

// DeleteFolder handles removing a folder from a workspace.
func (c *Controllers) DeleteFolder(ctx *gin.Context) {
	workspaceID, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid workspace id"))
		return
	}

	folderID, err := uuid.Parse(ctx.Param("folderId"))
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid folder id"))
		return
	}

	if err := c.svc.DeleteFolder(ctx.Request.Context(), workspaceID, folderID); err != nil {
		c.wrapError(ctx, err)
		return
	}
	c.respondOK(ctx, "folder deleted successfully", nil)
}

// MoveRecording handles moving a recording to a different folder.
func (c *Controllers) MoveRecording(ctx *gin.Context) {
	recordingID, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid recording id"))
		return
	}

	var req dtos.MoveRecordingRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	if err := c.svc.MoveRecording(ctx.Request.Context(), recordingID, req); err != nil {
		c.wrapError(ctx, err)
		return
	}
	c.respondOK(ctx, "recording moved successfully", nil)
}

