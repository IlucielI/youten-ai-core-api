package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
	"code-base-golang/internal/pkg/ctxmeta"
	"code-base-golang/internal/pkg/hasher"
	"code-base-golang/internal/pkg/jwt"
)

// resolveDailyQuota returns the effective daily quota for a user, honoring a per-user override when set.
func resolveDailyQuota(user *models.User) int {
	if user.DailyQuotaOverride != nil {
		return *user.DailyQuotaOverride
	}
	if user.Role != nil && user.Role.DailyQuota > 0 {
		return user.Role.DailyQuota
	}
	return constants.DefaultUserDailyQuota
}

// newUserResponse maps a user model into its public response representation.
func newUserResponse(user *models.User) dtos.UserResponse {
	var roleCode string
	var roleName string
	var permissions []string
	if user.Role != nil {
		roleCode = user.Role.Code
		roleName = user.Role.Name
		permissions = user.Role.PermissionsList()
	}
	return dtos.UserResponse{
		ID:                 user.ID,
		Email:              user.Email,
		FullName:           user.FullName,
		Status:             string(user.Status),
		RoleID:             user.RoleID,
		RoleCode:           roleCode,
		RoleName:           roleName,
		Permissions:        permissions,
		DailyQuota:         resolveDailyQuota(user),
		DailyQuotaOverride: user.DailyQuotaOverride,
		EmailVerified:      user.EmailVerified,
		CreatedAt:          user.CreatedAt,
	}
}

// Register creates a new user account with a hashed password and default daily quota.
func (s *Service) Register(ctx context.Context, req *dtos.RegisterRequest) (*dtos.UserResponse, error) {

	email := strings.TrimSpace(strings.ToLower(req.Email))

	// Check if user with this email already exists
	existingUser, err := s.repo.FindUserByEmail(ctx, email)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, s.wrapError(ctx, err)
	}
	if existingUser != nil {
		return nil, constants.ErrEmailAlreadyExists
	}

	// Hash password with bcrypt cost 12
	passwordHash, err := hasher.HashPassword(req.Password)
	if err != nil {
		return nil, s.wrapError(ctx, err)
	}

	// Find default user role
	defaultRole, err := s.repo.FindDefaultUserRole(ctx)
	var defaultRoleID *uuid.UUID
	if err == nil && defaultRole != nil {
		defaultRoleID = &defaultRole.ID
	} else {
		fallbackID := uuid.MustParse("00000000-0000-0000-0000-000000000020")
		defaultRoleID = &fallbackID
	}

	user := &models.User{
		Email:         email,
		PasswordHash:  passwordHash,
		FullName:      strings.TrimSpace(req.FullName),
		Status:        constants.UserStatusActive,
		RoleID:        defaultRoleID,
		Role:          defaultRole,
		EmailVerified: false,
	}

	if err := s.repo.CreateUser(ctx, user); err != nil {
		return nil, s.wrapError(ctx, err)
	}

	resp := newUserResponse(user)
	return &resp, nil
}

