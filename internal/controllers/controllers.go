package controllers

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"code-base-golang/internal/config"
	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/services"
)

// Controllers is the central container for all handler methods.
type Controllers struct {
	cfg       config.Config
	svc       *services.Service
	startedAt time.Time
}

// New initializes the central controllers container with application dependencies.
func New(cfg config.Config, svc *services.Service) *Controllers {
	return &Controllers{
		cfg:       cfg,
		svc:       svc,
		startedAt: time.Now(),
	}
}

// SetService allows overriding or injecting custom / mock Service for tests.
func (c *Controllers) SetService(s *services.Service) {
	c.svc = s
}

// Service returns the underlying service container.
func (c *Controllers) Service() *services.Service {
	return c.svc
}

// wrapError translates any error into a standard JSON response using AppError metadata.
func (c *Controllers) wrapError(ctx *gin.Context, err error) {
	if err == nil {
		return
	}

	appErr := constants.ErrInternalServerError.Wrap(err)

	status := constants.ResponseStatusError
	if appErr.HTTPStatus >= 400 && appErr.HTTPStatus < 500 {
		status = constants.ResponseStatusFail
	}

	ctx.JSON(appErr.HTTPStatus, dtos.BaseResponse{
		Status:    status,
		Code:      appErr.Code,
		Message:   appErr.Message,
		Timestamp: time.Now(),
	})
}

// extractOwnershipToken extracts an ownership/share token with cascading fallback:
// optional body token -> X-Ownership-Token header -> "token" query param -> "ownership_token" query param.
func (c *Controllers) extractOwnershipToken(ctx *gin.Context, bodyTokens ...string) string {
	for _, bt := range bodyTokens {
		if t := strings.TrimSpace(bt); t != "" {
			return t
		}
	}
	if t := strings.TrimSpace(ctx.GetHeader("X-Ownership-Token")); t != "" {
		return t
	}
	if t := strings.TrimSpace(ctx.Query("token")); t != "" {
		return t
	}
	return strings.TrimSpace(ctx.Query("ownership_token"))
}

