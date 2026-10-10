package controllers

import (
	"github.com/gin-gonic/gin"
)

// ListTemplates handles GET /v1/templates (retrieving active prompt templates for users and client applications).
func (c *Controllers) ListTemplates(ctx *gin.Context) {
	resp, err := c.svc.ListActiveTemplates(ctx.Request.Context())
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondOK(ctx, "templates retrieved successfully", resp)
}
