package controllers

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/validations"
)

// DispatchMeetingBot handles initiating a voice bot to join a meeting or voice channel.
func (c *Controllers) DispatchMeetingBot(ctx *gin.Context) {
	var req dtos.DispatchBotRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid request payload"))
		return
	}

	if err := validations.ValidateDispatchBotRequest(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	resp, err := c.svc.DispatchMeetingBot(ctx.Request.Context(), req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondOK(ctx, "Meeting voice bot successfully dispatched", resp)
}

// GetBotSessionStatus queries the live or past status of a bot session.
func (c *Controllers) GetBotSessionStatus(ctx *gin.Context) {
	idStr := ctx.Param("id")
	sessionID, err := uuid.Parse(idStr)
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid session ID format"))
		return
	}

	resp, err := c.svc.GetBotSessionStatus(ctx.Request.Context(), sessionID)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondOK(ctx, "Bot session status retrieved successfully", resp)
}

// StopBotSession commands an active voice bot to leave the meeting.
func (c *Controllers) StopBotSession(ctx *gin.Context) {
	idStr := ctx.Param("id")
	sessionID, err := uuid.Parse(idStr)
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid session ID format"))
		return
	}

	resp, err := c.svc.StopBotSession(ctx.Request.Context(), sessionID)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondOK(ctx, "Bot session stopped successfully", resp)
}

// GetCapabilities reports runtime availability for media ingestion and bot platforms.
func (c *Controllers) GetCapabilities(ctx *gin.Context) {
	resp := c.svc.GetCapabilities(ctx.Request.Context())
	c.respondOK(ctx, "Platform capabilities retrieved successfully", resp)
}

// HandleGoogleMeetWebhook handles incoming status and completion webhook events from Google Meet bot workers.
func (c *Controllers) HandleGoogleMeetWebhook(ctx *gin.Context) {
	var req dtos.GoogleMeetWebhookRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid request payload"))
		return
	}

	if err := validations.ValidateGoogleMeetWebhookRequest(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	secretHeader := ctx.GetHeader("X-Bot-Webhook-Secret")
	if err := c.svc.HandleGoogleMeetWebhook(ctx.Request.Context(), req, secretHeader); err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondOK(ctx, "Google Meet bot webhook processed successfully", nil)
}

// HandleMSTeamsWebhook handles incoming status and completion webhook events from Microsoft Teams / Azure Calling bot workers.
func (c *Controllers) HandleMSTeamsWebhook(ctx *gin.Context) {
	var req dtos.MSTeamsWebhookRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid request payload"))
		return
	}

	if err := validations.ValidateMSTeamsWebhookRequest(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	secretHeader := ctx.GetHeader("X-Bot-Webhook-Secret")
	if err := c.svc.HandleMSTeamsWebhook(ctx.Request.Context(), req, secretHeader); err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondOK(ctx, "Microsoft Teams bot webhook processed successfully", nil)
}

