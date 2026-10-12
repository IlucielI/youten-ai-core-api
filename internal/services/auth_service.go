package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
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
	"code-base-golang/internal/templates"
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
		Status:        constants.UserStatusPending,
		RoleID:        defaultRoleID,
		EmailVerified: false,
	}

	if err := s.repo.CreateUser(ctx, user); err != nil {
		return nil, s.wrapError(ctx, err)
	}

	user.Role = defaultRole

	if req.AnonToken != "" {
		s.autoClaimGuestRecordings(ctx, user.ID, req.AnonToken)
	}

	// Generate verification token and 6-digit code
	code, rawToken, codeHash, tokenHash, err := generateVerificationCodeAndToken(email)
	if err == nil {
		expiresAt := time.Now().Add(24 * time.Hour)
		_ = s.repo.CreateAuthToken(ctx, &models.AuthToken{
			UserID:    user.ID,
			Type:      constants.AuthTokenTypeEmailVerification,
			TokenHash: tokenHash,
			ExpiresAt: expiresAt,
		})
		_ = s.repo.CreateAuthToken(ctx, &models.AuthToken{
			UserID:    user.ID,
			Type:      constants.AuthTokenTypeEmailVerificationCode,
			TokenHash: codeHash,
			ExpiresAt: expiresAt,
		})
		if s.mailer != nil {
			go func() {
				bgCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				_ = s.sendVerificationEmail(bgCtx, user.Email, user.FullName, code, rawToken)
			}()
		}
	}

	resp := newUserResponse(user)
	return &resp, nil
}

