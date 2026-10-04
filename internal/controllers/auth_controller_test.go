package controllers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
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

func setupTestControllers(t *testing.T) (*Controllers, sqlmock.Sqlmock, func()) {
	gin.SetMode(gin.TestMode)

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
	ctrls := New(cfg, svc)

	cleanup := func() {
		sqlDB.Close()
	}

	return ctrls, mock, cleanup
}

func TestControllers_Register_Success(t *testing.T) {
	ctrls, mock, cleanup := setupTestControllers(t)
	defer cleanup()

	email := "alex@example.com"
	reqPayload := dtos.RegisterRequest{
		Email:    email,
		Password: "SecurePassword123!",
		FullName: "Alex Mercer",
	}
	body, _ := json.Marshal(reqPayload)

	// Mock DB expectation: 1. check existing email (returns 0 rows)
	mock.ExpectQuery(`SELECT \* FROM "users" WHERE \(email = \$1 AND deleted_at IS NULL\) AND "users"\."deleted_at" IS NULL ORDER BY "users"\."id" LIMIT \$2`).
		WithArgs(email, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	// Mock DB expectation: 2. insert new user
	newID := uuid.New()
	now := time.Now()
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "users"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(newID, now, now))
	mock.ExpectCommit()

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/auth/register", bytes.NewBuffer(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	ctrls.Register(ctx)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d. Body: %s", w.Code, w.Body.String())
	}

	var resp dtos.APIResponse[*dtos.UserResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response body: %v", err)
	}

	if resp.Status != constants.ResponseStatusSuccess {
		t.Errorf("expected status 'success', got '%s'", resp.Status)
	}
	if resp.Data == nil {
		t.Fatal("expected data to be non-nil")
	}
	if resp.Data.Email != email {
		t.Errorf("expected email '%s', got '%s'", email, resp.Data.Email)
	}
	if resp.Data.FullName != "Alex Mercer" {
		t.Errorf("expected full_name 'Alex Mercer', got '%s'", resp.Data.FullName)
	}
	if resp.Data.DailyQuota != constants.DefaultUserDailyQuota {
		t.Errorf("expected daily quota %d, got %d", constants.DefaultUserDailyQuota, resp.Data.DailyQuota)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestControllers_Register_InvalidJSON(t *testing.T) {
	ctrls, _, cleanup := setupTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/auth/register", bytes.NewBufferString("{invalid-json"))
	ctx.Request.Header.Set("Content-Type", "application/json")

	ctrls.Register(ctx)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}

	var resp dtos.BaseResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Code != constants.ResponseCodeBadRequest {
		t.Errorf("expected code BAD_REQUEST, got %s", resp.Code)
	}
}

func TestControllers_Register_ValidationError(t *testing.T) {
	ctrls, _, cleanup := setupTestControllers(t)
	defer cleanup()

	reqPayload := dtos.RegisterRequest{
		Email:    "not-an-email",
		Password: "short",
		FullName: "",
	}
	body, _ := json.Marshal(reqPayload)

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/auth/register", bytes.NewBuffer(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	ctrls.Register(ctx)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}

	var resp dtos.BaseResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Code != constants.ResponseCodeBadRequest {
		t.Errorf("expected code BAD_REQUEST, got %s", resp.Code)
	}
}

func TestControllers_Register_DuplicateEmail(t *testing.T) {
	ctrls, mock, cleanup := setupTestControllers(t)
	defer cleanup()

	email := "existing@example.com"
	reqPayload := dtos.RegisterRequest{
		Email:    email,
		Password: "Password123!",
		FullName: "Existing Person",
	}
	body, _ := json.Marshal(reqPayload)

	// Mock DB query finding existing user
	mock.ExpectQuery(`SELECT \* FROM "users"`).
		WithArgs(email, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email"}).AddRow(uuid.New(), email))

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/auth/register", bytes.NewBuffer(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	ctrls.Register(ctx)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d. Body: %s", w.Code, w.Body.String())
	}

	var resp dtos.BaseResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Code != "ERR_EMAIL_ALREADY_EXISTS" {
		t.Errorf("expected code ERR_EMAIL_ALREADY_EXISTS, got %s", resp.Code)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestControllers_Login_Success(t *testing.T) {
	ctrls, mock, cleanup := setupTestControllers(t)
	defer cleanup()

	email := "alex@example.com"
	password := "Password123!"
	hashedPassword, err := hasher.HashPassword(password)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}

	userID := uuid.New()
	now := time.Now()

	// 1. FindUserByEmail
	mock.ExpectQuery(`SELECT \* FROM "users" WHERE \(email = \$1 AND deleted_at IS NULL\) AND "users"\."deleted_at" IS NULL ORDER BY "users"\."id" LIMIT \$2`).
		WithArgs(email, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "password_hash", "full_name", "status", "created_at"}).
			AddRow(userID, email, hashedPassword, "Alex Mercer", constants.UserStatusActive, now))

	// 2. CreateAuthToken
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "auth_tokens"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(uuid.New(), now, now))
	mock.ExpectCommit()

	reqPayload := dtos.LoginRequest{
		Email:    email,
		Password: password,
	}
	body, _ := json.Marshal(reqPayload)

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/auth/login", bytes.NewBuffer(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	ctrls.Login(ctx)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	var resp dtos.APIResponse[*dtos.AuthResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Data == nil {
		t.Fatal("expected non-nil auth data")
	}
	if resp.Data.AccessToken == "" || resp.Data.RefreshToken == "" {
		t.Error("expected non-empty access and refresh tokens")
	}
	if resp.Data.User.Email != email {
		t.Errorf("expected user email %s, got %s", email, resp.Data.User.Email)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestControllers_Login_InvalidJSON(t *testing.T) {
	ctrls, _, cleanup := setupTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/auth/login", bytes.NewBufferString("{invalid-json"))
	ctx.Request.Header.Set("Content-Type", "application/json")

	ctrls.Login(ctx)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}

	var resp dtos.BaseResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Code != constants.ResponseCodeBadRequest {
		t.Errorf("expected code BAD_REQUEST, got %s", resp.Code)
	}
}

func TestControllers_Login_ValidationError(t *testing.T) {
	ctrls, _, cleanup := setupTestControllers(t)
	defer cleanup()

	reqPayload := dtos.LoginRequest{
		Email:    "invalid-email",
		Password: "",
	}
	body, _ := json.Marshal(reqPayload)

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/auth/login", bytes.NewBuffer(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	ctrls.Login(ctx)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}

	var resp dtos.BaseResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Code != constants.ResponseCodeBadRequest {
		t.Errorf("expected code BAD_REQUEST, got %s", resp.Code)
	}
}

func TestControllers_Login_InvalidCredentials(t *testing.T) {
	ctrls, mock, cleanup := setupTestControllers(t)
	defer cleanup()

	email := "alex@example.com"
	mock.ExpectQuery(`SELECT \* FROM "users" WHERE \(email = \$1 AND deleted_at IS NULL\) AND "users"\."deleted_at" IS NULL ORDER BY "users"\."id" LIMIT \$2`).
		WithArgs(email, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	reqPayload := dtos.LoginRequest{
		Email:    email,
		Password: "WrongPassword123!",
	}
	body, _ := json.Marshal(reqPayload)

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/auth/login", bytes.NewBuffer(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	ctrls.Login(ctx)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", w.Code)
	}

	var resp dtos.BaseResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Code != constants.ResponseCodeUnauthorized {
		t.Errorf("expected code UNAUTHORIZED, got %s", resp.Code)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestControllers_RefreshToken_Success(t *testing.T) {
	ctrls, mock, cleanup := setupTestControllers(t)
	defer cleanup()

	userID := uuid.New()
	email := "alex@example.com"
	now := time.Now()

	cfg := config.Config{
		AppName:              "youten-test",
		JWTSecret:            "test-jwt-secret-key-1234567890",
		JWTAccessExpiration:  15 * time.Minute,
		JWTRefreshExpiration: 7 * 24 * time.Hour,
	}

	tokenPair, err := jwt.GenerateTokenPair(cfg, userID, email, "session-old")
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	tokenHash := hasher.HashToken(tokenPair.RefreshToken)
	tokenID := uuid.New()

	// 1. FindAuthTokenByHashAndType
	mock.ExpectQuery(`SELECT \* FROM "auth_tokens"`).
		WithArgs(tokenHash, constants.AuthTokenTypeRefresh, sqlmock.AnyArg(), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "type", "token_hash", "expires_at", "created_at", "updated_at"}).
			AddRow(tokenID, userID, constants.AuthTokenTypeRefresh, tokenHash, now.Add(24*time.Hour), now, now))

	// 2. FindUserByID
	mock.ExpectQuery(`SELECT \* FROM "users"`).
		WithArgs(userID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "full_name", "status", "created_at"}).
			AddRow(userID, email, "Alex Mercer", constants.UserStatusActive, now))

	// 3. RotateAuthToken (Transaction: revoke old token + insert new token)
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "auth_tokens"`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), tokenID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(`INSERT INTO "auth_tokens"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(uuid.New(), now, now))
	mock.ExpectCommit()

	reqPayload := dtos.RefreshTokenRequest{
		RefreshToken: tokenPair.RefreshToken,
	}
	body, _ := json.Marshal(reqPayload)

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/auth/refresh", bytes.NewBuffer(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	ctrls.RefreshToken(ctx)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	var resp dtos.APIResponse[*dtos.AuthResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Data == nil {
		t.Fatal("expected non-nil auth response")
	}
	if resp.Data.AccessToken == "" || resp.Data.RefreshToken == "" {
		t.Error("expected non-empty tokens")
	}
	if resp.Data.RefreshToken == reqPayload.RefreshToken {
		t.Error("expected fresh rotated refresh token, got identical token")
	}
	if resp.Data.User.Email != email {
		t.Errorf("expected user email %s, got %s", email, resp.Data.User.Email)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestControllers_RefreshToken_InvalidJSON(t *testing.T) {
	ctrls, _, cleanup := setupTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/auth/refresh", bytes.NewBufferString("{invalid-json"))
	ctx.Request.Header.Set("Content-Type", "application/json")

	ctrls.RefreshToken(ctx)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}
}

func TestControllers_RefreshToken_ValidationError(t *testing.T) {
	ctrls, _, cleanup := setupTestControllers(t)
	defer cleanup()

	reqPayload := dtos.RefreshTokenRequest{
		RefreshToken: "",
	}
	body, _ := json.Marshal(reqPayload)

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/auth/refresh", bytes.NewBuffer(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	ctrls.RefreshToken(ctx)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}
}

func TestControllers_RefreshToken_InvalidToken(t *testing.T) {
	ctrls, _, cleanup := setupTestControllers(t)
	defer cleanup()

	reqPayload := dtos.RefreshTokenRequest{
		RefreshToken: "malformed.jwt.token",
	}
	body, _ := json.Marshal(reqPayload)

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/auth/refresh", bytes.NewBuffer(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	ctrls.RefreshToken(ctx)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", w.Code)
	}
}

