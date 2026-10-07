package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"code-base-golang/internal/config"
	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
	"code-base-golang/internal/repositories"
)

func setupAdminServiceMock(t *testing.T) (*Service, sqlmock.Sqlmock, func()) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}

	gormDB, err := gorm.Open(postgres.New(postgres.Config{
		Conn: sqlDB,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open gorm: %v", err)
	}

	repo := repositories.New(gormDB, nil)
	cfg := config.Config{
		AppName: "youten-admin-test",
	}

	svc := New(cfg, repo, nil)

	cleanup := func() {
		sqlDB.Close()
	}

	return svc, mock, cleanup
}

func TestService_AdminListUsers(t *testing.T) {
	t.Run("success with pagination and metrics", func(t *testing.T) {
		svc, mock, cleanup := setupAdminServiceMock(t)
		defer cleanup()

		userID1 := uuid.New()
		userID2 := uuid.New()
		now := time.Now()
		overrideQuota := 20

		// 1. Count query
		mock.ExpectQuery(`SELECT count\(\*\) FROM "users"`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

		// 2. Select users query
		mock.ExpectQuery(`SELECT \* FROM "users"`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "email", "full_name", "status", "daily_quota_override", "email_verified", "created_at"}).
				AddRow(userID1, "alice@example.com", "Alice Admin", constants.UserStatusActive, nil, true, now).
				AddRow(userID2, "bob@example.com", "Bob Worker", constants.UserStatusActive, overrideQuota, false, now))

		// 3. CountUserRecordingsToday for user 1
		mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings"`).
			WithArgs(userID1, sqlmock.AnyArg()).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))

		// 4. CountUserRecordingsToday for user 2
		mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings"`).
			WithArgs(userID2, sqlmock.AnyArg()).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(5))

		resp, err := svc.AdminListUsers(context.Background(), dtos.AdminUserListQuery{
			Search: "example",
			Status: "active",
			Page:   1,
			Limit:  10,
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp == nil {
			t.Fatal("expected non-nil response")
		}
		if len(resp.Items) != 2 {
			t.Fatalf("expected 2 items, got %d", len(resp.Items))
		}

		// User 1 assertions (default quota 5, used 3)
		if resp.Items[0].DailyQuota != constants.DefaultUserDailyQuota {
			t.Errorf("expected daily quota %d, got %d", constants.DefaultUserDailyQuota, resp.Items[0].DailyQuota)
		}
		if resp.Items[0].QuotaUsedToday != 3 {
			t.Errorf("expected quota used 3, got %d", resp.Items[0].QuotaUsedToday)
		}

		// User 2 assertions (override quota 20, used 5)
		if resp.Items[1].DailyQuota != 20 {
			t.Errorf("expected daily quota 20, got %d", resp.Items[1].DailyQuota)
		}
		if resp.Items[1].QuotaUsedToday != 5 {
			t.Errorf("expected quota used 5, got %d", resp.Items[1].QuotaUsedToday)
		}

		if resp.Pagination.TotalItems != 2 || resp.Pagination.TotalPages != 1 {
			t.Errorf("unexpected pagination: %+v", resp.Pagination)
		}
	})

	t.Run("database error on count returns wrapped error", func(t *testing.T) {
		svc, mock, cleanup := setupAdminServiceMock(t)
		defer cleanup()

		mock.ExpectQuery(`SELECT count\(\*\) FROM "users"`).
			WillReturnError(errors.New("db count connection failed"))

		_, err := svc.AdminListUsers(context.Background(), dtos.AdminUserListQuery{
			Page:  1,
			Limit: 10,
		})

		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestService_AdminOverrideUserQuota(t *testing.T) {
	t.Run("success override to positive value", func(t *testing.T) {
		svc, mock, cleanup := setupAdminServiceMock(t)
		defer cleanup()

		adminID := uuid.New()
		userID := uuid.New()
		newQuota := 15

		// 1. Find user query
		mock.ExpectQuery(`SELECT \* FROM "users" WHERE id = \$1 AND "users"\."deleted_at" IS NULL ORDER BY "users"\."id" LIMIT \$2`).
			WithArgs(userID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "email", "full_name", "status", "daily_quota_override"}).
				AddRow(userID, "user@example.com", "Test User", "active", nil))

		// 2. Update user quota
		mock.ExpectBegin()
		mock.ExpectExec(`UPDATE "users" SET "daily_quota_override"=\$1,"updated_at"=\$2 WHERE id = \$3 AND "users"\."deleted_at" IS NULL`).
			WithArgs(newQuota, sqlmock.AnyArg(), userID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		// 3. Insert admin audit log
		mock.ExpectBegin()
		mock.ExpectQuery(`INSERT INTO "admin_audit_logs"`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(uuid.New(), time.Now()))
		mock.ExpectCommit()

		resp, err := svc.AdminOverrideUserQuota(context.Background(), AdminActionMeta{AdminID: adminID}, userID, dtos.AdminUserQuotaOverrideRequest{
			DailyQuotaOverride: &newQuota,
		})

		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if resp == nil {
			t.Fatal("expected non-nil response")
		}
		if resp.UserID != userID {
			t.Errorf("expected user ID %v, got %v", userID, resp.UserID)
		}
		if resp.DailyQuotaOverride == nil || *resp.DailyQuotaOverride != 15 {
			t.Errorf("expected daily quota override 15, got %v", resp.DailyQuotaOverride)
		}
		if resp.EffectiveQuota != 15 {
			t.Errorf("expected effective quota 15, got %d", resp.EffectiveQuota)
		}
	})

	t.Run("success reset override to nil", func(t *testing.T) {
		svc, mock, cleanup := setupAdminServiceMock(t)
		defer cleanup()

		adminID := uuid.New()
		userID := uuid.New()
		oldQuota := 20

		mock.ExpectQuery(`SELECT \* FROM "users" WHERE id = \$1 AND "users"\."deleted_at" IS NULL ORDER BY "users"\."id" LIMIT \$2`).
			WithArgs(userID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "email", "full_name", "status", "daily_quota_override"}).
				AddRow(userID, "user@example.com", "Test User", "active", oldQuota))

		mock.ExpectBegin()
		mock.ExpectExec(`UPDATE "users" SET "daily_quota_override"=\$1,"updated_at"=\$2 WHERE id = \$3 AND "users"\."deleted_at" IS NULL`).
			WithArgs(nil, sqlmock.AnyArg(), userID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		mock.ExpectBegin()
		mock.ExpectQuery(`INSERT INTO "admin_audit_logs"`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(uuid.New(), time.Now()))
		mock.ExpectCommit()

		resp, err := svc.AdminOverrideUserQuota(context.Background(), AdminActionMeta{AdminID: adminID}, userID, dtos.AdminUserQuotaOverrideRequest{
			DailyQuotaOverride: nil,
		})

		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if resp.DailyQuotaOverride != nil {
			t.Errorf("expected nil daily quota override, got %v", resp.DailyQuotaOverride)
		}
		// Default quota is constants.DefaultUserDailyQuota (5)
		if resp.EffectiveQuota != constants.DefaultUserDailyQuota {
			t.Errorf("expected default effective quota %d, got %d", constants.DefaultUserDailyQuota, resp.EffectiveQuota)
		}
	})

	t.Run("user not found returns ErrNotFound", func(t *testing.T) {
		svc, mock, cleanup := setupAdminServiceMock(t)
		defer cleanup()

		adminID := uuid.New()
		userID := uuid.New()
		newQuota := 10

		mock.ExpectQuery(`SELECT \* FROM "users" WHERE id = \$1 AND "users"\."deleted_at" IS NULL ORDER BY "users"\."id" LIMIT \$2`).
			WithArgs(userID, 1).
			WillReturnError(gorm.ErrRecordNotFound)

		_, err := svc.AdminOverrideUserQuota(context.Background(), AdminActionMeta{AdminID: adminID}, userID, dtos.AdminUserQuotaOverrideRequest{
			DailyQuotaOverride: &newQuota,
		})

		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("validation failure on negative quota", func(t *testing.T) {
		svc, _, cleanup := setupAdminServiceMock(t)
		defer cleanup()

		adminID := uuid.New()
		userID := uuid.New()
		negQuota := -5

		_, err := svc.AdminOverrideUserQuota(context.Background(), AdminActionMeta{AdminID: adminID}, userID, dtos.AdminUserQuotaOverrideRequest{
			DailyQuotaOverride: &negQuota,
		})

		if err == nil {
			t.Fatal("expected validation error, got nil")
		}
	})
}

func TestService_AdminRevokeUserSessions(t *testing.T) {
	t.Run("success revokes tokens and logs audit", func(t *testing.T) {
		svc, mock, cleanup := setupAdminServiceMock(t)
		defer cleanup()

		adminID := uuid.New()
		userID := uuid.New()

		// 1. Find user query
		mock.ExpectQuery(`SELECT \* FROM "users" WHERE id = \$1 AND "users"\."deleted_at" IS NULL ORDER BY "users"\."id" LIMIT \$2`).
			WithArgs(userID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "email", "full_name", "status"}).
				AddRow(userID, "user@example.com", "Test User", "active"))

		// 2. Revoke all tokens
		mock.ExpectBegin()
		mock.ExpectExec(`UPDATE "auth_tokens" SET "revoked_at"=\$1,"updated_at"=\$2 WHERE user_id = \$3 AND revoked_at IS NULL`).
			WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), userID).
			WillReturnResult(sqlmock.NewResult(1, 3))
		mock.ExpectCommit()

		// 3. Insert audit log
		mock.ExpectBegin()
		mock.ExpectQuery(`INSERT INTO "admin_audit_logs"`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(uuid.New(), time.Now()))
		mock.ExpectCommit()

		resp, err := svc.AdminRevokeUserSessions(context.Background(), AdminActionMeta{
			AdminID:   adminID,
			IPAddress: "127.0.0.1",
			UserAgent: "test-agent",
		}, userID)

		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if resp == nil {
			t.Fatal("expected non-nil response")
		}
		if resp.UserID != userID {
			t.Errorf("expected userID %v, got %v", userID, resp.UserID)
		}
		if !resp.Revoked {
			t.Error("expected revoked true")
		}
	})

	t.Run("user not found returns ErrNotFound", func(t *testing.T) {
		svc, mock, cleanup := setupAdminServiceMock(t)
		defer cleanup()

		adminID := uuid.New()
		userID := uuid.New()

		mock.ExpectQuery(`SELECT \* FROM "users" WHERE id = \$1 AND "users"\."deleted_at" IS NULL ORDER BY "users"\."id" LIMIT \$2`).
			WithArgs(userID, 1).
			WillReturnError(gorm.ErrRecordNotFound)

		_, err := svc.AdminRevokeUserSessions(context.Background(), AdminActionMeta{AdminID: adminID}, userID)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestService_AdminListRoles(t *testing.T) {
	t.Run("success returns roles with decoded permissions", func(t *testing.T) {
		svc, mock, cleanup := setupAdminServiceMock(t)
		defer cleanup()

		roleID := uuid.New()
		now := time.Now()

		mock.ExpectQuery(`SELECT \* FROM "admin_roles"`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name", "description", "permissions", "is_system", "created_at", "updated_at"}).
				AddRow(roleID, "Super Admin", "Root admin", []byte(`["*"]`), true, now, now))

		resp, err := svc.AdminListRoles(context.Background())
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if len(resp.Items) != 1 {
			t.Fatalf("expected 1 role, got %d", len(resp.Items))
		}
		if resp.Items[0].Name != "Super Admin" {
			t.Errorf("expected role name Super Admin, got %s", resp.Items[0].Name)
		}
		if len(resp.Items[0].Permissions) != 1 || resp.Items[0].Permissions[0] != "*" {
			t.Errorf("expected permissions [*], got %v", resp.Items[0].Permissions)
		}
	})

	t.Run("db failure returns wrapped error", func(t *testing.T) {
		svc, mock, cleanup := setupAdminServiceMock(t)
		defer cleanup()

		mock.ExpectQuery(`SELECT \* FROM "admin_roles"`).
			WillReturnError(errors.New("db error"))

		_, err := svc.AdminListRoles(context.Background())
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestService_AdminCreateRole(t *testing.T) {
	t.Run("success creates role and logs audit", func(t *testing.T) {
		svc, mock, cleanup := setupAdminServiceMock(t)
		defer cleanup()

		adminID := uuid.New()
		req := dtos.AdminCreateRoleRequest{
			Name:        "Support Agent",
			Description: "Customer support staff",
			Permissions: []string{"users:read"},
		}

		// 1. FindRoleByName (not found)
		mock.ExpectQuery(`SELECT \* FROM "admin_roles" WHERE name = \$1 ORDER BY "admin_roles"\."id" LIMIT \$2`).
			WithArgs(req.Name, 1).
			WillReturnError(gorm.ErrRecordNotFound)

		// 2. Create role
		mock.ExpectBegin()
		mock.ExpectQuery(`INSERT INTO "admin_roles"`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(uuid.New(), time.Now(), time.Now()))
		mock.ExpectCommit()

		// 3. Create audit log
		mock.ExpectBegin()
		mock.ExpectQuery(`INSERT INTO "admin_audit_logs"`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(uuid.New(), time.Now()))
		mock.ExpectCommit()

		resp, err := svc.AdminCreateRole(context.Background(), AdminActionMeta{
			AdminID:   adminID,
			IPAddress: "127.0.0.1",
			UserAgent: "test-agent",
		}, req)

		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if resp.Name != req.Name {
			t.Errorf("expected role name %s, got %s", req.Name, resp.Name)
		}
		if resp.IsSystem {
			t.Error("expected custom role to have IsSystem false")
		}
	})

	t.Run("duplicate role name returns ErrConflict", func(t *testing.T) {
		svc, mock, cleanup := setupAdminServiceMock(t)
		defer cleanup()

		adminID := uuid.New()
		req := dtos.AdminCreateRoleRequest{
			Name:        "Super Admin",
			Permissions: []string{"*"},
		}

		mock.ExpectQuery(`SELECT \* FROM "admin_roles" WHERE name = \$1 ORDER BY "admin_roles"\."id" LIMIT \$2`).
			WithArgs(req.Name, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(uuid.New(), req.Name))

		_, err := svc.AdminCreateRole(context.Background(), AdminActionMeta{AdminID: adminID}, req)
		if err == nil {
			t.Fatal("expected conflict error, got nil")
		}
	})
}

func TestService_AdminUpdateRole(t *testing.T) {
	t.Run("success updates custom role and logs audit", func(t *testing.T) {
		svc, mock, cleanup := setupAdminServiceMock(t)
		defer cleanup()

		adminID := uuid.New()
		roleID := uuid.New()
		req := dtos.AdminUpdateRoleRequest{
			Name:        "Updated Role",
			Description: "Updated description",
			Permissions: []string{"recordings:read", "recordings:write"},
		}

		// 1. FindRoleByID
		mock.ExpectQuery(`SELECT \* FROM "admin_roles" WHERE id = \$1 ORDER BY "admin_roles"\."id" LIMIT \$2`).
			WithArgs(roleID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name", "description", "permissions", "is_system", "created_at", "updated_at"}).
				AddRow(roleID, "Old Role", "Old description", []byte(`["recordings:read"]`), false, time.Now(), time.Now()))

		// 2. FindRoleByName for name check
		mock.ExpectQuery(`SELECT \* FROM "admin_roles" WHERE name = \$1 ORDER BY "admin_roles"\."id" LIMIT \$2`).
			WithArgs(req.Name, 1).
			WillReturnError(gorm.ErrRecordNotFound)

		// 3. UpdateRole
		mock.ExpectBegin()
		mock.ExpectExec(`UPDATE "admin_roles" SET`).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		// 4. Create audit log
		mock.ExpectBegin()
		mock.ExpectQuery(`INSERT INTO "admin_audit_logs"`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(uuid.New(), time.Now()))
		mock.ExpectCommit()

		resp, err := svc.AdminUpdateRole(context.Background(), AdminActionMeta{
			AdminID:   adminID,
			IPAddress: "127.0.0.1",
			UserAgent: "test-agent",
		}, roleID, req)

		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if resp.Name != req.Name {
			t.Errorf("expected updated name %s, got %s", req.Name, resp.Name)
		}
	})

	t.Run("rejects modifying system role with ErrForbidden", func(t *testing.T) {
		svc, mock, cleanup := setupAdminServiceMock(t)
		defer cleanup()

		adminID := uuid.New()
		roleID := uuid.New()
		req := dtos.AdminUpdateRoleRequest{
			Name:        "Super Admin Modified",
			Permissions: []string{"*"},
		}

		mock.ExpectQuery(`SELECT \* FROM "admin_roles" WHERE id = \$1 ORDER BY "admin_roles"\."id" LIMIT \$2`).
			WithArgs(roleID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name", "description", "permissions", "is_system"}).
				AddRow(roleID, "Super Admin", "Root", []byte(`["*"]`), true))

		_, err := svc.AdminUpdateRole(context.Background(), AdminActionMeta{AdminID: adminID}, roleID, req)
		if err == nil {
			t.Fatal("expected ErrForbidden for system role modification, got nil")
		}
	})
}

func TestService_AdminListTemplates(t *testing.T) {
	t.Run("success returns all templates", func(t *testing.T) {
		svc, mock, cleanup := setupAdminServiceMock(t)
		defer cleanup()

		tmplID := uuid.New()
		now := time.Now()

		mock.ExpectQuery(`SELECT \* FROM "templates" ORDER BY category_key ASC`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "category_key", "name", "description", "prompt", "output_schema", "version", "is_active", "created_at", "updated_at"}).
				AddRow(tmplID, "MOM", "Minutes of Meeting", "Standard MOM", "Extract minutes...", []byte(`{}`), 1, true, now, now))

		resp, err := svc.AdminListTemplates(context.Background())
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if len(resp.Items) != 1 {
			t.Fatalf("expected 1 template, got %d", len(resp.Items))
		}
		if resp.Items[0].CategoryKey != "MOM" {
			t.Errorf("expected CategoryKey MOM, got %s", resp.Items[0].CategoryKey)
		}
	})

	t.Run("db failure returns error", func(t *testing.T) {
		svc, mock, cleanup := setupAdminServiceMock(t)
		defer cleanup()

		mock.ExpectQuery(`SELECT \* FROM "templates" ORDER BY category_key ASC`).
			WillReturnError(errors.New("db error"))

		_, err := svc.AdminListTemplates(context.Background())
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestService_AdminCreateTemplate(t *testing.T) {
	t.Run("success creates template and writes audit log", func(t *testing.T) {
		svc, mock, cleanup := setupAdminServiceMock(t)
		defer cleanup()

		adminID := uuid.New()
		req := dtos.AdminCreateTemplateRequest{
			CategoryKey: "TUTORIAL",
			Name:        "Tutorial Video",
			Description: "Tutorial breakdown",
			Prompt:      "Extract tutorial steps...",
			OutputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"steps": map[string]interface{}{"type": "array"},
				},
			},
		}

		mock.ExpectQuery(`SELECT \* FROM "templates" WHERE category_key = \$1 ORDER BY "templates"\."id" LIMIT \$2`).
			WithArgs(req.CategoryKey, 1).
			WillReturnError(gorm.ErrRecordNotFound)

		mock.ExpectBegin()
		mock.ExpectQuery(`INSERT INTO "templates"`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(uuid.New(), time.Now(), time.Now()))
		mock.ExpectCommit()

		mock.ExpectBegin()
		mock.ExpectQuery(`INSERT INTO "admin_audit_logs"`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(uuid.New(), time.Now()))
		mock.ExpectCommit()

		resp, err := svc.AdminCreateTemplate(context.Background(), AdminActionMeta{
			AdminID:   adminID,
			IPAddress: "127.0.0.1",
			UserAgent: "test-agent",
		}, req)

		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if resp.CategoryKey != req.CategoryKey {
			t.Errorf("expected CategoryKey %s, got %s", req.CategoryKey, resp.CategoryKey)
		}
		if resp.Version != 1 {
			t.Errorf("expected version 1, got %d", resp.Version)
		}
	})

	t.Run("duplicate category key returns ErrConflict", func(t *testing.T) {
		svc, mock, cleanup := setupAdminServiceMock(t)
		defer cleanup()

		adminID := uuid.New()
		req := dtos.AdminCreateTemplateRequest{
			CategoryKey: "MOM",
			Name:        "Meeting",
			Prompt:      "Prompt",
			OutputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"title": map[string]interface{}{"type": "string"},
				},
			},
		}

		mock.ExpectQuery(`SELECT \* FROM "templates" WHERE category_key = \$1 ORDER BY "templates"\."id" LIMIT \$2`).
			WithArgs(req.CategoryKey, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "category_key"}).AddRow(uuid.New(), req.CategoryKey))

		_, err := svc.AdminCreateTemplate(context.Background(), AdminActionMeta{AdminID: adminID}, req)
		if err == nil {
			t.Fatal("expected conflict error, got nil")
		}
	})
}

