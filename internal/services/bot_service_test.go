package services_test

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"

	"code-base-golang/internal/config"
	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
	"code-base-golang/internal/pkg/ctxmeta"
	"code-base-golang/internal/services"
)

type testBotProvider struct {
	name      string
	enabled   bool
	extID     string
	status    string
	dispErr   error
	statusErr error
	stopErr   error
}

func (p *testBotProvider) ProviderName() string {
	return p.name
}

func (p *testBotProvider) IsEnabled() bool {
	return p.enabled
}

func (p *testBotProvider) Dispatch(ctx context.Context, params services.BotDispatchParams) (string, error) {
	if p.dispErr != nil {
		return "", p.dispErr
	}
	return p.extID, nil
}

func (p *testBotProvider) GetStatus(ctx context.Context, externalSessionID string) (*services.BotProviderStatus, error) {
	if p.statusErr != nil {
		return nil, p.statusErr
	}
	return &services.BotProviderStatus{
		ExternalSessionID: externalSessionID,
		Status:            p.status,
	}, nil
}

func (p *testBotProvider) Stop(ctx context.Context, externalSessionID string) error {
	return p.stopErr
}

func TestService_DispatchMeetingBot_Success(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)

	provider := &testBotProvider{
		name:    constants.BotProviderDiscord,
		enabled: true,
		extID:   "discord-ext-123",
		status:  constants.BotSessionStatusRecording,
	}
	svc.RegisterBotProvider(provider)

	ctx := ctxmeta.WithClientMeta(context.Background(), "127.0.0.1", "TestAgent")

	// Quota check query for guest
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "recordings" WHERE (is_guest = TRUE AND guest_ip = $1 AND created_at >= $2) AND "recordings"."deleted_at" IS NULL`)).
		WithArgs("127.0.0.1", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	// Insert recording
	recID := uuid.New()
	sessID := uuid.New()
	now := time.Now()

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "recordings"`)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(recID, now, now))
	mock.ExpectCommit()

	// Insert bot session
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "bot_sessions"`)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(sessID, now, now))
	mock.ExpectCommit()

	// Update bot session external id
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "bot_sessions" SET "external_session_id"=$1,"updated_at"=$2 WHERE id = $3`)).
		WithArgs("discord-ext-123", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	// Update bot session status to recording
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "bot_sessions" SET`)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	req := dtos.DispatchBotRequest{
		Provider:  constants.BotProviderDiscord,
		ChannelID: "chan-123",
		GuildID:   "guild-456",
		Title:     "Sprint Planning",
	}

	resp, err := svc.DispatchMeetingBot(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error dispatching bot: %v", err)
	}

	if resp.Provider != constants.BotProviderDiscord {
		t.Errorf("expected provider %s, got %s", constants.BotProviderDiscord, resp.Provider)
	}
	if resp.Status != constants.BotSessionStatusRecording {
		t.Errorf("expected status %s, got %s", constants.BotSessionStatusRecording, resp.Status)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %s", err)
	}
}

func TestService_DispatchMeetingBot_UnsupportedAndDisabled(t *testing.T) {
	svc, _, _, _ := setupRecordingTestService(t)
	ctx := context.Background()

	t.Run("unsupported provider returns error", func(t *testing.T) {
		_, err := svc.DispatchMeetingBot(ctx, dtos.DispatchBotRequest{
			Provider: "unsupported",
		})
		if err == nil {
			t.Errorf("expected error for unsupported provider")
		}
	})

	t.Run("disabled provider returns coming soon error", func(t *testing.T) {
		disabledProvider := &testBotProvider{
			name:    constants.BotProviderGoogleMeet,
			enabled: false,
		}
		svc.RegisterBotProvider(disabledProvider)

		_, err := svc.DispatchMeetingBot(ctx, dtos.DispatchBotRequest{
			Provider: constants.BotProviderGoogleMeet,
		})
		if err == nil {
			t.Errorf("expected error for disabled provider")
		}
	})
}

func TestService_GetBotSessionStatus_And_Stop(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	ctx := context.Background()

	sessID := uuid.New()
	recID := uuid.New()
	extID := "ext-sess-789"
	now := time.Now()

	t.Run("get session status success", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "bot_sessions" WHERE id = $1 AND "bot_sessions"."deleted_at" IS NULL ORDER BY "bot_sessions"."id" LIMIT $2`)).
			WithArgs(sessID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "provider", "external_session_id", "status", "created_at", "updated_at"}).
				AddRow(sessID, recID, constants.BotProviderDiscord, extID, constants.BotSessionStatusRecording, now, now))

		res, err := svc.GetBotSessionStatus(ctx, sessID)
		if err != nil {
			t.Fatalf("unexpected error getting status: %v", err)
		}
		if res.SessionID != sessID {
			t.Errorf("expected session ID %v, got %v", sessID, res.SessionID)
		}
		if res.Status != constants.BotSessionStatusRecording {
			t.Errorf("expected status %s, got %s", constants.BotSessionStatusRecording, res.Status)
		}
	})

	t.Run("stop bot session success", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "bot_sessions" WHERE id = $1 AND "bot_sessions"."deleted_at" IS NULL ORDER BY "bot_sessions"."id" LIMIT $2`)).
			WithArgs(sessID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "provider", "external_session_id", "status", "created_at", "updated_at"}).
				AddRow(sessID, recID, constants.BotProviderDiscord, extID, constants.BotSessionStatusRecording, now, now))

		// Update bot session status to completed
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "bot_sessions" SET`)).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		// Lookup recording and update status
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "recordings" WHERE id = $1 AND "recordings"."deleted_at" IS NULL ORDER BY "recordings"."id" LIMIT $2`)).
			WithArgs(recID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).AddRow(recID, constants.BotSessionStatusRecording))

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "recordings" SET "status"=$1,"updated_at"=$2 WHERE "recordings"."deleted_at" IS NULL AND "id" = $3`)).
			WithArgs(models.RecordingStatusPending, sqlmock.AnyArg(), recID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		res, err := svc.StopBotSession(ctx, sessID)
		if err != nil {
			t.Fatalf("unexpected error stopping session: %v", err)
		}
		if res.Status != constants.BotSessionStatusCompleted {
			t.Errorf("expected status %s, got %s", constants.BotSessionStatusCompleted, res.Status)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled expectations: %s", err)
		}
	})
}