// Login authenticates a user by email and password, returning an access and refresh token pair.
func (s *Service) Login(ctx context.Context, req *dtos.LoginRequest) (*dtos.AuthResponse, error) {

	email := strings.TrimSpace(strings.ToLower(req.Email))

	user, err := s.repo.FindUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrInvalidCredentials
		}
		return nil, s.wrapError(ctx, err)
	}

	if user == nil {
		return nil, constants.ErrInvalidCredentials
	}

	if user.Status != constants.UserStatusActive {
		return nil, constants.ErrInvalidCredentials
	}

	if !hasher.VerifyPassword(user.PasswordHash, req.Password) {
		return nil, constants.ErrInvalidCredentials
	}

	// Retrieve user role if available
	var roleCode string
	var perms []string
	if user.RoleID != nil {
		if user.Role != nil {
			roleCode = user.Role.Code
			perms = user.Role.PermissionsList()
		} else {
			if r, rErr := s.repo.FindUserRoleByID(ctx, *user.RoleID); rErr == nil && r != nil {
				user.Role = r
				roleCode = r.Code
				perms = r.PermissionsList()
			}
		}
	}
	dailyQuota := resolveDailyQuota(user)

	sessionID := uuid.New().String()
	tokenPair, err := jwt.GenerateTokenPair(s.cfg, user.ID, user.Email, sessionID, jwt.UserRoleClaims{
		RoleCode:    roleCode,
		Permissions: perms,
		DailyQuota:  dailyQuota,
	})
	if err != nil {
		return nil, s.wrapError(ctx, err)
	}

	// Persist refresh token hash in auth_tokens table
	refreshTokenHash := hasher.HashToken(tokenPair.RefreshToken)
	authToken := &models.AuthToken{
		UserID:    user.ID,
		Type:      constants.AuthTokenTypeRefresh,
		TokenHash: refreshTokenHash,
		ExpiresAt: time.Now().Add(s.cfg.JWTRefreshExpiration),
	}

	if err := s.repo.CreateAuthToken(ctx, authToken); err != nil {
		return nil, s.wrapError(ctx, err)
	}

	return &dtos.AuthResponse{
		AccessToken:      tokenPair.AccessToken,
		RefreshToken:     tokenPair.RefreshToken,
		TokenType:        tokenPair.TokenType,
		ExpiresIn:        tokenPair.ExpiresIn,
		RefreshExpiresIn: tokenPair.RefreshExpiresIn,
		User:             newUserResponse(user),
	}, nil
}

// RefreshToken rotates session tokens by validating the active refresh token, revoking it, and issuing a new token pair.
func (s *Service) RefreshToken(ctx context.Context, req *dtos.RefreshTokenRequest) (*dtos.AuthResponse, error) {
	rawToken := strings.TrimSpace(req.RefreshToken)

	// 1. Validate JWT structure and signature
	claims, err := jwt.ValidateToken(s.cfg, rawToken)
	if err != nil || claims == nil || claims.TokenType != constants.JWTTokenTypeRefresh {
		return nil, constants.ErrInvalidToken
	}

	// 2. Lookup unrevoked, unexpired token in database by SHA-256 hash
	tokenHash := hasher.HashToken(rawToken)
	authToken, err := s.repo.FindAuthTokenByHashAndType(ctx, tokenHash, constants.AuthTokenTypeRefresh)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrInvalidToken
		}
		return nil, s.wrapError(ctx, err)
	}
	if authToken == nil {
		return nil, constants.ErrInvalidToken
	}

	// 3. Retrieve associated user and verify active status
	user, err := s.repo.FindUserByID(ctx, authToken.UserID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrInvalidToken
		}
		return nil, s.wrapError(ctx, err)
	}
	if user == nil || user.Status != constants.UserStatusActive {
		// Invalidate token for inactive user
		if revokeErr := s.repo.RevokeAuthToken(ctx, authToken.ID); revokeErr != nil {
			return nil, s.wrapError(ctx, revokeErr)
		}
		return nil, constants.ErrInvalidToken
	}

	// 4. Generate fresh token pair with a new session ID
	sessionID := uuid.New().String()
	tokenPair, err := jwt.GenerateTokenPair(s.cfg, user.ID, user.Email, sessionID)
	if err != nil {
		return nil, s.wrapError(ctx, err)
	}

	// 5. Atomically rotate tokens: revoke old refresh token and insert new refresh token in transaction
	newTokenHash := hasher.HashToken(tokenPair.RefreshToken)
	newAuthToken := &models.AuthToken{
		UserID:    user.ID,
		Type:      constants.AuthTokenTypeRefresh,
		TokenHash: newTokenHash,
		ExpiresAt: time.Now().Add(s.cfg.JWTRefreshExpiration),
	}

	if err := s.repo.RotateAuthToken(ctx, authToken.ID, newAuthToken); err != nil {
		return nil, s.wrapError(ctx, err)
	}

	return &dtos.AuthResponse{
		AccessToken:      tokenPair.AccessToken,
		RefreshToken:     tokenPair.RefreshToken,
		TokenType:        tokenPair.TokenType,
		ExpiresIn:        tokenPair.ExpiresIn,
		RefreshExpiresIn: tokenPair.RefreshExpiresIn,
		User:             newUserResponse(user),
	}, nil
}