func TestService_AdminUpdateTemplate(t *testing.T) {
	t.Run("increments version when prompt changes", func(t *testing.T) {
		svc, mock, cleanup := setupAdminServiceMock(t)
		defer cleanup()

		adminID := uuid.New()
		tmplID := uuid.New()
		now := time.Now()

		req := dtos.AdminUpdateTemplateRequest{
			Name:        "Minutes of Meeting V2",
			Description: "Updated description",
			Prompt:      "New prompt text for meeting minutes",
			OutputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"title": map[string]interface{}{"type": "string"},
				},
			},
		}

		// 1. FindTemplateByID
		mock.ExpectQuery(`SELECT \* FROM "templates" WHERE id = \$1 ORDER BY "templates"\."id" LIMIT \$2`).
			WithArgs(tmplID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "category_key", "name", "description", "prompt", "output_schema", "version", "is_active", "created_at", "updated_at"}).
				AddRow(tmplID, "MOM", "Old Name", "Old Desc", "Old prompt", []byte(`{"type":"object","properties":{"title":{"type":"string"}}}`), 1, true, now, now))

		// 2. UpdateTemplate
		mock.ExpectBegin()
		mock.ExpectExec(`UPDATE "templates" SET`).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		// 3. Create audit log
		mock.ExpectBegin()
		mock.ExpectQuery(`INSERT INTO "admin_audit_logs"`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(uuid.New(), time.Now()))
		mock.ExpectCommit()

		resp, err := svc.AdminUpdateTemplate(context.Background(), AdminActionMeta{
			AdminID:   adminID,
			IPAddress: "127.0.0.1",
			UserAgent: "test-agent",
		}, tmplID, req)

		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if resp.Version != 2 {
			t.Errorf("expected version 2 after prompt change, got %d", resp.Version)
		}
		if resp.Name != req.Name {
			t.Errorf("expected name %s, got %s", req.Name, resp.Name)
		}
	})

	t.Run("maintains version when prompt and schema are unchanged", func(t *testing.T) {
		svc, mock, cleanup := setupAdminServiceMock(t)
		defer cleanup()

		adminID := uuid.New()
		tmplID := uuid.New()
		now := time.Now()

		req := dtos.AdminUpdateTemplateRequest{
			Name:        "Renamed Template",
			Description: "New description",
			Prompt:      "Same prompt",
			OutputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"title": map[string]interface{}{"type": "string"},
				},
			},
		}

		mock.ExpectQuery(`SELECT \* FROM "templates" WHERE id = \$1 ORDER BY "templates"\."id" LIMIT \$2`).
			WithArgs(tmplID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "category_key", "name", "description", "prompt", "output_schema", "version", "is_active", "created_at", "updated_at"}).
				AddRow(tmplID, "MOM", "Old Name", "Old Desc", "Same prompt", []byte(`{"properties":{"title":{"type":"string"}},"type":"object"}`), 3, true, now, now))

		mock.ExpectBegin()
		mock.ExpectExec(`UPDATE "templates" SET`).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		mock.ExpectBegin()
		mock.ExpectQuery(`INSERT INTO "admin_audit_logs"`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(uuid.New(), time.Now()))
		mock.ExpectCommit()

		resp, err := svc.AdminUpdateTemplate(context.Background(), AdminActionMeta{AdminID: adminID}, tmplID, req)
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if resp.Version != 3 {
			t.Errorf("expected version to remain 3, got %d", resp.Version)
		}
	})

	t.Run("template not found returns ErrNotFound", func(t *testing.T) {
		svc, mock, cleanup := setupAdminServiceMock(t)
		defer cleanup()

		adminID := uuid.New()
		tmplID := uuid.New()

		req := dtos.AdminUpdateTemplateRequest{
			Name:   "Name",
			Prompt: "Prompt",
			OutputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"title": map[string]interface{}{"type": "string"},
				},
			},
		}

		mock.ExpectQuery(`SELECT \* FROM "templates" WHERE id = \$1 ORDER BY "templates"\."id" LIMIT \$2`).
			WithArgs(tmplID, 1).
			WillReturnError(gorm.ErrRecordNotFound)

		_, err := svc.AdminUpdateTemplate(context.Background(), AdminActionMeta{AdminID: adminID}, tmplID, req)
		if err == nil {
			t.Fatal("expected ErrNotFound, got nil")
		}
	})
}

