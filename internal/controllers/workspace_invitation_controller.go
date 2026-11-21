package controllers

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/validations"
)

// InviteWorkspaceMember handles inviting a new user to a workspace.
func (c *Controllers) InviteWorkspaceMember(ctx *gin.Context) {
	workspaceID, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.Wrap(err))
		return
	}

	var req dtos.InviteMemberRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	if err := validations.ValidateInviteMemberRequest(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	resp, err := c.svc.InviteMember(ctx.Request.Context(), workspaceID, req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}
	c.respondCreated(ctx, "invitation sent successfully", resp)
}

// ListWorkspaceInvitations handles listing all pending invites in a workspace.
func (c *Controllers) ListWorkspaceInvitations(ctx *gin.Context) {
	workspaceID, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.Wrap(err))
		return
	}

	resp, err := c.svc.ListWorkspaceInvitations(ctx.Request.Context(), workspaceID)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}
	c.respondOK(ctx, "workspace invitations retrieved successfully", resp)
}

// RevokeWorkspaceInvitation handles revoking a pending invitation.
func (c *Controllers) RevokeWorkspaceInvitation(ctx *gin.Context) {
	workspaceID, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.Wrap(err))
		return
	}

	inviteID, err := uuid.Parse(ctx.Param("inviteId"))
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.Wrap(err))
		return
	}

	if err := c.svc.RevokeInvitation(ctx.Request.Context(), workspaceID, inviteID); err != nil {
		c.wrapError(ctx, err)
		return
	}
	c.respondOK(ctx, "invitation revoked successfully", nil)
}

// GetInvitationDetail handles public inspection of an invite token.
func (c *Controllers) GetInvitationDetail(ctx *gin.Context) {
	token := ctx.Param("token")
	if token == "" {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invitation token is required"))
		return
	}

	resp, err := c.svc.GetInvitationDetail(ctx.Request.Context(), token)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}
	c.respondOK(ctx, "invitation details retrieved successfully", resp)
}

// AcceptWorkspaceInvitation handles accepting an invitation by an authenticated user.
func (c *Controllers) AcceptWorkspaceInvitation(ctx *gin.Context) {
	token := ctx.Param("token")
	if token == "" {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invitation token is required"))
		return
	}

	resp, err := c.svc.AcceptInvitation(ctx.Request.Context(), token)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}
	c.respondOK(ctx, "invitation accepted successfully", resp)
}
