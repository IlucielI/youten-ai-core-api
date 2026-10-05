package controllers

import (
	"bytes"
	"encoding/json"
	"errors"
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

	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
	"code-base-golang/internal/pkg/ctxmeta"
)

func TestControllers_UpdateTranscriptSpeakers_InvalidUUID(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "invalid-uuid"}}
	req := httptest.NewRequest(http.MethodPut, "/v1/recordings/invalid/speakers", strings.NewReader(`{"speakers":{"SPEAKER_00":"Bayu"}}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	ctrls.UpdateTranscriptSpeakers(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_UpdateTranscriptSpeakers_InvalidJSON(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req := httptest.NewRequest(http.MethodPut, "/v1/recordings/"+recID.String()+"/speakers", strings.NewReader(`{invalid-json`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	ctrls.UpdateTranscriptSpeakers(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_UpdateTranscriptSpeakers_ValidationError(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req := httptest.NewRequest(http.MethodPut, "/v1/recordings/"+recID.String()+"/speakers", strings.NewReader(`{"speakers":{}}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	ctrls.UpdateTranscriptSpeakers(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_UpdateTranscriptSpeakers_NotFound(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req := httptest.NewRequest(http.MethodPut, "/v1/recordings/"+recID.String()+"/speakers", strings.NewReader(`{"speakers":{"SPEAKER_00":"Bayu"}}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	ctrls.UpdateTranscriptSpeakers(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", w.Code)
	}
}

func TestControllers_UpdateTranscriptSpeakers_Forbidden(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	ownerID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req := httptest.NewRequest(http.MethodPut, "/v1/recordings/"+recID.String()+"/speakers", strings.NewReader(`{"speakers":{"SPEAKER_00":"Bayu"}}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "ownership_token"}).
			AddRow(recID, &ownerID, "secret-token"))

	ctrls.UpdateTranscriptSpeakers(c)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden, got %d", w.Code)
	}
}

func TestControllers_UpdateTranscriptSpeakers_Success_TokenInBody(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	guestToken := "guest-ownership-token"
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req := httptest.NewRequest(http.MethodPut, "/v1/recordings/"+recID.String()+"/speakers", strings.NewReader(`{"speakers":{"SPEAKER_00":"Bayu"},"ownership_token":"`+guestToken+`"}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token"}).
			AddRow(recID, guestToken))

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "transcript_segments" SET "speaker_name"=\$1,"updated_at"=\$2 WHERE recording_id = \$3 AND speaker_label = \$4`).
		WithArgs("Bayu", sqlmock.AnyArg(), recID, "SPEAKER_00").
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectCommit()

	ctrls.UpdateTranscriptSpeakers(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d (body: %s)", w.Code, w.Body.String())
	}

	var resp dtos.APIResponse[dtos.UpdateSpeakersResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse json response: %v", err)
	}
	if resp.Data.UpdatedCount != 3 {
		t.Errorf("expected UpdatedCount 3, got %d", resp.Data.UpdatedCount)
	}
	if resp.Data.Speakers["SPEAKER_00"] != "Bayu" {
		t.Errorf("expected speaker name Bayu, got %s", resp.Data.Speakers["SPEAKER_00"])
	}
}

func TestControllers_UpdateTranscriptSpeakers_Success_HeaderToken(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	guestToken := "guest-ownership-token"
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req := httptest.NewRequest(http.MethodPut, "/v1/recordings/"+recID.String()+"/speakers", strings.NewReader(`{"speakers":{"SPEAKER_01":"Alice"}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Ownership-Token", guestToken)
	c.Request = req

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token"}).
			AddRow(recID, guestToken))

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "transcript_segments" SET "speaker_name"=\$1,"updated_at"=\$2 WHERE recording_id = \$3 AND speaker_label = \$4`).
		WithArgs("Alice", sqlmock.AnyArg(), recID, "SPEAKER_01").
		WillReturnResult(sqlmock.NewResult(0, 5))
	mock.ExpectCommit()

	ctrls.UpdateTranscriptSpeakers(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d (body: %s)", w.Code, w.Body.String())
	}

	var resp dtos.APIResponse[dtos.UpdateSpeakersResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse json response: %v", err)
	}
	if resp.Data.UpdatedCount != 5 {
		t.Errorf("expected UpdatedCount 5, got %d", resp.Data.UpdatedCount)
	}
}

func TestControllers_RegenerateSummary_InvalidUUID(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "invalid-uuid"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/recordings/invalid-uuid/regenerate", nil)

	ctrls.RegenerateSummary(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_RegenerateSummary_InvalidJSON(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/regenerate", strings.NewReader(`{invalid json`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	ctrls.RegenerateSummary(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_RegenerateSummary_ValidationError(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	tooLongCategory := strings.Repeat("c", 101)
	body, _ := json.Marshal(dtos.RegenerateSummaryRequest{TemplateCategory: tooLongCategory})
	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/regenerate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	ctrls.RegenerateSummary(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request on validation error, got %d", w.Code)
	}
}

func TestControllers_RegenerateSummary_NotFound(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/regenerate", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	ctrls.RegenerateSummary(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", w.Code)
	}
}

func TestControllers_RegenerateSummary_Forbidden(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/regenerate", strings.NewReader(`{"ownership_token":"wrong"}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token"}).
			AddRow(recID, "secret-token"))

	ctrls.RegenerateSummary(c)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden, got %d", w.Code)
	}
}