// Logout terminates a user session by revoking the provided refresh token.
// Revocation is implemented idempotently: if the token is non-existent, expired, or already revoked,
// the method returns nil so that callers can safely terminate sessions without error.
func (s *Service) Logout(ctx context.Context, req *dtos.LogoutRequest) error {
	refreshToken := strings.TrimSpace(req.RefreshToken)

	tokenHash := hasher.HashToken(refreshToken)
	authToken, err := s.repo.FindAuthTokenByHashAndType(ctx, tokenHash, constants.AuthTokenTypeRefresh)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return s.wrapError(ctx, err)
	}
	if authToken == nil {
		return nil
	}

	if err := s.repo.RevokeAuthToken(ctx, authToken.ID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return s.wrapError(ctx, err)
	}

	return nil
}

// ForgotPassword initiates the password recovery flow by generating a one-time reset token
// and dispatching a transactional reset email. To prevent email enumeration attacks, this method
// always succeeds silently (returns nil) even if the email does not exist in the system.
func (s *Service) ForgotPassword(ctx context.Context, req *dtos.ForgotPasswordRequest) error {
	email := strings.TrimSpace(strings.ToLower(req.Email))

	// Look up user by email
	user, err := s.repo.FindUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Non-existent email: return nil to prevent account enumeration
			return nil
		}
		return s.wrapError(ctx, err)
	}
	if user == nil || user.Status != constants.UserStatusActive {
		// Inactive or missing user: safe no-op response
		return nil
	}

	// Generate secure 32-byte cryptographically random token
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return s.wrapError(ctx, err)
	}
	rawToken := hex.EncodeToString(randomBytes)
	tokenHash := hasher.HashToken(rawToken)

	// Revoke any previous unconsumed reset tokens for this user
	if err := s.repo.RevokeAllAuthTokensByUserID(ctx, user.ID, constants.AuthTokenTypeReset); err != nil {
		return s.wrapError(ctx, err)
	}

	// Store new reset token with 15-minute expiry
	authToken := &models.AuthToken{
		UserID:    user.ID,
		Type:      constants.AuthTokenTypeReset,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(15 * time.Minute),
	}
	if err := s.repo.CreateAuthToken(ctx, authToken); err != nil {
		return s.wrapError(ctx, err)
	}

	// Dispatch transactional email if mailer is configured
	if s.mailer != nil {
		msg := EmailMessage{
			To:      []string{user.Email},
			Subject: "Reset Your Password - Youten AI",
			TextBody: fmt.Sprintf(
				"Hello %s,\n\nYou requested a password reset. Use the token below within 15 minutes to reset your password:\n\n%s\n\nIf you did not request this, please disregard this email.\n",
				user.FullName, rawToken,
			),
			HTMLBody: fmt.Sprintf(
				"<p>Hello <strong>%s</strong>,</p><p>You requested a password reset. Use the token below within 15 minutes to reset your password:</p><p><strong style=\"font-size: 18px;\">%s</strong></p><p>If you did not request this, please disregard this email.</p>",
				html.EscapeString(user.FullName), rawToken,
			),
		}
		if err := s.mailer.Send(ctx, msg); err != nil {
			return s.wrapError(ctx, err)
		}
	}

	return nil
}

// ResetPassword confirms a password reset request using a valid, unexpired reset token.
// Upon successful reset, the user's password is updated with bcrypt hash, the reset token
// is marked as revoked, and all other active user sessions (refresh tokens) are revoked.
func (s *Service) ResetPassword(ctx context.Context, req *dtos.ResetPasswordRequest) error {
	token := strings.TrimSpace(req.Token)

	// Hash raw token with SHA256 to query database
	tokenHash := hasher.HashToken(token)

	// Retrieve valid unrevoked, unexpired reset token
	authToken, err := s.repo.FindAuthTokenByHashAndType(ctx, tokenHash, constants.AuthTokenTypeReset)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return constants.ErrInvalidToken.WithMessage("invalid or expired reset token")
		}
		return s.wrapError(ctx, err)
	}
	if authToken == nil {
		return constants.ErrInvalidToken.WithMessage("invalid or expired reset token")
	}

	// Retrieve user by ID
	user, err := s.repo.FindUserByID(ctx, authToken.UserID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return constants.ErrNotFound.WithMessage("user not found")
		}
		return s.wrapError(ctx, err)
	}
	if user == nil || user.Status != constants.UserStatusActive {
		return constants.ErrUserInactive
	}

	// Hash new password using bcrypt
	passwordHash, err := hasher.HashPassword(req.NewPassword)
	if err != nil {
		return s.wrapError(ctx, err)
	}

	// Update user's password in database
	if err := s.repo.UpdateUserPassword(ctx, user.ID, passwordHash); err != nil {
		return s.wrapError(ctx, err)
	}

	// Mark the reset token as consumed
	if err := s.repo.RevokeAuthToken(ctx, authToken.ID); err != nil {
		return s.wrapError(ctx, err)
	}

	// Invalidate all other active sessions (refresh tokens) for security
	if err := s.repo.RevokeAllAuthTokensByUserID(ctx, user.ID, constants.AuthTokenTypeRefresh); err != nil {
		return s.wrapError(ctx, err)
	}

	return nil
}

