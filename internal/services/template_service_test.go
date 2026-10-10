package services_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestService_ListActiveTemplates(t *testing.T) {
	t.Run("success returns active templates ordered by name", func(t *testing.T) {
		svc, mock, _, _ := setupRecordingTestService(t)

		id1 := uuid.New()
		id2 := uuid.New()
		now := time.Now()

		rows := sqlmock.NewRows([]string{
			"id", "category_key", "name", "description", "prompt", "output_schema", "version", "is_active", "created_at", "updated_at",
		}).
			AddRow(id1, "GENERAL", "Executive Brief", "General summary", "prompt 1", `{}`, 1, true, now, now).
			AddRow(id2, "MOM", "Minutes of Meeting", "MOM summary", "prompt 2", `{}`, 1, true, now, now)

		mock.ExpectQuery(`SELECT \* FROM "templates" WHERE is_active = TRUE ORDER BY name ASC`).
			WillReturnRows(rows)

		resp, err := svc.ListActiveTemplates(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp == nil || len(resp.Items) != 2 {
			t.Fatalf("expected 2 items, got %+v", resp)
		}
		if resp.Items[0].CategoryKey != "GENERAL" || resp.Items[0].Name != "Executive Brief" {
			t.Errorf("unexpected first item: %+v", resp.Items[0])
		}
		if resp.Items[1].CategoryKey != "MOM" || resp.Items[1].Name != "Minutes of Meeting" {
			t.Errorf("unexpected second item: %+v", resp.Items[1])
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("there were unfulfilled expectations: %s", err)
		}
	})

	t.Run("db error propagates cleanly", func(t *testing.T) {
		svc, mock, _, _ := setupRecordingTestService(t)

		mock.ExpectQuery(`SELECT \* FROM "templates" WHERE is_active = TRUE ORDER BY name ASC`).
			WillReturnError(errors.New("db connection failure"))

		resp, err := svc.ListActiveTemplates(context.Background())
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if resp != nil {
			t.Fatalf("expected nil response on error, got %+v", resp)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("there were unfulfilled expectations: %s", err)
		}
	})
}
