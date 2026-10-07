package services_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	gormPostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"

	"code-base-golang/internal/config"
	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/pkg/hasher"
	"code-base-golang/internal/pkg/jwt"
	"code-base-golang/internal/repositories"
	"code-base-golang/internal/services"
)

type mockMailer struct {
	pingErr error
	sendErr error
	sent    []services.EmailMessage
}

func (m *mockMailer) Ping(ctx context.Context) error {
	return m.pingErr
}

func (m *mockMailer) Send(ctx context.Context, msg services.EmailMessage) error {
	if m.sendErr != nil {
		return m.sendErr
	}
	m.sent = append(m.sent, msg)
	return nil
}

func setupAuthServiceMock(t *testing.T) (*services.Service, sqlmock.Sqlmock, func()) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {

		t.Fatalf("failed to open sqlmock: %v", err)
	}

	gormDB, err := gorm.Open(gormPostgres.New(gormPostgres.Config{
		Conn: sqlDB,
	}), &gorm.Config{})
	if err != nil {
		sqlDB.Close()
		t.Fatalf("failed to initialize gorm: %v", err)
	}

	repo := repositories.New(gormDB)
	cfg := config.Config{
		AppName:              "youten-test",
		JWTSecret:            "test-jwt-secret-key-1234567890",
		JWTAccessExpiration:  15 * time.Minute,
		JWTRefreshExpiration: 7 * 24 * time.Hour,
	}
	svc := services.New(cfg, repo, nil)

	cleanup := func() {
		sqlDB.Close()
	}

	return svc, mock, cleanup
}

