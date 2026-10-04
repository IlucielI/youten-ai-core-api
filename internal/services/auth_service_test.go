package services_test

import (
	"context"
	"errors"
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

func TestService_Register_NilService(t *testing.T) {
	var svc *services.Service
	resp, err := svc.Register(context.Background(), &dtos.RegisterRequest{})
	if err == nil {
		t.Fatal("expected error on nil service, got nil")
	}
	if resp != nil {
		t.Errorf("expected nil response, got %v", resp)
	}
}

func TestService_Register_NilRequest(t *testing.T) {
	svc, _, cleanup := setupAuthServiceMock(t)
	defer cleanup()

	resp, err := svc.Register(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error on nil request, got nil")
	}
	if resp != nil {
		t.Errorf("expected nil response, got %v", resp)
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

func TestService_Login_NilService(t *testing.T) {
	var svc *services.Service
	resp, err := svc.Login(context.Background(), &dtos.LoginRequest{Email: "a@b.com", Password: "p"})
	if err == nil {
		t.Fatal("expected error on nil service, got nil")
	}
	if resp != nil {
		t.Errorf("expected nil response, got %v", resp)
	}
}

func TestService_Login_NilRequest(t *testing.T) {
	svc, _, cleanup := setupAuthServiceMock(t)
	defer cleanup()

	resp, err := svc.Login(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error on nil request, got nil")
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

	// 3. CreateAuthToken (insert new refresh token)
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "auth_tokens"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(uuid.New(), now, now))
	mock.ExpectCommit()

	// 4. RevokeAuthToken
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "auth_tokens" SET "revoked_at"=\$1,"updated_at"=\$2 WHERE id = \$3`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), tokenID).
		WillReturnResult(sqlmock.NewResult(1, 1))
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

func TestService_RefreshToken_NilChecks(t *testing.T) {
	var nilSvc *services.Service
	resp, err := nilSvc.RefreshToken(context.Background(), &dtos.RefreshTokenRequest{RefreshToken: "t"})
	if err == nil {
		t.Fatal("expected error on nil service, got nil")
	}
	if resp != nil {
		t.Errorf("expected nil response, got %v", resp)
	}

	svc, _, cleanup := setupAuthServiceMock(t)
	defer cleanup()

	resp, err = svc.RefreshToken(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error on nil request, got nil")
	}
	if resp != nil {
		t.Errorf("expected nil response, got %v", resp)
	}
}
