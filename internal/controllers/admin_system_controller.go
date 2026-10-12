package controllers

import (
	"github.com/gin-gonic/gin"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/validations"
)

// AdminGetSystemConfig handles GET /v1/admin/config.
func (c *Controllers) AdminGetSystemConfig(ctx *gin.Context) {
	meta, ok := c.getAdminActionMeta(ctx)
	if !ok {
		return
	}

	resp, err := c.svc.AdminGetSystemConfig(ctx.Request.Context(), meta)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondOK(ctx, "System configuration retrieved successfully", resp)
}

// AdminUpdateSystemConfig handles PATCH /v1/admin/config.
func (c *Controllers) AdminUpdateSystemConfig(ctx *gin.Context) {
	meta, ok := c.getAdminActionMeta(ctx)
	if !ok {
		return
	}

	var req dtos.UpdateSystemConfigRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid json payload"))
		return
	}
	if err := validations.ValidateUpdateSystemConfigRequest(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	resp, err := c.svc.AdminUpdateSystemConfig(ctx.Request.Context(), meta, req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondOK(ctx, "System configuration updated successfully", resp)
}

// AdminListAuditLogs handles GET /v1/admin/audit-logs.
func (c *Controllers) AdminListAuditLogs(ctx *gin.Context) {
	if _, ok := c.getAdminActionMeta(ctx); !ok {
		return
	}

	var q dtos.AdminAuditLogListQuery
	if err := ctx.ShouldBindQuery(&q); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid query parameters"))
		return
	}
	q.SetDefaults()

	resp, err := c.svc.AdminListAuditLogs(ctx.Request.Context(), q)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondOK(ctx, "Audit logs retrieved successfully", resp)
}

// AdminGetOverviewStats handles GET /v1/admin/stats/overview.
func (c *Controllers) AdminGetOverviewStats(ctx *gin.Context) {
	_, ok := c.getAdminActionMeta(ctx)
	if !ok {
		return
	}

	resp, err := c.svc.AdminGetOverviewStats(ctx.Request.Context())
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondOK(ctx, "Overview stats retrieved successfully", resp)
}

// AdminGetCostOversight handles GET /v1/admin/stats/costs.
func (c *Controllers) AdminGetCostOversight(ctx *gin.Context) {
	_, ok := c.getAdminActionMeta(ctx)
	if !ok {
		return
	}

	resp, err := c.svc.AdminGetCostOversight(ctx.Request.Context())
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondOK(ctx, "Cost oversight retrieved successfully", resp)
}