func TestService_Register_Success(t *testing.T) {
	svc, mock, cleanup := setupAuthServiceMock(t)
	defer cleanup()

	email := "newuser@example.com"
	req := &dtos.RegisterRequest{
		Email:    email,
		Password: "Password123",
		FullName: "Test User",
	}

	// 1. Expect query for checking duplicate email (returns 0 rows -> record not found)
	mock.ExpectQuery(`SELECT \* FROM "users" WHERE \(email = \$1 AND deleted_at IS NULL\) AND "users"\."deleted_at" IS NULL ORDER BY "users"\."id" LIMIT \$2`).
		WithArgs(email, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	// 2. Expect insert user
	userID := uuid.New()
	now := time.Now()
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "users"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(userID, now, now))
	mock.ExpectCommit()

	resp, err := svc.Register(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error registering user: %v", err)
	}

	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	if resp.Email != email {
		t.Errorf("expected email %s, got %s", email, resp.Email)
	}
	if resp.FullName != req.FullName {
		t.Errorf("expected full_name %s, got %s", req.FullName, resp.FullName)
	}
	if resp.Status != constants.UserStatusActive {
		t.Errorf("expected status %s, got %s", constants.UserStatusActive, resp.Status)
	}
	if resp.DailyQuota != constants.DefaultUserDailyQuota {
		t.Errorf("expected default quota %d, got %d", constants.DefaultUserDailyQuota, resp.DailyQuota)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestService_Register_DuplicateEmail(t *testing.T) {
	svc, mock, cleanup := setupAuthServiceMock(t)
	defer cleanup()

	email := "existing@example.com"
	req := &dtos.RegisterRequest{
		Email:    email,
		Password: "Password123",
		FullName: "Existing User",
	}

	// Expect query finding existing user
	userID := uuid.New()
	mock.ExpectQuery(`SELECT \* FROM "users" WHERE \(email = \$1 AND deleted_at IS NULL\) AND "users"\."deleted_at" IS NULL ORDER BY "users"\."id" LIMIT \$2`).
		WithArgs(email, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "full_name"}).AddRow(userID, email, "Existing User"))

	resp, err := svc.Register(context.Background(), req)
	if err == nil {
		t.Fatal("expected duplicate email error, got nil")
	}

	if !errors.Is(err, constants.ErrEmailAlreadyExists) {
		t.Errorf("expected ErrEmailAlreadyExists, got %v", err)
	}
	if resp != nil {
		t.Errorf("expected nil response, got %v", resp)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestService_Register_DBError(t *testing.T) {
	svc, mock, cleanup := setupAuthServiceMock(t)
	defer cleanup()

	email := "error@example.com"
	req := &dtos.RegisterRequest{
		Email:    email,
		Password: "Password123",
		FullName: "Error User",
	}

	mock.ExpectQuery(`SELECT \* FROM "users"`).
		WithArgs(email, 1).
		WillReturnError(errors.New("database connection failed"))

	resp, err := svc.Register(context.Background(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if resp != nil {
		t.Errorf("expected nil response, got %v", resp)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestService_Login_Success(t *testing.T) {
	svc, mock, cleanup := setupAuthServiceMock(t)
	defer cleanup()

	email := "login@example.com"
	password := "Password123"
	hashedPassword, err := hasher.HashPassword(password)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}

	userID := uuid.New()
	now := time.Now()

	// 1. Expect FindUserByEmail
	mock.ExpectQuery(`SELECT \* FROM "users" WHERE \(email = \$1 AND deleted_at IS NULL\) AND "users"\."deleted_at" IS NULL ORDER BY "users"\."id" LIMIT \$2`).
		WithArgs(email, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "password_hash", "full_name", "status", "created_at"}).
			AddRow(userID, email, hashedPassword, "Login User", constants.UserStatusActive, now))

	// 2. Expect CreateAuthToken
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "auth_tokens"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(uuid.New(), now, now))
	mock.ExpectCommit()

	req := &dtos.LoginRequest{
		Email:    email,
		Password: password,
	}

	resp, err := svc.Login(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error on login: %v", err)
	}

	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	if resp.AccessToken == "" {
		t.Error("expected non-empty access token")
	}
	if resp.RefreshToken == "" {
		t.Error("expected non-empty refresh token")
	}
	if resp.TokenType != "Bearer" {
		t.Errorf("expected token type Bearer, got %s", resp.TokenType)
	}
	if resp.User.Email != email {
		t.Errorf("expected user email %s, got %s", email, resp.User.Email)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestService_Login_InvalidPassword(t *testing.T) {
	svc, mock, cleanup := setupAuthServiceMock(t)
	defer cleanup()

	email := "login@example.com"
	correctPassword := "Password123"
	hashedPassword, _ := hasher.HashPassword(correctPassword)
	userID := uuid.New()
	now := time.Now()

	mock.ExpectQuery(`SELECT \* FROM "users" WHERE \(email = \$1 AND deleted_at IS NULL\) AND "users"\."deleted_at" IS NULL ORDER BY "users"\."id" LIMIT \$2`).
		WithArgs(email, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "password_hash", "full_name", "status", "created_at"}).
			AddRow(userID, email, hashedPassword, "Login User", constants.UserStatusActive, now))

	req := &dtos.LoginRequest{
		Email:    email,
		Password: "WrongPassword999",
	}

	resp, err := svc.Login(context.Background(), req)
	if err == nil {
		t.Fatal("expected unauthorized error, got nil")
	}
	if !errors.Is(err, constants.ErrInvalidCredentials) {
		t.Errorf("expected ErrInvalidCredentials, got %v", err)
	}
	if resp != nil {
		t.Errorf("expected nil response, got %v", resp)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestService_Login_UserNotFound(t *testing.T) {
	svc, mock, cleanup := setupAuthServiceMock(t)
	defer cleanup()

	email := "notfound@example.com"
	mock.ExpectQuery(`SELECT \* FROM "users" WHERE \(email = \$1 AND deleted_at IS NULL\) AND "users"\."deleted_at" IS NULL ORDER BY "users"\."id" LIMIT \$2`).
		WithArgs(email, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	req := &dtos.LoginRequest{
		Email:    email,
		Password: "Password123",
	}

	resp, err := svc.Login(context.Background(), req)
	if err == nil {
		t.Fatal("expected unauthorized error, got nil")
	}
	if !errors.Is(err, constants.ErrInvalidCredentials) {
		t.Errorf("expected ErrInvalidCredentials, got %v", err)
	}
	if resp != nil {
		t.Errorf("expected nil response, got %v", resp)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestService_Login_SuspendedUser(t *testing.T) {
	svc, mock, cleanup := setupAuthServiceMock(t)
	defer cleanup()

	email := "suspended@example.com"
	password := "Password123"
	hashedPassword, _ := hasher.HashPassword(password)
	userID := uuid.New()
	now := time.Now()

	mock.ExpectQuery(`SELECT \* FROM "users" WHERE \(email = \$1 AND deleted_at IS NULL\) AND "users"\."deleted_at" IS NULL ORDER BY "users"\."id" LIMIT \$2`).
		WithArgs(email, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "password_hash", "full_name", "status", "created_at"}).
			AddRow(userID, email, hashedPassword, "Suspended User", constants.UserStatusSuspended, now))

	req := &dtos.LoginRequest{
		Email:    email,
		Password: password,
	}

	resp, err := svc.Login(context.Background(), req)
	if err == nil {
		t.Fatal("expected unauthorized error, got nil")
	}
	if !errors.Is(err, constants.ErrInvalidCredentials) {
		t.Errorf("expected ErrInvalidCredentials, got %v", err)
	}
	if resp != nil {
		t.Errorf("expected nil response, got %v", resp)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestService_Login_NilUserNoError(t *testing.T) {
	svc, mock, cleanup := setupAuthServiceMock(t)
	defer cleanup()

	email := "niluser@example.com"
	mock.ExpectQuery(`SELECT \* FROM "users"`).
		WithArgs(email, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	req := &dtos.LoginRequest{
		Email:    email,
		Password: "Password123",
	}

	resp, err := svc.Login(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for nil user, got nil")
	}
	if !errors.Is(err, constants.ErrInvalidCredentials) {
		t.Errorf("expected ErrInvalidCredentials, got %v", err)
	}
	if resp != nil {
		t.Errorf("expected nil response, got %v", resp)
	}
}

func TestService_RefreshToken_Success(t *testing.T) {
	svc, mock, cleanup := setupAuthServiceMock(t)
	defer cleanup()

	userID := uuid.New()
	email := "refresh@example.com"
	now := time.Now()

	cfg := config.Config{
		AppName:              "youten-test",
		JWTSecret:            "test-jwt-secret-key-1234567890",
		JWTAccessExpiration:  15 * time.Minute,
		JWTRefreshExpiration: 7 * 24 * time.Hour,
	}

	tokenPair, err := jwt.GenerateTokenPair(cfg, userID, email, "session-old")
	if err != nil {
		t.Fatalf("failed to generate token pair: %v", err)
	}

	tokenHash := hasher.HashToken(tokenPair.RefreshToken)
	tokenID := uuid.New()

	// 1. FindAuthTokenByHashAndType
	mock.ExpectQuery(`SELECT \* FROM "auth_tokens" WHERE token_hash = \$1 AND type = \$2 AND revoked_at IS NULL AND expires_at > \$3 ORDER BY "auth_tokens"\."id" LIMIT \$4`).
		WithArgs(tokenHash, constants.AuthTokenTypeRefresh, sqlmock.AnyArg(), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "type", "token_hash", "expires_at", "created_at", "updated_at"}).
			AddRow(tokenID, userID, constants.AuthTokenTypeRefresh, tokenHash, now.Add(24*time.Hour), now, now))

	// 2. FindUserByID
	mock.ExpectQuery(`SELECT \* FROM "users" WHERE id = \$1 AND "users"\."deleted_at" IS NULL ORDER BY "users"\."id" LIMIT \$2`).
		WithArgs(userID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "full_name", "status", "created_at"}).
			AddRow(userID, email, "Refresh User", constants.UserStatusActive, now))

	// 3. RotateAuthToken (Transaction: revoke old token + insert new token)
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "auth_tokens"`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), tokenID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(`INSERT INTO "auth_tokens"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(uuid.New(), now, now))
	mock.ExpectCommit()

	req := &dtos.RefreshTokenRequest{
		RefreshToken: tokenPair.RefreshToken,
	}

	resp, err := svc.RefreshToken(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error refreshing token: %v", err)
	}

	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Error("expected non-empty tokens in response")
	}
	if resp.User.Email != email {
		t.Errorf("expected email %s, got %s", email, resp.User.Email)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestService_RefreshToken_InvalidJWT(t *testing.T) {
	svc, _, cleanup := setupAuthServiceMock(t)
	defer cleanup()

	req := &dtos.RefreshTokenRequest{
		RefreshToken: "invalid-token-string",
	}

	resp, err := svc.RefreshToken(context.Background(), req)
	if err == nil {
		t.Fatal("expected invalid token error, got nil")
	}
	if !errors.Is(err, constants.ErrInvalidToken) {
		t.Errorf("expected ErrInvalidToken, got %v", err)
	}
	if resp != nil {
		t.Errorf("expected nil response, got %v", resp)
	}
}

func TestService_RefreshToken_WrongTokenType(t *testing.T) {
	svc, _, cleanup := setupAuthServiceMock(t)
	defer cleanup()

	cfg := config.Config{
		AppName:              "youten-test",
		JWTSecret:            "test-jwt-secret-key-1234567890",
		JWTAccessExpiration:  15 * time.Minute,
		JWTRefreshExpiration: 7 * 24 * time.Hour,
	}

	tokenPair, err := jwt.GenerateTokenPair(cfg, uuid.New(), "user@example.com", "session-1")
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	// Pass access token instead of refresh token
	req := &dtos.RefreshTokenRequest{
		RefreshToken: tokenPair.AccessToken,
	}

	resp, err := svc.RefreshToken(context.Background(), req)
	if err == nil {
		t.Fatal("expected error when passing access token to refresh, got nil")
	}
	if !errors.Is(err, constants.ErrInvalidToken) {
		t.Errorf("expected ErrInvalidToken, got %v", err)
	}
	if resp != nil {
		t.Errorf("expected nil response, got %v", resp)
	}
}

func TestService_RefreshToken_TokenNotFoundOrRevoked(t *testing.T) {
	svc, mock, cleanup := setupAuthServiceMock(t)
	defer cleanup()

	cfg := config.Config{
		AppName:              "youten-test",
		JWTSecret:            "test-jwt-secret-key-1234567890",
		JWTAccessExpiration:  15 * time.Minute,
		JWTRefreshExpiration: 7 * 24 * time.Hour,
	}

	tokenPair, err := jwt.GenerateTokenPair(cfg, uuid.New(), "user@example.com", "session-1")
	if err != nil {
		t.Fatalf("failed to generate token pair: %v", err)
	}
	tokenHash := hasher.HashToken(tokenPair.RefreshToken)

	mock.ExpectQuery(`SELECT \* FROM "auth_tokens" WHERE token_hash = \$1 AND type = \$2 AND revoked_at IS NULL AND expires_at > \$3 ORDER BY "auth_tokens"\."id" LIMIT \$4`).
		WithArgs(tokenHash, constants.AuthTokenTypeRefresh, sqlmock.AnyArg(), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	req := &dtos.RefreshTokenRequest{
		RefreshToken: tokenPair.RefreshToken,
	}

	resp, err := svc.RefreshToken(context.Background(), req)
	if err == nil {
		t.Fatal("expected invalid token error, got nil")
	}
	if !errors.Is(err, constants.ErrInvalidToken) {
		t.Errorf("expected ErrInvalidToken, got %v", err)
	}
	if resp != nil {
		t.Errorf("expected nil response, got %v", resp)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestService_RefreshToken_InactiveUser(t *testing.T) {
	svc, mock, cleanup := setupAuthServiceMock(t)
	defer cleanup()

	userID := uuid.New()
	email := "suspended@example.com"
	now := time.Now()

	cfg := config.Config{
		AppName:              "youten-test",
		JWTSecret:            "test-jwt-secret-key-1234567890",
		JWTAccessExpiration:  15 * time.Minute,
		JWTRefreshExpiration: 7 * 24 * time.Hour,
	}

	tokenPair, err := jwt.GenerateTokenPair(cfg, userID, email, "session-1")
	if err != nil {
		t.Fatalf("failed to generate token pair: %v", err)
	}
	tokenHash := hasher.HashToken(tokenPair.RefreshToken)
	tokenID := uuid.New()

	// 1. FindAuthTokenByHashAndType
	mock.ExpectQuery(`SELECT \* FROM "auth_tokens" WHERE token_hash = \$1 AND type = \$2 AND revoked_at IS NULL AND expires_at > \$3 ORDER BY "auth_tokens"\."id" LIMIT \$4`).
		WithArgs(tokenHash, constants.AuthTokenTypeRefresh, sqlmock.AnyArg(), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "type", "token_hash", "expires_at", "created_at", "updated_at"}).
			AddRow(tokenID, userID, constants.AuthTokenTypeRefresh, tokenHash, now.Add(24*time.Hour), now, now))

	// 2. FindUserByID -> status suspended
	mock.ExpectQuery(`SELECT \* FROM "users"`).
		WithArgs(userID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "full_name", "status", "created_at"}).
			AddRow(userID, email, "Suspended User", constants.UserStatusSuspended, now))

	// 3. RevokeAuthToken
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "auth_tokens" SET "revoked_at"=\$1,"updated_at"=\$2 WHERE id = \$3`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), tokenID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	req := &dtos.RefreshTokenRequest{
		RefreshToken: tokenPair.RefreshToken,
	}

	resp, err := svc.RefreshToken(context.Background(), req)
	if err == nil {
		t.Fatal("expected invalid token error for inactive user, got nil")
	}
	if !errors.Is(err, constants.ErrInvalidToken) {
		t.Errorf("expected ErrInvalidToken, got %v", err)
	}
	if resp != nil {
		t.Errorf("expected nil response, got %v", resp)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestService_RefreshToken_InactiveUser_RevokeError(t *testing.T) {
	svc, mock, cleanup := setupAuthServiceMock(t)
	defer cleanup()

	userID := uuid.New()
	email := "suspended@example.com"
	now := time.Now()

	cfg := config.Config{
		AppName:              "youten-test",
		JWTSecret:            "test-jwt-secret-key-1234567890",
		JWTAccessExpiration:  15 * time.Minute,
		JWTRefreshExpiration: 7 * 24 * time.Hour,
	}

	tokenPair, err := jwt.GenerateTokenPair(cfg, userID, email, "session-1")
	if err != nil {
		t.Fatalf("failed to generate token pair: %v", err)
	}
	tokenHash := hasher.HashToken(tokenPair.RefreshToken)
	tokenID := uuid.New()

	// 1. FindAuthTokenByHashAndType
	mock.ExpectQuery(`SELECT \* FROM "auth_tokens"`).
		WithArgs(tokenHash, constants.AuthTokenTypeRefresh, sqlmock.AnyArg(), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "type", "token_hash", "expires_at", "created_at", "updated_at"}).
			AddRow(tokenID, userID, constants.AuthTokenTypeRefresh, tokenHash, now.Add(24*time.Hour), now, now))

	// 2. FindUserByID -> status suspended
	mock.ExpectQuery(`SELECT \* FROM "users"`).
		WithArgs(userID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "full_name", "status", "created_at"}).
			AddRow(userID, email, "Suspended User", constants.UserStatusSuspended, now))

	// 3. RevokeAuthToken -> DB error
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "auth_tokens"`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), tokenID).
		WillReturnError(errors.New("db error on revoke"))
	mock.ExpectRollback()

	req := &dtos.RefreshTokenRequest{
		RefreshToken: tokenPair.RefreshToken,
	}

	resp, err := svc.RefreshToken(context.Background(), req)
	if err == nil {
		t.Fatal("expected error on revoke failure, got nil")
	}
	if resp != nil {
		t.Errorf("expected nil response, got %v", resp)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestService_Logout_Success(t *testing.T) {
	svc, mock, cleanup := setupAuthServiceMock(t)
	defer cleanup()

	refreshToken := "valid.refresh.token.for.logout"
	tokenHash := hasher.HashToken(refreshToken)
	tokenID := uuid.New()
	userID := uuid.New()
	now := time.Now()

	// 1. FindAuthTokenByHashAndType
	mock.ExpectQuery(`SELECT \* FROM "auth_tokens" WHERE token_hash = \$1 AND type = \$2 AND revoked_at IS NULL AND expires_at > \$3 ORDER BY "auth_tokens"\."id" LIMIT \$4`).
		WithArgs(tokenHash, constants.AuthTokenTypeRefresh, sqlmock.AnyArg(), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "type", "token_hash", "expires_at", "created_at", "updated_at"}).
			AddRow(tokenID, userID, constants.AuthTokenTypeRefresh, tokenHash, now.Add(24*time.Hour), now, now))

	// 2. RevokeAuthToken
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "auth_tokens" SET "revoked_at"=\$1,"updated_at"=\$2 WHERE id = \$3`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), tokenID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	req := &dtos.LogoutRequest{
		RefreshToken: refreshToken,
	}

	err := svc.Logout(context.Background(), req)
	if err != nil {
		t.Fatalf("expected nil error on logout, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestService_Logout_Idempotent_NotFound(t *testing.T) {
	svc, mock, cleanup := setupAuthServiceMock(t)
	defer cleanup()

	refreshToken := "unknown.or.expired.refresh.token"
	tokenHash := hasher.HashToken(refreshToken)

	// Token not found in DB
	mock.ExpectQuery(`SELECT \* FROM "auth_tokens" WHERE token_hash = \$1 AND type = \$2 AND revoked_at IS NULL AND expires_at > \$3 ORDER BY "auth_tokens"\."id" LIMIT \$4`).
		WithArgs(tokenHash, constants.AuthTokenTypeRefresh, sqlmock.AnyArg(), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	req := &dtos.LogoutRequest{
		RefreshToken: refreshToken,
	}

	err := svc.Logout(context.Background(), req)
	if err != nil {
		t.Fatalf("expected nil error on idempotent logout when token not found, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestService_Logout_Idempotent_AlreadyRevoked(t *testing.T) {
	svc, mock, cleanup := setupAuthServiceMock(t)
	defer cleanup()

	refreshToken := "already.revoked.refresh.token"
	tokenHash := hasher.HashToken(refreshToken)
	tokenID := uuid.New()
	userID := uuid.New()
	now := time.Now()

	// 1. FindAuthTokenByHashAndType
	mock.ExpectQuery(`SELECT \* FROM "auth_tokens" WHERE token_hash = \$1 AND type = \$2 AND revoked_at IS NULL AND expires_at > \$3 ORDER BY "auth_tokens"\."id" LIMIT \$4`).
		WithArgs(tokenHash, constants.AuthTokenTypeRefresh, sqlmock.AnyArg(), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "type", "token_hash", "expires_at", "created_at", "updated_at"}).
			AddRow(tokenID, userID, constants.AuthTokenTypeRefresh, tokenHash, now.Add(24*time.Hour), now, now))

	// 2. RevokeAuthToken returns 0 rows affected (concurrently revoked)
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "auth_tokens" SET "revoked_at"=\$1,"updated_at"=\$2 WHERE id = \$3`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), tokenID).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	req := &dtos.LogoutRequest{
		RefreshToken: refreshToken,
	}

	err := svc.Logout(context.Background(), req)
	if err != nil {
		t.Fatalf("expected nil error on idempotent logout when already revoked, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestService_Logout_DatabaseError(t *testing.T) {
	svc, mock, cleanup := setupAuthServiceMock(t)
	defer cleanup()

	refreshToken := "error.refresh.token"
	tokenHash := hasher.HashToken(refreshToken)

	mock.ExpectQuery(`SELECT \* FROM "auth_tokens" WHERE token_hash = \$1 AND type = \$2 AND revoked_at IS NULL AND expires_at > \$3 ORDER BY "auth_tokens"\."id" LIMIT \$4`).
		WithArgs(tokenHash, constants.AuthTokenTypeRefresh, sqlmock.AnyArg(), 1).
		WillReturnError(errors.New("db connection failure"))

	req := &dtos.LogoutRequest{
		RefreshToken: refreshToken,
	}

	err := svc.Logout(context.Background(), req)
	if err == nil {
		t.Fatal("expected error on DB lookup failure, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestService_Logout_RevokeDatabaseError(t *testing.T) {
	svc, mock, cleanup := setupAuthServiceMock(t)
	defer cleanup()

	refreshToken := "revoke.error.refresh.token"
	tokenHash := hasher.HashToken(refreshToken)
	tokenID := uuid.New()
	userID := uuid.New()
	now := time.Now()

	mock.ExpectQuery(`SELECT \* FROM "auth_tokens" WHERE token_hash = \$1 AND type = \$2 AND revoked_at IS NULL AND expires_at > \$3 ORDER BY "auth_tokens"\."id" LIMIT \$4`).
		WithArgs(tokenHash, constants.AuthTokenTypeRefresh, sqlmock.AnyArg(), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "type", "token_hash", "expires_at", "created_at", "updated_at"}).
			AddRow(tokenID, userID, constants.AuthTokenTypeRefresh, tokenHash, now.Add(24*time.Hour), now, now))

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "auth_tokens" SET "revoked_at"=\$1,"updated_at"=\$2 WHERE id = \$3`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), tokenID).
		WillReturnError(errors.New("db update deadlock"))
	mock.ExpectRollback()

	req := &dtos.LogoutRequest{
		RefreshToken: refreshToken,
	}

	err := svc.Logout(context.Background(), req)
	if err == nil {
		t.Fatal("expected error on DB revoke failure, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestService_ForgotPassword_Success(t *testing.T) {
	svc, mock, cleanup := setupAuthServiceMock(t)
	defer cleanup()

	mailer := &mockMailer{}
	svc.SetMailer(mailer)

	email := "forgot@example.com"
	userID := uuid.New()
	now := time.Now()

	// 1. FindUserByEmail
	mock.ExpectQuery(`SELECT \* FROM "users" WHERE \(email = \$1 AND deleted_at IS NULL\) AND "users"\."deleted_at" IS NULL ORDER BY "users"\."id" LIMIT \$2`).
		WithArgs(email, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "full_name", "status", "created_at"}).
			AddRow(userID, email, "Forgot User", constants.UserStatusActive, now))

	// 2. RevokeAllAuthTokensByUserID
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "auth_tokens" SET "revoked_at"=\$1,"updated_at"=\$2 WHERE \(user_id = \$3 AND revoked_at IS NULL\) AND type = \$4`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), userID, constants.AuthTokenTypeReset).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	// 3. CreateAuthToken
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "auth_tokens"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).
			AddRow(uuid.New(), now, now))
	mock.ExpectCommit()

	req := &dtos.ForgotPasswordRequest{
		Email: email,
	}

	err := svc.ForgotPassword(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error on forgot password: %v", err)
	}

	if len(mailer.sent) != 1 {
		t.Fatalf("expected 1 email to be sent, got %d", len(mailer.sent))
	}
	sentMsg := mailer.sent[0]
	if len(sentMsg.To) != 1 || sentMsg.To[0] != email {
		t.Errorf("expected recipient %s, got %v", email, sentMsg.To)
	}
	if !strings.Contains(sentMsg.Subject, "Reset Your Password") {
		t.Errorf("expected subject to contain 'Reset Your Password', got %s", sentMsg.Subject)
	}
	if !strings.Contains(sentMsg.TextBody, "Forgot User") {
		t.Errorf("expected text body to contain user name 'Forgot User', got %s", sentMsg.TextBody)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestService_ForgotPassword_UserNotFound(t *testing.T) {
	svc, mock, cleanup := setupAuthServiceMock(t)
	defer cleanup()

	mailer := &mockMailer{}
	svc.SetMailer(mailer)

	email := "nonexistent@example.com"

	// Mock email not found in DB -> safe silent success to prevent enumeration
	mock.ExpectQuery(`SELECT \* FROM "users" WHERE \(email = \$1 AND deleted_at IS NULL\) AND "users"\."deleted_at" IS NULL ORDER BY "users"\."id" LIMIT \$2`).
		WithArgs(email, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	req := &dtos.ForgotPasswordRequest{
		Email: email,
	}

	err := svc.ForgotPassword(context.Background(), req)
	if err != nil {
		t.Fatalf("expected nil error on non-existent email, got %v", err)
	}

	if len(mailer.sent) != 0 {
		t.Errorf("expected 0 emails to be sent, got %d", len(mailer.sent))
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestService_ForgotPassword_InactiveUser(t *testing.T) {
	svc, mock, cleanup := setupAuthServiceMock(t)
	defer cleanup()

	mailer := &mockMailer{}
	svc.SetMailer(mailer)

	email := "suspended@example.com"
	userID := uuid.New()
	now := time.Now()

	// Suspended user
	mock.ExpectQuery(`SELECT \* FROM "users" WHERE \(email = \$1 AND deleted_at IS NULL\) AND "users"\."deleted_at" IS NULL ORDER BY "users"\."id" LIMIT \$2`).
		WithArgs(email, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "full_name", "status", "created_at"}).
			AddRow(userID, email, "Suspended User", constants.UserStatusSuspended, now))

	req := &dtos.ForgotPasswordRequest{
		Email: email,
	}

	err := svc.ForgotPassword(context.Background(), req)
	if err != nil {
		t.Fatalf("expected nil error on suspended user, got %v", err)
	}

	if len(mailer.sent) != 0 {
		t.Errorf("expected 0 emails to be sent for inactive user, got %d", len(mailer.sent))
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestService_ForgotPassword_DatabaseError(t *testing.T) {
	svc, mock, cleanup := setupAuthServiceMock(t)
	defer cleanup()

	email := "dberr@example.com"

	mock.ExpectQuery(`SELECT \* FROM "users" WHERE \(email = \$1 AND deleted_at IS NULL\) AND "users"\."deleted_at" IS NULL ORDER BY "users"\."id" LIMIT \$2`).
		WithArgs(email, 1).
		WillReturnError(errors.New("db query timeout"))

	req := &dtos.ForgotPasswordRequest{
		Email: email,
	}

	err := svc.ForgotPassword(context.Background(), req)
	if err == nil {
		t.Fatal("expected error on DB failure, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestService_ForgotPassword_RevokeTokensError(t *testing.T) {
	svc, mock, cleanup := setupAuthServiceMock(t)
	defer cleanup()

	email := "revokeerr@example.com"
	userID := uuid.New()
	now := time.Now()

	mock.ExpectQuery(`SELECT \* FROM "users" WHERE \(email = \$1 AND deleted_at IS NULL\) AND "users"\."deleted_at" IS NULL ORDER BY "users"\."id" LIMIT \$2`).
		WithArgs(email, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "full_name", "status", "created_at"}).
			AddRow(userID, email, "Revoke Err User", constants.UserStatusActive, now))

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "auth_tokens" SET "revoked_at"=\$1,"updated_at"=\$2 WHERE \(user_id = \$3 AND revoked_at IS NULL\) AND type = \$4`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), userID, constants.AuthTokenTypeReset).
		WillReturnError(errors.New("db update error"))
	mock.ExpectRollback()

	req := &dtos.ForgotPasswordRequest{
		Email: email,
	}

	err := svc.ForgotPassword(context.Background(), req)
	if err == nil {
		t.Fatal("expected error on token revocation failure, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestService_ForgotPassword_CreateTokenError(t *testing.T) {
	svc, mock, cleanup := setupAuthServiceMock(t)
	defer cleanup()

	email := "createerr@example.com"
	userID := uuid.New()
	now := time.Now()

	mock.ExpectQuery(`SELECT \* FROM "users" WHERE \(email = \$1 AND deleted_at IS NULL\) AND "users"\."deleted_at" IS NULL ORDER BY "users"\."id" LIMIT \$2`).
		WithArgs(email, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "full_name", "status", "created_at"}).
			AddRow(userID, email, "Create Err User", constants.UserStatusActive, now))

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "auth_tokens" SET "revoked_at"=\$1,"updated_at"=\$2 WHERE \(user_id = \$3 AND revoked_at IS NULL\) AND type = \$4`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), userID, constants.AuthTokenTypeReset).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "auth_tokens"`).
		WillReturnError(errors.New("db insert token deadlock"))
	mock.ExpectRollback()

	req := &dtos.ForgotPasswordRequest{
		Email: email,
	}

	err := svc.ForgotPassword(context.Background(), req)
	if err == nil {
		t.Fatal("expected error on token creation failure, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestService_ForgotPassword_MailerError(t *testing.T) {
	svc, mock, cleanup := setupAuthServiceMock(t)
	defer cleanup()

	mailer := &mockMailer{sendErr: errors.New("smtp connection refused")}
	svc.SetMailer(mailer)

	email := "mailerr@example.com"
	userID := uuid.New()
	now := time.Now()

	mock.ExpectQuery(`SELECT \* FROM "users" WHERE \(email = \$1 AND deleted_at IS NULL\) AND "users"\."deleted_at" IS NULL ORDER BY "users"\."id" LIMIT \$2`).
		WithArgs(email, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "full_name", "status", "created_at"}).
			AddRow(userID, email, "Mail Err User", constants.UserStatusActive, now))

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "auth_tokens" SET "revoked_at"=\$1,"updated_at"=\$2 WHERE \(user_id = \$3 AND revoked_at IS NULL\) AND type = \$4`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), userID, constants.AuthTokenTypeReset).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "auth_tokens"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).
			AddRow(uuid.New(), now, now))
	mock.ExpectCommit()

	req := &dtos.ForgotPasswordRequest{
		Email: email,
	}

	err := svc.ForgotPassword(context.Background(), req)
	if err == nil {
		t.Fatal("expected error on mailer send failure, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestService_ResetPassword_Success(t *testing.T) {
	svc, mock, cleanup := setupAuthServiceMock(t)
	defer cleanup()

	rawToken := "valid-reset-token-hex"
	tokenHash := hasher.HashToken(rawToken)
	tokenID := uuid.New()
	userID := uuid.New()
	now := time.Now()

	// 1. Mock finding reset token in auth_tokens
	mock.ExpectQuery(`SELECT \* FROM "auth_tokens" WHERE token_hash = \$1 AND type = \$2 AND revoked_at IS NULL AND expires_at > \$3 ORDER BY "auth_tokens"\."id" LIMIT \$4`).
		WithArgs(tokenHash, constants.AuthTokenTypeReset, sqlmock.AnyArg(), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "type", "token_hash", "expires_at", "created_at", "updated_at"}).
			AddRow(tokenID, userID, constants.AuthTokenTypeReset, tokenHash, now.Add(15*time.Minute), now, now))

	// 2. Mock finding user in users table
	mock.ExpectQuery(`SELECT \* FROM "users" WHERE id = \$1 AND "users"\."deleted_at" IS NULL ORDER BY "users"\."id" LIMIT \$2`).
		WithArgs(userID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "full_name", "status", "created_at"}).
			AddRow(userID, "user@example.com", "Test User", constants.UserStatusActive, now))

	// 3. Mock updating user password hash
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "users" SET "password_hash"=\$1,"updated_at"=\$2 WHERE id = \$3 AND "users"\."deleted_at" IS NULL`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), userID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	// 4. Mock revoking the reset token
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "auth_tokens" SET "revoked_at"=\$1,"updated_at"=\$2 WHERE id = \$3`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), tokenID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	// 5. Mock revoking all active refresh tokens for the user
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "auth_tokens" SET "revoked_at"=\$1,"updated_at"=\$2 WHERE \(user_id = \$3 AND revoked_at IS NULL\) AND type = \$4`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), userID, constants.AuthTokenTypeRefresh).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	req := &dtos.ResetPasswordRequest{
		Token:       rawToken,
		NewPassword: "newSecurePassword123",
	}

	err := svc.ResetPassword(context.Background(), req)
	if err != nil {
		t.Fatalf("expected nil error on valid reset password, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestService_ResetPassword_TokenNotFoundOrExpired(t *testing.T) {
	svc, mock, cleanup := setupAuthServiceMock(t)
	defer cleanup()

	rawToken := "expired-or-invalid-token"
	tokenHash := hasher.HashToken(rawToken)

	mock.ExpectQuery(`SELECT \* FROM "auth_tokens" WHERE token_hash = \$1 AND type = \$2 AND revoked_at IS NULL AND expires_at > \$3 ORDER BY "auth_tokens"\."id" LIMIT \$4`).
		WithArgs(tokenHash, constants.AuthTokenTypeReset, sqlmock.AnyArg(), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	req := &dtos.ResetPasswordRequest{
		Token:       rawToken,
		NewPassword: "newSecurePassword123",
	}

	err := svc.ResetPassword(context.Background(), req)
	if err == nil {
		t.Fatal("expected error on non-existent or expired reset token, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestService_ResetPassword_UserNotFound(t *testing.T) {
	svc, mock, cleanup := setupAuthServiceMock(t)
	defer cleanup()

	rawToken := "valid-token-for-missing-user"
	tokenHash := hasher.HashToken(rawToken)
	tokenID := uuid.New()
	userID := uuid.New()
	now := time.Now()

	mock.ExpectQuery(`SELECT \* FROM "auth_tokens" WHERE token_hash = \$1 AND type = \$2 AND revoked_at IS NULL AND expires_at > \$3 ORDER BY "auth_tokens"\."id" LIMIT \$4`).
		WithArgs(tokenHash, constants.AuthTokenTypeReset, sqlmock.AnyArg(), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "type", "token_hash", "expires_at", "created_at", "updated_at"}).
			AddRow(tokenID, userID, constants.AuthTokenTypeReset, tokenHash, now.Add(15*time.Minute), now, now))

	mock.ExpectQuery(`SELECT \* FROM "users" WHERE id = \$1 AND "users"\."deleted_at" IS NULL ORDER BY "users"\."id" LIMIT \$2`).
		WithArgs(userID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	req := &dtos.ResetPasswordRequest{
		Token:       rawToken,
		NewPassword: "newSecurePassword123",
	}

	err := svc.ResetPassword(context.Background(), req)
	if err == nil {
		t.Fatal("expected error when user not found, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestService_ResetPassword_InactiveUser(t *testing.T) {
	svc, mock, cleanup := setupAuthServiceMock(t)
	defer cleanup()

	rawToken := "valid-token-for-suspended-user"
	tokenHash := hasher.HashToken(rawToken)
	tokenID := uuid.New()
	userID := uuid.New()
	now := time.Now()

	mock.ExpectQuery(`SELECT \* FROM "auth_tokens" WHERE token_hash = \$1 AND type = \$2 AND revoked_at IS NULL AND expires_at > \$3 ORDER BY "auth_tokens"\."id" LIMIT \$4`).
		WithArgs(tokenHash, constants.AuthTokenTypeReset, sqlmock.AnyArg(), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "type", "token_hash", "expires_at", "created_at", "updated_at"}).
			AddRow(tokenID, userID, constants.AuthTokenTypeReset, tokenHash, now.Add(15*time.Minute), now, now))

	mock.ExpectQuery(`SELECT \* FROM "users" WHERE id = \$1 AND "users"\."deleted_at" IS NULL ORDER BY "users"\."id" LIMIT \$2`).
		WithArgs(userID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "full_name", "status", "created_at"}).
			AddRow(userID, "suspended@example.com", "Suspended User", constants.UserStatusSuspended, now))

	req := &dtos.ResetPasswordRequest{
		Token:       rawToken,
		NewPassword: "newSecurePassword123",
	}

	err := svc.ResetPassword(context.Background(), req)
	if err == nil {
		t.Fatal("expected error on inactive user, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestService_ResetPassword_UpdatePasswordError(t *testing.T) {
	svc, mock, cleanup := setupAuthServiceMock(t)
	defer cleanup()

	rawToken := "token-update-pass-err"
	tokenHash := hasher.HashToken(rawToken)
	tokenID := uuid.New()
	userID := uuid.New()
	now := time.Now()

	mock.ExpectQuery(`SELECT \* FROM "auth_tokens" WHERE token_hash = \$1 AND type = \$2 AND revoked_at IS NULL AND expires_at > \$3 ORDER BY "auth_tokens"\."id" LIMIT \$4`).
		WithArgs(tokenHash, constants.AuthTokenTypeReset, sqlmock.AnyArg(), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "type", "token_hash", "expires_at", "created_at", "updated_at"}).
			AddRow(tokenID, userID, constants.AuthTokenTypeReset, tokenHash, now.Add(15*time.Minute), now, now))

	mock.ExpectQuery(`SELECT \* FROM "users" WHERE id = \$1 AND "users"\."deleted_at" IS NULL ORDER BY "users"\."id" LIMIT \$2`).
		WithArgs(userID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "full_name", "status", "created_at"}).
			AddRow(userID, "user@example.com", "Test User", constants.UserStatusActive, now))

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "users" SET "password_hash"=\$1,"updated_at"=\$2 WHERE id = \$3 AND "users"\."deleted_at" IS NULL`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), userID).
		WillReturnError(errors.New("db deadlock on password update"))
	mock.ExpectRollback()

	req := &dtos.ResetPasswordRequest{
		Token:       rawToken,
		NewPassword: "newSecurePassword123",
	}

	err := svc.ResetPassword(context.Background(), req)
	if err == nil {
		t.Fatal("expected error on password update DB error, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestService_ResetPassword_RevokeTokenError(t *testing.T) {
	svc, mock, cleanup := setupAuthServiceMock(t)
	defer cleanup()

	rawToken := "token-revoke-err"
	tokenHash := hasher.HashToken(rawToken)
	tokenID := uuid.New()
	userID := uuid.New()
	now := time.Now()

	mock.ExpectQuery(`SELECT \* FROM "auth_tokens" WHERE token_hash = \$1 AND type = \$2 AND revoked_at IS NULL AND expires_at > \$3 ORDER BY "auth_tokens"\."id" LIMIT \$4`).
		WithArgs(tokenHash, constants.AuthTokenTypeReset, sqlmock.AnyArg(), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "type", "token_hash", "expires_at", "created_at", "updated_at"}).
			AddRow(tokenID, userID, constants.AuthTokenTypeReset, tokenHash, now.Add(15*time.Minute), now, now))

	mock.ExpectQuery(`SELECT \* FROM "users" WHERE id = \$1 AND "users"\."deleted_at" IS NULL ORDER BY "users"\."id" LIMIT \$2`).
		WithArgs(userID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "full_name", "status", "created_at"}).
			AddRow(userID, "user@example.com", "Test User", constants.UserStatusActive, now))

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "users" SET "password_hash"=\$1,"updated_at"=\$2 WHERE id = \$3 AND "users"\."deleted_at" IS NULL`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), userID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "auth_tokens" SET "revoked_at"=\$1,"updated_at"=\$2 WHERE id = \$3`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), tokenID).
		WillReturnError(errors.New("db error revoking reset token"))
	mock.ExpectRollback()

	req := &dtos.ResetPasswordRequest{
		Token:       rawToken,
		NewPassword: "newSecurePassword123",
	}

	err := svc.ResetPassword(context.Background(), req)
	if err == nil {
		t.Fatal("expected error on token revocation DB error, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestService_Authenticate(t *testing.T) {
	svc, _, cleanup := setupAuthServiceMock(t)
	defer cleanup()

	cfg := config.Config{
		AppName:              "youten-test",
		JWTSecret:            "test-jwt-secret-key-1234567890",
		JWTAccessExpiration:  15 * time.Minute,
		JWTRefreshExpiration: 7 * 24 * time.Hour,
	}

	userID := uuid.New()
	email := "authuser@example.com"
	sessionID := "sess-xyz"

	// 1. Valid Access Token
	tokenPair, err := jwt.GenerateTokenPair(cfg, userID, email, sessionID)
	if err != nil {
		t.Fatalf("failed to generate token pair: %v", err)
	}

	authUser, err := svc.Authenticate(context.Background(), tokenPair.AccessToken)
	if err != nil {
		t.Fatalf("expected valid authentication, got error: %v", err)
	}
	if authUser.UserID != userID || authUser.Email != email || authUser.SessionID != sessionID || authUser.IsGuest {
		t.Fatalf("unexpected authUser: %+v", authUser)
	}

	// 2. Reject Refresh Token passed as Access Token
	_, err = svc.Authenticate(context.Background(), tokenPair.RefreshToken)
	if !errors.Is(err, constants.ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken when refresh token passed, got: %v", err)
	}

	// 3. Reject invalid/malformed token string
	_, err = svc.Authenticate(context.Background(), "invalid-token-string")
	if !errors.Is(err, constants.ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken on malformed token, got: %v", err)
	}
}

func TestService_AuthenticateAdmin(t *testing.T) {
	svc, mock, cleanup := setupAuthServiceMock(t)
	defer cleanup()

	cfg := config.Config{
		AppName:             "youten-test",
		JWTSecret:           "test-jwt-secret-key-1234567890",
		JWTAccessExpiration: 15 * time.Minute,
	}

	adminID := uuid.New()
	roleID := uuid.New()
	username := "admin_bob"
	fullName := "Bob Administrator"
	roleName := "Security"
	permissions := []string{"users:read", "audit:read"}

	tokenStr, err := jwt.GenerateAdminToken(cfg, adminID, username)
	if err != nil {
		t.Fatalf("failed to generate admin token: %v", err)
	}

	// 1. Success case
	mock.ExpectQuery(`SELECT \* FROM "admin_users"`).
		WithArgs(adminID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "full_name", "role_id", "status"}).
			AddRow(adminID, username, fullName, roleID, constants.UserStatusActive))

	mock.ExpectQuery(`SELECT \* FROM "admin_roles"`).
		WithArgs(roleID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "permissions"}).
			AddRow(roleID, roleName, []byte(`["users:read", "audit:read"]`)))

	adminUser, err := svc.AuthenticateAdmin(context.Background(), tokenStr)
	if err != nil {
		t.Fatalf("expected valid admin authentication, got: %v", err)
	}
	if adminUser.AdminID != adminID || adminUser.Username != username || adminUser.RoleName != roleName {
		t.Fatalf("unexpected adminUser: %+v", adminUser)
	}
	if len(adminUser.Permissions) != len(permissions) {
		t.Fatalf("expected %d permissions, got %d", len(permissions), len(adminUser.Permissions))
	}

	// 2. Inactive admin user returns ErrUserInactive
	mock.ExpectQuery(`SELECT \* FROM "admin_users"`).
		WithArgs(adminID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "full_name", "role_id", "status"}).
			AddRow(adminID, username, fullName, roleID, "suspended"))

	mock.ExpectQuery(`SELECT \* FROM "admin_roles"`).
		WithArgs(roleID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "permissions"}).
			AddRow(roleID, roleName, []byte(`["users:read"]`)))

	_, err = svc.AuthenticateAdmin(context.Background(), tokenStr)
	if !errors.Is(err, constants.ErrUserInactive) {
		t.Fatalf("expected ErrUserInactive, got: %v", err)
	}

	// 3. Admin not found in DB returns ErrUnauthorized
	mock.ExpectQuery(`SELECT \* FROM "admin_users"`).
		WithArgs(adminID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	_, err = svc.AuthenticateAdmin(context.Background(), tokenStr)
	if !errors.Is(err, constants.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got: %v", err)
	}

	// 4. Malformed token returns ErrInvalidToken
	_, err = svc.AuthenticateAdmin(context.Background(), "invalid-token-string")
	if !errors.Is(err, constants.ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken on malformed token, got: %v", err)
	}
}

