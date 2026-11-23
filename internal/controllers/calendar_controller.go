package controllers

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/pkg/ctxmeta"
	"code-base-golang/internal/validations"
)

// GetGoogleCalendarAuthURL generates the OAuth authorization URL for the caller.
func (c *Controllers) GetGoogleCalendarAuthURL(ctx *gin.Context) {
	userID, ok := ctxmeta.GetAuthUserID(ctx.Request.Context())
	if !ok {
		c.wrapError(ctx, constants.ErrUnauthorized)
		return
	}

	resp, err := c.svc.GetGoogleCalendarAuthURL(ctx.Request.Context(), userID)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondOK(ctx, "Google calendar authorization URL generated", resp)
}

// HandleGoogleCalendarCallback exchanges the authorization code from OAuth callback.
func (c *Controllers) HandleGoogleCalendarCallback(ctx *gin.Context) {
	userID, ok := ctxmeta.GetAuthUserID(ctx.Request.Context())
	if !ok {
		c.wrapError(ctx, constants.ErrUnauthorized)
		return
	}

	var req dtos.GoogleCalendarCallbackRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid callback payload"))
		return
	}

	if err := validations.ValidateGoogleCalendarCallbackRequest(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	resp, err := c.svc.HandleGoogleCalendarCallback(ctx.Request.Context(), userID, req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondOK(ctx, "Google calendar successfully connected", resp)
}

// GetGoogleCalendarStatus returns integration status for the authenticated user.
func (c *Controllers) GetGoogleCalendarStatus(ctx *gin.Context) {
	userID, ok := ctxmeta.GetAuthUserID(ctx.Request.Context())
	if !ok {
		c.wrapError(ctx, constants.ErrUnauthorized)
		return
	}

	resp, err := c.svc.GetGoogleCalendarStatus(ctx.Request.Context(), userID)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondOK(ctx, "Google calendar status retrieved", resp)
}

// UpdateCalendarSettings updates auto-recording preferences for the user.
func (c *Controllers) UpdateCalendarSettings(ctx *gin.Context) {
	userID, ok := ctxmeta.GetAuthUserID(ctx.Request.Context())
	if !ok {
		c.wrapError(ctx, constants.ErrUnauthorized)
		return
	}

	var req dtos.UpdateCalendarSettingsRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid settings payload"))
		return
	}

	if err := validations.ValidateUpdateCalendarSettingsRequest(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	resp, err := c.svc.UpdateCalendarSettings(ctx.Request.Context(), userID, req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondOK(ctx, "Calendar settings updated successfully", resp)
}

// DisconnectGoogleCalendar removes the user's Google Calendar connection.
func (c *Controllers) DisconnectGoogleCalendar(ctx *gin.Context) {
	userID, ok := ctxmeta.GetAuthUserID(ctx.Request.Context())
	if !ok {
		c.wrapError(ctx, constants.ErrUnauthorized)
		return
	}

	if err := c.svc.DisconnectGoogleCalendar(ctx.Request.Context(), userID); err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondEmpty(ctx, "Google calendar disconnected successfully")
}

// SyncGoogleCalendar triggers an on-demand synchronization for the user's calendar.
func (c *Controllers) SyncGoogleCalendar(ctx *gin.Context) {
	userID, ok := ctxmeta.GetAuthUserID(ctx.Request.Context())
	if !ok {
		c.wrapError(ctx, constants.ErrUnauthorized)
		return
	}

	resp, err := c.svc.SyncUserCalendar(ctx.Request.Context(), userID)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondOK(ctx, "Calendar synchronized successfully", resp)
}

// ListCalendarEvents returns paginated scheduled events for the user.
func (c *Controllers) ListCalendarEvents(ctx *gin.Context) {
	userID, ok := ctxmeta.GetAuthUserID(ctx.Request.Context())
	if !ok {
		c.wrapError(ctx, constants.ErrUnauthorized)
		return
	}

	var query dtos.ListCalendarEventsQuery
	if err := ctx.ShouldBindQuery(&query); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid query parameters"))
		return
	}

	if err := validations.ValidateListCalendarEventsQuery(&query); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	events, total, err := c.svc.ListCalendarEvents(ctx.Request.Context(), userID, query)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	totalPages := 0
	if query.Limit > 0 {
		totalPages = int((total + int64(query.Limit) - 1) / int64(query.Limit))
	}

	meta := gin.H{
		"page":        query.Page,
		"limit":       query.Limit,
		"total_items": total,
		"total_pages": totalPages,
	}

	c.respondOK(ctx, "Calendar events retrieved successfully", gin.H{
		"items":      events,
		"pagination": meta,
	})
}

// ToggleEventAutoRecord toggles auto-recording for an individual scheduled event.
func (c *Controllers) ToggleEventAutoRecord(ctx *gin.Context) {
	userID, ok := ctxmeta.GetAuthUserID(ctx.Request.Context())
	if !ok {
		c.wrapError(ctx, constants.ErrUnauthorized)
		return
	}

	eventID, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid event id format"))
		return
	}

	var req dtos.ToggleEventAutoRecordRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid request payload"))
		return
	}

	if err := c.svc.ToggleEventAutoRecord(ctx.Request.Context(), eventID, userID, req); err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondEmpty(ctx, "Event auto-record preference updated successfully")
}

