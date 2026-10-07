package controllers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
)

// AdminListUsers handles GET /v1/admin/users.
func (c *Controllers) AdminListUsers(ctx *gin.Context) {
	var query dtos.AdminUserListQuery
	if err := ctx.ShouldBindQuery(&query); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid query parameters"))
		return
	}

	query.SetDefaults()

	resp, err := c.svc.AdminListUsers(ctx.Request.Context(), query)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, dtos.APIResponse[*dtos.AdminUserListResponse]{
		Status:    constants.ResponseStatusSuccess,
		Code:      constants.ResponseCodeSuccess,
		Message:   constants.ResponseMessageSuccess,
		Data:      resp,
		Timestamp: time.Now(),
	})
}
