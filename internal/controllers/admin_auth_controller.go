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

// AdminForgotPassword handles POST /v1/admin/forgot-password to request an admin password reset email.
// Always returns 200 OK with a generic message to prevent email enumeration.
func (c *Controllers) AdminForgotPassword(ctx *gin.Context) {
	var req dtos.AdminForgotPasswordRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid request payload"))
		return
	}

	if err := validations.ValidateAdminForgotPasswordRequest(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	if err := c.svc.AdminForgotPassword(ctx.Request.Context(), req); err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondEmpty(ctx, "If your email is registered, you will receive a password reset link shortly")
}

// AdminResetPassword handles POST /v1/admin/reset-password to reset administrator credentials using a token.
func (c *Controllers) AdminResetPassword(ctx *gin.Context) {
	var req dtos.AdminResetPasswordRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid request payload"))
		return
	}

	if err := validations.ValidateAdminResetPasswordRequest(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	meta := c.getClientActionMeta(ctx)

	if err := c.svc.AdminResetPassword(ctx.Request.Context(), req, &meta); err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondEmpty(ctx, "Admin password reset successfully")
}

// AdminRefreshToken handles POST /v1/admin/refresh to rotate administrator session tokens.
func (c *Controllers) AdminRefreshToken(ctx *gin.Context) {
	var req dtos.AdminRefreshTokenRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid request payload"))
		return
	}

	if err := validations.ValidateAdminRefreshTokenRequest(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	meta := c.getClientActionMeta(ctx)

	resp, err := c.svc.AdminRefreshToken(ctx.Request.Context(), req, &meta)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondOK(ctx, "Admin session rotated successfully", resp)
}

