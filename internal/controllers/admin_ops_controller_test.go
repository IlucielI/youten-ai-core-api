package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
	"code-base-golang/internal/pkg/ctxmeta"
)

func TestControllers_AdminGetDLQPipeline(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("success returns 200 with DLQ items", func(t *testing.T) {
		ctrls, mock, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		adminID := uuid.New()
		recID := uuid.New()
		now := time.Now()

		mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings" WHERE status = 'FAILED' AND "recordings"\."deleted_at" IS NULL`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

		mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings" WHERE \(status IN \('QUEUED', 'VALIDATING', 'EXTRACTING', 'TRANSCRIBING', 'SUMMARIZING', 'INDEXING'\) AND updated_at < \$1\) AND "recordings"\."deleted_at" IS NULL`).
			WithArgs(sqlmock.AnyArg()).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

		mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE \(\(status = 'FAILED'\) OR \(status IN \('QUEUED', 'VALIDATING', 'EXTRACTING', 'TRANSCRIBING', 'SUMMARIZING', 'INDEXING'\) AND updated_at < \$1\)\) AND "recordings"\."deleted_at" IS NULL ORDER BY updated_at DESC LIMIT \$2`).
			WithArgs(sqlmock.AnyArg(), 20).
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "title", "original_filename", "status", "source_type", "created_at", "updated_at",
			}).
				AddRow(recID, "Failed Meeting", "meeting.mp4", models.RecordingStatusFailed, "UPLOAD", now, now))

		r := gin.New()
		r.Use(func(c *gin.Context) {
			ctx := ctxmeta.WithAdminAuthUser(c.Request.Context(), ctxmeta.AdminAuthUser{
				AdminID:  adminID,
				Username: "ops_admin",
			})
			c.Request = c.Request.WithContext(ctx)
			c.Next()
		})
		r.GET("/v1/admin/pipeline/dlq", ctrls.AdminGetDLQPipeline)

		req := httptest.NewRequest(http.MethodGet, "/v1/admin/pipeline/dlq?page=1&limit=20", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d, body: %s", w.Code, w.Body.String())
		}

		var apiResp dtos.APIResponse[dtos.DLQMessagesResponse]
		if err := json.Unmarshal(w.Body.Bytes(), &apiResp); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}
		if apiResp.Data.Total != 1 {
			t.Fatalf("expected total 1, got %d", apiResp.Data.Total)
		}
	})

	t.Run("unauthorized returns 401", func(t *testing.T) {
		ctrls, _, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		r := gin.New()
		r.GET("/v1/admin/pipeline/dlq", ctrls.AdminGetDLQPipeline)

		req := httptest.NewRequest(http.MethodGet, "/v1/admin/pipeline/dlq", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", w.Code)
		}
	})
}

func TestControllers_AdminRetryDLQJob(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("success returns 200 with retry response", func(t *testing.T) {
		ctrls, mock, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		adminID := uuid.New()
		recID := uuid.New()
		now := time.Now()

		mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL`).
			WithArgs(recID, 1).
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "status", "title", "original_filename", "selected_template", "output_language", "audio_url", "created_at", "updated_at",
			}).
				AddRow(recID, models.RecordingStatusFailed, "Meeting", "meet.mp4", "GENERAL", "id", nil, now, now))

		mock.ExpectQuery(`SELECT \* FROM "transcript_segments" WHERE recording_id = \$1`).
			WithArgs(recID).
			WillReturnRows(sqlmock.NewRows([]string{"id"}))

		mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND is_active = TRUE`).
			WithArgs(recID, 1).
			WillReturnError(gorm.ErrRecordNotFound)

		mock.ExpectQuery(`SELECT .* FROM "transcript_chunks" WHERE recording_id = \$1 ORDER BY chunk_index ASC`).
			WithArgs(recID).
			WillReturnRows(sqlmock.NewRows([]string{"id"}))

		mock.ExpectBegin()
		mock.ExpectExec(`UPDATE "recordings" SET .* WHERE id = .*`).
			WithArgs(models.RecordingStatusQueued, sqlmock.AnyArg(), recID).
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
				Username: "ops_admin",
			})
			c.Request = c.Request.WithContext(ctx)
			c.Next()
		})
		r.POST("/v1/admin/pipeline/dlq/retry", ctrls.AdminRetryDLQJob)

		body := fmt.Sprintf(`{"recording_id": "%s"}`, recID.String())
		req := httptest.NewRequest(http.MethodPost, "/v1/admin/pipeline/dlq/retry", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("unauthorized returns 401", func(t *testing.T) {
		ctrls, _, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		r := gin.New()
		r.POST("/v1/admin/pipeline/dlq/retry", ctrls.AdminRetryDLQJob)

		body := fmt.Sprintf(`{"recording_id": "%s"}`, uuid.New().String())
		req := httptest.NewRequest(http.MethodPost, "/v1/admin/pipeline/dlq/retry", strings.NewReader(body))
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
		r.Use(func(c *gin.Context) {
			ctx := ctxmeta.WithAdminAuthUser(c.Request.Context(), ctxmeta.AdminAuthUser{
				AdminID:  uuid.New(),
				Username: "ops_admin",
			})
			c.Request = c.Request.WithContext(ctx)
			c.Next()
		})
		r.POST("/v1/admin/pipeline/dlq/retry", ctrls.AdminRetryDLQJob)

		body := `{"recording_id": "00000000-0000-0000-0000-000000000000"}`
		req := httptest.NewRequest(http.MethodPost, "/v1/admin/pipeline/dlq/retry", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", w.Code)
		}
	})
}

func TestControllers_AdminListReports(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("success returns 200", func(t *testing.T) {
		ctrls, mock, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		repID := uuid.New()
		recID := uuid.New()
		now := time.Now()

		mock.ExpectQuery(`SELECT count\(\*\) FROM "reports"`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

		mock.ExpectQuery(`SELECT \* FROM "reports" ORDER BY created_at DESC LIMIT \$1`).
			WithArgs(10).
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "recording_id", "reporter_type", "reason", "status", "created_at", "updated_at",
			}).AddRow(
				repID, recID, "guest", "spam", "open", now, now,
			))

		mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE "recordings"\."id" = \$1 AND "recordings"\."deleted_at" IS NULL`).
			WithArgs(recID).
			WillReturnRows(sqlmock.NewRows([]string{"id", "title"}).AddRow(recID, "Target Recording"))

		r := gin.New()
		r.Use(func(c *gin.Context) {
			ctx := ctxmeta.WithAdminAuthUser(c.Request.Context(), ctxmeta.AdminAuthUser{
				AdminID:  uuid.New(),
				Username: "moderator",
			})
			c.Request = c.Request.WithContext(ctx)
			c.Next()
		})
		r.GET("/v1/admin/reports", ctrls.AdminListReports)

		req := httptest.NewRequest(http.MethodGet, "/v1/admin/reports?page=1&limit=10", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("unauthorized without admin context returns 401", func(t *testing.T) {
		ctrls, _, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		r := gin.New()
		r.GET("/v1/admin/reports", ctrls.AdminListReports)

		req := httptest.NewRequest(http.MethodGet, "/v1/admin/reports", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", w.Code)
		}
	})
}