func TestControllers_RegenerateSummary_ConflictProcessing(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	guestToken := "guest-token"
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/regenerate", strings.NewReader(`{"ownership_token":"`+guestToken+`"}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "status"}).
			AddRow(recID, guestToken, models.RecordingStatusTranscribing))

	ctrls.RegenerateSummary(c)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict, got %d", w.Code)
	}

	var resp dtos.BaseResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Code != "CONFLICT_PROCESSING" {
		t.Errorf("expected code CONFLICT_PROCESSING, got %s", resp.Code)
	}
}

func TestControllers_RegenerateSummary_VersionLimit(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	guestToken := "guest-token"
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/regenerate", strings.NewReader(`{"ownership_token":"`+guestToken+`"}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "status"}).
			AddRow(recID, guestToken, models.RecordingStatusCompleted))

	mock.ExpectQuery(`SELECT count\(\*\) FROM "summaries" WHERE recording_id = \$1`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(5))

	ctrls.RegenerateSummary(c)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict, got %d", w.Code)
	}

	var resp dtos.BaseResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Code != "SUMMARY_VERSION_LIMIT" {
		t.Errorf("expected code SUMMARY_VERSION_LIMIT, got %s", resp.Code)
	}
}

func TestControllers_RegenerateSummary_Success_BodyToken(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	ctrls.svc.SetLLM(&mockControllerLLM{})

	recID := uuid.New()
	guestToken := "guest-token"
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	bodyJSON := `{"template_category":"MOM","custom_angle":"Focus on decisions","ownership_token":"` + guestToken + `"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/regenerate", strings.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "status", "detected_language", "output_language", "selected_template"}).
			AddRow(recID, guestToken, models.RecordingStatusCompleted, "en", "en", "GENERAL"))

	mock.ExpectQuery(`SELECT count\(\*\) FROM "summaries" WHERE recording_id = \$1`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	mock.ExpectQuery(`SELECT \* FROM "transcript_segments" WHERE recording_id = \$1 ORDER BY sequence_order ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "speaker_name", "text", "start_time", "end_time"}).
			AddRow(uuid.New(), recID, "Bayu", "Discussing database schema.", 0.0, 10.0))

	mock.ExpectQuery(`SELECT \* FROM "templates" WHERE category_key = \$1 AND is_active = TRUE.*LIMIT \$2`).
		WithArgs("MOM", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "category_key", "prompt", "output_schema"}).
			AddRow(uuid.New(), "MOM", "MOM prompt", "{}"))

	// SaveNewSummaryVersion expectations
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT count\(\*\) FROM "summaries" WHERE recording_id = \$1`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`SELECT COALESCE\(MAX\(version\), 0\) FROM "summaries" WHERE recording_id = \$1`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"coalesce"}).AddRow(1))
	mock.ExpectExec(`UPDATE "summaries" SET "is_active"=\$1,"updated_at"=\$2 WHERE recording_id = \$3`).
		WithArgs(false, sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(`INSERT INTO "summaries"`).
		WithArgs(recID, "MOM", sqlmock.AnyArg(), 2, true, sqlmock.AnyArg(), "# Regenerated Summary", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(uuid.New(), time.Now(), time.Now()))
	mock.ExpectCommit()

	ctrls.RegenerateSummary(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d (body: %s)", w.Code, w.Body.String())
	}

	var resp dtos.APIResponse[dtos.SummaryVersionResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse json response: %v", err)
	}
	if resp.Data.Version != 2 {
		t.Errorf("expected version 2, got %d", resp.Data.Version)
	}
	if resp.Data.TemplateCategory != "MOM" {
		t.Errorf("expected template MOM, got %s", resp.Data.TemplateCategory)
	}
	if resp.Data.CustomAngle == nil || *resp.Data.CustomAngle != "Focus on decisions" {
		t.Errorf("expected custom angle 'Focus on decisions', got %v", resp.Data.CustomAngle)
	}
	if resp.Data.MarkdownContent != "# Regenerated Summary" {
		t.Errorf("unexpected MarkdownContent: %s", resp.Data.MarkdownContent)
	}
}

func TestControllers_RegenerateSummary_Success_HeaderToken(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	ctrls.svc.SetLLM(&mockControllerLLM{})

	recID := uuid.New()
	guestToken := "header-ownership-token"
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/regenerate", nil)
	req.Header.Set("X-Ownership-Token", guestToken)
	c.Request = req

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "status", "detected_language", "output_language", "selected_template"}).
			AddRow(recID, guestToken, models.RecordingStatusCompleted, "id", "id", "GENERAL"))

	mock.ExpectQuery(`SELECT count\(\*\) FROM "summaries" WHERE recording_id = \$1`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	mock.ExpectQuery(`SELECT \* FROM "transcript_segments" WHERE recording_id = \$1 ORDER BY sequence_order ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "speaker_name", "text", "start_time", "end_time"}).
			AddRow(uuid.New(), recID, "Alice", "Pembahasan sistem.", 0.0, 10.0))

	mock.ExpectQuery(`SELECT \* FROM "templates" WHERE category_key = \$1 AND is_active = TRUE.*LIMIT \$2`).
		WithArgs("GENERAL", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "category_key", "prompt", "output_schema"}).
			AddRow(uuid.New(), "GENERAL", "General prompt", "{}"))

	// SaveNewSummaryVersion expectations
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
		WithArgs(recID, "GENERAL", nil, 3, true, sqlmock.AnyArg(), "# Regenerated Summary", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(uuid.New(), time.Now(), time.Now()))
	mock.ExpectCommit()

	ctrls.RegenerateSummary(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d (body: %s)", w.Code, w.Body.String())
	}

	var resp dtos.APIResponse[dtos.SummaryVersionResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse json response: %v", err)
	}
	if resp.Data.Version != 3 {
		t.Errorf("expected version 3, got %d", resp.Data.Version)
	}
}

