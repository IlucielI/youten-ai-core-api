package controllers

import (
	"github.com/gin-gonic/gin"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/validations"
)

// JoinBotWaitlist handles public applicant registration for the meeting voice bot beta waitlist.
func (c *Controllers) JoinBotWaitlist(ctx *gin.Context) {
	var req dtos.WaitlistRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid request payload"))
		return
	}

	if err := validations.ValidateWaitlistRequest(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	resp, err := c.svc.JoinBotWaitlist(ctx.Request.Context(), req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondOK(ctx, "Successfully joined meeting voice bot beta waitlist", resp)
}
