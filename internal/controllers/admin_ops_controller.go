package controllers

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/validations"
)

// AdminGetDLQPipeline retrieves dead-lettered and stuck pipeline jobs.
func (c *Controllers) AdminGetDLQPipeline(ctx *gin.Context) {
	meta, ok := c.getAdminActionMeta(ctx)
	if !ok {
		return
	}

	var query dtos.AdminDLQListQuery
	if err := ctx.ShouldBindQuery(&query); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid query parameters"))
		return
	}

	query.SetDefaults()

	if err := validations.ValidateAdminDLQListQuery(&query); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	resp, err := c.svc.AdminGetDLQMessages(ctx.Request.Context(), meta, query.Page, query.Limit)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondOK(ctx, "DLQ messages retrieved successfully", resp)
}

// AdminRetryDLQJob handles POST /v1/admin/pipeline/dlq/retry.
func (c *Controllers) AdminRetryDLQJob(ctx *gin.Context) {
	meta, ok := c.getAdminActionMeta(ctx)
	if !ok {
		return
	}

	var req dtos.DLQRetryRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid json payload"))
		return
	}

	if err := validations.ValidateDLQRetryRequest(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	resp, err := c.svc.AdminRetryDLQJob(ctx.Request.Context(), meta, req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondOK(ctx, "DLQ job retried successfully", resp)
}

// AdminListReports handles GET /v1/admin/reports.
func (c *Controllers) AdminListReports(ctx *gin.Context) {
	if _, ok := c.getAdminActionMeta(ctx); !ok {
		return
	}

	var q dtos.AdminReportListQuery
	if err := ctx.ShouldBindQuery(&q); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid query parameters"))
		return
	}
	q.SetDefaults()

	resp, err := c.svc.AdminListReports(ctx.Request.Context(), q)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondOK(ctx, "Reports retrieved successfully", resp)
}

// AdminResolveReport handles POST /v1/admin/reports/:id/resolve.
func (c *Controllers) AdminResolveReport(ctx *gin.Context) {
	meta, ok := c.getAdminActionMeta(ctx)
	if !ok {
		return
	}

	reportIDStr := ctx.Param("id")
	reportID, err := uuid.Parse(reportIDStr)
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid report ID"))
		return
	}

	var req dtos.ResolveReportRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid JSON payload"))
		return
	}
	if err := validations.ValidateResolveReportRequest(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	resp, err := c.svc.AdminResolveReport(ctx.Request.Context(), meta, reportID, req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondOK(ctx, "Report resolved successfully", resp)
}

// AdminListJobs handles GET /v1/admin/jobs to retrieve all platform generation jobs.
func (c *Controllers) AdminListJobs(ctx *gin.Context) {
	meta, ok := c.getAdminActionMeta(ctx)
	if !ok {
		return
	}

	var query dtos.AdminJobListQuery
	if err := ctx.ShouldBindQuery(&query); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid query parameters"))
		return
	}

	resp, err := c.svc.AdminListJobs(ctx.Request.Context(), meta, query)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondOK(ctx, "Jobs retrieved successfully", resp)
}
