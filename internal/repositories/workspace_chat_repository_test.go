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

	"code-base-golang/internal/models"
)

func setupMockChatDB(t *testing.T) (*Repositories, sqlmock.Sqlmock, func()) {
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
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("there were unfulfilled expectations: %s", err)
		}
		sqlDB.Close()
	}
	return repo, mock, cleanup
}

func TestRepositories_CreateWorkspaceChatSession(t *testing.T) {
	repo, mock, cleanup := setupMockChatDB(t)
	defer cleanup()

	sessID := uuid.New()
	wsID := uuid.New()
	userID := uuid.New()
	session := &models.WorkspaceChatSession{
		BaseModel:   models.BaseModel{ID: sessID},
		WorkspaceID: wsID,
		UserID:      userID,
		Title:       "Test Session",
	}

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "workspace_chat_sessions"`)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).
			AddRow(sessID, time.Now(), time.Now()))
	mock.ExpectCommit()

	err := repo.CreateWorkspaceChatSession(context.Background(), session)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRepositories_FindWorkspaceChatSessionByID(t *testing.T) {
	repo, mock, cleanup := setupMockChatDB(t)
	defer cleanup()

	sessID := uuid.New()
	wsID := uuid.New()
	userID := uuid.New()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspace_chat_sessions" WHERE id = $1 AND "workspace_chat_sessions"."deleted_at" IS NULL`)).
		WithArgs(sessID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "workspace_id", "user_id", "title"}).
			AddRow(sessID, wsID, userID, "Sprint Discussion"))

	res, err := repo.FindWorkspaceChatSessionByID(context.Background(), sessID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Title != "Sprint Discussion" {
		t.Errorf("expected title 'Sprint Discussion', got %s", res.Title)
	}
}

func TestRepositories_FindWorkspaceChatSessions(t *testing.T) {
	repo, mock, cleanup := setupMockChatDB(t)
	defer cleanup()

	sessID := uuid.New()
	wsID := uuid.New()
	userID := uuid.New()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspace_chat_sessions" WHERE (workspace_id = $1 AND user_id = $2) AND folder_id IS NULL AND "workspace_chat_sessions"."deleted_at" IS NULL ORDER BY updated_at DESC`)).
		WithArgs(wsID, userID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "workspace_id", "user_id", "title", "updated_at"}).
			AddRow(sessID, wsID, userID, "Folder Chat", time.Now()))

	res, err := repo.FindWorkspaceChatSessions(context.Background(), wsID, userID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("expected 1 session, got %d", len(res))
	}
}

func TestRepositories_CreateAndFindWorkspaceChatMessage(t *testing.T) {
	repo, mock, cleanup := setupMockChatDB(t)
	defer cleanup()

	msgID := uuid.New()
	sessID := uuid.New()
	msg := &models.WorkspaceChatMessage{
		BaseModel:  models.BaseModel{ID: msgID},
		SessionID:  sessID,
		SenderRole: "user",
		Content:    "What were the action items?",
	}

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "workspace_chat_messages"`)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "citations", "retrieved_chunk_ids"}).
			AddRow(msgID, time.Now(), time.Now(), []byte("[]"), []byte("[]")))
	mock.ExpectCommit()

	if err := repo.CreateWorkspaceChatMessage(context.Background(), msg); err != nil {
		t.Fatalf("unexpected error creating message: %v", err)
	}

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspace_chat_messages" WHERE session_id = $1 AND "workspace_chat_messages"."deleted_at" IS NULL ORDER BY created_at ASC`)).
		WithArgs(sessID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "session_id", "sender_role", "content"}).
			AddRow(msgID, sessID, "user", "What were the action items?"))

	msgs, err := repo.FindWorkspaceChatMessagesBySession(context.Background(), sessID)
	if err != nil {
		t.Fatalf("unexpected error fetching messages: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Content != "What were the action items?" {
		t.Errorf("unexpected message result: %+v", msgs)
	}
}

