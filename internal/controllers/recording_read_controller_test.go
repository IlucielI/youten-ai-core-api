package controllers

import (
	"context"
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
	"code-base-golang/internal/pkg/ctxmeta"
	"code-base-golang/internal/sse"
)

func TestControllers_GetRecordingDetail_InvalidUUID(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings/not-a-uuid", nil)
	c.Params = gin.Params{{Key: "id", Value: "not-a-uuid"}}

	ctrls.GetRecordingDetail(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestControllers_GetRecordingDetail_NotFound(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings/"+recID.String(), nil)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	ctrls.GetRecordingDetail(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestControllers_GetRecordingDetail_Forbidden(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings/"+recID.String(), nil)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "is_guest"}).
			AddRow(recID, "secret-token", true))

	ctrls.GetRecordingDetail(c)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestControllers_GetRecordingDetail_Success_QueryToken(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	token := "valid-token-123"
	now := time.Now()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := httptest.NewRequest(http.MethodGet, "/v1/recordings/"+recID.String()+"?token="+token, nil)
	c.Request = req
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "title", "original_filename", "audio_url", "source_type",
			"status", "selected_template", "output_language", "is_guest", "ownership_token", "created_at", "updated_at",
		}).AddRow(
			recID, "Meeting Recording", "meeting.mp3", "recordings/"+recID.String()+"/meeting.mp3", "UPLOAD",
			"COMPLETED", "MOM", "id", true, token, now, now,
		))

	mock.ExpectQuery(`SELECT \* FROM "transcript_segments" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND is_active = TRUE`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	mock.ExpectQuery(`SELECT \* FROM "chapters" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC LIMIT \$2`).
		WithArgs(recID, 100).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	mock.ExpectQuery(`SELECT \* FROM "highlights" WHERE recording_id = \$1 ORDER BY start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	ctrls.GetRecordingDetail(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp dtos.APIResponse[dtos.RecordingDetailResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}
	if resp.Data.ID != recID.String() {
		t.Errorf("expected ID %s, got %s", recID.String(), resp.Data.ID)
	}
	if resp.Data.Title != "Meeting Recording" {
		t.Errorf("expected title Meeting Recording, got %s", resp.Data.Title)
	}
}

func TestControllers_GetRecordingDetail_Success_HeaderToken(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	token := "header-token-456"
	now := time.Now()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := httptest.NewRequest(http.MethodGet, "/v1/recordings/"+recID.String(), nil)
	req.Header.Set("X-Ownership-Token", token)
	c.Request = req
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "title", "original_filename", "audio_url", "source_type",
			"status", "selected_template", "output_language", "is_guest", "ownership_token", "created_at", "updated_at",
		}).AddRow(
			recID, "Header Auth Meeting", "voice.wav", "", "UPLOAD",
			"PENDING", "GENERAL", "en", true, token, now, now,
		))

	mock.ExpectQuery(`SELECT \* FROM "transcript_segments" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND is_active = TRUE`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	mock.ExpectQuery(`SELECT \* FROM "chapters" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC LIMIT \$2`).
		WithArgs(recID, 100).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	mock.ExpectQuery(`SELECT \* FROM "highlights" WHERE recording_id = \$1 ORDER BY start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	ctrls.GetRecordingDetail(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp dtos.APIResponse[dtos.RecordingDetailResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}
	if resp.Data.ID != recID.String() {
		t.Errorf("expected ID %s, got %s", recID.String(), resp.Data.ID)
	}
}

func TestControllers_ListRecordings_Unauthorized(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings", nil)

	ctrls.ListRecordings(c)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d", w.Code)
	}
}

func TestControllers_ListRecordings_Success(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	userID := uuid.New()
	recID := uuid.New()
	now := time.Now()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := httptest.NewRequest(http.MethodGet, "/v1/recordings?search=sprint&status=COMPLETED&template=MOM&page=1&limit=10&sort_by=created_at&sort_order=desc", nil)
	ctx := ctxmeta.WithAuthUser(req.Context(), ctxmeta.AuthUser{UserID: userID})
	c.Request = req.WithContext(ctx)

	mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings" WHERE user_id = \$1 AND \(LOWER\(title\) LIKE \$2 OR LOWER\(original_filename\) LIKE \$3\) AND status = \$4 AND selected_template = \$5 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(userID, "%sprint%", "%sprint%", "COMPLETED", "MOM").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE user_id = \$1 AND \(LOWER\(title\) LIKE \$2 OR LOWER\(original_filename\) LIKE \$3\) AND status = \$4 AND selected_template = \$5 AND "recordings"\."deleted_at" IS NULL ORDER BY created_at DESC LIMIT \$6`).
		WithArgs(userID, "%sprint%", "%sprint%", "COMPLETED", "MOM", 10).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "title", "original_filename", "file_size_bytes", "duration_seconds",
			"source_type", "status", "selected_template", "output_language", "is_guest", "created_at", "updated_at",
		}).AddRow(
			recID, userID, "Sprint Planning Meeting", "sprint.mp3", 2048, 180.0,
			"UPLOAD", "COMPLETED", "MOM", "id", false, now, now,
		))

	ctrls.ListRecordings(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp dtos.APIResponse[dtos.RecordingListResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if len(resp.Data.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(resp.Data.Items))
	}
	if resp.Data.Items[0].Title != "Sprint Planning Meeting" {
		t.Errorf("expected Title 'Sprint Planning Meeting', got %s", resp.Data.Items[0].Title)
	}
	if resp.Data.Pagination.TotalItems != 1 {
		t.Errorf("expected total items 1, got %d", resp.Data.Pagination.TotalItems)
	}
}