func TestService_GetProfile_Success_DefaultQuota(t *testing.T) {
	svc, mock, cleanup := setupAuthServiceMock(t)
	defer cleanup()

	userID := uuid.New()
	email := "profile@example.com"
	now := time.Now()

	// 1. FindUserByID
	mock.ExpectQuery(`SELECT \* FROM "users"`).
		WithArgs(userID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "full_name", "status", "daily_quota_override", "email_verified", "created_at"}).
			AddRow(userID, email, "Jane Doe", constants.UserStatusActive, nil, true, now))

	// 2. CountUserRecordingsToday
	mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings"`).
		WithArgs(userID, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	resp, err := svc.GetProfile(context.Background(), userID)
	if err != nil {
		t.Fatalf("expected successful profile retrieval, got: %v", err)
	}
	if resp.ID != userID || resp.Email != email || resp.FullName != "Jane Doe" {
		t.Fatalf("unexpected user info: %+v", resp)
	}
	if resp.DailyQuota != constants.DefaultUserDailyQuota {
		t.Errorf("expected daily quota %d, got %d", constants.DefaultUserDailyQuota, resp.DailyQuota)
	}
	if resp.QuotaUsedToday != 2 {
		t.Errorf("expected quota used today 2, got %d", resp.QuotaUsedToday)
	}
	if resp.QuotaRemaining != constants.DefaultUserDailyQuota-2 {
		t.Errorf("expected quota remaining %d, got %d", constants.DefaultUserDailyQuota-2, resp.QuotaRemaining)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %s", err)
	}
}

