package controllers

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/validations"
)

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