func TestControllers_ListSummaryVersions_InvalidUUID(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "invalid-uuid"}}
	c.Request, _ = http.NewRequest(http.MethodGet, "/v1/recordings/invalid-uuid/summaries", nil)

	ctrls.ListSummaryVersions(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_ListSummaryVersions_NotFound(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	c.Request, _ = http.NewRequest(http.MethodGet, "/v1/recordings/"+recID.String()+"/summaries", nil)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	ctrls.ListSummaryVersions(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", w.Code)
	}
}

func TestControllers_ListSummaryVersions_Forbidden(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	c.Request, _ = http.NewRequest(http.MethodGet, "/v1/recordings/"+recID.String()+"/summaries", nil)
	c.Request.Header.Set("X-Ownership-Token", "wrong-token")

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id"}).
			AddRow(recID, "valid-token", nil))

	ctrls.ListSummaryVersions(c)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden, got %d", w.Code)
	}
}

func TestControllers_ListSummaryVersions_Success_GuestWithHeader(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	guestToken := "guest-token-123"

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req, _ := http.NewRequest(http.MethodGet, "/v1/recordings/"+recID.String()+"/summaries", nil)
	req.Header.Set("X-Ownership-Token", guestToken)
	c.Request = req

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id"}).
			AddRow(recID, guestToken, nil))

	sum1ID := uuid.New()
	sum2ID := uuid.New()
	now := time.Now()

	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 ORDER BY version ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "version", "template_category", "structured_data", "markdown_content", "is_active", "created_at"}).
			AddRow(sum1ID, recID, 1, "GENERAL", models.JSONMap{"summary": "v1"}, "# V1", false, now.Add(-10*time.Minute)).
			AddRow(sum2ID, recID, 2, "MOM", models.JSONMap{"summary": "v2"}, "# V2", true, now))

	ctrls.ListSummaryVersions(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d (body: %s)", w.Code, w.Body.String())
	}

	var resp dtos.APIResponse[[]dtos.SummaryVersionResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode json response: %v", err)
	}

	if len(resp.Data) != 2 {
		t.Fatalf("expected 2 summary versions, got %d", len(resp.Data))
	}
	if resp.Data[0].Version != 1 || resp.Data[0].IsActive {
		t.Errorf("unexpected version 1 data: %+v", resp.Data[0])
	}
	if resp.Data[1].Version != 2 || !resp.Data[1].IsActive {
		t.Errorf("unexpected version 2 data: %+v", resp.Data[1])
	}
}

