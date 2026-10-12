package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"gorm.io/gorm"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
	"code-base-golang/internal/payload"
	"code-base-golang/internal/pkg/hasher"
	"code-base-golang/internal/pkg/jwt"
	"code-base-golang/internal/templates"
)

// AdminLogin authenticates staff administrator credentials and issues an admin access token.
func (s *Service) AdminLogin(ctx context.Context, req dtos.AdminLoginRequest, meta *dtos.AdminActionMeta) (*dtos.AdminLoginResponse, error) {
	admin, err := s.repo.FindAdminByUsername(ctx, req.Username)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrInvalidCredentials
		}
		return nil, s.wrapError(ctx, err)
	}

	if admin.Status != constants.UserStatusActive {
		return nil, constants.ErrUserInactive
	}

	if !hasher.VerifyPassword(admin.PasswordHash, req.Password) {
		return nil, constants.ErrInvalidCredentials
	}

	now := time.Now().UTC()
	if err := s.repo.UpdateAdminLastLogin(ctx, admin.ID, now); err != nil {
		return nil, s.wrapError(ctx, err)
	}

	tokenStr, err := jwt.GenerateAdminToken(s.cfg, admin.ID, admin.Username)
	if err != nil {
		return nil, s.wrapError(ctx, err)
	}

	// Record audit log asynchronously via broker
	var ipPtr, uaPtr *string
	if meta != nil {
		if meta.IPAddress != "" {
			ip := meta.IPAddress
			ipPtr = &ip
		}
		if meta.UserAgent != "" {
			ua := meta.UserAgent
			uaPtr = &ua
		}
	}
	entityIDStr := admin.ID.String()
	s.emitAdminAuditLog(ctx, payload.AdminAuditPayload{
		AdminID:   &admin.ID,
		Action:    constants.AuditActionAdminLogin,
		Entity:    constants.AuditEntityAdmin,
		EntityID:  &entityIDStr,
		IPAddress: ipPtr,
		UserAgent: uaPtr,
	})

	roleName := ""
	var perms []string
	if admin.Role != nil {
		roleName = admin.Role.Name
		perms = admin.Role.PermissionsList()
	}

	return &dtos.AdminLoginResponse{
		Token:     tokenStr,
		ExpiresAt: now.Add(s.cfg.JWTAccessExpiration),
		Admin: dtos.AdminUserDTO{
			ID:          admin.ID,
			Username:    admin.Username,
			Email:       admin.Email,
			FullName:    admin.FullName,
			RoleID:      admin.RoleID,
			RoleName:    roleName,
			Permissions: perms,
		},
	}, nil
}

// AdminForgotPassword initiates staff administrator password recovery by generating a one-time
// reset token and dispatching an email. Always succeeds silently (returns nil) to prevent email enumeration.
func (s *Service) AdminForgotPassword(ctx context.Context, req dtos.AdminForgotPasswordRequest) error {
	admin, err := s.repo.FindAdminByEmail(ctx, req.Email)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return s.wrapError(ctx, err)
	}
	if admin == nil || admin.Status != constants.UserStatusActive {
		return nil
	}

	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return s.wrapError(ctx, err)
	}
	rawToken := hex.EncodeToString(randomBytes)
	tokenHash := hasher.HashToken(rawToken)

	if err := s.repo.RevokeAllAdminAuthTokens(ctx, admin.ID, constants.AuthTokenTypeReset); err != nil {
		return s.wrapError(ctx, err)
	}

	token := &models.AdminAuthToken{
		AdminID:   admin.ID,
		Type:      constants.AuthTokenTypeReset,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(15 * time.Minute),
	}
	if err := s.repo.CreateAdminAuthToken(ctx, token); err != nil {
		return s.wrapError(ctx, err)
	}

	if s.mailer != nil && admin.Email != "" {
		htmlBody, textBody, err := templates.RenderAdminResetPasswordEmail(admin.FullName, rawToken)
		if err != nil {
			return s.wrapError(ctx, err)
		}

		msg := dtos.EmailMessage{
			To:       []string{admin.Email},
			Subject:  "Reset Your Admin Password - Youten AI",
			TextBody: textBody,
			HTMLBody: htmlBody,
		}
		if err := s.mailer.Send(ctx, msg); err != nil {
			return s.wrapError(ctx, err)
		}
	}

	return nil
}

// AdminResetPassword confirms an administrator password recovery request using a valid, unexpired token.
func (s *Service) AdminResetPassword(ctx context.Context, req dtos.AdminResetPasswordRequest, meta *dtos.AdminActionMeta) error {
	tokenHash := hasher.HashToken(req.Token)

	authToken, err := s.repo.FindAdminAuthTokenByHashAndType(ctx, tokenHash, constants.AuthTokenTypeReset)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return constants.ErrInvalidToken.WithMessage("invalid or expired reset token")
		}
		return s.wrapError(ctx, err)
	}
	if authToken == nil {
		return constants.ErrInvalidToken.WithMessage("invalid or expired reset token")
	}

	admin, err := s.repo.FindAdminByID(ctx, authToken.AdminID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return constants.ErrNotFound.WithMessage("admin not found")
		}
		return s.wrapError(ctx, err)
	}
	if admin == nil || admin.Status != constants.UserStatusActive {
		return constants.ErrUserInactive
	}

	passwordHash, err := hasher.HashPassword(req.NewPassword)
	if err != nil {
		return s.wrapError(ctx, err)
	}

	if err := s.repo.UpdateAdminPassword(ctx, admin.ID, passwordHash); err != nil {
		return s.wrapError(ctx, err)
	}

	if err := s.repo.RevokeAdminAuthToken(ctx, authToken.ID); err != nil {
		return s.wrapError(ctx, err)
	}

	var ipPtr, uaPtr *string
	if meta != nil {
		if meta.IPAddress != "" {
			ip := meta.IPAddress
			ipPtr = &ip
		}
		if meta.UserAgent != "" {
			ua := meta.UserAgent
			uaPtr = &ua
		}
	}
	entityIDStr := admin.ID.String()
	s.emitAdminAuditLog(ctx, payload.AdminAuditPayload{
		AdminID:   &admin.ID,
		Action:    constants.AuditActionAdminPasswordReset,
		Entity:    constants.AuditEntityAdmin,
		EntityID:  &entityIDStr,
		IPAddress: ipPtr,
		UserAgent: uaPtr,
	})

	return nil
}
