package controllers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/pkg/ctxmeta"
	"code-base-golang/internal/services"
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

	ctx.JSON(http.StatusOK, dtos.APIResponse[*dtos.AdminUserListResponse]{
		Status:    constants.ResponseStatusSuccess,
		Code:      constants.ResponseCodeSuccess,
		Message:   constants.ResponseMessageSuccess,
		Data:      resp,
		Timestamp: time.Now(),
	})
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

	if err := req.Validate(); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	adminUser, ok := ctxmeta.GetAdminAuthUser(ctx.Request.Context())
	if !ok || adminUser.AdminID == uuid.Nil {
		c.wrapError(ctx, constants.ErrUnauthorized.WithMessage("admin authentication required"))
		return
	}

	meta := services.AdminActionMeta{
		AdminID:   adminUser.AdminID,
		IPAddress: ctxmeta.GetClientIP(ctx.Request.Context()),
		UserAgent: ctxmeta.GetUserAgent(ctx.Request.Context()),
	}

	resp, err := c.svc.AdminOverrideUserQuota(ctx.Request.Context(), meta, userID, req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, dtos.APIResponse[*dtos.AdminUserQuotaResponse]{
		Status:    constants.ResponseStatusSuccess,
		Code:      constants.ResponseCodeSuccess,
		Message:   constants.ResponseMessageSuccess,
		Data:      resp,
		Timestamp: time.Now(),
	})
}

// AdminRevokeUserSessions handles POST /v1/admin/users/:id/revoke-sessions.
func (c *Controllers) AdminRevokeUserSessions(ctx *gin.Context) {
	idStr := ctx.Param("id")
	userID, err := uuid.Parse(idStr)
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid user ID"))
		return
	}

	adminUser, ok := ctxmeta.GetAdminAuthUser(ctx.Request.Context())
	if !ok || adminUser.AdminID == uuid.Nil {
		c.wrapError(ctx, constants.ErrUnauthorized.WithMessage("admin authentication required"))
		return
	}

	meta := services.AdminActionMeta{
		AdminID:   adminUser.AdminID,
		IPAddress: ctxmeta.GetClientIP(ctx.Request.Context()),
		UserAgent: ctxmeta.GetUserAgent(ctx.Request.Context()),
	}

	resp, err := c.svc.AdminRevokeUserSessions(ctx.Request.Context(), meta, userID)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, dtos.APIResponse[*dtos.AdminRevokeUserSessionsResponse]{
		Status:    constants.ResponseStatusSuccess,
		Code:      constants.ResponseCodeSuccess,
		Message:   constants.ResponseMessageSuccess,
		Data:      resp,
		Timestamp: time.Now(),
	})
}

// AdminListRoles handles GET /v1/admin/roles.
func (c *Controllers) AdminListRoles(ctx *gin.Context) {
	resp, err := c.svc.AdminListRoles(ctx.Request.Context())
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, dtos.APIResponse[*dtos.AdminRoleListResponse]{
		Status:    constants.ResponseStatusSuccess,
		Code:      constants.ResponseCodeSuccess,
		Message:   constants.ResponseMessageSuccess,
		Data:      resp,
		Timestamp: time.Now(),
	})
}
