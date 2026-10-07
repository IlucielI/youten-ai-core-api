package controllers

import (
	"log"
	"net/http"
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
	} else if appErr.HTTPStatus >= 500 {
		reqMethod := "UNKNOWN"
		reqPath := "UNKNOWN"
		if ctx.Request != nil {
			reqMethod = ctx.Request.Method
			if ctx.Request.URL != nil {
				reqPath = ctx.Request.URL.Path
			}
		}
		log.Printf("[API ERROR %d] %s %s: %v", appErr.HTTPStatus, reqMethod, reqPath, err)
	}

	ctx.JSON(appErr.HTTPStatus, dtos.BaseResponse{
		Status:    status,
		Code:      appErr.Code,
		Message:   appErr.Message,
		Timestamp: time.Now(),
	})
}

// respondSuccess sends a standardized JSON success response envelope.
func (c *Controllers) respondSuccess(ctx *gin.Context, httpStatus int, message string, data any) {
	ctx.JSON(httpStatus, dtos.APIResponse[any]{
		Status:    constants.ResponseStatusSuccess,
		Code:      constants.ResponseCodeSuccess,
		Message:   message,
		Data:      data,
		Timestamp: time.Now(),
	})
}

// respondOK sends a standard HTTP 200 OK JSON success envelope.
func (c *Controllers) respondOK(ctx *gin.Context, message string, data any) {
	c.respondSuccess(ctx, http.StatusOK, message, data)
}

// respondCreated sends a standard HTTP 201 Created JSON success envelope.
func (c *Controllers) respondCreated(ctx *gin.Context, message string, data any) {
	c.respondSuccess(ctx, http.StatusCreated, message, data)
}

// respondEmpty sends a standard HTTP 200 OK JSON response without a data payload.
func (c *Controllers) respondEmpty(ctx *gin.Context, message string) {
	ctx.JSON(http.StatusOK, dtos.BaseResponse{
		Status:    constants.ResponseStatusSuccess,
		Code:      constants.ResponseCodeSuccess,
		Message:   message,
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