func TestService_GetProfile_Success_QuotaOverride_Exceeded(t *testing.T) {
	svc, mock, cleanup := setupAuthServiceMock(t)
	defer cleanup()

	userID := uuid.New()
	email := "power@example.com"
	now := time.Now()
	override := 10

	// 1. FindUserByID with override = 10
	mock.ExpectQuery(`SELECT \* FROM "users"`).
		WithArgs(userID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "full_name", "status", "daily_quota_override", "email_verified", "created_at"}).
			AddRow(userID, email, "Power User", constants.UserStatusActive, override, false, now))

	// 2. CountUserRecordingsToday returns 12 (exceeded)
	mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings"`).
		WithArgs(userID, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(12))

	resp, err := svc.GetProfile(context.Background(), userID)
	if err != nil {
		t.Fatalf("expected successful profile retrieval, got: %v", err)
	}
	if resp.DailyQuota != 10 {
		t.Errorf("expected daily quota 10, got %d", resp.DailyQuota)
	}
	if resp.QuotaUsedToday != 12 {
		t.Errorf("expected quota used today 12, got %d", resp.QuotaUsedToday)
	}
	if resp.QuotaRemaining != 0 {
		t.Errorf("expected quota remaining 0 when exceeded, got %d", resp.QuotaRemaining)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %s", err)
	}
}

