package services_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/services"
)

type waitlistMockMailer struct {
	pingErr error
	sendErr error
	sent    []services.EmailMessage
}

func (m *waitlistMockMailer) Ping(ctx context.Context) error {
	return m.pingErr
}

func (m *waitlistMockMailer) Send(ctx context.Context, msg services.EmailMessage) error {
	if m.sendErr != nil {
		return m.sendErr
	}
	m.sent = append(m.sent, msg)
	return nil
}

func TestService_JoinBotWaitlist_SuccessWithMailer(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	mailer := &waitlistMockMailer{}
	svc.SetMailer(mailer)

	entryID := uuid.New()
	now := time.Now()

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "bot_waitlists"`).
		WithArgs("beta.user@example.com", "zoom", "11-50", "PENDING").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(entryID, now, now))
	mock.ExpectCommit()

	req := dtos.WaitlistRequest{
		Email:       " Beta.User@Example.Com ",
		Platform:    "zoom",
		CompanySize: "11-50",
	}

	res, err := svc.JoinBotWaitlist(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res == nil {
		t.Fatal("expected non-nil response")
	}
	if res.Email != "beta.user@example.com" {
		t.Errorf("expected normalized email beta.user@example.com, got %s", res.Email)
	}
	if res.Platform != "zoom" {
		t.Errorf("expected platform zoom, got %s", res.Platform)
	}
	if res.CompanySize != "11-50" {
		t.Errorf("expected company size 11-50, got %s", res.CompanySize)
	}
	if res.Status != constants.WaitlistStatusPending {
		t.Errorf("expected status %s, got %s", constants.WaitlistStatusPending, res.Status)
	}
	if !strings.Contains(res.Message, "successfully") {
		t.Errorf("expected success message, got %s", res.Message)
	}

	if len(mailer.sent) != 1 {
		t.Fatalf("expected 1 email to be sent, got %d", len(mailer.sent))
	}
	sentEmail := mailer.sent[0]
	if len(sentEmail.To) != 1 || sentEmail.To[0] != "beta.user@example.com" {
		t.Errorf("expected email recipient beta.user@example.com, got %v", sentEmail.To)
	}
	if sentEmail.Subject != "Welcome to Youten Voice Bot Beta Waitlist!" {
		t.Errorf("expected subject 'Welcome to Youten Voice Bot Beta Waitlist!', got %s", sentEmail.Subject)
	}
	if !strings.Contains(sentEmail.TextBody, "zoom") || !strings.Contains(sentEmail.HTMLBody, "11-50") {
		t.Errorf("expected email bodies to contain platform and company size details")
	}
}

func TestService_JoinBotWaitlist_DefaultsAndNilMailer(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	// Mailer is not set (nil)

	entryID := uuid.New()
	now := time.Now()

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "bot_waitlists"`).
		WithArgs("founder@startup.io", "google_meet", "1-10", "PENDING").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(entryID, now, now))
	mock.ExpectCommit()

	req := dtos.WaitlistRequest{
		Email: "founder@startup.io",
	}

	res, err := svc.JoinBotWaitlist(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res == nil {
		t.Fatal("expected non-nil response")
	}
	if res.Platform != "google_meet" {
		t.Errorf("expected default platform google_meet, got %s", res.Platform)
	}
	if res.CompanySize != "1-10" {
		t.Errorf("expected default company size 1-10, got %s", res.CompanySize)
	}
}

func TestService_JoinBotWaitlist_DBError(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "bot_waitlists"`).
		WithArgs("dberr@example.com", "google_meet", "1-10", "PENDING").
		WillReturnError(errors.New("disk full"))
	mock.ExpectRollback()

	req := dtos.WaitlistRequest{
		Email: "dberr@example.com",
	}

	res, err := svc.JoinBotWaitlist(context.Background(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if res != nil {
		t.Fatalf("expected nil response on db error, got %+v", res)
	}
}

func TestService_JoinBotWaitlist_MailerError(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	mailer := &waitlistMockMailer{sendErr: errors.New("smtp connection reset")}
	svc.SetMailer(mailer)

	entryID := uuid.New()
	now := time.Now()

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "bot_waitlists"`).
		WithArgs("mailerr@example.com", "google_meet", "1-10", "PENDING").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(entryID, now, now))
	mock.ExpectCommit()

	req := dtos.WaitlistRequest{
		Email: "mailerr@example.com",
	}

	res, err := svc.JoinBotWaitlist(context.Background(), req)
	if err == nil {
		t.Fatal("expected error on mailer send failure, got nil")
	}
	if res != nil {
		t.Fatalf("expected nil response on mailer failure, got %+v", res)
	}
}
