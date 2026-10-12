package controllers

import (
	"github.com/gin-gonic/gin"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/validations"
)

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

	meta := c.getClientActionMeta(ctx)

	resp, err := c.svc.AdminLogin(ctx.Request.Context(), req, &meta)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondOK(ctx, "Admin logged in successfully", resp)
}