func TestService_GetProfile_Errors(t *testing.T) {
	svc, mock, cleanup := setupAuthServiceMock(t)
	defer cleanup()

	// 1. User not found
	unknownID := uuid.New()
	mock.ExpectQuery(`SELECT \* FROM "users"`).
		WithArgs(unknownID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	_, err := svc.GetProfile(context.Background(), unknownID)
	if !errors.Is(err, constants.ErrUserNotFound) {
		t.Fatalf("expected ErrUserNotFound, got: %v", err)
	}

	// 2. DB error on count
	existingID := uuid.New()
	mock.ExpectQuery(`SELECT \* FROM "users"`).
		WithArgs(existingID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "full_name", "status", "created_at"}).
			AddRow(existingID, "user@example.com", "User", constants.UserStatusActive, time.Now()))

	mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings"`).
		WithArgs(existingID, sqlmock.AnyArg()).
		WillReturnError(errors.New("db connection failure"))

	_, err = svc.GetProfile(context.Background(), existingID)
	if err == nil {
		t.Fatal("expected error on count failure, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %s", err)
	}
}

func TestService_UpdateProfile_Success(t *testing.T) {
	svc, mock, cleanup := setupAuthServiceMock(t)
	defer cleanup()

	userID := uuid.New()
	email := "updated@example.com"
	now := time.Now()

	// 1. UpdateUserFullName
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "users" SET "full_name"=\$1,"updated_at"=\$2 WHERE id = \$3 AND "users"\."deleted_at" IS NULL`).
		WithArgs("John Wick", sqlmock.AnyArg(), userID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	// 2. GetProfile -> FindUserByID
	mock.ExpectQuery(`SELECT \* FROM "users"`).
		WithArgs(userID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "full_name", "status", "daily_quota_override", "email_verified", "created_at"}).
			AddRow(userID, email, "John Wick", constants.UserStatusActive, nil, true, now))

	// 3. GetProfile -> CountUserRecordingsToday
	mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings"`).
		WithArgs(userID, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	resp, err := svc.UpdateProfile(context.Background(), userID, &dtos.UpdateProfileRequest{
		FullName: "  John Wick  ",
	})
	if err != nil {
		t.Fatalf("expected successful profile update, got error: %v", err)
	}

	if resp.FullName != "John Wick" {
		t.Errorf("expected updated full name 'John Wick', got %q", resp.FullName)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %s", err)
	}
}

