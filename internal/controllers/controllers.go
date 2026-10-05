package controllers

import (
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
	if c == nil {
		return nil
	}
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

