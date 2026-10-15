package controllers

import (
	"strings"

	"github.com/gin-gonic/gin"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/validations"
)

// SubmitFeedback handles user feedback submission (bug reports, feature requests, praise, etc.)
// with client metadata propagation, idempotency protection, and optional user authentication.
func (c *Controllers) SubmitFeedback(ctx *gin.Context) {
	var req dtos.FeedbackRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid request payload"))
		return
	}

	// Extract idempotency key from header if not provided in body
	if req.IdempotencyKey == nil || strings.TrimSpace(*req.IdempotencyKey) == "" {
		if headerKey := strings.TrimSpace(ctx.GetHeader("Idempotency-Key")); headerKey != "" {
			req.IdempotencyKey = &headerKey
		}
	}

	if err := validations.ValidateFeedbackRequest(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	resp, err := c.svc.SubmitFeedback(ctx.Request.Context(), req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondCreated(ctx, "Feedback submitted successfully", resp)
}
