package controllers

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/pkg/ctxmeta"
	"code-base-golang/internal/services"
	"code-base-golang/internal/validations"
)

// getAdminActionMeta extracts authenticated administrator metadata from the request context.
// If the caller is unauthenticated or the admin ID is nil, it responds with HTTP 401 Unauthorized and returns false.
func (c *Controllers) getAdminActionMeta(ctx *gin.Context) (services.AdminActionMeta, bool) {
	adminUser, ok := ctxmeta.GetAdminAuthUser(ctx.Request.Context())
	if !ok || adminUser.AdminID == uuid.Nil {
		c.wrapError(ctx, constants.ErrUnauthorized.WithMessage("admin authentication required"))
		return services.AdminActionMeta{}, false
	}

	return services.AdminActionMeta{
		AdminID:   adminUser.AdminID,
		IPAddress: ctxmeta.GetClientIP(ctx.Request.Context()),
		UserAgent: ctxmeta.GetUserAgent(ctx.Request.Context()),
	}, true
}

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

// AdminListRoles handles GET /v1/admin/roles.
func (c *Controllers) AdminListRoles(ctx *gin.Context) {
	resp, err := c.svc.AdminListRoles(ctx.Request.Context())
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondOK(ctx, "Roles retrieved successfully", resp)
}

// AdminCreateRole handles POST /v1/admin/roles.
func (c *Controllers) AdminCreateRole(ctx *gin.Context) {
	var req dtos.AdminCreateRoleRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid request payload"))
		return
	}

	if err := validations.ValidateAdminCreateRoleRequest(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	meta, ok := c.getAdminActionMeta(ctx)
	if !ok {
		return
	}

	resp, err := c.svc.AdminCreateRole(ctx.Request.Context(), meta, req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondCreated(ctx, "Role created successfully", resp)
}

// AdminUpdateRole handles PUT /v1/admin/roles/:id.
func (c *Controllers) AdminUpdateRole(ctx *gin.Context) {
	idStr := ctx.Param("id")
	roleID, err := uuid.Parse(idStr)
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid role ID"))
		return
	}

	var req dtos.AdminUpdateRoleRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid request payload"))
		return
	}

	if err := validations.ValidateAdminUpdateRoleRequest(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	meta, ok := c.getAdminActionMeta(ctx)
	if !ok {
		return
	}

	resp, err := c.svc.AdminUpdateRole(ctx.Request.Context(), meta, roleID, req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondOK(ctx, "Role updated successfully", resp)
}

// AdminListUserRoles handles GET /v1/admin/user-roles.
func (c *Controllers) AdminListUserRoles(ctx *gin.Context) {
	resp, err := c.svc.AdminListUserRoles(ctx.Request.Context())
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondOK(ctx, "User roles retrieved successfully", resp)
}

// AdminCreateUserRole handles POST /v1/admin/user-roles.
func (c *Controllers) AdminCreateUserRole(ctx *gin.Context) {
	var req dtos.AdminCreateUserRoleRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid request payload"))
		return
	}

	if err := validations.ValidateAdminCreateUserRoleRequest(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	meta, ok := c.getAdminActionMeta(ctx)
	if !ok {
		return
	}

	resp, err := c.svc.AdminCreateUserRole(ctx.Request.Context(), meta, req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondCreated(ctx, "User role created successfully", resp)
}

// AdminUpdateUserRole handles PUT /v1/admin/user-roles/:id.
func (c *Controllers) AdminUpdateUserRole(ctx *gin.Context) {
	idStr := ctx.Param("id")
	roleID, err := uuid.Parse(idStr)
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid user role ID"))
		return
	}

	var req dtos.AdminUpdateUserRoleRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid request payload"))
		return
	}

	if err := validations.ValidateAdminUpdateUserRoleRequest(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	meta, ok := c.getAdminActionMeta(ctx)
	if !ok {
		return
	}

	resp, err := c.svc.AdminUpdateUserRole(ctx.Request.Context(), meta, roleID, req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondOK(ctx, "User role updated successfully", resp)
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

// AdminListTemplates handles GET /v1/admin/templates.
func (c *Controllers) AdminListTemplates(ctx *gin.Context) {
	resp, err := c.svc.AdminListTemplates(ctx.Request.Context())
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondOK(ctx, "Templates retrieved successfully", resp)
}

// AdminCreateTemplate handles POST /v1/admin/templates.
func (c *Controllers) AdminCreateTemplate(ctx *gin.Context) {
	var req dtos.AdminCreateTemplateRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid request payload"))
		return
	}

	if err := validations.ValidateAdminCreateTemplateRequest(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	meta, ok := c.getAdminActionMeta(ctx)
	if !ok {
		return
	}

	resp, err := c.svc.AdminCreateTemplate(ctx.Request.Context(), meta, req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondCreated(ctx, "Template created successfully", resp)
}

// AdminUpdateTemplate handles PUT /v1/admin/templates/:id.
func (c *Controllers) AdminUpdateTemplate(ctx *gin.Context) {
	idStr := ctx.Param("id")
	templateID, err := uuid.Parse(idStr)
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid template ID"))
		return
	}

	var req dtos.AdminUpdateTemplateRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid request payload"))
		return
	}

	if err := validations.ValidateAdminUpdateTemplateRequest(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	meta, ok := c.getAdminActionMeta(ctx)
	if !ok {
		return
	}

	resp, err := c.svc.AdminUpdateTemplate(ctx.Request.Context(), meta, templateID, req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondOK(ctx, "Template updated successfully", resp)
}

// AdminTestTemplate handles POST /v1/admin/templates/test.
func (c *Controllers) AdminTestTemplate(ctx *gin.Context) {
	var req dtos.AdminTestTemplateRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid request payload"))
		return
	}

	if err := validations.ValidateAdminTestTemplateRequest(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	meta, ok := c.getAdminActionMeta(ctx)
	if !ok {
		return
	}

	resp, err := c.svc.AdminTestTemplate(ctx.Request.Context(), meta, req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondOK(ctx, "Template test executed successfully", resp)
}

// AdminGetDLQPipeline retrieves dead-lettered and stuck pipeline jobs.
func (c *Controllers) AdminGetDLQPipeline(ctx *gin.Context) {
	meta, ok := c.getAdminActionMeta(ctx)
	if !ok {
		return
	}

	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(ctx.DefaultQuery("limit", "20"))

	resp, err := c.svc.AdminGetDLQMessages(ctx.Request.Context(), meta, page, limit)
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

// AdminLogin handles POST /v1/admin/login to authenticate staff administrators.
func (c *Controllers) AdminLogin(ctx *gin.Context) {
	var req dtos.AdminLoginRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid request payload"))
		return
	}

	if err := validations.ValidateAdminLoginRequest(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	meta := &services.AdminActionMeta{
		IPAddress: ctxmeta.GetClientIP(ctx.Request.Context()),
		UserAgent: ctxmeta.GetUserAgent(ctx.Request.Context()),
	}

	resp, err := c.svc.AdminLogin(ctx.Request.Context(), req, meta)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondOK(ctx, "Admin logged in successfully", resp)
}