func TestService_UpdateProfile_Errors(t *testing.T) {
	svc, mock, cleanup := setupAuthServiceMock(t)
	defer cleanup()

	userID := uuid.New()

	// 1. User not found (0 rows affected)
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "users" SET "full_name"=\$1,"updated_at"=\$2 WHERE id = \$3 AND "users"\."deleted_at" IS NULL`).
		WithArgs("Ghost", sqlmock.AnyArg(), userID).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	_, err := svc.UpdateProfile(context.Background(), userID, &dtos.UpdateProfileRequest{FullName: "Ghost"})
	if !errors.Is(err, constants.ErrUserNotFound) {
		t.Fatalf("expected ErrUserNotFound on 0 rows affected, got: %v", err)
	}

	// 2. DB error
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "users" SET "full_name"=\$1,"updated_at"=\$2 WHERE id = \$3 AND "users"\."deleted_at" IS NULL`).
		WithArgs("Failure", sqlmock.AnyArg(), userID).
		WillReturnError(errors.New("db update error"))
	mock.ExpectRollback()

	_, err = svc.UpdateProfile(context.Background(), userID, &dtos.UpdateProfileRequest{FullName: "Failure"})
	if err == nil {
		t.Fatal("expected DB error on update failure, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %s", err)
	}
}