type mockAdminLLM struct {
	GenerateStructuredFunc func(ctx context.Context, systemPrompt, userPrompt string, schema map[string]interface{}) (*dtos.StructuredResponse, error)
}

func (m *mockAdminLLM) GenerateStructured(ctx context.Context, systemPrompt, userPrompt string, schema map[string]interface{}) (*dtos.StructuredResponse, error) {
	if m.GenerateStructuredFunc != nil {
		return m.GenerateStructuredFunc(ctx, systemPrompt, userPrompt, schema)
	}
	return nil, nil
}

func (m *mockAdminLLM) GenerateChatResponse(ctx context.Context, systemPrompt string, messages []dtos.ChatMessageInput, opts dtos.ChatOptions) (*dtos.ChatResponse, error) {
	return nil, nil
}

func (m *mockAdminLLM) StreamChatResponse(ctx context.Context, systemPrompt string, messages []dtos.ChatMessageInput, opts dtos.ChatOptions) (<-chan dtos.StreamChunk, error) {
	return nil, nil
}

func TestService_AdminTestTemplate(t *testing.T) {
	t.Run("success runs prompt sandbox and returns metrics", func(t *testing.T) {
		svc, _, cleanup := setupAdminServiceMock(t)
		defer cleanup()

		adminID := uuid.New()
		mockLLM := &mockAdminLLM{
			GenerateStructuredFunc: func(ctx context.Context, systemPrompt, userPrompt string, schema map[string]interface{}) (*dtos.StructuredResponse, error) {
				return &dtos.StructuredResponse{
					RawJSON: `{"action_items":["Deploy to prod"]}`,
					Usage: dtos.LLMUsage{
						PromptTokens:     50,
						CompletionTokens: 20,
						TotalTokens:      70,
					},
				}, nil
			},
		}
		svc.SetLLM(mockLLM)

		req := dtos.AdminTestTemplateRequest{
			Prompt:           "Extract action items",
			SampleTranscript: "Alice: Deploy to prod.",
			OutputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"action_items": map[string]interface{}{"type": "array"},
				},
			},
		}

		resp, err := svc.AdminTestTemplate(context.Background(), AdminActionMeta{AdminID: adminID}, req)
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if resp.TokensUsed != 70 {
			t.Errorf("expected 70 tokens used, got %d", resp.TokensUsed)
		}
		if resp.RawOutput != `{"action_items":["Deploy to prod"]}` {
			t.Errorf("expected RawOutput match, got %s", resp.RawOutput)
		}
	})

	t.Run("nil LLM provider returns ErrInternal", func(t *testing.T) {
		svc, _, cleanup := setupAdminServiceMock(t)
		defer cleanup()

		adminID := uuid.New()
		svc.SetLLM(nil)

		req := dtos.AdminTestTemplateRequest{
			Prompt:           "Prompt",
			SampleTranscript: "Sample",
			OutputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"title": map[string]interface{}{"type": "string"},
				},
			},
		}

		_, err := svc.AdminTestTemplate(context.Background(), AdminActionMeta{AdminID: adminID}, req)
		if err == nil {
			t.Fatal("expected error for nil LLM, got nil")
		}
	})

	t.Run("LLM execution error returns error", func(t *testing.T) {
		svc, _, cleanup := setupAdminServiceMock(t)
		defer cleanup()

		adminID := uuid.New()
		mockLLM := &mockAdminLLM{
			GenerateStructuredFunc: func(ctx context.Context, systemPrompt, userPrompt string, schema map[string]interface{}) (*dtos.StructuredResponse, error) {
				return nil, errors.New("llm provider timeout")
			},
		}
		svc.SetLLM(mockLLM)

		req := dtos.AdminTestTemplateRequest{
			Prompt:           "Prompt",
			SampleTranscript: "Sample",
			OutputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"title": map[string]interface{}{"type": "string"},
				},
			},
		}

		_, err := svc.AdminTestTemplate(context.Background(), AdminActionMeta{AdminID: adminID}, req)
		if err == nil {
			t.Fatal("expected LLM error, got nil")
		}
	})
}