func TestControllers_ListSummaryVersions_Success_GuestWithQuery(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	guestToken := "guest-token-123"

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req, _ := http.NewRequest(http.MethodGet, "/v1/recordings/"+recID.String()+"/summaries?token="+guestToken, nil)
	c.Request = req

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id"}).
			AddRow(recID, guestToken, nil))

	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 ORDER BY version ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "version", "template_category", "structured_data", "markdown_content", "is_active", "created_at"}).
			AddRow(uuid.New(), recID, 1, "GENERAL", models.JSONMap{"summary": "v1"}, "# V1", true, time.Now()))

	ctrls.ListSummaryVersions(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestControllers_ListSummaryVersions_Success_AuthenticatedOwner(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	userID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req, _ := http.NewRequest(http.MethodGet, "/v1/recordings/"+recID.String()+"/summaries", nil)
	c.Request = req.WithContext(ctxmeta.WithAuthUser(req.Context(), ctxmeta.AuthUser{UserID: userID}))

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id"}).
			AddRow(recID, "token", &userID))

	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 ORDER BY version ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "version", "template_category", "structured_data", "markdown_content", "is_active", "created_at"}).
			AddRow(uuid.New(), recID, 1, "GENERAL", models.JSONMap{"summary": "v1"}, "# V1", true, time.Now()))

	ctrls.ListSummaryVersions(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestControllers_ActivateSummaryVersion_InvalidID(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: "invalid-uuid"},
		{Key: "versionId", Value: "1"},
	}
	c.Request, _ = http.NewRequest(http.MethodPatch, "/v1/recordings/invalid-uuid/summaries/1/activate", nil)

	ctrls.ActivateSummaryVersion(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_ActivateSummaryVersion_NotFound_Recording(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: recID.String()},
		{Key: "versionId", Value: "1"},
	}
	c.Request, _ = http.NewRequest(http.MethodPatch, "/v1/recordings/"+recID.String()+"/summaries/1/activate", nil)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	ctrls.ActivateSummaryVersion(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", w.Code)
	}
}