func TestControllers_Logout_Success(t *testing.T) {
	ctrls, mock, cleanup := setupTestControllers(t)
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

	reqPayload := dtos.LogoutRequest{
		RefreshToken: refreshToken,
	}
	body, _ := json.Marshal(reqPayload)

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/auth/logout", bytes.NewBuffer(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	ctrls.Logout(ctx)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp dtos.BaseResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Code != constants.ResponseCodeSuccess {
		t.Errorf("expected code SUCCESS, got %s", resp.Code)
	}
	if resp.Message != "Logout successful" {
		t.Errorf("expected message 'Logout successful', got %s", resp.Message)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestControllers_Logout_InvalidJSON(t *testing.T) {
	ctrls, _, cleanup := setupTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/auth/logout", bytes.NewBufferString("{invalid-json"))
	ctx.Request.Header.Set("Content-Type", "application/json")

	ctrls.Logout(ctx)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}
}

func TestControllers_Logout_ValidationError(t *testing.T) {
	ctrls, _, cleanup := setupTestControllers(t)
	defer cleanup()

	reqPayload := dtos.LogoutRequest{
		RefreshToken: "",
	}
	body, _ := json.Marshal(reqPayload)

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/auth/logout", bytes.NewBuffer(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	ctrls.Logout(ctx)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}
}

func TestControllers_Logout_Idempotent_NotFound(t *testing.T) {
	ctrls, mock, cleanup := setupTestControllers(t)
	defer cleanup()

	refreshToken := "nonexistent.refresh.token"
	tokenHash := hasher.HashToken(refreshToken)

	// Mock token not found in DB -> idempotent return
	mock.ExpectQuery(`SELECT \* FROM "auth_tokens" WHERE token_hash = \$1 AND type = \$2 AND revoked_at IS NULL AND expires_at > \$3 ORDER BY "auth_tokens"\."id" LIMIT \$4`).
		WithArgs(tokenHash, constants.AuthTokenTypeRefresh, sqlmock.AnyArg(), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	reqPayload := dtos.LogoutRequest{
		RefreshToken: refreshToken,
	}
	body, _ := json.Marshal(reqPayload)

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/auth/logout", bytes.NewBuffer(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	ctrls.Logout(ctx)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 on idempotent logout, got %d", w.Code)
	}

	var resp dtos.BaseResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Code != constants.ResponseCodeSuccess {
		t.Errorf("expected code SUCCESS, got %s", resp.Code)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestControllers_ForgotPassword_Success(t *testing.T) {
	ctrls, mock, cleanup := setupTestControllers(t)
	defer cleanup()

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

	reqPayload := dtos.ForgotPasswordRequest{
		Email: email,
	}
	body, _ := json.Marshal(reqPayload)

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/auth/forgot-password", bytes.NewBuffer(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	ctrls.ForgotPassword(ctx)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp dtos.BaseResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Code != constants.ResponseCodeSuccess {
		t.Errorf("expected code SUCCESS, got %s", resp.Code)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestControllers_ForgotPassword_UserNotFound(t *testing.T) {
	ctrls, mock, cleanup := setupTestControllers(t)
	defer cleanup()

	email := "unknown@example.com"

	mock.ExpectQuery(`SELECT \* FROM "users" WHERE \(email = \$1 AND deleted_at IS NULL\) AND "users"\."deleted_at" IS NULL ORDER BY "users"\."id" LIMIT \$2`).
		WithArgs(email, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	reqPayload := dtos.ForgotPasswordRequest{
		Email: email,
	}
	body, _ := json.Marshal(reqPayload)

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/auth/forgot-password", bytes.NewBuffer(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	ctrls.ForgotPassword(ctx)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 on non-existent email, got %d", w.Code)
	}

	var resp dtos.BaseResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Code != constants.ResponseCodeSuccess {
		t.Errorf("expected code SUCCESS, got %s", resp.Code)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestControllers_ForgotPassword_InvalidJSON(t *testing.T) {
	ctrls, _, cleanup := setupTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/auth/forgot-password", bytes.NewBufferString("{invalid-json"))
	ctx.Request.Header.Set("Content-Type", "application/json")

	ctrls.ForgotPassword(ctx)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}
}

func TestControllers_ForgotPassword_ValidationError(t *testing.T) {
	ctrls, _, cleanup := setupTestControllers(t)
	defer cleanup()

	reqPayload := dtos.ForgotPasswordRequest{
		Email: "not-an-email",
	}
	body, _ := json.Marshal(reqPayload)

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/auth/forgot-password", bytes.NewBuffer(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	ctrls.ForgotPassword(ctx)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}
}

func TestControllers_ForgotPassword_ServiceError(t *testing.T) {
	ctrls, mock, cleanup := setupTestControllers(t)
	defer cleanup()

	email := "dberr@example.com"

	mock.ExpectQuery(`SELECT \* FROM "users" WHERE \(email = \$1 AND deleted_at IS NULL\) AND "users"\."deleted_at" IS NULL ORDER BY "users"\."id" LIMIT \$2`).
		WithArgs(email, 1).
		WillReturnError(errors.New("db query failure"))

	reqPayload := dtos.ForgotPasswordRequest{
		Email: email,
	}
	body, _ := json.Marshal(reqPayload)

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/auth/forgot-password", bytes.NewBuffer(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	ctrls.ForgotPassword(ctx)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", w.Code)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}


