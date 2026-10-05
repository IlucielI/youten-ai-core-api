package controllers

import (
	"time"

	"github.com/gin-gonic/gin"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
)

// HealthCheck handles health status inquiries.
func (c *Controllers) HealthCheck(ctx *gin.Context) {
	servicesStatus := c.svc.CheckHealth(ctx.Request.Context())

	c.respondOK(ctx, constants.ResponseMessageSuccess, dtos.HealthData{
		Version:  c.cfg.Version,
		GitHash:  c.cfg.GitHash,
		Uptime:   time.Since(c.startedAt).String(),
		Services: servicesStatus,
	})
}
