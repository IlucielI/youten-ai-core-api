package repositories

import (
	"context"
	"errors"
	"regexp"
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
		WithArgs(recID, nil, nil, 10.5, nil, "Bayu", "Action item here", nil, commID).
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

func TestRepositories_DeleteInlineComment(t *testing.T) {
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
	commID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM "inline_comments" WHERE id = \$1`).
		WithArgs(commID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	if err := repo.DeleteInlineComment(context.Background(), commID); err != nil {
		t.Fatalf("unexpected error deleting comment: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestRepositories_AggregateWorkspaceSpeakers(t *testing.T) {
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
	userID := uuid.New()
	now := time.Now().Truncate(time.Second)

	expectedSQL := regexp.QuoteMeta(`SELECT ts.speaker_name AS name, COUNT(DISTINCT ts.recording_id) AS total_meetings, COALESCE(SUM(GREATEST(0, ts.end_time - ts.start_time)), 0) AS total_talk_time, MAX(r.created_at) AS last_active FROM transcript_segments ts JOIN recordings r ON ts.recording_id = r.id WHERE r.user_id = $1 AND r.deleted_at IS NULL AND TRIM(ts.speaker_name) != '' GROUP BY "ts"."speaker_name" ORDER BY total_meetings DESC, total_talk_time DESC, name ASC`)

	t.Run("success with multiple speakers", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{"name", "total_meetings", "total_talk_time", "last_active"}).
			AddRow("Alice", 3, 450.5, now).
			AddRow("Bob", 1, 120.0, now.Add(-time.Hour))

		mock.ExpectQuery(expectedSQL).
			WithArgs(userID).
			WillReturnRows(rows)

		stats, err := repo.AggregateWorkspaceSpeakers(context.Background(), userID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(stats) != 2 {
			t.Fatalf("expected 2 stats, got %d", len(stats))
		}
		if stats[0].Name != "Alice" || stats[0].TotalMeetings != 3 || stats[0].TotalTalkTime != 450.5 {
			t.Errorf("unexpected stat 0: %+v", stats[0])
		}
		if stats[1].Name != "Bob" || stats[1].TotalMeetings != 1 || stats[1].TotalTalkTime != 120.0 {
			t.Errorf("unexpected stat 1: %+v", stats[1])
		}
	})

	t.Run("database error", func(t *testing.T) {
		mock.ExpectQuery(expectedSQL).
			WithArgs(userID).
			WillReturnError(errors.New("db query error"))

		stats, err := repo.AggregateWorkspaceSpeakers(context.Background(), userID)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if stats != nil {
			t.Fatalf("expected nil stats on error, got: %+v", stats)
		}
	})

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestRepositories_Waitlist(t *testing.T) {
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

	t.Run("upsert waitlist success", func(t *testing.T) {
		entry := &models.BotWaitlist{
			ID:          uuid.New(),
			Email:       "test@example.com",
			Platform:    "zoom",
			CompanySize: "11-50",
			Status:      "PENDING",
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		mock.ExpectBegin()
		mock.ExpectQuery(`INSERT INTO "bot_waitlists"`).
			WithArgs(entry.Email, entry.Platform, entry.CompanySize, entry.Status, entry.ID, sqlmock.AnyArg(), sqlmock.AnyArg()).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(entry.ID, entry.CreatedAt, entry.UpdatedAt))
		mock.ExpectCommit()

		saved, err := repo.UpsertBotWaitlist(context.Background(), entry)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if saved == nil || saved.Email != "test@example.com" {
			t.Fatalf("unexpected saved entry: %+v", saved)
		}
	})

	t.Run("upsert waitlist error", func(t *testing.T) {
		entry := &models.BotWaitlist{
			ID:    uuid.New(),
			Email: "err@example.com",
		}

		mock.ExpectBegin()
		mock.ExpectQuery(`INSERT INTO "bot_waitlists"`).
			WillReturnError(errors.New("db insert error"))
		mock.ExpectRollback()

		saved, err := repo.UpsertBotWaitlist(context.Background(), entry)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if saved != nil {
			t.Fatalf("expected nil on error, got: %+v", saved)
		}
	})

	t.Run("find by email success", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{"id", "email", "platform", "company_size", "status"}).
			AddRow(uuid.New(), "found@example.com", "google_meet", "1-10", "PENDING")

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "bot_waitlists" WHERE LOWER(email) = LOWER($1) ORDER BY "bot_waitlists"."id" LIMIT $2`)).
			WithArgs("found@example.com", 1).
			WillReturnRows(rows)

		found, err := repo.FindBotWaitlistByEmail(context.Background(), "found@example.com")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if found == nil || found.Email != "found@example.com" {
			t.Fatalf("unexpected found entry: %+v", found)
		}
	})

	t.Run("find by email not found", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "bot_waitlists" WHERE LOWER(email) = LOWER($1) ORDER BY "bot_waitlists"."id" LIMIT $2`)).
			WithArgs("missing@example.com", 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "email"}))

		found, err := repo.FindBotWaitlistByEmail(context.Background(), "missing@example.com")
		if err != nil {
			t.Fatalf("unexpected error on not found: %v", err)
		}
		if found != nil {
			t.Fatalf("expected nil when not found, got: %+v", found)
		}
	})

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestRepositories_ClientApp(t *testing.T) {
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
	appID := uuid.New()
	clientID := "client-app"

	t.Run("CreateClientApp success", func(t *testing.T) {
		mock.ExpectBegin()
		retRows := sqlmock.NewRows([]string{"id", "allowed_scopes", "created_at", "updated_at"}).
			AddRow(appID, []byte(`["recordings:create"]`), time.Now(), time.Now())
		mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "client_apps"`)).
			WillReturnRows(retRows)
		mock.ExpectCommit()

		app := &models.ClientApp{
			ID:               appID,
			ClientID:         clientID,
			ClientSecretHash: "hashed",
			Name:             "Official Client",
			AllowedScopes:    []byte(`["recordings:create"]`),
			IsActive:         true,
		}
		if err := repo.CreateClientApp(context.Background(), app); err != nil {
			t.Fatalf("unexpected error creating client app: %v", err)
		}
	})

	t.Run("FindClientAppByID success", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{"id", "client_id", "name", "is_active"}).
			AddRow(appID, clientID, "Official Client", true)

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "client_apps" WHERE id = $1 ORDER BY "client_apps"."id" LIMIT $2`)).
			WithArgs(appID, 1).
			WillReturnRows(rows)

		found, err := repo.FindClientAppByID(context.Background(), appID)
		if err != nil {
			t.Fatalf("unexpected error finding client app by ID: %v", err)
		}
		if found == nil || found.ClientID != clientID {
			t.Fatalf("unexpected client app: %+v", found)
		}
	})

	t.Run("FindClientAppByClientID success", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{"id", "client_id", "name", "is_active"}).
			AddRow(appID, clientID, "Official Client", true)

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "client_apps" WHERE client_id = $1 ORDER BY "client_apps"."id" LIMIT $2`)).
			WithArgs(clientID, 1).
			WillReturnRows(rows)

		found, err := repo.FindClientAppByClientID(context.Background(), clientID)
		if err != nil {
			t.Fatalf("unexpected error finding client app by client ID: %v", err)
		}
		if found == nil || found.ID != appID {
			t.Fatalf("unexpected client app: %+v", found)
		}
	})

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}