func TestService_AdminGetDLQMessages(t *testing.T) {
	t.Run("success returns dlq items with counts", func(t *testing.T) {
		svc, mock, cleanup := setupAdminServiceMock(t)
		defer cleanup()

		adminID := uuid.New()
		recID1 := uuid.New()
		recID2 := uuid.New()
		userID := uuid.New()
		now := time.Now()
		errMsg := "transcription timeout"
		errCode := "ERR_TRANSCRIPTION_FAILED"

		// 1. Count failed
		mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings" WHERE status = 'FAILED' AND "recordings"\."deleted_at" IS NULL`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

		// 2. Count stuck
		mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings" WHERE \(status IN \('QUEUED', 'VALIDATING', 'EXTRACTING', 'TRANSCRIBING', 'SUMMARIZING', 'INDEXING'\) AND updated_at < \$1\) AND "recordings"\."deleted_at" IS NULL`).
			WithArgs(sqlmock.AnyArg()).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

		// 3. Query recordings
		mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE \(\(status = 'FAILED'\) OR \(status IN \('QUEUED', 'VALIDATING', 'EXTRACTING', 'TRANSCRIBING', 'SUMMARIZING', 'INDEXING'\) AND updated_at < \$1\)\) AND "recordings"\."deleted_at" IS NULL ORDER BY updated_at DESC LIMIT \$2`).
			WithArgs(sqlmock.AnyArg(), 20).
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "user_id", "title", "original_filename", "status", "source_type", "error_code", "error_message", "created_at", "updated_at",
			}).
				AddRow(recID1, userID, "Failed Video", "video.mp4", models.RecordingStatusFailed, "UPLOAD", errCode, errMsg, now, now).
				AddRow(recID2, nil, "Stuck Import", "import.mp4", models.RecordingStatusExtracting, constants.RecordingSourceTypeLink, nil, nil, now, now))

		// Preload users for recID1
		mock.ExpectQuery(`SELECT \* FROM "users" WHERE "users"\."id" = \$1`).
			WithArgs(userID).
			WillReturnRows(sqlmock.NewRows([]string{"id", "email", "full_name"}).
				AddRow(userID, "user@example.com", "Jane Doe"))

		resp, err := svc.AdminGetDLQMessages(context.Background(), AdminActionMeta{AdminID: adminID}, 1, 20)
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if resp.Total != 2 {
			t.Errorf("expected total 2, got %d", resp.Total)
		}
		if resp.FailedCount != 1 || resp.StuckCount != 1 {
			t.Errorf("expected failed=1, stuck=1, got failed=%d, stuck=%d", resp.FailedCount, resp.StuckCount)
		}
		if len(resp.Items) != 2 {
			t.Fatalf("expected 2 items, got %d", len(resp.Items))
		}
		if resp.Items[0].Queue != "recording.dlq" {
			t.Errorf("expected recording.dlq, got %s", resp.Items[0].Queue)
		}
		if resp.Items[0].UserEmail != "user@example.com" {
			t.Errorf("expected user@example.com, got %s", resp.Items[0].UserEmail)
		}
		if resp.Items[1].Queue != constants.TopicRecordingImport {
			t.Errorf("expected import topic, got %s", resp.Items[1].Queue)
		}
	})

	t.Run("database error returns error", func(t *testing.T) {
		svc, mock, cleanup := setupAdminServiceMock(t)
		defer cleanup()

		adminID := uuid.New()
		mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings" WHERE status = 'FAILED'`).
			WillReturnError(errors.New("db disconnect"))

		_, err := svc.AdminGetDLQMessages(context.Background(), AdminActionMeta{AdminID: adminID}, 1, 20)
		if err == nil {
			t.Fatal("expected database error, got nil")
		}
	})
}