func TestService_GetCapabilities(t *testing.T) {
	svc, _, _, _ := setupRecordingTestService(t)

	discordProvider := &testBotProvider{
		name:    constants.BotProviderDiscord,
		enabled: true,
	}
	meetProvider := &testBotProvider{
		name:    constants.BotProviderGoogleMeet,
		enabled: false,
	}
	svc.RegisterBotProvider(discordProvider)
	svc.RegisterBotProvider(meetProvider)

	caps := svc.GetCapabilities(context.Background())
	if caps.MeetingBot[constants.BotProviderDiscord] != "available" {
		t.Errorf("expected discord to be available, got %s", caps.MeetingBot[constants.BotProviderDiscord])
	}
	if caps.MeetingBot[constants.BotProviderGoogleMeet] != "coming_soon" {
		t.Errorf("expected google_meet to be coming_soon, got %s", caps.MeetingBot[constants.BotProviderGoogleMeet])
	}
	if caps.MeetingBot[constants.BotProviderMSTeams] != "coming_soon" {
		t.Errorf("expected ms_teams to be coming_soon, got %s", caps.MeetingBot[constants.BotProviderMSTeams])
	}
	if caps.MeetingBot[constants.BotProviderZoom] != "coming_soon" {
		t.Errorf("expected zoom to be coming_soon, got %s", caps.MeetingBot[constants.BotProviderZoom])
	}

	t.Run("nil provider registration does not panic", func(t *testing.T) {
		svc.RegisterBotProvider(nil)
	})
}

