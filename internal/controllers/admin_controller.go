package controllers

import (
	"net/http"
	"strconv"
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

// AdminCreateRole handles POST /v1/admin/roles.
func (c *Controllers) AdminCreateRole(ctx *gin.Context) {
	var req dtos.AdminCreateRoleRequest
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

	resp, err := c.svc.AdminCreateRole(ctx.Request.Context(), meta, req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	ctx.JSON(http.StatusCreated, dtos.APIResponse[*dtos.AdminRoleItem]{
		Status:    constants.ResponseStatusSuccess,
		Code:      constants.ResponseCodeSuccess,
		Message:   constants.ResponseMessageSuccess,
		Data:      resp,
		Timestamp: time.Now(),
	})
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

	resp, err := c.svc.AdminUpdateRole(ctx.Request.Context(), meta, roleID, req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, dtos.APIResponse[*dtos.AdminRoleItem]{
		Status:    constants.ResponseStatusSuccess,
		Code:      constants.ResponseCodeSuccess,
		Message:   constants.ResponseMessageSuccess,
		Data:      resp,
		Timestamp: time.Now(),
	})
}

// AdminListTemplates handles GET /v1/admin/templates.
func (c *Controllers) AdminListTemplates(ctx *gin.Context) {
	resp, err := c.svc.AdminListTemplates(ctx.Request.Context())
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, dtos.APIResponse[*dtos.AdminTemplateListResponse]{
		Status:    constants.ResponseStatusSuccess,
		Code:      constants.ResponseCodeSuccess,
		Message:   constants.ResponseMessageSuccess,
		Data:      resp,
		Timestamp: time.Now(),
	})
}

// AdminCreateTemplate handles POST /v1/admin/templates.
func (c *Controllers) AdminCreateTemplate(ctx *gin.Context) {
	var req dtos.AdminCreateTemplateRequest
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

	resp, err := c.svc.AdminCreateTemplate(ctx.Request.Context(), meta, req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	ctx.JSON(http.StatusCreated, dtos.APIResponse[*dtos.AdminTemplateItem]{
		Status:    constants.ResponseStatusSuccess,
		Code:      constants.ResponseCodeSuccess,
		Message:   constants.ResponseMessageSuccess,
		Data:      resp,
		Timestamp: time.Now(),
	})
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

	resp, err := c.svc.AdminUpdateTemplate(ctx.Request.Context(), meta, templateID, req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, dtos.APIResponse[*dtos.AdminTemplateItem]{
		Status:    constants.ResponseStatusSuccess,
		Code:      constants.ResponseCodeSuccess,
		Message:   constants.ResponseMessageSuccess,
		Data:      resp,
		Timestamp: time.Now(),
	})
}

// AdminTestTemplate handles POST /v1/admin/templates/test.
func (c *Controllers) AdminTestTemplate(ctx *gin.Context) {
	var req dtos.AdminTestTemplateRequest
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

	resp, err := c.svc.AdminTestTemplate(ctx.Request.Context(), meta, req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, dtos.APIResponse[*dtos.AdminTestTemplateResponse]{
		Status:    constants.ResponseStatusSuccess,
		Code:      constants.ResponseCodeSuccess,
		Message:   constants.ResponseMessageSuccess,
		Data:      resp,
		Timestamp: time.Now(),
	})
}

// AdminGetDLQPipeline retrieves dead-lettered and stuck pipeline jobs.
func (c *Controllers) AdminGetDLQPipeline(ctx *gin.Context) {
	adminUser, ok := ctxmeta.GetAdminAuthUser(ctx.Request.Context())
	if !ok || adminUser.AdminID == uuid.Nil {
		c.wrapError(ctx, constants.ErrUnauthorized.WithMessage("admin authentication required"))
		return
	}

	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(ctx.DefaultQuery("limit", "20"))

	meta := services.AdminActionMeta{
		AdminID:   adminUser.AdminID,
		IPAddress: ctxmeta.GetClientIP(ctx.Request.Context()),
		UserAgent: ctxmeta.GetUserAgent(ctx.Request.Context()),
	}

	resp, err := c.svc.AdminGetDLQMessages(ctx.Request.Context(), meta, page, limit)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, dtos.APIResponse[*dtos.DLQMessagesResponse]{
		Status:    constants.ResponseStatusSuccess,
		Code:      constants.ResponseCodeSuccess,
		Message:   constants.ResponseMessageSuccess,
		Data:      resp,
		Timestamp: time.Now(),
	})
}