// autoClaimGuestRecordings claims any pending guest recordings associated with the anon_token or ownership token.
func (s *Service) autoClaimGuestRecordings(ctx context.Context, userID uuid.UUID, rawAnonToken string) int {
	rawAnonToken = strings.TrimSpace(rawAnonToken)
	if rawAnonToken == "" {
		return 0
	}

	tokens := []string{rawAnonToken}
	if anonClaims, err := jwt.ValidateAnonToken(s.cfg, rawAnonToken); err == nil && anonClaims != nil {
		tokens = append(tokens, anonClaims.SessionID.String())
	}

	claimedIDs, err := s.repo.ClaimRecordingsByTokens(ctx, tokens, userID)
	if err != nil {
		return 0
	}
	return len(claimedIDs)
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

	if !hasher.VerifyPassword(user.PasswordHash, req.Password) {
		return nil, constants.ErrInvalidCredentials
	}

	if user.Status != constants.UserStatusActive && user.Status != constants.UserStatusPending {
		return nil, constants.ErrInvalidCredentials
	}

	if !user.EmailVerified || user.Status == constants.UserStatusPending {
		return nil, constants.ErrEmailNotVerified
	}

	// Retrieve user role if available
	var roleCode string
	var perms []string
	if user.RoleID != nil {
		if user.Role != nil {
			roleCode = user.Role.Code
			perms = user.Role.PermissionsList()
		} else {
			r, rErr := s.repo.FindUserRoleByID(ctx, *user.RoleID)
			if rErr != nil {
				return nil, s.wrapError(ctx, rErr)
			}
			if r != nil {
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

	claimedCount := 0
	if req.AnonToken != "" {
		claimedCount = s.autoClaimGuestRecordings(ctx, user.ID, req.AnonToken)
	}

	return &dtos.AuthResponse{
		AccessToken:            tokenPair.AccessToken,
		RefreshToken:           tokenPair.RefreshToken,
		TokenType:              tokenPair.TokenType,
		ExpiresIn:              tokenPair.ExpiresIn,
		RefreshExpiresIn:       tokenPair.RefreshExpiresIn,
		User:                   newUserResponse(user),
		ClaimedRecordingsCount: claimedCount,
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
		htmlBody, textBody, err := templates.RenderResetPasswordEmail(templates.ResetPasswordEmailData{
			FullName: user.FullName,
			Token:    rawToken,
		})
		if err != nil {
			return s.wrapError(ctx, fmt.Errorf("render reset password email template: %w", err))
		}

		msg := dtos.EmailMessage{
			To:       []string{user.Email},
			Subject:  "Reset Your Password - Youten AI",
			TextBody: textBody,
			HTMLBody: htmlBody,
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

// Authenticate validates a JWT token (user access token or scoped anonymous session token)
// and returns the AuthUser context metadata.
func (s *Service) Authenticate(ctx context.Context, tokenStr string) (*ctxmeta.AuthUser, error) {
	// 1. Attempt validation as registered user access token
	claims, err := jwt.ValidateToken(s.cfg, tokenStr)
	if err == nil && claims != nil && claims.TokenType == constants.JWTTokenTypeAccess {
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

	// 2. Attempt validation as scoped anonymous guest session token
	anonClaims, aErr := jwt.ValidateAnonToken(s.cfg, tokenStr)
	if aErr == nil && anonClaims != nil && anonClaims.TokenType == constants.JWTTokenTypeAnonAccess {
		return &ctxmeta.AuthUser{
			UserID:      anonClaims.SessionID,
			Email:       "",
			SessionID:   anonClaims.SessionID.String(),
			RoleCode:    "ANON",
			Permissions: anonClaims.Scopes,
			DailyQuota:  constants.DefaultGuestDailyQuota,
			IsGuest:     true,
		}, nil
	}

	return nil, constants.ErrInvalidToken
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
			r, rErr := s.repo.FindUserRoleByID(ctx, *user.RoleID)
			if rErr != nil {
				return nil, s.wrapError(ctx, rErr)
			}
			if r != nil {
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

// generateVerificationCodeAndToken generates a secure 6-digit OTP code and a 32-byte hex token.
func generateVerificationCodeAndToken(email string) (code string, token string, codeHash string, tokenHash string, err error) {
	nBig, err := rand.Int(rand.Reader, big.NewInt(900000))
	if err != nil {
		return "", "", "", "", err
	}
	code = fmt.Sprintf("%06d", nBig.Int64()+100000)

	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", "", "", "", err
	}
	token = hex.EncodeToString(tokenBytes)

	codeHash = hasher.HashToken(email + ":" + code)
	tokenHash = hasher.HashToken(token)
	return code, token, codeHash, tokenHash, nil
}

// sendVerificationEmail dispatches an email containing the 6-digit OTP code and magic link.
func (s *Service) sendVerificationEmail(ctx context.Context, email, fullName, code, rawToken string) error {
	if s.mailer == nil {
		return nil
	}

	appURL := strings.TrimRight(s.cfg.AppURL, "/")
	if appURL == "" {
		appURL = "http://localhost:3000"
	}
	verifyURL := fmt.Sprintf("%s/verify-email?token=%s", appURL, rawToken)

	htmlBody, textBody, err := templates.RenderVerifyEmail(templates.VerifyEmailData{
		FullName:  fullName,
		Code:      code,
		VerifyURL: verifyURL,
	})
	if err != nil {
		return s.wrapError(ctx, fmt.Errorf("render verification email template: %w", err))
	}

	msg := dtos.EmailMessage{
		To:       []string{email},
		Subject:  "Verify Your Email - Youten AI",
		TextBody: textBody,
		HTMLBody: htmlBody,
	}
	return s.mailer.Send(ctx, msg)
}

// VerifyEmail confirms email verification using 6-digit OTP code or magic link token,
// marks the user active and email_verified=true, and generates fresh session tokens.
func (s *Service) VerifyEmail(ctx context.Context, req *dtos.VerifyEmailRequest) (*dtos.AuthResponse, error) {
	var user *models.User
	var err error

	token := strings.TrimSpace(req.Token)
	code := strings.TrimSpace(req.Code)
	email := strings.TrimSpace(strings.ToLower(req.Email))

	if token != "" {
		tokenHash := hasher.HashToken(token)
		authToken, err := s.repo.FindAuthTokenByHashAndType(ctx, tokenHash, constants.AuthTokenTypeEmailVerification)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, constants.ErrInvalidVerificationCode
			}
			return nil, s.wrapError(ctx, err)
		}
		if authToken == nil || authToken.ExpiresAt.Before(time.Now()) {
			return nil, constants.ErrInvalidVerificationCode
		}

		user, err = s.repo.FindUserByIDWithRole(ctx, authToken.UserID)
		if err != nil {
			return nil, s.wrapError(ctx, err)
		}
	} else if code != "" && email != "" {
		u, err := s.repo.FindUserByEmail(ctx, email)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, constants.ErrInvalidVerificationCode
			}
			return nil, s.wrapError(ctx, err)
		}
		if u == nil {
			return nil, constants.ErrInvalidVerificationCode
		}

		codeHash := hasher.HashToken(email + ":" + code)
		authToken, err := s.repo.FindAuthTokenByHashAndType(ctx, codeHash, constants.AuthTokenTypeEmailVerificationCode)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, constants.ErrInvalidVerificationCode
			}
			return nil, s.wrapError(ctx, err)
		}
		if authToken == nil || authToken.UserID != u.ID || authToken.ExpiresAt.Before(time.Now()) {
			return nil, constants.ErrInvalidVerificationCode
		}

		user, err = s.repo.FindUserByIDWithRole(ctx, u.ID)
		if err != nil {
			return nil, s.wrapError(ctx, err)
		}
	} else {
		return nil, constants.ErrInvalidVerificationCode
	}

	if user == nil {
		return nil, constants.ErrInvalidVerificationCode
	}

	// Update user status
	if err := s.repo.MarkEmailVerified(ctx, user.ID); err != nil {
		return nil, s.wrapError(ctx, err)
	}
	user.EmailVerified = true
	user.Status = constants.UserStatusActive

	// Revoke all verification tokens for this user
	_ = s.repo.RevokeAllAuthTokensByUserID(ctx, user.ID, constants.AuthTokenTypeEmailVerification)
	_ = s.repo.RevokeAllAuthTokensByUserID(ctx, user.ID, constants.AuthTokenTypeEmailVerificationCode)

	// Generate authenticated session token pair
	sessionID := uuid.New().String()
	var roleCode string
	var perms []string
	if user.Role != nil {
		roleCode = user.Role.Code
		perms = user.Role.PermissionsList()
	}

	tokenPair, err := jwt.GenerateTokenPair(s.cfg, user.ID, user.Email, sessionID, jwt.UserRoleClaims{
		RoleCode:    roleCode,
		Permissions: perms,
	})
	if err != nil {
		return nil, s.wrapError(ctx, err)
	}

	refreshTokenHash := hasher.HashToken(tokenPair.RefreshToken)
	sessionAuthToken := &models.AuthToken{
		UserID:    user.ID,
		Type:      constants.AuthTokenTypeRefresh,
		TokenHash: refreshTokenHash,
		ExpiresAt: time.Now().Add(s.cfg.JWTRefreshExpiration),
	}
	if err := s.repo.CreateAuthToken(ctx, sessionAuthToken); err != nil {
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

// ResendVerificationEmail sends a fresh verification code and link to a pending user.
// Enforces rate limiting (60s cooldown and max 3 resends per 24 hours).
func (s *Service) ResendVerificationEmail(ctx context.Context, req *dtos.ResendVerificationRequest) error {
	email := strings.TrimSpace(strings.ToLower(req.Email))

	if err := s.checkResendRateLimit(ctx, email); err != nil {
		return err
	}

	user, err := s.repo.FindUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return s.wrapError(ctx, err)
	}
	if user == nil || user.EmailVerified {
		return nil
	}

	_ = s.repo.RevokeAllAuthTokensByUserID(ctx, user.ID, constants.AuthTokenTypeEmailVerification)
	_ = s.repo.RevokeAllAuthTokensByUserID(ctx, user.ID, constants.AuthTokenTypeEmailVerificationCode)

	code, rawToken, codeHash, tokenHash, err := generateVerificationCodeAndToken(email)
	if err != nil {
		return s.wrapError(ctx, err)
	}

	expiresAt := time.Now().Add(24 * time.Hour)
	if err := s.repo.CreateAuthToken(ctx, &models.AuthToken{
		UserID:    user.ID,
		Type:      constants.AuthTokenTypeEmailVerification,
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
	}); err != nil {
		return s.wrapError(ctx, err)
	}
	if err := s.repo.CreateAuthToken(ctx, &models.AuthToken{
		UserID:    user.ID,
		Type:      constants.AuthTokenTypeEmailVerificationCode,
		TokenHash: codeHash,
		ExpiresAt: expiresAt,
	}); err != nil {
		return s.wrapError(ctx, err)
	}

	s.recordResendAttempt(ctx, email)

	if s.mailer != nil {
		return s.sendVerificationEmail(ctx, user.Email, user.FullName, code, rawToken)
	}
	return nil
}

func (s *Service) checkResendRateLimit(ctx context.Context, email string) error {
	cooldownKey := fmt.Sprintf("email_verify:cooldown:%s", email)
	dailyKey := fmt.Sprintf("email_verify:daily:%s", email)

	if s.repo != nil && s.repo.Redis() != nil && s.repo.Redis().Client() != nil {
		rdb := s.repo.Redis().Client()
		cooldownExists, err := rdb.Exists(ctx, cooldownKey).Result()
		if err == nil && cooldownExists > 0 {
			return constants.ErrResendCooldown
		}

		dailyCount, err := rdb.Get(ctx, dailyKey).Int()
		if err == nil && dailyCount >= 3 {
			return constants.ErrResendDailyLimit
		}
	} else {
		s.resendMu.Lock()
		defer s.resendMu.Unlock()
		if s.resendCooldowns != nil {
			if expireTime, ok := s.resendCooldowns[email]; ok && time.Now().Before(expireTime) {
				return constants.ErrResendCooldown
			}
		}
		if s.resendDailyCounts != nil {
			if count, ok := s.resendDailyCounts[email]; ok && count >= 3 {
				return constants.ErrResendDailyLimit
			}
		}
	}
	return nil
}

func (s *Service) recordResendAttempt(ctx context.Context, email string) {
	cooldownKey := fmt.Sprintf("email_verify:cooldown:%s", email)
	dailyKey := fmt.Sprintf("email_verify:daily:%s", email)

	if s.repo != nil && s.repo.Redis() != nil && s.repo.Redis().Client() != nil {
		rdb := s.repo.Redis().Client()
		_ = rdb.Set(ctx, cooldownKey, "1", 60*time.Second).Err()
		pipe := rdb.Pipeline()
		pipe.Incr(ctx, dailyKey)
		pipe.Expire(ctx, dailyKey, 24*time.Hour)
		_, _ = pipe.Exec(ctx)
	} else {
		s.resendMu.Lock()
		defer s.resendMu.Unlock()
		if s.resendCooldowns == nil {
			s.resendCooldowns = make(map[string]time.Time)
		}
		s.resendCooldowns[email] = time.Now().Add(60 * time.Second)

		if s.resendDailyCounts == nil {
			s.resendDailyCounts = make(map[string]int)
		}
		s.resendDailyCounts[email]++
	}
}