func TestService_HandleGoogleMeetWebhook(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid webhook secret", func(t *testing.T) {
		cfg := config.Config{
			GoogleMeetBotWebhookSecret: "super-secret",
		}
		svc := services.New(cfg, nil, nil, nil)

		err := svc.HandleGoogleMeetWebhook(ctx, dtos.GoogleMeetWebhookRequest{
			ExternalSessionID: "meet-123",
			Event:             "joined",
		}, "wrong-secret")
		if err == nil {
			t.Errorf("expected unauthorized error for invalid secret")
		}
	})

	t.Run("session not found", func(t *testing.T) {
		svc, mock, _, _ := setupRecordingTestService(t)

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "bot_sessions" WHERE external_session_id = $1 AND "bot_sessions"."deleted_at" IS NULL ORDER BY "bot_sessions"."id" LIMIT $2`)).
			WithArgs("meet-unknown", 1).
			WillReturnRows(sqlmock.NewRows([]string{"id"}))

		err := svc.HandleGoogleMeetWebhook(ctx, dtos.GoogleMeetWebhookRequest{
			ExternalSessionID: "meet-unknown",
			Event:             "joined",
		}, "")
		if err == nil {
			t.Errorf("expected not found error")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled mock expectations: %v", err)
		}
	})

	t.Run("unrecognized event", func(t *testing.T) {
		svc, mock, _, _ := setupRecordingTestService(t)
		sessID := uuid.New()
		recID := uuid.New()

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "bot_sessions" WHERE external_session_id = $1 AND "bot_sessions"."deleted_at" IS NULL ORDER BY "bot_sessions"."id" LIMIT $2`)).
			WithArgs("meet-123", 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "external_session_id"}).AddRow(sessID, recID, "meet-123"))

		err := svc.HandleGoogleMeetWebhook(ctx, dtos.GoogleMeetWebhookRequest{
			ExternalSessionID: "meet-123",
			Event:             "invalid_event",
		}, "")
		if err == nil {
			t.Errorf("expected error for unrecognized event")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled mock expectations: %v", err)
		}
	})

	t.Run("completed with audio url", func(t *testing.T) {
		svc, mock, _, _ := setupRecordingTestService(t)
		sessID := uuid.New()
		recID := uuid.New()
		audioURL := "s3://recordings/meet-123.wav"
		durationSec := 120

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "bot_sessions" WHERE external_session_id = $1 AND "bot_sessions"."deleted_at" IS NULL ORDER BY "bot_sessions"."id" LIMIT $2`)).
			WithArgs("meet-123", 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "external_session_id"}).AddRow(sessID, recID, "meet-123"))

		// Update bot session status
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "bot_sessions" SET "ended_at"=$1,"status"=$2,"updated_at"=$3 WHERE id = $4`)).
			WithArgs(sqlmock.AnyArg(), constants.BotSessionStatusCompleted, sqlmock.AnyArg(), sessID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		// Update recording audio url
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "recordings" SET "audio_url"=$1,"duration_seconds"=$2,"updated_at"=$3 WHERE id = $4 AND "recordings"."deleted_at" IS NULL`)).
			WithArgs(audioURL, float64(durationSec), sqlmock.AnyArg(), recID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		// Update recording status to TRANSCRIBING
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "recordings" SET "status"=$1,"updated_at"=$2 WHERE id = $3 AND "recordings"."deleted_at" IS NULL`)).
			WithArgs(models.RecordingStatusTranscribing, sqlmock.AnyArg(), recID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := svc.HandleGoogleMeetWebhook(ctx, dtos.GoogleMeetWebhookRequest{
			ExternalSessionID: "meet-123",
			Event:             "completed",
			AudioURL:          &audioURL,
			DurationSeconds:   &durationSec,
		}, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled mock expectations: %v", err)
		}
	})

	t.Run("completed with audio url db update failure", func(t *testing.T) {
		svc, mock, _, _ := setupRecordingTestService(t)
		sessID := uuid.New()
		recID := uuid.New()
		audioURL := "s3://recordings/meet-123.wav"
		durationSec := 120

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "bot_sessions" WHERE external_session_id = $1 AND "bot_sessions"."deleted_at" IS NULL ORDER BY "bot_sessions"."id" LIMIT $2`)).
			WithArgs("meet-123", 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "external_session_id"}).AddRow(sessID, recID, "meet-123"))

		// Update bot session status
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "bot_sessions" SET "ended_at"=$1,"status"=$2,"updated_at"=$3 WHERE id = $4`)).
			WithArgs(sqlmock.AnyArg(), constants.BotSessionStatusCompleted, sqlmock.AnyArg(), sessID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		// Update recording audio url fails
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "recordings" SET "audio_url"=$1,"duration_seconds"=$2,"updated_at"=$3 WHERE id = $4 AND "recordings"."deleted_at" IS NULL`)).
			WithArgs(audioURL, float64(durationSec), sqlmock.AnyArg(), recID).
			WillReturnError(errors.New("db connection failure"))
		mock.ExpectRollback()

		err := svc.HandleGoogleMeetWebhook(ctx, dtos.GoogleMeetWebhookRequest{
			ExternalSessionID: "meet-123",
			Event:             "completed",
			AudioURL:          &audioURL,
			DurationSeconds:   &durationSec,
		}, "")
		if err == nil {
			t.Errorf("expected error when UpdateRecordingAudioURL fails")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled mock expectations: %v", err)
		}
	})

	t.Run("failed with error message", func(t *testing.T) {
		svc, mock, _, _ := setupRecordingTestService(t)
		sessID := uuid.New()
		recID := uuid.New()
		errMsg := "admission denied by host"

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "bot_sessions" WHERE external_session_id = $1 AND "bot_sessions"."deleted_at" IS NULL ORDER BY "bot_sessions"."id" LIMIT $2`)).
			WithArgs("meet-123", 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "external_session_id"}).AddRow(sessID, recID, "meet-123"))

		// Update bot session status
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "bot_sessions" SET "ended_at"=$1,"error_message"=$2,"status"=$3,"updated_at"=$4 WHERE id = $5`)).
			WithArgs(sqlmock.AnyArg(), errMsg, constants.BotSessionStatusFailed, sqlmock.AnyArg(), sessID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		// Update recording status to FAILED
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "recordings" SET "error_code"=$1,"error_message"=$2,"status"=$3,"updated_at"=$4 WHERE id = $5 AND "recordings"."deleted_at" IS NULL`)).
			WithArgs("BOT_SESSION_FAILED", errMsg, models.RecordingStatusFailed, sqlmock.AnyArg(), recID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := svc.HandleGoogleMeetWebhook(ctx, dtos.GoogleMeetWebhookRequest{
			ExternalSessionID: "meet-123",
			Event:             "failed",
			ErrorMessage:      &errMsg,
		}, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled mock expectations: %v", err)
		}
	})
}