func TestService_AdminRetryDLQJob(t *testing.T) {
	t.Run("invalid request returns error", func(t *testing.T) {
		svc, _, cleanup := setupAdminServiceMock(t)
		defer cleanup()

		_, err := svc.AdminRetryDLQJob(context.Background(), AdminActionMeta{}, dtos.DLQRetryRequest{RecordingID: uuid.Nil})
		if err == nil {
			t.Fatal("expected error for nil recording ID, got nil")
		}
	})

	t.Run("recording not found returns ErrRecordingNotFound", func(t *testing.T) {
		svc, mock, cleanup := setupAdminServiceMock(t)
		defer cleanup()

		recID := uuid.New()
		mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL`).
			WithArgs(recID, 1).
			WillReturnError(gorm.ErrRecordNotFound)

		_, err := svc.AdminRetryDLQJob(context.Background(), AdminActionMeta{}, dtos.DLQRetryRequest{RecordingID: recID})
		if err == nil {
			t.Fatal("expected ErrNotFound, got nil")
		}
	})

	t.Run("already completed recording returns ErrConflict", func(t *testing.T) {
		svc, mock, cleanup := setupAdminServiceMock(t)
		defer cleanup()

		recID := uuid.New()
		now := time.Now()
		mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL`).
			WithArgs(recID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "status", "created_at", "updated_at"}).
				AddRow(recID, models.RecordingStatusCompleted, now, now))

		_, err := svc.AdminRetryDLQJob(context.Background(), AdminActionMeta{}, dtos.DLQRetryRequest{RecordingID: recID})
		if err == nil {
			t.Fatal("expected ErrRecordingAlreadyCompleted, got nil")
		}
	})

	t.Run("success resumes pipeline and logs audit action", func(t *testing.T) {
		svc, mock, cleanup := setupAdminServiceMock(t)
		defer cleanup()

		adminID := uuid.New()
		recID := uuid.New()
		now := time.Now()

		// 1. FindRecordingByID
		mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL`).
			WithArgs(recID, 1).
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "status", "title", "original_filename", "selected_template", "output_language", "audio_url", "created_at", "updated_at",
			}).
				AddRow(recID, models.RecordingStatusFailed, "Meeting", "meet.mp4", "GENERAL", "id", nil, now, now))

		// 2. ResumeRecordingPipeline: ListTranscriptSegmentsByRecordingID
		mock.ExpectQuery(`SELECT \* FROM "transcript_segments" WHERE recording_id = \$1`).
			WithArgs(recID).
			WillReturnRows(sqlmock.NewRows([]string{"id"}))

		// 3. FindActiveSummaryByRecordingID
		mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND is_active = TRUE`).
			WithArgs(recID, 1).
			WillReturnError(gorm.ErrRecordNotFound)

		// 4. ListTranscriptChunksByRecordingID
		mock.ExpectQuery(`SELECT .* FROM "transcript_chunks" WHERE recording_id = \$1 ORDER BY chunk_index ASC`).
			WithArgs(recID).
			WillReturnRows(sqlmock.NewRows([]string{"id"}))

		// 5. UpdateRecordingStatus to QUEUED
		mock.ExpectBegin()
		mock.ExpectExec(`UPDATE "recordings" SET .* WHERE id = .*`).
			WithArgs(models.RecordingStatusQueued, sqlmock.AnyArg(), recID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		// 6. CreateAdminAuditLog
		mock.ExpectBegin()
		mock.ExpectQuery(`INSERT INTO "admin_audit_logs"`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(uuid.New(), time.Now()))
		mock.ExpectCommit()

		resp, err := svc.AdminRetryDLQJob(context.Background(), AdminActionMeta{AdminID: adminID}, dtos.DLQRetryRequest{RecordingID: recID})
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if resp.RecordingID != recID {
			t.Fatalf("expected recording ID %s, got %s", recID, resp.RecordingID)
		}
		if resp.Status != models.RecordingStatusQueued {
			t.Errorf("expected status %s, got %s", models.RecordingStatusQueued, resp.Status)
		}
	})
}

