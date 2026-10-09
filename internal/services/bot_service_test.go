package services_test

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"

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
}