// Authenticate validates a JWT access token, checks its validity and claims, and returns the AuthUser context metadata.
func (s *Service) Authenticate(ctx context.Context, tokenStr string) (*ctxmeta.AuthUser, error) {
	claims, err := jwt.ValidateToken(s.cfg, tokenStr)
	if err != nil || claims == nil {
		return nil, constants.ErrInvalidToken
	}
	if claims.TokenType != constants.JWTTokenTypeAccess {
		return nil, constants.ErrInvalidToken
	}
	return &ctxmeta.AuthUser{
		UserID:      claims.UserID,
		Email:       claims.Email,
		SessionID:   claims.SessionID,
		RoleCode:    claims.RoleCode,
		Permissions: claims.Permissions,
		DailyQuota:  claims.DailyQuota,
		IsGuest:     false,
	}, nil
}

// AuthenticateAdmin validates an Admin JWT token, checks its validity and status, and returns the AdminAuthUser context metadata.
func (s *Service) AuthenticateAdmin(ctx context.Context, tokenStr string) (*ctxmeta.AdminAuthUser, error) {
	claims, err := jwt.ValidateAdminToken(s.cfg, tokenStr)
	if err != nil || claims == nil {
		return nil, constants.ErrInvalidToken
	}
	if claims.TokenType != constants.JWTTokenTypeAdminAccess {
		return nil, constants.ErrInvalidToken
	}

	// Verify admin exists and is active in database
	admin, err := s.repo.FindAdminByID(ctx, claims.AdminID)
	if err != nil || admin == nil {
		return nil, constants.ErrUnauthorized
	}
	if admin.Status != constants.UserStatusActive {
		return nil, constants.ErrUserInactive
	}

	var permissions []string
	roleName := ""
	if admin.Role != nil {
		roleName = admin.Role.Name
		permissions = admin.Role.PermissionsList()
	}

	return &ctxmeta.AdminAuthUser{
		AdminID:     admin.ID,
		Username:    admin.Username,
		FullName:    admin.FullName,
		RoleID:      admin.RoleID,
		RoleName:    roleName,
		Permissions: permissions,
	}, nil
}

// GetProfile retrieves the profile and dynamic daily quota calculation for an authenticated user.
func (s *Service) GetProfile(ctx context.Context, userID uuid.UUID) (*dtos.UserProfileResponse, error) {
	user, err := s.repo.FindUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrUserNotFound
		}
		return nil, s.wrapError(ctx, err)
	}

	var roleCode string
	var roleName string
	var permissions []string
	if user.RoleID != nil {
		if user.Role != nil {
			roleCode = user.Role.Code
			roleName = user.Role.Name
			permissions = user.Role.PermissionsList()
		} else {
			if r, rErr := s.repo.FindUserRoleByID(ctx, *user.RoleID); rErr == nil && r != nil {
				user.Role = r
				roleCode = r.Code
				roleName = r.Name
				permissions = r.PermissionsList()
			}
		}
	}

	dailyQuota := resolveDailyQuota(user)

	countToday, err := s.repo.CountUserRecordingsToday(ctx, user.ID)
	if err != nil {
		return nil, s.wrapError(ctx, err)
	}

	quotaUsedToday := int(countToday)
	quotaRemaining := dailyQuota - quotaUsedToday
	if quotaRemaining < 0 {
		quotaRemaining = 0
	}

	return &dtos.UserProfileResponse{
		ID:             user.ID,
		Email:          user.Email,
		FullName:       user.FullName,
		Status:         string(user.Status),
		RoleID:         user.RoleID,
		RoleCode:       roleCode,
		RoleName:       roleName,
		Permissions:    permissions,
		DailyQuota:     dailyQuota,
		QuotaUsedToday: quotaUsedToday,
		QuotaRemaining: quotaRemaining,
		EmailVerified:  user.EmailVerified,
		CreatedAt:      user.CreatedAt,
	}, nil
}