func TestControllers_ListRecordings_ServiceError(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	userID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := httptest.NewRequest(http.MethodGet, "/v1/recordings", nil)
	ctx := ctxmeta.WithAuthUser(req.Context(), ctxmeta.AuthUser{UserID: userID})
	c.Request = req.WithContext(ctx)

	mock.ExpectQuery(`SELECT count\(\*\) FROM "recordings" WHERE user_id = \$1 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(userID).
		WillReturnError(errors.New("db query error"))

	ctrls.ListRecordings(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 Internal Server Error, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestControllers_StreamRecordingProgress_InvalidUUID(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "invalid-uuid"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings/invalid-uuid/progress", nil)

	ctrls.StreamRecordingProgress(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_StreamRecordingProgress_NotFound(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings/"+recID.String()+"/progress", nil)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	ctrls.StreamRecordingProgress(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", w.Code)
	}
}

func TestControllers_StreamRecordingProgress_Forbidden(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	userID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings/"+recID.String()+"/progress", nil)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "ownership_token", "is_share_enabled"}).
			AddRow(recID, &userID, "token-1", false))

	ctrls.StreamRecordingProgress(c)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden, got %d", w.Code)
	}
}

func TestControllers_StreamRecordingProgress_TerminalInitial_Completed(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	userID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req := httptest.NewRequest(http.MethodGet, "/v1/recordings/"+recID.String()+"/progress", nil)
	ctx := ctxmeta.WithAuthUser(req.Context(), ctxmeta.AuthUser{UserID: userID})
	c.Request = req.WithContext(ctx)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "status", "ownership_token"}).
			AddRow(recID, &userID, "COMPLETED", "token-1"))

	ctrls.StreamRecordingProgress(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}
	if !strings.Contains(w.Header().Get("Content-Type"), "text/event-stream") {
		t.Errorf("expected text/event-stream Content-Type, got %s", w.Header().Get("Content-Type"))
	}
	body := w.Body.String()
	if !strings.Contains(body, "event: progress\n") || !strings.Contains(body, `"status":"COMPLETED"`) {
		t.Errorf("expected progress event with COMPLETED, got: %s", body)
	}
}

func TestControllers_StreamRecordingProgress_StreamTransitions(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	userID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req := httptest.NewRequest(http.MethodGet, "/v1/recordings/"+recID.String()+"/progress", nil)
	ctx := ctxmeta.WithAuthUser(req.Context(), ctxmeta.AuthUser{UserID: userID})
	c.Request = req.WithContext(ctx)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "status", "ownership_token"}).
			AddRow(recID, &userID, "EXTRACTING", "token-1"))

	// In background, broadcast next events to hub, culminating in COMPLETED
	go func() {
		time.Sleep(50 * time.Millisecond)
		ctrls.Service().SSEHub().Publish(recID, sse.ProgressEvent{
			RecordingID: recID.String(),
			Status:      "TRANSCRIBING",
			Stage:       "TRANSCRIBING",
			Progress:    55,
		})
		time.Sleep(50 * time.Millisecond)
		ctrls.Service().SSEHub().Publish(recID, sse.ProgressEvent{
			RecordingID: recID.String(),
			Status:      "COMPLETED",
			Stage:       "COMPLETED",
			Progress:    100,
		})
	}()

	ctrls.StreamRecordingProgress(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"status":"EXTRACTING"`) {
		t.Errorf("expected initial EXTRACTING status, got: %s", body)
	}
	if !strings.Contains(body, `"status":"TRANSCRIBING"`) {
		t.Errorf("expected streamed TRANSCRIBING status, got: %s", body)
	}
	if !strings.Contains(body, `"status":"COMPLETED"`) {
		t.Errorf("expected final COMPLETED status, got: %s", body)
	}
}