func TestRepositories_SearchWorkspaceTranscriptChunksByFolderRecursive(t *testing.T) {
	repo, mock, cleanup := setupMockChatDB(t)
	defer cleanup()

	wsID := uuid.New()
	folderID := uuid.New()
	chunkID := uuid.New()
	recID := uuid.New()
	embedding := []float32{0.1, 0.2, 0.3}

	mock.ExpectQuery(`WITH RECURSIVE folder_tree AS`).
		WithArgs(folderID, wsID, wsID, models.Vector(embedding), models.Vector(embedding), wsID, models.Vector(embedding), 0.45, 5).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "recording_title", "folder_id", "folder_name", "chunk_index", "content", "start_time", "end_time", "distance"}).
			AddRow(chunkID, recID, "Sprint Review", folderID, "Dev Folder", 0, "Discussion on Q4 roadmap", 0.0, 10.0, 0.25))

	results, err := repo.SearchWorkspaceTranscriptChunksByFolderRecursive(context.Background(), wsID, folderID, embedding, 5, 0.45)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].RecordingTitle != "Sprint Review" {
		t.Errorf("expected recording title 'Sprint Review', got %s", results[0].RecordingTitle)
	}
}

func TestRepositories_CountWorkspaceChatMessagesBySessions(t *testing.T) {
	repo, mock, cleanup := setupMockChatDB(t)
	defer cleanup()

	sessID1 := uuid.New()
	sessID2 := uuid.New()

	mock.ExpectQuery(`SELECT session_id, count\(\*\) as count FROM "workspace_chat_messages"`).
		WithArgs(sessID1, sessID2).
		WillReturnRows(sqlmock.NewRows([]string{"session_id", "count"}).
			AddRow(sessID1, 5).
			AddRow(sessID2, 3))

	res, err := repo.CountWorkspaceChatMessagesBySessions(context.Background(), []uuid.UUID{sessID1, sessID2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res[sessID1] != 5 || res[sessID2] != 3 {
		t.Errorf("unexpected counts: %+v", res)
	}
}

func TestRepositories_GetWorkspaceMeetingCatalog_WorkspaceLevel(t *testing.T) {
	repo, mock, cleanup := setupMockChatDB(t)
	defer cleanup()

	wsID := uuid.New()
	recID := uuid.New()
	now := time.Now()

	mock.ExpectQuery(`SELECT r.id AS recording_id, r.title, r.folder_id, f.name AS folder_name, r.created_at, r.duration_seconds`).
		WithArgs(wsID, 50).
		WillReturnRows(sqlmock.NewRows([]string{"recording_id", "title", "folder_id", "folder_name", "created_at", "duration_seconds", "overview"}).
			AddRow(recID, "All-Hands Meeting", nil, "", now, 1200.0, "Company quarterly progress review."))

	results, err := repo.GetWorkspaceMeetingCatalog(context.Background(), wsID, nil, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 record, got %d", len(results))
	}
	if results[0].Title != "All-Hands Meeting" {
		t.Errorf("expected title 'All-Hands Meeting', got %s", results[0].Title)
	}
	if results[0].Overview != "Company quarterly progress review." {
		t.Errorf("expected overview, got %s", results[0].Overview)
	}
}

func TestRepositories_GetWorkspaceMeetingCatalog_FolderLevel(t *testing.T) {
	repo, mock, cleanup := setupMockChatDB(t)
	defer cleanup()

	wsID := uuid.New()
	folderID := uuid.New()
	recID := uuid.New()
	now := time.Now()

	mock.ExpectQuery(`WITH RECURSIVE folder_tree AS`).
		WithArgs(folderID, wsID, wsID, wsID, 10).
		WillReturnRows(sqlmock.NewRows([]string{"recording_id", "title", "folder_id", "folder_name", "created_at", "duration_seconds", "overview"}).
			AddRow(recID, "Sprint Review", folderID, "Dev Folder", now, 600.0, "Review of sprint backlog."))

	results, err := repo.GetWorkspaceMeetingCatalog(context.Background(), wsID, &folderID, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 record, got %d", len(results))
	}
	if results[0].FolderName != "Dev Folder" {
		t.Errorf("expected folder name 'Dev Folder', got %s", results[0].FolderName)
	}
}