func TestService_ChangePassword_Success(t *testing.T) {
	svc, mock, cleanup := setupAuthServiceMock(t)
	defer cleanup()

	userID := uuid.New()
	email := "change@example.com"
	now := time.Now()
	oldPassword := "OldPassword123"
	oldHash, err := hasher.HashPassword(oldPassword)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}

	// 1. FindUserByID
	mock.ExpectQuery(`SELECT \* FROM "users"`).
		WithArgs(userID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "password_hash", "status", "created_at"}).
			AddRow(userID, email, oldHash, constants.UserStatusActive, now))

	// 2. UpdateUserPassword
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "users" SET "password_hash"=\$1,"updated_at"=\$2 WHERE id = \$3 AND "users"\."deleted_at" IS NULL`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), userID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	// 3. RevokeAllAuthTokensByUserID
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "auth_tokens" SET "revoked_at"=\$1,"updated_at"=\$2 WHERE \(user_id = \$3 AND revoked_at IS NULL\) AND type = \$4`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), userID, constants.AuthTokenTypeRefresh).
		WillReturnResult(sqlmock.NewResult(2, 2))
	mock.ExpectCommit()

	err = svc.ChangePassword(context.Background(), userID, &dtos.ChangePasswordRequest{
		OldPassword: oldPassword,
		NewPassword: "BrandNewPassword456",
	})
	if err != nil {
		t.Fatalf("expected successful password change, got: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %s", err)
	}
}

