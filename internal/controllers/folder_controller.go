package controllers

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/pkg/ctxmeta"
	"code-base-golang/internal/validations"
)

// resolveWorkspaceID resolves the target workspace ID from URL parameter :id, query param ?workspaceId,
// request context X-Workspace-ID, or active workspace fallback.
func (c *Controllers) resolveWorkspaceID(ctx *gin.Context) (uuid.UUID, error) {
	if idParam := strings.TrimSpace(ctx.Param("id")); idParam != "" {
		parsed, err := uuid.Parse(idParam)
		if err != nil {
			return uuid.Nil, constants.ErrBadRequest.WithMessage("invalid workspace id")
		}
		return parsed, nil
	}
	if qID := strings.TrimSpace(ctx.Query("workspaceId")); qID != "" {
		if parsed, err := uuid.Parse(qID); err == nil && parsed != uuid.Nil {
			return parsed, nil
		}
	}
	if wsID, ok := ctxmeta.GetWorkspaceID(ctx.Request.Context()); ok && wsID != uuid.Nil {
		return wsID, nil
	}
	if ctxmeta.IsAuthenticated(ctx.Request.Context()) {
		workspaces, err := c.svc.ListUserWorkspaces(ctx.Request.Context())
		if err == nil && len(workspaces) > 0 {
			return workspaces[0].ID, nil
		}
	}
	return uuid.Nil, constants.ErrBadRequest.WithMessage("workspace context is required")
}

// resolveFolderAndWorkspaceID resolves folder ID and associated workspace ID for updates/deletes.
func (c *Controllers) resolveFolderAndWorkspaceID(ctx *gin.Context) (uuid.UUID, uuid.UUID, error) {
	folderIDParam := strings.TrimSpace(ctx.Param("folderId"))
	if folderIDParam == "" {
		folderIDParam = strings.TrimSpace(ctx.Param("id"))
	}
	if folderIDParam == "" {
		return uuid.Nil, uuid.Nil, constants.ErrBadRequest.WithMessage("invalid folder id")
	}

	folderID, err := uuid.Parse(folderIDParam)
	if err != nil {
		return uuid.Nil, uuid.Nil, constants.ErrBadRequest.WithMessage("invalid folder id")
	}

	// If workspace :id is provided in URL path (e.g., /v1/workspaces/:id/folders/:folderId)
	if wsParam := strings.TrimSpace(ctx.Param("id")); wsParam != "" && wsParam != folderIDParam {
		wsID, err := uuid.Parse(wsParam)
		if err != nil {
			return uuid.Nil, uuid.Nil, constants.ErrBadRequest.WithMessage("invalid workspace id")
		}
		return wsID, folderID, nil
	}

	// Otherwise resolve via context/fallback
	wsID, err := c.resolveWorkspaceID(ctx)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	return wsID, folderID, nil
}

// CreateFolder handles creating a folder within a workspace.
func (c *Controllers) CreateFolder(ctx *gin.Context) {
	workspaceID, err := c.resolveWorkspaceID(ctx)
	if err != nil {
		c.wrapError(ctx, err)
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
	workspaceID, err := c.resolveWorkspaceID(ctx)
	if err != nil {
		c.wrapError(ctx, err)
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
	workspaceID, folderID, err := c.resolveFolderAndWorkspaceID(ctx)
	if err != nil {
		c.wrapError(ctx, err)
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
	workspaceID, folderID, err := c.resolveFolderAndWorkspaceID(ctx)
	if err != nil {
		c.wrapError(ctx, err)
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

