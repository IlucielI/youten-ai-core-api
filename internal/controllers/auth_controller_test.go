package controllers

import (
	"bytes"
	"encoding/json"
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