func TestService_ChangePassword_Errors(t *testing.T) {
	svc, mock, cleanup := setupAuthServiceMock(t)
	defer cleanup()

	userID := uuid.New()
	now := time.Now()
	correctPassword := "CorrectPassword123"
	correctHash, _ := hasher.HashPassword(correctPassword)

	// 1. User not found
	mock.ExpectQuery(`SELECT \* FROM "users"`).
		WithArgs(userID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	err := svc.ChangePassword(context.Background(), userID, &dtos.ChangePasswordRequest{
		OldPassword: correctPassword,
		NewPassword: "NewPassword123",
	})
	if !errors.Is(err, constants.ErrUserNotFound) {
		t.Fatalf("expected ErrUserNotFound, got: %v", err)
	}

	// 2. Inactive/Suspended user
	mock.ExpectQuery(`SELECT \* FROM "users"`).
		WithArgs(userID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "password_hash", "status", "created_at"}).
			AddRow(userID, "user@example.com", correctHash, constants.UserStatusSuspended, now))

	err = svc.ChangePassword(context.Background(), userID, &dtos.ChangePasswordRequest{
		OldPassword: correctPassword,
		NewPassword: "NewPassword123",
	})
	if !errors.Is(err, constants.ErrUserInactive) {
		t.Fatalf("expected ErrUserInactive, got: %v", err)
	}

	// 3. Incorrect old password
	mock.ExpectQuery(`SELECT \* FROM "users"`).
		WithArgs(userID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "password_hash", "status", "created_at"}).
			AddRow(userID, "user@example.com", correctHash, constants.UserStatusActive, now))

	err = svc.ChangePassword(context.Background(), userID, &dtos.ChangePasswordRequest{
		OldPassword: "WrongOldPassword999",
		NewPassword: "NewPassword123",
	})
	if err == nil {
		t.Fatal("expected error on incorrect old password, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %s", err)
	}
}
