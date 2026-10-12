package controllers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/adapters/llm"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/pkg/ctxmeta"
)

func TestControllers_AdminListTemplates(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("success returns 200 with default pagination", func(t *testing.T) {
		ctrls, mock, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		tmplID := uuid.New()
		now := time.Now()

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "templates"`)).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "templates" ORDER BY category_key ASC LIMIT $1`)).
			WithArgs(10).
			WillReturnRows(sqlmock.NewRows([]string{"id", "category_key", "name", "description", "prompt", "output_schema", "version", "is_active", "created_at", "updated_at"}).
				AddRow(tmplID, "MOM", "Minutes of Meeting", "Standard MOM", "Prompt", []byte(`{}`), 1, true, now, now))

		r := gin.New()
		r.GET("/v1/admin/templates", ctrls.AdminListTemplates)

		req := httptest.NewRequest(http.MethodGet, "/v1/admin/templates", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled expectations: %v", err)
		}
	})

	t.Run("success returns 200 with search query parameters", func(t *testing.T) {
		ctrls, mock, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		tmplID := uuid.New()
		now := time.Now()

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "templates" WHERE name ILIKE $1 OR category_key ILIKE $2 OR description ILIKE $3`)).
			WithArgs("%mom%", "%mom%", "%mom%").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "templates" WHERE name ILIKE $1 OR category_key ILIKE $2 OR description ILIKE $3 ORDER BY category_key ASC LIMIT $4 OFFSET $5`)).
			WithArgs("%mom%", "%mom%", "%mom%", 5, 5).
			WillReturnRows(sqlmock.NewRows([]string{"id", "category_key", "name", "description", "prompt", "output_schema", "version", "is_active", "created_at", "updated_at"}).
				AddRow(tmplID, "MOM", "Minutes of Meeting", "Standard MOM", "Prompt", []byte(`{}`), 1, true, now, now))

		r := gin.New()
		r.GET("/v1/admin/templates", ctrls.AdminListTemplates)

		req := httptest.NewRequest(http.MethodGet, "/v1/admin/templates?search=mom&page=2&limit=5", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled expectations: %v", err)
		}
	})

	t.Run("invalid query returns 400", func(t *testing.T) {
		ctrls, _, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		r := gin.New()
		r.GET("/v1/admin/templates", ctrls.AdminListTemplates)

		req := httptest.NewRequest(http.MethodGet, "/v1/admin/templates?page=abc", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("service failure returns 500", func(t *testing.T) {
		ctrls, mock, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "templates"`)).
			WillReturnError(gorm.ErrInvalidDB)

		r := gin.New()
		r.GET("/v1/admin/templates", ctrls.AdminListTemplates)

		req := httptest.NewRequest(http.MethodGet, "/v1/admin/templates", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected status 500, got %d", w.Code)
		}
	})
}

func TestControllers_AdminCreateTemplate(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("success returns 201", func(t *testing.T) {
		ctrls, mock, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		adminID := uuid.New()

		mock.ExpectQuery(`SELECT \* FROM "templates" WHERE category_key = \$1 ORDER BY "templates"\."id" LIMIT \$2`).
			WithArgs("TUTORIAL", 1).
			WillReturnError(gorm.ErrRecordNotFound)

		mock.ExpectBegin()
		mock.ExpectQuery(`INSERT INTO "templates"`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(uuid.New(), time.Now(), time.Now()))
		mock.ExpectCommit()

		mock.ExpectBegin()
		mock.ExpectQuery(`INSERT INTO "admin_audit_logs"`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(uuid.New(), time.Now()))
		mock.ExpectCommit()

		r := gin.New()
		r.Use(func(c *gin.Context) {
			ctx := ctxmeta.WithAdminAuthUser(c.Request.Context(), ctxmeta.AdminAuthUser{
				AdminID:  adminID,
				Username: "admin_tester",
			})
			c.Request = c.Request.WithContext(ctx)
			c.Next()
		})
		r.POST("/v1/admin/templates", ctrls.AdminCreateTemplate)

		body := `{"category_key": "TUTORIAL", "name": "Tutorial", "prompt": "Extract steps", "output_schema": {"type": "object", "properties": {"steps": {"type": "array"}}}}`
		req := httptest.NewRequest(http.MethodPost, "/v1/admin/templates", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("expected status 201, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		ctrls, _, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		r := gin.New()
		r.POST("/v1/admin/templates", ctrls.AdminCreateTemplate)

		body := `{"category_key": "", "name": ""}`
		req := httptest.NewRequest(http.MethodPost, "/v1/admin/templates", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", w.Code)
		}
	})

	t.Run("missing admin context returns 401", func(t *testing.T) {
		ctrls, _, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		r := gin.New()
		r.POST("/v1/admin/templates", ctrls.AdminCreateTemplate)

		body := `{"category_key": "TUTORIAL", "name": "Tutorial", "prompt": "Extract steps", "output_schema": {"type": "object", "properties": {"steps": {"type": "array"}}}}`
		req := httptest.NewRequest(http.MethodPost, "/v1/admin/templates", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", w.Code)
		}
	})
}

func TestControllers_AdminUpdateTemplate(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("success returns 200", func(t *testing.T) {
		ctrls, mock, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		adminID := uuid.New()
		tmplID := uuid.New()
		now := time.Now()

		mock.ExpectQuery(`SELECT \* FROM "templates" WHERE id = \$1 ORDER BY "templates"\."id" LIMIT \$2`).
			WithArgs(tmplID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "category_key", "name", "description", "prompt", "output_schema", "version", "is_active", "created_at", "updated_at"}).
				AddRow(tmplID, "MOM", "Old Name", "Old Desc", "Old prompt", []byte(`{"type":"object","properties":{"title":{"type":"string"}}}`), 1, true, now, now))

		mock.ExpectBegin()
		mock.ExpectExec(`UPDATE "templates" SET`).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		mock.ExpectBegin()
		mock.ExpectQuery(`INSERT INTO "admin_audit_logs"`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(uuid.New(), time.Now()))
		mock.ExpectCommit()

		r := gin.New()
		r.Use(func(c *gin.Context) {
			ctx := ctxmeta.WithAdminAuthUser(c.Request.Context(), ctxmeta.AdminAuthUser{
				AdminID:  adminID,
				Username: "admin_tester",
			})
			c.Request = c.Request.WithContext(ctx)
			c.Next()
		})
		r.PUT("/v1/admin/templates/:id", ctrls.AdminUpdateTemplate)

		body := `{"name": "New Name", "prompt": "New prompt", "output_schema": {"type": "object", "properties": {"title": {"type": "string"}}}}`
		req := httptest.NewRequest(http.MethodPut, "/v1/admin/templates/"+tmplID.String(), strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("invalid UUID returns 400", func(t *testing.T) {
		ctrls, _, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		r := gin.New()
		r.PUT("/v1/admin/templates/:id", ctrls.AdminUpdateTemplate)

		req := httptest.NewRequest(http.MethodPut, "/v1/admin/templates/not-a-uuid", strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", w.Code)
		}
	})
}

func TestControllers_AdminTestTemplate(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("success returns 200 with test results", func(t *testing.T) {
		ctrls, _, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		adminID := uuid.New()
		mockLLM := llm.NewMock()
		mockLLM.GenerateStructuredFunc = func(ctx context.Context, systemPrompt, userPrompt string, schema map[string]interface{}) (*dtos.StructuredResponse, error) {
			return &dtos.StructuredResponse{
				RawJSON: `{"title": "Sample Title"}`,
				Usage: dtos.LLMUsage{
					TotalTokens: 42,
				},
			}, nil
		}
		ctrls.Service().SetLLM(mockLLM)

		r := gin.New()
		r.Use(func(c *gin.Context) {
			ctx := ctxmeta.WithAdminAuthUser(c.Request.Context(), ctxmeta.AdminAuthUser{
				AdminID:  adminID,
				Username: "admin_tester",
			})
			c.Request = c.Request.WithContext(ctx)
			c.Next()
		})
		r.POST("/v1/admin/templates/test", ctrls.AdminTestTemplate)

		body := `{"prompt": "Test Prompt", "sample_transcript": "Transcript", "output_schema": {"type": "object", "properties": {"title": {"type": "string"}}}}`
		req := httptest.NewRequest(http.MethodPost, "/v1/admin/templates/test", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("missing admin context returns 401", func(t *testing.T) {
		ctrls, _, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		r := gin.New()
		r.POST("/v1/admin/templates/test", ctrls.AdminTestTemplate)

		body := `{"prompt": "Test", "sample_transcript": "Text", "output_schema": {"type": "object", "properties": {"a": {"type": "string"}}}}`
		req := httptest.NewRequest(http.MethodPost, "/v1/admin/templates/test", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", w.Code)
		}
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		ctrls, _, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		r := gin.New()
		r.POST("/v1/admin/templates/test", ctrls.AdminTestTemplate)

		body := `{"prompt": "", "sample_transcript": ""}`
		req := httptest.NewRequest(http.MethodPost, "/v1/admin/templates/test", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", w.Code)
		}
	})
}