func TestControllers_ActivateSummaryVersion_NotFound_Summary(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	missingSumID := uuid.New()
	guestToken := "guest-token"

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: recID.String()},
		{Key: "versionId", Value: missingSumID.String()},
	}
	req, _ := http.NewRequest(http.MethodPatch, "/v1/recordings/"+recID.String()+"/summaries/"+missingSumID.String()+"/activate", nil)
	req.Header.Set("X-Ownership-Token", guestToken)
	c.Request = req

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id"}).
			AddRow(recID, guestToken, nil))

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND id = \$2.*LIMIT \$3`).
		WithArgs(recID, missingSumID, 1).
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectRollback()

	ctrls.ActivateSummaryVersion(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestControllers_ActivateSummaryVersion_Forbidden(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: recID.String()},
		{Key: "versionId", Value: "1"},
	}
	req, _ := http.NewRequest(http.MethodPatch, "/v1/recordings/"+recID.String()+"/summaries/1/activate", nil)
	req.Header.Set("X-Ownership-Token", "wrong-token")
	c.Request = req

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id"}).
			AddRow(recID, "valid-token", nil))

	ctrls.ActivateSummaryVersion(c)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden, got %d", w.Code)
	}
}

func TestControllers_ActivateSummaryVersion_Success_GuestWithHeader(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	targetSumID := uuid.New()
	guestToken := "guest-token"

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: recID.String()},
		{Key: "versionId", Value: targetSumID.String()},
	}
	req, _ := http.NewRequest(http.MethodPatch, "/v1/recordings/"+recID.String()+"/summaries/"+targetSumID.String()+"/activate", nil)
	req.Header.Set("X-Ownership-Token", guestToken)
	c.Request = req

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id"}).
			AddRow(recID, guestToken, nil))

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND id = \$2.*LIMIT \$3`).
		WithArgs(recID, targetSumID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "version", "template_category", "structured_data", "markdown_content", "is_active", "created_at"}).
			AddRow(targetSumID, recID, 2, "GENERAL", models.JSONMap{"summary": "v2"}, "# V2", false, time.Now()))
	mock.ExpectExec(`UPDATE "summaries" SET "is_active"=\$1,"updated_at"=\$2 WHERE recording_id = \$3`).
		WithArgs(false, sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 2))
	mock.ExpectExec(`UPDATE "summaries" SET "is_active"=\$1,"updated_at"=\$2 WHERE id = \$3`).
		WithArgs(true, sqlmock.AnyArg(), targetSumID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	ctrls.ActivateSummaryVersion(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d (body: %s)", w.Code, w.Body.String())
	}

	var resp dtos.APIResponse[dtos.SummaryVersionResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response json: %v", err)
	}

	if resp.Data.ID != targetSumID.String() || !resp.Data.IsActive || resp.Data.Version != 2 {
		t.Errorf("unexpected activated response data: %+v", resp.Data)
	}
}

