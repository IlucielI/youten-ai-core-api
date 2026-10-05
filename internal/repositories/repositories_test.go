package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-redis/redismock/v9"
	"github.com/google/uuid"
	gormPostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"

	"code-base-golang/internal/adapters/redis"
	"code-base-golang/internal/constants"
	"code-base-golang/internal/models"
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

func TestRepositories_SummaryVersionOperations(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
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
	recID := uuid.New()

	// 1. CountSummaryVersions
	mock.ExpectQuery(`SELECT count\(\*\) FROM "summaries" WHERE recording_id = \$1`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	count, err := repo.CountSummaryVersions(context.Background(), recID)
	if err != nil {
		t.Fatalf("unexpected error counting summaries: %v", err)
	}
	if count != 2 {
		t.Errorf("expected count 2, got %d", count)
	}

	// 2. GetLatestSummaryVersion
	mock.ExpectQuery(`SELECT COALESCE\(MAX\(version\), 0\) FROM "summaries" WHERE recording_id = \$1`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"coalesce"}).AddRow(2))

	maxVer, err := repo.GetLatestSummaryVersion(context.Background(), recID)
	if err != nil {
		t.Fatalf("unexpected error getting max version: %v", err)
	}
	if maxVer != 2 {
		t.Errorf("expected maxVer 2, got %d", maxVer)
	}

	// 3. SaveNewSummaryVersion - Success (incrementing to version 3)
	summary := models.Summary{
		ID:               uuid.New(),
		RecordingID:      recID,
		TemplateCategory: "GENERAL",
		StructuredData:   models.JSONMap{"summary": "test"},
		MarkdownContent:  "# Summary",
	}

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT count\(\*\) FROM "summaries" WHERE recording_id = \$1`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectQuery(`SELECT COALESCE\(MAX\(version\), 0\) FROM "summaries" WHERE recording_id = \$1`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"coalesce"}).AddRow(2))
	mock.ExpectExec(`UPDATE "summaries" SET "is_active"=\$1,"updated_at"=\$2 WHERE recording_id = \$3`).
		WithArgs(false, sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 2))
	mock.ExpectQuery(`INSERT INTO "summaries"`).
		WithArgs(recID, "GENERAL", nil, 3, true, sqlmock.AnyArg(), "# Summary", summary.ID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(summary.ID, time.Now(), time.Now()))
	mock.ExpectCommit()

	if err := repo.SaveNewSummaryVersion(context.Background(), &summary); err != nil {
		t.Fatalf("unexpected error saving new summary version: %v", err)
	}
	if summary.Version != 3 {
		t.Errorf("expected version 3, got %d", summary.Version)
	}
	if !summary.IsActive {
		t.Errorf("expected is_active true, got %v", summary.IsActive)
	}

	// 4. SaveNewSummaryVersion - Version Cap Reached (count >= 5)
	summaryOverLimit := models.Summary{
		ID:               uuid.New(),
		RecordingID:      recID,
		TemplateCategory: "GENERAL",
	}

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT count\(\*\) FROM "summaries" WHERE recording_id = \$1`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(5))
	mock.ExpectRollback()

	err = repo.SaveNewSummaryVersion(context.Background(), &summaryOverLimit)
	if err == nil {
		t.Fatal("expected error when limit reached, got nil")
	}

	// 5. ListSummaryVersions - ordered by version ASC
	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 ORDER BY version ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "version", "is_active"}).
			AddRow(uuid.New(), recID, 1, false).
			AddRow(uuid.New(), recID, 2, true))

	versions, err := repo.ListSummaryVersions(context.Background(), recID)
	if err != nil {
		t.Fatalf("unexpected error listing summary versions: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(versions))
	}
	if versions[0].Version != 1 || versions[1].Version != 2 {
		t.Errorf("expected versions in ASC order (1, 2), got (%d, %d)", versions[0].Version, versions[1].Version)
	}

	// 6. ActivateSummaryVersion - Success by UUID
	targetSumID := uuid.New()
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND id = \$2.*LIMIT \$3`).
		WithArgs(recID, targetSumID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "version", "is_active"}).
			AddRow(targetSumID, recID, 1, false))
	mock.ExpectExec(`UPDATE "summaries" SET "is_active"=\$1,"updated_at"=\$2 WHERE recording_id = \$3`).
		WithArgs(false, sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 2))
	mock.ExpectExec(`UPDATE "summaries" SET "is_active"=\$1,"updated_at"=\$2 WHERE id = \$3`).
		WithArgs(true, sqlmock.AnyArg(), targetSumID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	actSum, err := repo.ActivateSummaryVersion(context.Background(), recID, targetSumID.String())
	if err != nil {
		t.Fatalf("unexpected error activating summary version: %v", err)
	}
	if !actSum.IsActive {
		t.Errorf("expected activated summary is_active = true, got %v", actSum.IsActive)
	}

	// 7. ActivateSummaryVersion - Target Version Not Found
	missingSumID := uuid.New()
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND id = \$2.*LIMIT \$3`).
		WithArgs(recID, missingSumID, 1).
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectRollback()

	_, err = repo.ActivateSummaryVersion(context.Background(), recID, missingSumID.String())
	if !errors.Is(err, constants.ErrSummaryNotFound) {
		t.Fatalf("expected ErrSummaryNotFound, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestRepositories_InlineCommentOperations(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer sqlDB.Close()

	gormDB, err := gorm.Open(gormPostgres.New(gormPostgres.Config{
		Conn: sqlDB,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open gorm db: %v", err)
	}

	repo := New(gormDB, nil)
	recID := uuid.New()
	commID := uuid.New()

	comment := models.InlineComment{
		ID:           commID,
		RecordingID:  recID,
		TimestampSec: 10.5,
		AuthorName:   "Bayu",
		CommentText:  "Action item here",
	}

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "inline_comments"`).
		WithArgs(recID, nil, 10.5, nil, "Bayu", "Action item here", nil, commID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(commID, time.Now(), time.Now()))
	mock.ExpectCommit()

	if err := repo.CreateInlineComment(context.Background(), &comment); err != nil {
		t.Fatalf("unexpected error creating inline comment: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestRepositories_ListInlineCommentsByRecordingID(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer sqlDB.Close()

	gormDB, err := gorm.Open(gormPostgres.New(gormPostgres.Config{
		Conn: sqlDB,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open gorm db: %v", err)
	}

	repo := New(gormDB, nil)
	recID := uuid.New()
	commID := uuid.New()
	replyID := uuid.New()

	mock.ExpectQuery(`SELECT \* FROM "inline_comments" WHERE recording_id = \$1 AND parent_id IS NULL ORDER BY timestamp_sec ASC, created_at ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "timestamp_sec", "comment_text", "parent_id"}).
			AddRow(commID, recID, 12.0, "Top level comment", nil))

	mock.ExpectQuery(`SELECT \* FROM "inline_comments" WHERE "inline_comments"\."parent_id" = \$1`).
		WithArgs(commID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "timestamp_sec", "comment_text", "parent_id"}).
			AddRow(replyID, recID, 12.0, "Child reply", commID))

	comments, err := repo.ListInlineCommentsByRecordingID(context.Background(), recID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(comments) != 1 {
		t.Fatalf("expected 1 top-level comment, got %d", len(comments))
	}
	if len(comments[0].Replies) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(comments[0].Replies))
	}
	if comments[0].Replies[0].ID != replyID {
		t.Errorf("expected reply ID %v, got %v", replyID, comments[0].Replies[0].ID)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}



