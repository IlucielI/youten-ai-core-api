package controllers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/validations"
)

// JoinBotWaitlist handles public applicant registration for the meeting voice bot beta waitlist.
func (c *Controllers) JoinBotWaitlist(ctx *gin.Context) {
	if c.svc == nil {
		ctx.JSON(http.StatusInternalServerError, dtos.BaseResponse{
			Status:    constants.ResponseStatusError,
			Code:      constants.ResponseCodeInternalError,
			Message:   "controller service uninitialized",
			Timestamp: time.Now(),
		})
		return
	}

	var req dtos.WaitlistRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid request payload"))
		return
	}

	if err := validations.Validate(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	resp, err := c.svc.JoinBotWaitlist(ctx.Request.Context(), req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, dtos.APIResponse[*dtos.WaitlistResponse]{
		Status:    constants.ResponseStatusSuccess,
		Code:      constants.ResponseCodeSuccess,
		Message:   "Successfully joined meeting voice bot beta waitlist",
		Data:      resp,
		Timestamp: time.Now(),
	})
}