func TestControllers_AdminResolveReport(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("success returns 200", func(t *testing.T) {
		ctrls, mock, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		repID := uuid.New()
		recID := uuid.New()
		adminID := uuid.New()
		now := time.Now()

		mock.ExpectQuery(`SELECT \* FROM "reports" WHERE id = \$1 ORDER BY "reports"\."id" LIMIT \$2`).
			WithArgs(repID, 1).
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "recording_id", "reporter_type", "reason", "status", "created_at", "updated_at",
			}).AddRow(repID, recID, "guest", "spam", "open", now, now))

		mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE "recordings"\."id" = \$1 AND "recordings"\."deleted_at" IS NULL`).
			WithArgs(recID).
			WillReturnRows(sqlmock.NewRows([]string{"id", "title"}).AddRow(recID, "Target"))

		mock.ExpectBegin()
		mock.ExpectExec(`UPDATE "reports" SET .* WHERE id = .*`).
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
				Username: "moderator",
			})
			c.Request = c.Request.WithContext(ctx)
			c.Next()
		})
		r.POST("/v1/admin/reports/:id/resolve", ctrls.AdminResolveReport)

		body := `{"action": "DISMISS", "resolution_note": "Spam dismiss"}`
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/v1/admin/reports/%s/resolve", repID.String()), strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		ctrls, _, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		repID := uuid.New()
		r := gin.New()
		r.Use(func(c *gin.Context) {
			ctx := ctxmeta.WithAdminAuthUser(c.Request.Context(), ctxmeta.AdminAuthUser{
				AdminID:  uuid.New(),
				Username: "moderator",
			})
			c.Request = c.Request.WithContext(ctx)
			c.Next()
		})
		r.POST("/v1/admin/reports/:id/resolve", ctrls.AdminResolveReport)

		body := `{"action": "INVALID_ACTION"}`
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/v1/admin/reports/%s/resolve", repID.String()), strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", w.Code)
		}
	})

	t.Run("invalid id param returns 400", func(t *testing.T) {
		ctrls, _, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		r := gin.New()
		r.Use(func(c *gin.Context) {
			ctx := ctxmeta.WithAdminAuthUser(c.Request.Context(), ctxmeta.AdminAuthUser{
				AdminID:  uuid.New(),
				Username: "moderator",
			})
			c.Request = c.Request.WithContext(ctx)
			c.Next()
		})
		r.POST("/v1/admin/reports/:id/resolve", ctrls.AdminResolveReport)

		body := `{"action": "DISMISS"}`
		req := httptest.NewRequest(http.MethodPost, "/v1/admin/reports/invalid-uuid/resolve", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", w.Code)
		}
	})

	t.Run("unauthorized without admin context returns 401", func(t *testing.T) {
		ctrls, _, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		repID := uuid.New()
		r := gin.New()
		r.POST("/v1/admin/reports/:id/resolve", ctrls.AdminResolveReport)

		body := `{"action": "DISMISS"}`
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/v1/admin/reports/%s/resolve", repID.String()), strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", w.Code)
		}
	})
}

func TestControllers_AdminListJobs(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("successfully lists jobs with admin auth context", func(t *testing.T) {
		ctrls, mock, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		adminID := uuid.New()
		adminUser := ctxmeta.AdminAuthUser{
			AdminID:  adminID,
			Username: "admin",
		}

		mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings"`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

		mock.ExpectQuery(`SELECT \* FROM "recordings"`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "title", "status", "source_type", "selected_template", "output_language", "is_guest"}).
				AddRow(uuid.New(), "Meeting Analysis", "COMPLETED", "UPLOAD", "GENERAL", "id", false))

		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Request = c.Request.WithContext(ctxmeta.WithAdminAuthUser(c.Request.Context(), adminUser))
			c.Next()
		})
		r.GET("/v1/admin/jobs", ctrls.AdminListJobs)

		req := httptest.NewRequest(http.MethodGet, "/v1/admin/jobs?page=1&limit=10", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("unauthorized without admin context returns 401", func(t *testing.T) {
		ctrls, _, cleanup := setupAdminTestControllers(t)
		defer cleanup()

		r := gin.New()
		r.GET("/v1/admin/jobs", ctrls.AdminListJobs)

		req := httptest.NewRequest(http.MethodGet, "/v1/admin/jobs", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", w.Code)
		}
	})
}
