package controllers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/pkg/ctxmeta"
	"code-base-golang/internal/validations"
)

// Register handles user registration request.
func (c *Controllers) Register(ctx *gin.Context) {
	var req dtos.RegisterRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid request payload"))
		return
	}

	if err := validations.ValidateRegisterRequest(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	user, err := c.svc.Register(ctx.Request.Context(), &req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	ctx.JSON(http.StatusCreated, dtos.APIResponse[*dtos.UserResponse]{
		Status:    constants.ResponseStatusSuccess,
		Code:      constants.ResponseCodeSuccess,
		Message:   "User registered successfully",
		Data:      user,
		Timestamp: time.Now(),
	})
}

// Login handles user authentication and JWT session token generation.
func (c *Controllers) Login(ctx *gin.Context) {
	var req dtos.LoginRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid request payload"))
		return
	}

	if err := validations.ValidateLoginRequest(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	authResp, err := c.svc.Login(ctx.Request.Context(), &req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, dtos.APIResponse[*dtos.AuthResponse]{
		Status:    constants.ResponseStatusSuccess,
		Code:      constants.ResponseCodeSuccess,
		Message:   "Login successful",
		Data:      authResp,
		Timestamp: time.Now(),
	})
}

// RefreshToken handles session token rotation request.
func (c *Controllers) RefreshToken(ctx *gin.Context) {
	var req dtos.RefreshTokenRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid request payload"))
		return
	}

	if err := validations.ValidateRefreshTokenRequest(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	authResp, err := c.svc.RefreshToken(ctx.Request.Context(), &req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, dtos.APIResponse[*dtos.AuthResponse]{
		Status:    constants.ResponseStatusSuccess,
		Code:      constants.ResponseCodeSuccess,
		Message:   "Token refreshed successfully",
		Data:      authResp,
		Timestamp: time.Now(),
	})
}

// Logout handles session termination and refresh token revocation.
func (c *Controllers) Logout(ctx *gin.Context) {
	var req dtos.LogoutRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid request payload"))
		return
	}

	if err := validations.ValidateLogoutRequest(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	if err := c.svc.Logout(ctx.Request.Context(), &req); err != nil {
		c.wrapError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, dtos.BaseResponse{
		Status:    constants.ResponseStatusSuccess,
		Code:      constants.ResponseCodeSuccess,
		Message:   "Logout successful",
		Timestamp: time.Now(),
	})
}

// ForgotPassword handles requesting a password reset email.
// Always returns 200 OK with a generic message to prevent email enumeration.
func (c *Controllers) ForgotPassword(ctx *gin.Context) {
	var req dtos.ForgotPasswordRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid request payload"))
		return
	}

	if err := validations.ValidateForgotPasswordRequest(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	if err := c.svc.ForgotPassword(ctx.Request.Context(), &req); err != nil {
		c.wrapError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, dtos.BaseResponse{
		Status:    constants.ResponseStatusSuccess,
		Code:      constants.ResponseCodeSuccess,
		Message:   "If your email is registered, you will receive a password reset link shortly",
		Timestamp: time.Now(),
	})
}

// ResetPassword handles resetting the user password using a valid reset token.
func (c *Controllers) ResetPassword(ctx *gin.Context) {
	var req dtos.ResetPasswordRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid request payload"))
		return
	}

	if err := validations.ValidateResetPasswordRequest(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	if err := c.svc.ResetPassword(ctx.Request.Context(), &req); err != nil {
		c.wrapError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, dtos.BaseResponse{
		Status:    constants.ResponseStatusSuccess,
		Code:      constants.ResponseCodeSuccess,
		Message:   "Password reset successfully",
		Timestamp: time.Now(),
	})
}

// GetMe retrieves the authenticated user's profile and dynamic daily quota.
func (c *Controllers) GetMe(ctx *gin.Context) {
	userID, ok := ctxmeta.GetAuthUserID(ctx.Request.Context())
	if !ok || userID == uuid.Nil {
		c.wrapError(ctx, constants.ErrUnauthorized)
		return
	}

	profile, err := c.svc.GetProfile(ctx.Request.Context(), userID)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, dtos.APIResponse[*dtos.UserProfileResponse]{
		Status:    constants.ResponseStatusSuccess,
		Code:      constants.ResponseCodeSuccess,
		Message:   "User profile retrieved successfully",
		Data:      profile,
		Timestamp: time.Now(),
	})
}

// UpdateProfile handles updating the authenticated user's profile information.
func (c *Controllers) UpdateProfile(ctx *gin.Context) {
	userID, ok := ctxmeta.GetAuthUserID(ctx.Request.Context())
	if !ok || userID == uuid.Nil {
		c.wrapError(ctx, constants.ErrUnauthorized)
		return
	}

	var req dtos.UpdateProfileRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid request payload"))
		return
	}

	if err := validations.ValidateUpdateProfileRequest(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	profile, err := c.svc.UpdateProfile(ctx.Request.Context(), userID, &req)
	if err != nil {
		c.wrapError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, dtos.APIResponse[*dtos.UserProfileResponse]{
		Status:    constants.ResponseStatusSuccess,
		Code:      constants.ResponseCodeSuccess,
		Message:   "User profile updated successfully",
		Data:      profile,
		Timestamp: time.Now(),
	})
}

// ChangePassword handles password update for the authenticated user.
func (c *Controllers) ChangePassword(ctx *gin.Context) {
	userID, ok := ctxmeta.GetAuthUserID(ctx.Request.Context())
	if !ok || userID == uuid.Nil {
		c.wrapError(ctx, constants.ErrUnauthorized)
		return
	}

	var req dtos.ChangePasswordRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage("invalid request payload"))
		return
	}

	if err := validations.ValidateChangePasswordRequest(&req); err != nil {
		c.wrapError(ctx, constants.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	if err := c.svc.ChangePassword(ctx.Request.Context(), userID, &req); err != nil {
		c.wrapError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, dtos.BaseResponse{
		Status:    constants.ResponseStatusSuccess,
		Code:      constants.ResponseCodeSuccess,
		Message:   "Password changed successfully",
		Timestamp: time.Now(),
	})
}