// UpdateProfile updates mutable profile attributes for the authenticated user and returns the refreshed profile.
func (s *Service) UpdateProfile(ctx context.Context, userID uuid.UUID, req *dtos.UpdateProfileRequest) (*dtos.UserProfileResponse, error) {
	fullName := strings.TrimSpace(req.FullName)

	if err := s.repo.UpdateUserFullName(ctx, userID, fullName); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrUserNotFound
		}
		return nil, s.wrapError(ctx, err)
	}

	return s.GetProfile(ctx, userID)
}

// ChangePassword verifies the user's current password and securely updates it with a new hashed password.
// All other active refresh sessions for the user are revoked upon successful password change.
func (s *Service) ChangePassword(ctx context.Context, userID uuid.UUID, req *dtos.ChangePasswordRequest) error {
	user, err := s.repo.FindUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return constants.ErrUserNotFound
		}
		return s.wrapError(ctx, err)
	}
	if user == nil || user.Status != constants.UserStatusActive {
		return constants.ErrUserInactive
	}

	// Verify old password against stored hash
	if !hasher.VerifyPassword(user.PasswordHash, req.OldPassword) {
		return constants.ErrBadRequest.WithMessage("current password is incorrect")
	}

	// Hash new password using bcrypt
	newPasswordHash, err := hasher.HashPassword(req.NewPassword)
	if err != nil {
		return s.wrapError(ctx, err)
	}

	// Update user password in database
	if err := s.repo.UpdateUserPassword(ctx, user.ID, newPasswordHash); err != nil {
		return s.wrapError(ctx, err)
	}

	// Revoke all other active sessions (refresh tokens) for security
	if err := s.repo.RevokeAllAuthTokensByUserID(ctx, user.ID, constants.AuthTokenTypeRefresh); err != nil {
		return s.wrapError(ctx, err)
	}

	return nil
}

// GenerateAnonToken authenticates a client application via its client_id and secret,
// and issues a scoped 7-day anonymous session token.
func (s *Service) GenerateAnonToken(ctx context.Context, clientID, clientSecret string) (*dtos.AnonTokenResponse, error) {
	cleanClientID := strings.TrimSpace(clientID)
	cleanSecret := strings.TrimSpace(clientSecret)

	if cleanClientID == "" || cleanSecret == "" {
		return nil, constants.ErrUnauthorized.WithMessage("missing basic auth client credentials")
	}

	clientApp, err := s.repo.FindClientAppByClientID(ctx, cleanClientID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrUnauthorized.WithMessage("invalid basic auth client credentials")
		}
		return nil, s.wrapError(ctx, err)
	}

	if !clientApp.IsActive {
		return nil, constants.ErrUnauthorized.WithMessage("client application is inactive or revoked")
	}

	if !hasher.VerifyPassword(clientApp.ClientSecretHash, cleanSecret) {
		return nil, constants.ErrUnauthorized.WithMessage("invalid basic auth client credentials")
	}

	scopes := clientApp.ScopesList()
	if len(scopes) == 0 {
		scopes = constants.DefaultClientAppScopes()
	}

	sessionID := uuid.New()
	tokenStr, expiresIn, err := jwt.GenerateAnonToken(s.cfg, sessionID, clientApp.ClientID, scopes)
	if err != nil {
		return nil, s.wrapError(ctx, err)
	}

	return &dtos.AnonTokenResponse{
		AnonToken: tokenStr,
		SessionID: sessionID,
		ClientID:  clientApp.ClientID,
		TokenType: "Bearer",
		ExpiresIn: expiresIn,
		Scopes:    scopes,
	}, nil
}

