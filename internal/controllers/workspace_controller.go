package controllers

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/validations"
)

// ListWorkspaces handles retrieving all workspaces the authenticated user belongs to.
func (c *Controllers) ListWorkspaces(ctx *gin.Context) {
	resp, err := c.svc.ListUserWorkspaces(ctx.Request.Context())
	if err != nil {
		c.wrapError(ctx, err)
		return
	}
	c.respondOK(ctx, "workspaces retrieved successfully", resp)
}

// CreateWorkspace handles provisioning a new team workspace.
func (c *Controllers) CreateWorkspace(ctx *gin.Context) {
	var req dtos.CreateWorkspaceRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	if err := validations.ValidateCreateWorkspaceRequest(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	resp, err := c.svc.CreateWorkspace(ctx.Request.Context(), req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}
	c.respondCreated(ctx, "workspace created successfully", resp)
}

// GetWorkspaceDetail handles retrieving metadata for a specific workspace.
func (c *Controllers) GetWorkspaceDetail(ctx *gin.Context) {
	id, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.Wrap(err))
		return
	}

	resp, err := c.svc.GetWorkspaceDetail(ctx.Request.Context(), id)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}
	c.respondOK(ctx, "workspace retrieved successfully", resp)
}

// UpdateWorkspace handles modifying workspace name or logo.
func (c *Controllers) UpdateWorkspace(ctx *gin.Context) {
	id, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.Wrap(err))
		return
	}

	var req dtos.UpdateWorkspaceRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	if err := validations.ValidateUpdateWorkspaceRequest(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	resp, err := c.svc.UpdateWorkspace(ctx.Request.Context(), id, req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}
	c.respondOK(ctx, "workspace updated successfully", resp)
}

// DeleteWorkspace handles terminating a team workspace.
func (c *Controllers) DeleteWorkspace(ctx *gin.Context) {
	id, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.Wrap(err))
		return
	}

	if err := c.svc.DeleteWorkspace(ctx.Request.Context(), id); err != nil {
		c.wrapError(ctx, err)
		return
	}
	c.respondEmpty(ctx, "workspace deleted successfully")
}

// ListWorkspaceMembers handles retrieving all registered members of a workspace.
func (c *Controllers) ListWorkspaceMembers(ctx *gin.Context) {
	id, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.Wrap(err))
		return
	}

	resp, err := c.svc.ListWorkspaceMembers(ctx.Request.Context(), id)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}
	c.respondOK(ctx, "workspace members retrieved successfully", resp)
}

// UpdateWorkspaceMemberRole handles changing a member's role in a workspace.
func (c *Controllers) UpdateWorkspaceMemberRole(ctx *gin.Context) {
	workspaceID, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.Wrap(err))
		return
	}

	memberUserID, err := uuid.Parse(ctx.Param("userId"))
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.Wrap(err))
		return
	}

	var req dtos.UpdateWorkspaceMemberRoleRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	if err := validations.ValidateUpdateWorkspaceMemberRoleRequest(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	if err := c.svc.UpdateWorkspaceMemberRole(ctx.Request.Context(), workspaceID, memberUserID, req); err != nil {
		c.wrapError(ctx, err)
		return
	}
	c.respondEmpty(ctx, "member role updated successfully")
}

// RemoveWorkspaceMember handles kicking a member or leaving a workspace.
func (c *Controllers) RemoveWorkspaceMember(ctx *gin.Context) {
	workspaceID, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.Wrap(err))
		return
	}

	memberUserID, err := uuid.Parse(ctx.Param("userId"))
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.Wrap(err))
		return
	}

	if err := c.svc.RemoveWorkspaceMember(ctx.Request.Context(), workspaceID, memberUserID); err != nil {
		c.wrapError(ctx, err)
		return
	}
	c.respondEmpty(ctx, "member removed successfully")
}