func TestControllers_ActivateSummaryVersion_Success_AuthenticatedOwner(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	targetSumID := uuid.New()
	userID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: recID.String()},
		{Key: "versionId", Value: "3"},
	}
	req, _ := http.NewRequest(http.MethodPatch, "/v1/recordings/"+recID.String()+"/summaries/3/activate", nil)
	c.Request = req.WithContext(ctxmeta.WithAuthUser(req.Context(), ctxmeta.AuthUser{UserID: userID}))

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id"}).
			AddRow(recID, "token", &userID))

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND version = \$2.*LIMIT \$3`).
		WithArgs(recID, 3, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "version", "template_category", "structured_data", "markdown_content", "is_active", "created_at"}).
			AddRow(targetSumID, recID, 3, "EXECUTIVE", models.JSONMap{"summary": "v3"}, "# V3", false, time.Now()))
	mock.ExpectExec(`UPDATE "summaries" SET "is_active"=\$1,"updated_at"=\$2 WHERE recording_id = \$3`).
		WithArgs(false, sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 3))
	mock.ExpectExec(`UPDATE "summaries" SET "is_active"=\$1,"updated_at"=\$2 WHERE id = \$3`).
		WithArgs(true, sqlmock.AnyArg(), targetSumID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	ctrls.ActivateSummaryVersion(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestControllers_ExportRecording_InvalidUUID(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "invalid-uuid"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings/invalid-uuid/export", nil)

	ctrls.ExportRecording(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_ExportRecording_RecordingNotFound(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings/"+recID.String()+"/export?format=markdown", nil)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	ctrls.ExportRecording(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", w.Code)
	}
}

func TestControllers_ExportRecording_Forbidden(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	ownerID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings/"+recID.String()+"/export?format=markdown", nil)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id", "is_share_enabled"}).
			AddRow(recID, "secret-token", &ownerID, false))

	ctrls.ExportRecording(c)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden, got %d", w.Code)
	}
}

func TestControllers_ExportRecording_Success_Markdown(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	guestToken := "guest-token-123"
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req := httptest.NewRequest(http.MethodGet, "/v1/recordings/"+recID.String()+"/export?format=markdown", nil)
	req.Header.Set("X-Ownership-Token", guestToken)
	c.Request = req

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "title", "duration_seconds", "status", "created_at", "user_id", "ownership_token", "is_share_enabled"}).
			AddRow(recID, "Product Sync", 600.0, "COMPLETED", time.Now(), nil, guestToken, false))

	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND is_active = TRUE ORDER BY version DESC.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "version", "is_active", "markdown_content", "structured_data"}).
			AddRow(uuid.New(), recID, 1, true, "Product sync markdown summary", `{"executive_summary":"Sync summary"}`))

	mock.ExpectQuery(`SELECT \* FROM "transcript_segments" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "speaker_name", "speaker_label", "start_time", "end_time", "text"}).
			AddRow(uuid.New(), recID, "PM", "Speaker 0", 0.0, 5.0, "Let's review the roadmap."))

	mock.ExpectQuery(`SELECT \* FROM "chapters" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC.*`).
		WithArgs(recID, 100).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id"}))

	mock.ExpectQuery(`SELECT \* FROM "highlights" WHERE recording_id = \$1 ORDER BY start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id"}))

	ctrls.ExportRecording(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d, body: %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Content-Type") != "text/markdown; charset=utf-8" {
		t.Errorf("expected text/markdown, got %s", w.Header().Get("Content-Type"))
	}
	if !strings.Contains(w.Header().Get("Content-Disposition"), "attachment; filename=") {
		t.Errorf("expected Content-Disposition attachment header, got %s", w.Header().Get("Content-Disposition"))
	}
}

func TestControllers_ExportRecording_Success_JSON(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings/"+recID.String()+"/export?format=json&token=shared-token", nil)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "title", "duration_seconds", "status", "created_at", "user_id", "ownership_token", "is_share_enabled"}).
			AddRow(recID, "API Planning", 300.0, "COMPLETED", time.Now(), nil, "shared-token", false))

	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND is_active = TRUE ORDER BY version DESC.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	mock.ExpectQuery(`SELECT \* FROM "transcript_segments" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id"}))

	mock.ExpectQuery(`SELECT \* FROM "chapters" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC.*`).
		WithArgs(recID, 100).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id"}))

	mock.ExpectQuery(`SELECT \* FROM "highlights" WHERE recording_id = \$1 ORDER BY start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id"}))

	ctrls.ExportRecording(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}
	if w.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Errorf("expected application/json, got %s", w.Header().Get("Content-Type"))
	}
}

func TestControllers_ExportRecording_Success_PDF(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings/"+recID.String()+"/export?format=pdf", nil)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "title", "duration_seconds", "status", "created_at", "user_id", "ownership_token", "is_share_enabled"}).
			AddRow(recID, "PDF Export Title", 100.0, "COMPLETED", time.Now(), nil, "token", true))

	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND is_active = TRUE ORDER BY version DESC.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	mock.ExpectQuery(`SELECT \* FROM "transcript_segments" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id"}))

	mock.ExpectQuery(`SELECT \* FROM "chapters" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC.*`).
		WithArgs(recID, 100).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id"}))

	mock.ExpectQuery(`SELECT \* FROM "highlights" WHERE recording_id = \$1 ORDER BY start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id"}))

	ctrls.ExportRecording(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}
	if w.Header().Get("Content-Type") != "application/pdf" {
		t.Errorf("expected application/pdf, got %s", w.Header().Get("Content-Type"))
	}
	if !bytes.HasPrefix(w.Body.Bytes(), []byte("%PDF-1.4")) {
		t.Errorf("expected PDF header in body")
	}
}

func TestControllers_ExportRecording_Success_Txt(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings/"+recID.String()+"/export?format=txt", nil)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "title", "duration_seconds", "status", "created_at", "user_id", "ownership_token", "is_share_enabled"}).
			AddRow(recID, "Txt Export Title", 100.0, "COMPLETED", time.Now(), nil, "token", true))

	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND is_active = TRUE ORDER BY version DESC.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	mock.ExpectQuery(`SELECT \* FROM "transcript_segments" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id"}))

	mock.ExpectQuery(`SELECT \* FROM "chapters" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC.*`).
		WithArgs(recID, 100).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id"}))

	mock.ExpectQuery(`SELECT \* FROM "highlights" WHERE recording_id = \$1 ORDER BY start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id"}))

	ctrls.ExportRecording(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}
	if w.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Errorf("expected text/plain, got %s", w.Header().Get("Content-Type"))
	}
}

func TestControllers_GetWorkspaceSpeakers_Unauthorized(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/speakers", nil)

	ctrls.GetWorkspaceSpeakers(c)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d", w.Code)
	}
}

func TestControllers_GetWorkspaceSpeakers_ServiceError(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	userID := uuid.New()
	expectedSQL := regexp.QuoteMeta(`SELECT ts.speaker_name AS name, COUNT(DISTINCT ts.recording_id) AS total_meetings, COALESCE(SUM(GREATEST(0, ts.end_time - ts.start_time)), 0) AS total_talk_time, MAX(r.created_at) AS last_active FROM transcript_segments ts JOIN recordings r ON ts.recording_id = r.id WHERE r.user_id = $1 AND r.deleted_at IS NULL AND TRIM(ts.speaker_name) != '' GROUP BY "ts"."speaker_name" ORDER BY total_meetings DESC, total_talk_time DESC, name ASC`)

	mock.ExpectQuery(expectedSQL).
		WithArgs(userID).
		WillReturnError(errors.New("db aggregation failed"))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "/v1/speakers", nil)
	c.Request = req.WithContext(ctxmeta.WithAuthUser(req.Context(), ctxmeta.AuthUser{UserID: userID}))

	ctrls.GetWorkspaceSpeakers(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 Internal Server Error, got %d", w.Code)
	}
}

func TestControllers_GetWorkspaceSpeakers_Success(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	userID := uuid.New()
	now := time.Now().Truncate(time.Second)
	expectedSQL := regexp.QuoteMeta(`SELECT ts.speaker_name AS name, COUNT(DISTINCT ts.recording_id) AS total_meetings, COALESCE(SUM(GREATEST(0, ts.end_time - ts.start_time)), 0) AS total_talk_time, MAX(r.created_at) AS last_active FROM transcript_segments ts JOIN recordings r ON ts.recording_id = r.id WHERE r.user_id = $1 AND r.deleted_at IS NULL AND TRIM(ts.speaker_name) != '' GROUP BY "ts"."speaker_name" ORDER BY total_meetings DESC, total_talk_time DESC, name ASC`)

	rows := sqlmock.NewRows([]string{"name", "total_meetings", "total_talk_time", "last_active"}).
		AddRow("Alice", 4, 600.0, now).
		AddRow("Bob", 2, 180.0, now.Add(-time.Hour))

	mock.ExpectQuery(expectedSQL).
		WithArgs(userID).
		WillReturnRows(rows)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "/v1/speakers", nil)
	c.Request = req.WithContext(ctxmeta.WithAuthUser(req.Context(), ctxmeta.AuthUser{UserID: userID}))

	ctrls.GetWorkspaceSpeakers(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var resp dtos.APIResponse[dtos.SpeakerDirectoryResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Data.Count != 2 {
		t.Errorf("expected count 2, got %d", resp.Data.Count)
	}
	if len(resp.Data.Speakers) != 2 {
		t.Fatalf("expected 2 speakers, got %d", len(resp.Data.Speakers))
	}
	if resp.Data.Speakers[0].Name != "Alice" || resp.Data.Speakers[0].TotalMeetings != 4 {
		t.Errorf("unexpected Alice data: %+v", resp.Data.Speakers[0])
	}
	if resp.Data.Speakers[1].Name != "Bob" || resp.Data.Speakers[1].TotalMeetings != 2 {
		t.Errorf("unexpected Bob data: %+v", resp.Data.Speakers[1])
	}
}
