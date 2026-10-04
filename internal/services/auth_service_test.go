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
		t.Fatalf("failed to initialize gorm: %v", err)
	}

	repo := repositories.New(gormDB)
	svc := services.New(config.Config{}, repo, nil)

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