func TestService_HandleMSTeamsWebhook(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid webhook secret", func(t *testing.T) {
		cfg := config.Config{
			MSTeamsBotWebhookSecret: "super-teams-secret",
		}
		svc := services.New(cfg, nil, nil, nil)

		err := svc.HandleMSTeamsWebhook(ctx, dtos.MSTeamsWebhookRequest{
			ExternalSessionID: "teams-123",
			Event:             "call_connected",
		}, "wrong-secret")
		if err == nil {
			t.Errorf("expected unauthorized error for invalid secret")
		}
	})

	t.Run("session not found", func(t *testing.T) {
		svc, mock, _, _ := setupRecordingTestService(t)

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "bot_sessions" WHERE external_session_id = $1 AND "bot_sessions"."deleted_at" IS NULL ORDER BY "bot_sessions"."id" LIMIT $2`)).
			WithArgs("teams-unknown", 1).
			WillReturnRows(sqlmock.NewRows([]string{"id"}))

		err := svc.HandleMSTeamsWebhook(ctx, dtos.MSTeamsWebhookRequest{
			ExternalSessionID: "teams-unknown",
			Event:             "call_connected",
		}, "")
		if err == nil {
			t.Errorf("expected not found error")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled mock expectations: %v", err)
		}
	})

	t.Run("unrecognized event", func(t *testing.T) {
		svc, mock, _, _ := setupRecordingTestService(t)
		sessID := uuid.New()
		recID := uuid.New()

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "bot_sessions" WHERE external_session_id = $1 AND "bot_sessions"."deleted_at" IS NULL ORDER BY "bot_sessions"."id" LIMIT $2`)).
			WithArgs("teams-123", 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "external_session_id"}).AddRow(sessID, recID, "teams-123"))

		err := svc.HandleMSTeamsWebhook(ctx, dtos.MSTeamsWebhookRequest{
			ExternalSessionID: "teams-123",
			Event:             "invalid_teams_event",
		}, "")
		if err == nil {
			t.Errorf("expected error for unrecognized event")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled mock expectations: %v", err)
		}
	})

	t.Run("completed with audio url", func(t *testing.T) {
		svc, mock, _, _ := setupRecordingTestService(t)
		sessID := uuid.New()
		recID := uuid.New()
		audioURL := "s3://recordings/teams-123.wav"
		durationSec := 180

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "bot_sessions" WHERE external_session_id = $1 AND "bot_sessions"."deleted_at" IS NULL ORDER BY "bot_sessions"."id" LIMIT $2`)).
			WithArgs("teams-123", 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "external_session_id"}).AddRow(sessID, recID, "teams-123"))

		// Update bot session status
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "bot_sessions" SET "ended_at"=$1,"status"=$2,"updated_at"=$3 WHERE id = $4`)).
			WithArgs(sqlmock.AnyArg(), constants.BotSessionStatusCompleted, sqlmock.AnyArg(), sessID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		// Update recording audio url
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "recordings" SET "audio_url"=$1,"duration_seconds"=$2,"updated_at"=$3 WHERE id = $4 AND "recordings"."deleted_at" IS NULL`)).
			WithArgs(audioURL, float64(durationSec), sqlmock.AnyArg(), recID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		// Update recording status to TRANSCRIBING
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "recordings" SET "status"=$1,"updated_at"=$2 WHERE id = $3 AND "recordings"."deleted_at" IS NULL`)).
			WithArgs(models.RecordingStatusTranscribing, sqlmock.AnyArg(), recID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := svc.HandleMSTeamsWebhook(ctx, dtos.MSTeamsWebhookRequest{
			ExternalSessionID: "teams-123",
			Event:             "completed",
			AudioURL:          &audioURL,
			DurationSeconds:   &durationSec,
		}, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled mock expectations: %v", err)
		}
	})

	t.Run("completed with audio url db update failure", func(t *testing.T) {
		svc, mock, _, _ := setupRecordingTestService(t)
		sessID := uuid.New()
		recID := uuid.New()
		audioURL := "s3://recordings/teams-123.wav"
		durationSec := 180

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "bot_sessions" WHERE external_session_id = $1 AND "bot_sessions"."deleted_at" IS NULL ORDER BY "bot_sessions"."id" LIMIT $2`)).
			WithArgs("teams-123", 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "external_session_id"}).AddRow(sessID, recID, "teams-123"))

		// Update bot session status
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "bot_sessions" SET "ended_at"=$1,"status"=$2,"updated_at"=$3 WHERE id = $4`)).
			WithArgs(sqlmock.AnyArg(), constants.BotSessionStatusCompleted, sqlmock.AnyArg(), sessID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		// Update recording audio url fails
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "recordings" SET "audio_url"=$1,"duration_seconds"=$2,"updated_at"=$3 WHERE id = $4 AND "recordings"."deleted_at" IS NULL`)).
			WithArgs(audioURL, float64(durationSec), sqlmock.AnyArg(), recID).
			WillReturnError(errors.New("db connection failure"))
		mock.ExpectRollback()

		err := svc.HandleMSTeamsWebhook(ctx, dtos.MSTeamsWebhookRequest{
			ExternalSessionID: "teams-123",
			Event:             "completed",
			AudioURL:          &audioURL,
			DurationSeconds:   &durationSec,
		}, "")
		if err == nil {
			t.Errorf("expected error when UpdateRecordingAudioURL fails")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled mock expectations: %v", err)
		}
	})

	t.Run("failed with error message", func(t *testing.T) {
		svc, mock, _, _ := setupRecordingTestService(t)
		sessID := uuid.New()
		recID := uuid.New()
		errMsg := "call dropped by carrier"

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "bot_sessions" WHERE external_session_id = $1 AND "bot_sessions"."deleted_at" IS NULL ORDER BY "bot_sessions"."id" LIMIT $2`)).
			WithArgs("teams-123", 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "external_session_id"}).AddRow(sessID, recID, "teams-123"))

		// Update bot session status
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "bot_sessions" SET "ended_at"=$1,"error_message"=$2,"status"=$3,"updated_at"=$4 WHERE id = $5`)).
			WithArgs(sqlmock.AnyArg(), errMsg, constants.BotSessionStatusFailed, sqlmock.AnyArg(), sessID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		// Update recording status to FAILED
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "recordings" SET "error_code"=$1,"error_message"=$2,"status"=$3,"updated_at"=$4 WHERE id = $5 AND "recordings"."deleted_at" IS NULL`)).
			WithArgs("BOT_SESSION_FAILED", errMsg, models.RecordingStatusFailed, sqlmock.AnyArg(), recID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := svc.HandleMSTeamsWebhook(ctx, dtos.MSTeamsWebhookRequest{
			ExternalSessionID: "teams-123",
			Event:             "failed",
			ErrorMessage:      &errMsg,
		}, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled mock expectations: %v", err)
		}
	})
}