func TestService_AdminSystemConfig(t *testing.T) {
	t.Run("AdminGetSystemConfig and IsMaintenanceMode", func(t *testing.T) {
		svc, _, cleanup := setupAdminServiceMock(t)
		defer cleanup()

		resp, err := svc.AdminGetSystemConfig(context.Background(), AdminActionMeta{})
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if resp == nil {
			t.Fatal("expected non-nil config response")
		}

		isMaint := svc.IsMaintenanceMode(context.Background())
		if isMaint != resp.MaintenanceMode {
			t.Errorf("expected isMaintenanceMode %v, got %v", resp.MaintenanceMode, isMaint)
		}
	})

	t.Run("AdminUpdateSystemConfig updates flags and writes audit log", func(t *testing.T) {
		svc, mock, cleanup := setupAdminServiceMock(t)
		defer cleanup()

		adminID := uuid.New()
		m := true
		g := false
		b := true
		req := dtos.UpdateSystemConfigRequest{
			MaintenanceMode:    &m,
			AllowGuestUploads:  &g,
			BotWaitlistEnabled: &b,
		}

		mock.ExpectBegin()
		mock.ExpectQuery(`INSERT INTO "admin_audit_logs"`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(uuid.New(), time.Now()))
		mock.ExpectCommit()

		resp, err := svc.AdminUpdateSystemConfig(context.Background(), AdminActionMeta{
			AdminID:   adminID,
			IPAddress: "127.0.0.1",
			UserAgent: "admin-browser",
		}, req)
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if !resp.MaintenanceMode || resp.AllowGuestUploads || !resp.BotWaitlistEnabled {
			t.Fatalf("unexpected updated flags: %+v", resp)
		}

		// Verify IsMaintenanceMode returns true now
		if !svc.IsMaintenanceMode(context.Background()) {
			t.Fatal("expected IsMaintenanceMode to be true after update")
		}
	})
}








