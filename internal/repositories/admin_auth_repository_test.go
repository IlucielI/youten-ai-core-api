package repositories

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	gormPostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/models"
)

func setupTestAdminRepo(t *testing.T) (*Repositories, sqlmock.Sqlmock, func()) {
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

	repo := New(gormDB)
	cleanup := func() {
		sqlDB.Close()
	}

	return repo, mock, cleanup
}

func TestRepositories_AdminPasswordAndTokens(t *testing.T) {
	ctx := context.Background()

	t.Run("FindAdminByEmail success", func(t *testing.T) {
		repo, mock, cleanup := setupTestAdminRepo(t)
		defer cleanup()

		adminID := uuid.New()
		roleID := uuid.New()

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "admin_users" WHERE (LOWER(email) = LOWER($1) AND deleted_at IS NULL) AND "admin_users"."deleted_at" IS NULL ORDER BY "admin_users"."id" LIMIT $2`)).
			WithArgs("admin@youten.ai", 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "username", "email", "full_name", "role_id", "status"}).
				AddRow(adminID, "admin", "admin@youten.ai", "Platform Admin", roleID, "active"))

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "admin_roles" WHERE "admin_roles"."id" = $1`)).
			WithArgs(roleID).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name", "permissions"}).
				AddRow(roleID, "Super Admin", []byte(`["*"]`)))

		admin, err := repo.FindAdminByEmail(ctx, "admin@youten.ai")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if admin.Email != "admin@youten.ai" {
			t.Errorf("expected admin@youten.ai, got %s", admin.Email)
		}
	})

	t.Run("UpdateAdminPassword success", func(t *testing.T) {
		repo, mock, cleanup := setupTestAdminRepo(t)
		defer cleanup()

		adminID := uuid.New()

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "admin_users" SET "password_hash"=$1,"updated_at"=$2 WHERE id = $3 AND "admin_users"."deleted_at" IS NULL`)).
			WithArgs("new-hash", sqlmock.AnyArg(), adminID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := repo.UpdateAdminPassword(ctx, adminID, "new-hash")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("CreateAdminAuthToken success", func(t *testing.T) {
		repo, mock, cleanup := setupTestAdminRepo(t)
		defer cleanup()

		tokenID := uuid.New()
		adminID := uuid.New()
		token := &models.AdminAuthToken{
			ID:        tokenID,
			AdminID:   adminID,
			Type:      constants.AuthTokenTypeReset,
			TokenHash: "some-hash",
			ExpiresAt: time.Now().Add(15 * time.Minute),
		}

		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "admin_auth_tokens"`)).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).
				AddRow(tokenID, time.Now(), time.Now()))
		mock.ExpectCommit()

		err := repo.CreateAdminAuthToken(ctx, token)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("FindAdminAuthTokenByHashAndType success", func(t *testing.T) {
		repo, mock, cleanup := setupTestAdminRepo(t)
		defer cleanup()

		tokenID := uuid.New()
		adminID := uuid.New()

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "admin_auth_tokens" WHERE token_hash = $1 AND type = $2 AND revoked_at IS NULL AND expires_at > $3 ORDER BY "admin_auth_tokens"."id" LIMIT $4`)).
			WithArgs("some-hash", constants.AuthTokenTypeReset, sqlmock.AnyArg(), 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "admin_id", "type", "token_hash", "expires_at", "revoked_at"}).
				AddRow(tokenID, adminID, constants.AuthTokenTypeReset, "some-hash", time.Now().Add(10*time.Minute), nil))

		token, err := repo.FindAdminAuthTokenByHashAndType(ctx, "some-hash", constants.AuthTokenTypeReset)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if token.ID != tokenID {
			t.Errorf("expected token ID %v, got %v", tokenID, token.ID)
		}
	})

	t.Run("RevokeAllAdminAuthTokens success", func(t *testing.T) {
		repo, mock, cleanup := setupTestAdminRepo(t)
		defer cleanup()

		adminID := uuid.New()

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "admin_auth_tokens" SET "revoked_at"=$1,"updated_at"=$2 WHERE admin_id = $3 AND type = $4 AND revoked_at IS NULL`)).
			WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), adminID, constants.AuthTokenTypeReset).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := repo.RevokeAllAdminAuthTokens(ctx, adminID, constants.AuthTokenTypeReset)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("RevokeAdminAuthToken success", func(t *testing.T) {
		repo, mock, cleanup := setupTestAdminRepo(t)
		defer cleanup()

		tokenID := uuid.New()

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "admin_auth_tokens" SET "revoked_at"=$1,"updated_at"=$2 WHERE id = $3`)).
			WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), tokenID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := repo.RevokeAdminAuthToken(ctx, tokenID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
