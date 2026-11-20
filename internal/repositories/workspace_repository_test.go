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

func setupMockWorkspaceDB(t *testing.T) (*Repositories, sqlmock.Sqlmock, func()) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}

	gormDB, err := gorm.Open(gormPostgres.New(gormPostgres.Config{
		Conn: sqlDB,
	}), &gorm.Config{})
	if err != nil {
		sqlDB.Close()
		t.Fatalf("failed to open gorm db: %v", err)
	}

	repo := New(gormDB)
	cleanup := func() {
		sqlDB.Close()
	}
	return repo, mock, cleanup
}

func TestRepositories_CreateWorkspace(t *testing.T) {
	repo, mock, cleanup := setupMockWorkspaceDB(t)
	defer cleanup()

	wsID := uuid.New()
	userID := uuid.New()
	memberID := uuid.New()
	ws := &models.Workspace{
		BaseModel:   models.BaseModel{ID: wsID},
		Name:        "Engineering Team",
		Slug:        "engineering-team",
		Type:        constants.WorkspaceTypeTeam,
		OwnerUserID: userID,
	}

	wsRows := sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).
		AddRow(wsID, time.Now(), time.Now())
	memberRows := sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).
		AddRow(memberID, time.Now(), time.Now())

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "workspaces"`)).
		WillReturnRows(wsRows)
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "workspace_members"`)).
		WillReturnRows(memberRows)
	mock.ExpectCommit()

	err := repo.CreateWorkspace(context.Background(), ws)
	if err != nil {
		t.Fatalf("unexpected error creating workspace: %v", err)
	}
}

