package controllers

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/validations"
)

// AdminListRoles handles GET /v1/admin/roles.
func (c *Controllers) AdminListRoles(ctx *gin.Context) {
	var query dtos.AdminRoleListQuery
	if err := ctx.ShouldBindQuery(&query); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid query parameters"))
		return
	}

	query.SetDefaults()

	if err := validations.ValidateAdminRoleListQuery(&query); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	resp, err := c.svc.AdminListRoles(ctx.Request.Context(), query)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondOK(ctx, "Roles retrieved successfully", resp)
}

// AdminCreateRole handles POST /v1/admin/roles.
func (c *Controllers) AdminCreateRole(ctx *gin.Context) {
	var req dtos.AdminCreateRoleRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid request payload"))
		return
	}

	if err := validations.ValidateAdminCreateRoleRequest(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	meta, ok := c.getAdminActionMeta(ctx)
	if !ok {
		return
	}

	resp, err := c.svc.AdminCreateRole(ctx.Request.Context(), meta, req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondCreated(ctx, "Role created successfully", resp)
}

// AdminUpdateRole handles PUT /v1/admin/roles/:id.
func (c *Controllers) AdminUpdateRole(ctx *gin.Context) {
	idStr := ctx.Param("id")
	roleID, err := uuid.Parse(idStr)
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid role ID"))
		return
	}

	var req dtos.AdminUpdateRoleRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid request payload"))
		return
	}

	if err := validations.ValidateAdminUpdateRoleRequest(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	meta, ok := c.getAdminActionMeta(ctx)
	if !ok {
		return
	}

	resp, err := c.svc.AdminUpdateRole(ctx.Request.Context(), meta, roleID, req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondOK(ctx, "Role updated successfully", resp)
}

// AdminListUserRoles handles GET /v1/admin/user-roles.
func (c *Controllers) AdminListUserRoles(ctx *gin.Context) {
	var query dtos.AdminUserRoleListQuery
	if err := ctx.ShouldBindQuery(&query); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid query parameters"))
		return
	}

	query.SetDefaults()

	if err := validations.ValidateAdminUserRoleListQuery(&query); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	resp, err := c.svc.AdminListUserRoles(ctx.Request.Context(), query)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondOK(ctx, "User roles retrieved successfully", resp)
}

// AdminCreateUserRole handles POST /v1/admin/user-roles.
func (c *Controllers) AdminCreateUserRole(ctx *gin.Context) {
	var req dtos.AdminCreateUserRoleRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid request payload"))
		return
	}

	if err := validations.ValidateAdminCreateUserRoleRequest(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	meta, ok := c.getAdminActionMeta(ctx)
	if !ok {
		return
	}

	resp, err := c.svc.AdminCreateUserRole(ctx.Request.Context(), meta, req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondCreated(ctx, "User role created successfully", resp)
}

// AdminUpdateUserRole handles PUT /v1/admin/user-roles/:id.
func (c *Controllers) AdminUpdateUserRole(ctx *gin.Context) {
	idStr := ctx.Param("id")
	roleID, err := uuid.Parse(idStr)
	if err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid user role ID"))
		return
	}

	var req dtos.AdminUpdateUserRoleRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid request payload"))
		return
	}

	if err := validations.ValidateAdminUpdateUserRoleRequest(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	meta, ok := c.getAdminActionMeta(ctx)
	if !ok {
		return
	}

	resp, err := c.svc.AdminUpdateUserRole(ctx.Request.Context(), meta, roleID, req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	c.respondOK(ctx, "User role updated successfully", resp)
}
