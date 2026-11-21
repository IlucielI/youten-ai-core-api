package repositories

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	gormPostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"

	"code-base-golang/internal/models"
)

func setupMockFolderDB(t *testing.T) (*Repositories, sqlmock.Sqlmock, func()) {
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

func TestRepositories_CreateFolder(t *testing.T) {
	repo, mock, cleanup := setupMockFolderDB(t)
	defer cleanup()

	folderID := uuid.New()
	wsID := uuid.New()
	folder := &models.Folder{
		BaseModel:   models.BaseModel{ID: folderID},
		WorkspaceID: wsID,
		Name:        "Sprint Planning",
		Color:       "#4F46E5",
	}

	rows := sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).
		AddRow(folderID, time.Now(), time.Now())

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "folders"`)).
		WillReturnRows(rows)
	mock.ExpectCommit()

	err := repo.CreateFolder(context.Background(), folder)
	if err != nil {
		t.Fatalf("unexpected error creating folder: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}

func TestRepositories_FindFolderByID(t *testing.T) {
	repo, mock, cleanup := setupMockFolderDB(t)
	defer cleanup()

	folderID := uuid.New()
	wsID := uuid.New()

	rows := sqlmock.NewRows([]string{"id", "workspace_id", "name", "color", "deleted_at"}).
		AddRow(folderID, wsID, "Design Syncs", "#4F46E5", nil)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "folders" WHERE id = $1 AND "folders"."deleted_at" IS NULL ORDER BY "folders"."id" LIMIT $2`)).
		WithArgs(folderID, 1).
		WillReturnRows(rows)

	found, err := repo.FindFolderByID(context.Background(), folderID)
	if err != nil {
		t.Fatalf("unexpected error finding folder: %v", err)
	}
	if found.Name != "Design Syncs" {
		t.Errorf("expected folder name 'Design Syncs', got %s", found.Name)
	}
}

func TestRepositories_FindFoldersByWorkspaceID(t *testing.T) {
	repo, mock, cleanup := setupMockFolderDB(t)
	defer cleanup()

	wsID := uuid.New()
	f1ID := uuid.New()
	f2ID := uuid.New()

	rows := sqlmock.NewRows([]string{"id", "workspace_id", "name", "color", "deleted_at"}).
		AddRow(f1ID, wsID, "Architecture", "#10B981", nil).
		AddRow(f2ID, wsID, "Backlog", "#3B82F6", nil)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "folders" WHERE workspace_id = $1 AND "folders"."deleted_at" IS NULL ORDER BY name ASC`)).
		WithArgs(wsID).
		WillReturnRows(rows)

	list, err := repo.FindFoldersByWorkspaceID(context.Background(), wsID)
	if err != nil {
		t.Fatalf("unexpected error finding folders: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 folders, got %d", len(list))
	}
}

func TestRepositories_UpdateFolder(t *testing.T) {
	repo, mock, cleanup := setupMockFolderDB(t)
	defer cleanup()

	folderID := uuid.New()
	wsID := uuid.New()
	folder := &models.Folder{
		BaseModel:   models.BaseModel{ID: folderID},
		WorkspaceID: wsID,
		Name:        "Updated Name",
		Color:       "#EF4444",
	}

	// Test success
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "folders" SET`)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := repo.UpdateFolder(context.Background(), folder)
	if err != nil {
		t.Fatalf("unexpected error updating folder: %v", err)
	}

	// Test record not found
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "folders" SET`)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	err = repo.UpdateFolder(context.Background(), folder)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected ErrRecordNotFound, got: %v", err)
	}
}

func TestRepositories_DeleteFolder(t *testing.T) {
	repo, mock, cleanup := setupMockFolderDB(t)
	defer cleanup()

	folderID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "folders" SET "deleted_at"=$1 WHERE id = $2 AND "folders"."deleted_at" IS NULL`)).
		WithArgs(sqlmock.AnyArg(), folderID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := repo.DeleteFolder(context.Background(), folderID)
	if err != nil {
		t.Fatalf("unexpected error deleting folder: %v", err)
	}
}

func TestRepositories_IsDescendantOf(t *testing.T) {
	repo, mock, cleanup := setupMockFolderDB(t)
	defer cleanup()

	fRoot := uuid.New()
	fChild := uuid.New()

	// Direct self check
	isDesc, err := repo.IsDescendantOf(context.Background(), fRoot, fRoot)
	if err != nil || !isDesc {
		t.Fatalf("expected self reference to be true, got %v, err: %v", isDesc, err)
	}

	// Child check
	rows := sqlmock.NewRows([]string{"id", "parent_id"}).
		AddRow(fChild, fRoot)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, parent_id FROM "folders" WHERE id = $1 AND "folders"."deleted_at" IS NULL ORDER BY "folders"."id" LIMIT $2`)).
		WithArgs(fChild, 1).
		WillReturnRows(rows)

	isDesc, err = repo.IsDescendantOf(context.Background(), fRoot, fChild)
	if err != nil || !isDesc {
		t.Fatalf("expected child to be descendant of root, got %v, err: %v", isDesc, err)
	}
}

func TestRepositories_MoveRecording(t *testing.T) {
	repo, mock, cleanup := setupMockFolderDB(t)
	defer cleanup()

	recID := uuid.New()
	folderID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "recordings" SET "folder_id"=$1,"updated_at"=$2 WHERE id = $3 AND "recordings"."deleted_at" IS NULL`)).
		WithArgs(folderID, sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := repo.MoveRecording(context.Background(), recID, &folderID)
	if err != nil {
		t.Fatalf("unexpected error moving recording: %v", err)
	}
}
