package repositories

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-redis/redismock/v9"
	gormPostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"

	"code-base-golang/internal/adapters/redis"
)

func TestRepositories_Accessors(t *testing.T) {
	sqlDB, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer sqlDB.Close()

	gormDB, err := gorm.Open(gormPostgres.New(gormPostgres.Config{
		Conn: sqlDB,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to initialize gorm: %v", err)
	}

	repo := New(gormDB)

	if repo.DB() == nil {
		t.Error("expected non-nil DB")
	}

	var nilRepo *Repositories
	if nilRepo.DB() != nil {
		t.Error("expected nil DB for nil repo")
	}
	if nilRepo.Redis() != nil {
		t.Error("expected nil Redis for nil repo")
	}
}

func TestRepositories_PingDB(t *testing.T) {
	// 1. Nil repo
	var nilRepo *Repositories
	if err := nilRepo.PingDB(context.Background()); err == nil {
		t.Fatal("expected error on nil repo, got nil")
	}

	// 2. Nil db
	emptyRepo := New(nil)
	if err := emptyRepo.PingDB(context.Background()); err == nil {
		t.Fatal("expected error on nil db, got nil")
	}

	// 3. Valid sqlmock db
	sqlDB, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer sqlDB.Close()

	mock.ExpectPing() // for gorm.Open

	gormDB, err := gorm.Open(gormPostgres.New(gormPostgres.Config{
		Conn: sqlDB,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to initialize gorm: %v", err)
	}

	repo := New(gormDB)

	mock.ExpectPing() // for repo.PingDB
	if err := repo.PingDB(context.Background()); err != nil {
		t.Fatalf("unexpected error pinging db: %v", err)
	}

	mock.ExpectPing().WillReturnError(errors.New("db unreachable"))
	if err := repo.PingDB(context.Background()); err == nil {
		t.Fatal("expected error pinging broken db, got nil")
	}
}

func TestRepositories_PingRedis(t *testing.T) {
	// 1. Nil repo
	var nilRepo *Repositories
	if err := nilRepo.PingRedis(context.Background()); err == nil {
		t.Fatal("expected error on nil repo, got nil")
	}

	// 2. Nil redis
	emptyRepo := New(nil)
	if err := emptyRepo.PingRedis(context.Background()); err == nil {
		t.Fatal("expected error on nil redis, got nil")
	}

	// 3. Valid redismock
	rClient, rMock := redismock.NewClientMock()
	rdb := redis.NewWithClient(rClient)
	repo := New(nil, rdb)

	rMock.ExpectPing().SetVal("PONG")
	if err := repo.PingRedis(context.Background()); err != nil {
		t.Fatalf("unexpected error pinging redis: %v", err)
	}

	rMock.ExpectPing().SetErr(errors.New("redis timeout"))
	if err := repo.PingRedis(context.Background()); err == nil {
		t.Fatal("expected error on redis timeout, got nil")
	}
}
