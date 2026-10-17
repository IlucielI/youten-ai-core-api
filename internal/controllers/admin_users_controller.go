package controllers

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/validations"
)

// AdminListUsers handles GET /v1/admin/users.
func (c *Controllers) AdminListUsers(ctx *gin.Context) {
	var query dtos.AdminUserListQuery
	if err := ctx.ShouldBindQuery(&query); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid query parameters"))
		return
	}

	query.SetDefaults()

	resp, err := c.svc.AdminListUsers(ctx.Request.Context(), query)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondOK(ctx, "Users retrieved successfully", resp)
}

// AdminOverrideUserQuota handles PATCH /v1/admin/users/:id/quota.
func (c *Controllers) AdminOverrideUserQuota(ctx *gin.Context) {
	idStr := ctx.Param("id")
	userID, err := uuid.Parse(idStr)
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid user ID"))
		return
	}

	var req dtos.AdminUserQuotaOverrideRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid request payload"))
		return
	}

	if err := validations.ValidateAdminUserQuotaOverrideRequest(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	meta, ok := c.getAdminActionMeta(ctx)
	if !ok {
		return
	}

	resp, err := c.svc.AdminOverrideUserQuota(ctx.Request.Context(), meta, userID, req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondOK(ctx, "User quota updated successfully", resp)
}

// AdminOverrideUserMaxFileSize handles PATCH /v1/admin/users/:id/max-file-size.
func (c *Controllers) AdminOverrideUserMaxFileSize(ctx *gin.Context) {
	idStr := ctx.Param("id")
	userID, err := uuid.Parse(idStr)
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid user ID"))
		return
	}

	var req dtos.AdminUserMaxFileSizeRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid request payload"))
		return
	}

	if err := validations.ValidateAdminUserMaxFileSizeRequest(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	meta, ok := c.getAdminActionMeta(ctx)
	if !ok {
		return
	}

	resp, err := c.svc.AdminOverrideUserMaxFileSize(ctx.Request.Context(), meta, userID, req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondOK(ctx, "User max file size updated successfully", resp)
}

// AdminRevokeUserSessions handles POST /v1/admin/users/:id/revoke-sessions.
func (c *Controllers) AdminRevokeUserSessions(ctx *gin.Context) {
	idStr := ctx.Param("id")
	userID, err := uuid.Parse(idStr)
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid user ID"))
		return
	}

	meta, ok := c.getAdminActionMeta(ctx)
	if !ok {
		return
	}

	resp, err := c.svc.AdminRevokeUserSessions(ctx.Request.Context(), meta, userID)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondOK(ctx, "User sessions revoked successfully", resp)
}

// AdminAssignUserRole handles PATCH /v1/admin/users/:id/role.
func (c *Controllers) AdminAssignUserRole(ctx *gin.Context) {
	idStr := ctx.Param("id")
	userID, err := uuid.Parse(idStr)
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid user ID"))
		return
	}

	var req dtos.AdminAssignUserRoleRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid request payload"))
		return
	}

	if err := validations.ValidateAdminAssignUserRoleRequest(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	meta, ok := c.getAdminActionMeta(ctx)
	if !ok {
		return
	}

	resp, err := c.svc.AdminAssignUserRole(ctx.Request.Context(), meta, userID, req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondOK(ctx, "User role assigned successfully", resp)
}