func TestService_HandleZoomWebhook(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid webhook secret", func(t *testing.T) {
		cfg := config.Config{
			ZoomBotWebhookSecret: "super-zoom-secret",
		}
		svc := services.New(cfg, nil, nil, nil)

		err := svc.HandleZoomWebhook(ctx, dtos.ZoomWebhookRequest{
			ExternalSessionID: "zoom-123",
			Event:             "joined",
		}, "wrong-secret")
		if err == nil {
			t.Errorf("expected unauthorized error for invalid secret")
		}
	})

	t.Run("session not found", func(t *testing.T) {
		svc, mock, _, _ := setupRecordingTestService(t)

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "bot_sessions" WHERE external_session_id = $1 AND "bot_sessions"."deleted_at" IS NULL ORDER BY "bot_sessions"."id" LIMIT $2`)).
			WithArgs("zoom-unknown", 1).
			WillReturnRows(sqlmock.NewRows([]string{"id"}))

		err := svc.HandleZoomWebhook(ctx, dtos.ZoomWebhookRequest{
			ExternalSessionID: "zoom-unknown",
			Event:             "joined",
		}, "")
		if err == nil {
			t.Errorf("expected not found error")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled mock expectations: %v", err)
		}
	})

	t.Run("unrecognized event", func(t *testing.T) {
		svc, mock, _, _ := setupRecordingTestService(t)
		sessID := uuid.New()
		recID := uuid.New()

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "bot_sessions" WHERE external_session_id = $1 AND "bot_sessions"."deleted_at" IS NULL ORDER BY "bot_sessions"."id" LIMIT $2`)).
			WithArgs("zoom-123", 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "external_session_id"}).AddRow(sessID, recID, "zoom-123"))

		err := svc.HandleZoomWebhook(ctx, dtos.ZoomWebhookRequest{
			ExternalSessionID: "zoom-123",
			Event:             "invalid_zoom_event",
		}, "")
		if err == nil {
			t.Errorf("expected error for unrecognized event")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled mock expectations: %v", err)
		}
	})

	t.Run("waiting_admit event updates session status", func(t *testing.T) {
		svc, mock, _, _ := setupRecordingTestService(t)
		sessID := uuid.New()
		recID := uuid.New()

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "bot_sessions" WHERE external_session_id = $1 AND "bot_sessions"."deleted_at" IS NULL ORDER BY "bot_sessions"."id" LIMIT $2`)).
			WithArgs("zoom-123", 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "external_session_id"}).AddRow(sessID, recID, "zoom-123"))

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "bot_sessions" SET "status"=$1,"updated_at"=$2 WHERE id = $3`)).
			WithArgs(constants.BotSessionStatusWaitingAdmit, sqlmock.AnyArg(), sessID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := svc.HandleZoomWebhook(ctx, dtos.ZoomWebhookRequest{
			ExternalSessionID: "zoom-123",
			Event:             "waiting_admit",
		}, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled mock expectations: %v", err)
		}
	})

	t.Run("joined event sets started_at and status", func(t *testing.T) {
		svc, mock, _, _ := setupRecordingTestService(t)
		sessID := uuid.New()
		recID := uuid.New()

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "bot_sessions" WHERE external_session_id = $1 AND "bot_sessions"."deleted_at" IS NULL ORDER BY "bot_sessions"."id" LIMIT $2`)).
			WithArgs("zoom-123", 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "external_session_id"}).AddRow(sessID, recID, "zoom-123"))

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "bot_sessions" SET "started_at"=$1,"status"=$2,"updated_at"=$3 WHERE id = $4`)).
			WithArgs(sqlmock.AnyArg(), constants.BotSessionStatusJoined, sqlmock.AnyArg(), sessID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := svc.HandleZoomWebhook(ctx, dtos.ZoomWebhookRequest{
			ExternalSessionID: "zoom-123",
			Event:             "joined",
		}, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled mock expectations: %v", err)
		}
	})

	t.Run("recording event sets started_at if not set", func(t *testing.T) {
		svc, mock, _, _ := setupRecordingTestService(t)
		sessID := uuid.New()
		recID := uuid.New()

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "bot_sessions" WHERE external_session_id = $1 AND "bot_sessions"."deleted_at" IS NULL ORDER BY "bot_sessions"."id" LIMIT $2`)).
			WithArgs("zoom-123", 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "external_session_id"}).AddRow(sessID, recID, "zoom-123"))

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "bot_sessions" SET "started_at"=$1,"status"=$2,"updated_at"=$3 WHERE id = $4`)).
			WithArgs(sqlmock.AnyArg(), constants.BotSessionStatusRecording, sqlmock.AnyArg(), sessID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := svc.HandleZoomWebhook(ctx, dtos.ZoomWebhookRequest{
			ExternalSessionID: "zoom-123",
			Event:             "recording",
		}, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled mock expectations: %v", err)
		}
	})

	t.Run("completed event updates recording audio and transcribing status", func(t *testing.T) {
		svc, mock, _, _ := setupRecordingTestService(t)
		sessID := uuid.New()
		recID := uuid.New()
		audioURL := "s3://recordings/zoom-123.wav"
		durationSec := 240

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "bot_sessions" WHERE external_session_id = $1 AND "bot_sessions"."deleted_at" IS NULL ORDER BY "bot_sessions"."id" LIMIT $2`)).
			WithArgs("zoom-123", 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "external_session_id"}).AddRow(sessID, recID, "zoom-123"))

		// Update bot session status
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "bot_sessions" SET "ended_at"=$1,"status"=$2,"updated_at"=$3 WHERE id = $4`)).
			WithArgs(sqlmock.AnyArg(), constants.BotSessionStatusCompleted, sqlmock.AnyArg(), sessID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		// Update recording audio url
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "recordings" SET "audio_url"=$1,"duration_seconds"=$2,"updated_at"=$3 WHERE id = $4 AND "recordings"."deleted_at" IS NULL`)).
			WithArgs(audioURL, float64(durationSec), sqlmock.AnyArg(), recID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		// Update recording status to TRANSCRIBING
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "recordings" SET "status"=$1,"updated_at"=$2 WHERE id = $3 AND "recordings"."deleted_at" IS NULL`)).
			WithArgs(models.RecordingStatusTranscribing, sqlmock.AnyArg(), recID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := svc.HandleZoomWebhook(ctx, dtos.ZoomWebhookRequest{
			ExternalSessionID: "zoom-123",
			Event:             "completed",
			AudioURL:          &audioURL,
			DurationSeconds:   &durationSec,
		}, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled mock expectations: %v", err)
		}
	})

	t.Run("completed with audio url db update failure", func(t *testing.T) {
		svc, mock, _, _ := setupRecordingTestService(t)
		sessID := uuid.New()
		recID := uuid.New()
		audioURL := "s3://recordings/zoom-123.wav"
		durationSec := 240

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "bot_sessions" WHERE external_session_id = $1 AND "bot_sessions"."deleted_at" IS NULL ORDER BY "bot_sessions"."id" LIMIT $2`)).
			WithArgs("zoom-123", 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "external_session_id"}).AddRow(sessID, recID, "zoom-123"))

		// Update bot session status
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "bot_sessions" SET "ended_at"=$1,"status"=$2,"updated_at"=$3 WHERE id = $4`)).
			WithArgs(sqlmock.AnyArg(), constants.BotSessionStatusCompleted, sqlmock.AnyArg(), sessID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		// Update recording audio url fails
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "recordings" SET "audio_url"=$1,"duration_seconds"=$2,"updated_at"=$3 WHERE id = $4 AND "recordings"."deleted_at" IS NULL`)).
			WithArgs(audioURL, float64(durationSec), sqlmock.AnyArg(), recID).
			WillReturnError(errors.New("db connection failure"))
		mock.ExpectRollback()

		err := svc.HandleZoomWebhook(ctx, dtos.ZoomWebhookRequest{
			ExternalSessionID: "zoom-123",
			Event:             "completed",
			AudioURL:          &audioURL,
			DurationSeconds:   &durationSec,
		}, "")
		if err == nil {
			t.Errorf("expected error when UpdateRecordingAudioURL fails")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled mock expectations: %v", err)
		}
	})

	t.Run("failed with error message", func(t *testing.T) {
		svc, mock, _, _ := setupRecordingTestService(t)
		sessID := uuid.New()
		recID := uuid.New()
		errMsg := "zoom bot admission rejected by host"

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "bot_sessions" WHERE external_session_id = $1 AND "bot_sessions"."deleted_at" IS NULL ORDER BY "bot_sessions"."id" LIMIT $2`)).
			WithArgs("zoom-123", 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "external_session_id"}).AddRow(sessID, recID, "zoom-123"))

		// Update bot session status
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "bot_sessions" SET "ended_at"=$1,"error_message"=$2,"status"=$3,"updated_at"=$4 WHERE id = $5`)).
			WithArgs(sqlmock.AnyArg(), errMsg, constants.BotSessionStatusFailed, sqlmock.AnyArg(), sessID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		// Update recording status to FAILED
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "recordings" SET "error_code"=$1,"error_message"=$2,"status"=$3,"updated_at"=$4 WHERE id = $5 AND "recordings"."deleted_at" IS NULL`)).
			WithArgs("BOT_SESSION_FAILED", errMsg, models.RecordingStatusFailed, sqlmock.AnyArg(), recID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := svc.HandleZoomWebhook(ctx, dtos.ZoomWebhookRequest{
			ExternalSessionID: "zoom-123",
			Event:             "failed",
			ErrorMessage:      &errMsg,
		}, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled mock expectations: %v", err)
		}
	})
}