func TestControllers_StreamRecordingProgress_ClientDisconnect(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	userID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}

	reqCtx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/v1/recordings/"+recID.String()+"/progress", nil).WithContext(reqCtx)
	ctx := ctxmeta.WithAuthUser(req.Context(), ctxmeta.AuthUser{UserID: userID})
	c.Request = req.WithContext(ctx)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "status", "ownership_token"}).
			AddRow(recID, &userID, "EXTRACTING", "token-1"))

	// Cancel context shortly after start
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	ctrls.StreamRecordingProgress(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"status":"EXTRACTING"`) {
		t.Errorf("expected initial EXTRACTING status, got: %s", body)
	}
}

func TestControllers_SearchRecordings_InvalidQuery(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings/search?q=", nil)

	ctrls.SearchRecordings(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_SearchRecordings_ServiceError(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	// Missing auth context -> returns 401 Unauthorized
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings/search?q=something", nil)

	ctrls.SearchRecordings(c)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d", w.Code)
	}
}

func TestControllers_SearchRecordings_Success(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	userID := uuid.New()
	recID := uuid.New()
	chunkID := uuid.New()

	ctrls.Service().SetEmbedding(&dummyEmbeddingProvider{})

	expectedSQL := `SELECT tc.id, tc.recording_id, r.title as recording_title, tc.chunk_index, tc.content, tc.start_time, tc.end_time, (tc.embedding <=> $1) as distance FROM transcript_chunks tc JOIN recordings r ON tc.recording_id = r.id WHERE (r.user_id = $2 AND r.deleted_at IS NULL) AND (tc.embedding <=> $3) <= $4 ORDER BY tc.embedding <=> $5 LIMIT $6`
	mock.ExpectQuery(regexp.QuoteMeta(expectedSQL)).
		WithArgs(sqlmock.AnyArg(), userID, sqlmock.AnyArg(), 0.3, sqlmock.AnyArg(), 10).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "recording_title", "chunk_index", "content", "start_time", "end_time", "distance"}).
			AddRow(chunkID, recID, "Quarterly Business Review", 1, "We reviewed annual profit projections.", 30.0, 65.0, 0.12))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "/v1/recordings/search?q=profit%20projections&limit=10&threshold=0.7", nil)
	c.Request = req.WithContext(ctxmeta.WithAuthUser(req.Context(), ctxmeta.AuthUser{UserID: userID}))

	ctrls.SearchRecordings(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var resp dtos.APIResponse[dtos.SemanticSearchResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Data.Count != 1 {
		t.Errorf("expected count 1, got %d", resp.Data.Count)
	}
	if len(resp.Data.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Data.Results))
	}
	if resp.Data.Results[0].RecordingTitle != "Quarterly Business Review" {
		t.Errorf("expected recording title 'Quarterly Business Review', got %s", resp.Data.Results[0].RecordingTitle)
	}
	if resp.Data.Results[0].Score != 0.88 {
		t.Errorf("expected score 0.88, got %f", resp.Data.Results[0].Score)
	}
}
