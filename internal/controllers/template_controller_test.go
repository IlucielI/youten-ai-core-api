package controllers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"code-base-golang/internal/dtos"
)

func TestControllers_ListTemplates(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("success returns active templates", func(t *testing.T) {
		ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
		defer cleanup()

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

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/v1/templates", nil)

		ctrls.ListTemplates(c)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d, body: %s", w.Code, w.Body.String())
		}

		var resp dtos.APIResponse[dtos.TemplateListResponse]
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if len(resp.Data.Items) != 2 {
			t.Fatalf("expected 2 items, got %d", len(resp.Data.Items))
		}
		if resp.Data.Items[0].CategoryKey != "GENERAL" {
			t.Errorf("expected first item GENERAL, got %s", resp.Data.Items[0].CategoryKey)
		}
	})

	t.Run("failure returns 500 error", func(t *testing.T) {
		ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
		defer cleanup()

		mock.ExpectQuery(`SELECT \* FROM "templates" WHERE is_active = TRUE ORDER BY name ASC`).
			WillReturnError(errors.New("db error"))

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/v1/templates", nil)

		ctrls.ListTemplates(c)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500 Internal Server Error, got %d, body: %s", w.Code, w.Body.String())
		}
	})
}