func TestRepositories_FindWorkspaceByID(t *testing.T) {
	repo, mock, cleanup := setupMockWorkspaceDB(t)
	defer cleanup()

	wsID := uuid.New()
	userID := uuid.New()

	rows := sqlmock.NewRows([]string{"id", "name", "slug", "type", "owner_user_id", "created_at", "updated_at", "deleted_at"}).
		AddRow(wsID, "Acme Corp", "acme-corp", "team", userID, time.Now(), time.Now(), nil)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspaces" WHERE id = $1 AND "workspaces"."deleted_at" IS NULL`)).
		WithArgs(wsID, 1).
		WillReturnRows(rows)

	ws, err := repo.FindWorkspaceByID(context.Background(), wsID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ws.Name != "Acme Corp" {
		t.Errorf("expected name 'Acme Corp', got %q", ws.Name)
	}
}

func TestRepositories_FindWorkspaceBySlug(t *testing.T) {
	repo, mock, cleanup := setupMockWorkspaceDB(t)
	defer cleanup()

	wsID := uuid.New()
	userID := uuid.New()

	rows := sqlmock.NewRows([]string{"id", "name", "slug", "type", "owner_user_id", "created_at", "updated_at", "deleted_at"}).
		AddRow(wsID, "Acme Corp", "acme-corp", "team", userID, time.Now(), time.Now(), nil)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspaces" WHERE slug = $1 AND "workspaces"."deleted_at" IS NULL`)).
		WithArgs("acme-corp", 1).
		WillReturnRows(rows)

	ws, err := repo.FindWorkspaceBySlug(context.Background(), "acme-corp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ws.ID != wsID {
		t.Errorf("expected ID %s, got %s", wsID, ws.ID)
	}
}

func TestRepositories_FindWorkspacesByUserID(t *testing.T) {
	repo, mock, cleanup := setupMockWorkspaceDB(t)
	defer cleanup()

	wsID := uuid.New()
	userID := uuid.New()

	rows := sqlmock.NewRows([]string{"id", "name", "slug", "type", "owner_user_id", "created_at", "updated_at", "deleted_at"}).
		AddRow(wsID, "Personal WS", "personal-123", "personal", userID, time.Now(), time.Now(), nil)

	mock.ExpectQuery(`SELECT .+ FROM "workspaces" INNER JOIN workspace_members ON workspace_members\.workspace_id = workspaces\.id WHERE \(workspace_members\.user_id = \$1 AND workspace_members\.deleted_at IS NULL AND workspaces\.deleted_at IS NULL\) AND "workspaces"\."deleted_at" IS NULL`).
		WithArgs(userID).
		WillReturnRows(rows)

	list, err := repo.FindWorkspacesByUserID(context.Background(), userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 workspace, got %d", len(list))
	}
}

func TestRepositories_FindPersonalWorkspaceByUserID(t *testing.T) {
	repo, mock, cleanup := setupMockWorkspaceDB(t)
	defer cleanup()

	wsID := uuid.New()
	userID := uuid.New()

	rows := sqlmock.NewRows([]string{"id", "name", "slug", "type", "owner_user_id", "created_at", "updated_at", "deleted_at"}).
		AddRow(wsID, "Personal WS", "personal-123", "personal", userID, time.Now(), time.Now(), nil)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspaces" WHERE (owner_user_id = $1 AND type = $2) AND "workspaces"."deleted_at" IS NULL`)).
		WithArgs(userID, string(constants.WorkspaceTypePersonal), 1).
		WillReturnRows(rows)

	ws, err := repo.FindPersonalWorkspaceByUserID(context.Background(), userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ws.Type != constants.WorkspaceTypePersonal {
		t.Errorf("expected personal type, got %s", ws.Type)
	}
}

func TestRepositories_UpdateWorkspace(t *testing.T) {
	repo, mock, cleanup := setupMockWorkspaceDB(t)
	defer cleanup()

	wsID := uuid.New()
	logo := "https://example.com/logo.png"
	ws := &models.Workspace{
		BaseModel: models.BaseModel{ID: wsID},
		Name:      "Updated Name",
		LogoURL:   &logo,
	}

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "workspaces" SET`)).
		WithArgs(ws.LogoURL, ws.Name, sqlmock.AnyArg(), wsID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := repo.UpdateWorkspace(context.Background(), ws)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRepositories_DeleteWorkspace(t *testing.T) {
	repo, mock, cleanup := setupMockWorkspaceDB(t)
	defer cleanup()

	wsID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "workspaces" SET "deleted_at"=$1 WHERE id = $2 AND "workspaces"."deleted_at" IS NULL`)).
		WithArgs(sqlmock.AnyArg(), wsID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := repo.DeleteWorkspace(context.Background(), wsID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRepositories_AddWorkspaceMember(t *testing.T) {
	repo, mock, cleanup := setupMockWorkspaceDB(t)
	defer cleanup()

	wsID := uuid.New()
	userID := uuid.New()
	memberID := uuid.New()
	member := &models.WorkspaceMember{
		WorkspaceID: wsID,
		UserID:      userID,
		Role:        constants.WorkspaceRoleMember,
	}

	mock.ExpectBegin()
	memberRows := sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).
		AddRow(memberID, time.Now(), time.Now())
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "workspace_members"`)).
		WillReturnRows(memberRows)
	mock.ExpectCommit()

	err := repo.AddWorkspaceMember(context.Background(), member)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRepositories_FindWorkspaceMember(t *testing.T) {
	repo, mock, cleanup := setupMockWorkspaceDB(t)
	defer cleanup()

	wsID := uuid.New()
	userID := uuid.New()
	memberID := uuid.New()

	memberRows := sqlmock.NewRows([]string{"id", "workspace_id", "user_id", "role", "created_at", "updated_at", "deleted_at"}).
		AddRow(memberID, wsID, userID, "admin", time.Now(), time.Now(), nil)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspace_members" WHERE (workspace_id = $1 AND user_id = $2) AND "workspace_members"."deleted_at" IS NULL`)).
		WithArgs(wsID, userID, 1).
		WillReturnRows(memberRows)

	member, err := repo.FindWorkspaceMember(context.Background(), wsID, userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if member.Role != constants.WorkspaceRoleAdmin {
		t.Errorf("expected role admin, got %s", member.Role)
	}
}

func TestRepositories_ListWorkspaceMembers(t *testing.T) {
	repo, mock, cleanup := setupMockWorkspaceDB(t)
	defer cleanup()

	wsID := uuid.New()
	userID := uuid.New()
	memberID := uuid.New()

	memberRows := sqlmock.NewRows([]string{"id", "workspace_id", "user_id", "role", "created_at", "updated_at", "deleted_at"}).
		AddRow(memberID, wsID, userID, "owner", time.Now(), time.Now(), nil)

	userRows := sqlmock.NewRows([]string{"id", "email", "full_name"}).
		AddRow(userID, "owner@example.com", "Owner User")

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspace_members" WHERE workspace_id = $1 AND "workspace_members"."deleted_at" IS NULL`)).
		WithArgs(wsID).
		WillReturnRows(memberRows)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE "users"."id" = $1 AND "users"."deleted_at" IS NULL`)).
		WithArgs(userID).
		WillReturnRows(userRows)

	members, err := repo.ListWorkspaceMembers(context.Background(), wsID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(members) != 1 {
		t.Errorf("expected 1 member, got %d", len(members))
	}
}

func TestRepositories_UpdateWorkspaceMemberRole(t *testing.T) {
	repo, mock, cleanup := setupMockWorkspaceDB(t)
	defer cleanup()

	wsID := uuid.New()
	userID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "workspace_members" SET "role"=$1,"updated_at"=$2 WHERE (workspace_id = $3 AND user_id = $4) AND "workspace_members"."deleted_at" IS NULL`)).
		WithArgs(string(constants.WorkspaceRoleAdmin), sqlmock.AnyArg(), wsID, userID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := repo.UpdateWorkspaceMemberRole(context.Background(), wsID, userID, constants.WorkspaceRoleAdmin)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRepositories_RemoveWorkspaceMember(t *testing.T) {
	repo, mock, cleanup := setupMockWorkspaceDB(t)
	defer cleanup()

	wsID := uuid.New()
	userID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "workspace_members" SET "deleted_at"=$1 WHERE (workspace_id = $2 AND user_id = $3) AND "workspace_members"."deleted_at" IS NULL`)).
		WithArgs(sqlmock.AnyArg(), wsID, userID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := repo.RemoveWorkspaceMember(context.Background(), wsID, userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRepositories_CountWorkspaceMembers(t *testing.T) {
	repo, mock, cleanup := setupMockWorkspaceDB(t)
	defer cleanup()

	wsID := uuid.New()

	rows := sqlmock.NewRows([]string{"count"}).AddRow(5)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "workspace_members" WHERE workspace_id = $1 AND "workspace_members"."deleted_at" IS NULL`)).
		WithArgs(wsID).
		WillReturnRows(rows)

	count, err := repo.CountWorkspaceMembers(context.Background(), wsID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 5 {
		t.Errorf("expected count 5, got %d", count)
	}
}
