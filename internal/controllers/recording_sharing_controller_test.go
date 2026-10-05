package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/dtos"
	"code-base-golang/internal/pkg/ctxmeta"
)

func TestControllers_DeleteRecording_InvalidUUID(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "invalid-uuid"}}
	c.Request = httptest.NewRequest(http.MethodDelete, "/v1/recordings/invalid-uuid", nil)

	ctrls.DeleteRecording(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_DeleteRecording_Unauthorized(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	c.Request = httptest.NewRequest(http.MethodDelete, "/v1/recordings/"+recID.String(), nil)

	ctrls.DeleteRecording(c)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d", w.Code)
	}
}

func TestControllers_DeleteRecording_NotFound(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	userID := uuid.New()
	recID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}

	req := httptest.NewRequest(http.MethodDelete, "/v1/recordings/"+recID.String(), nil)
	ctx := ctxmeta.WithAuthUser(req.Context(), ctxmeta.AuthUser{UserID: userID})
	c.Request = req.WithContext(ctx)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	ctrls.DeleteRecording(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", w.Code)
	}
}

func TestControllers_DeleteRecording_Forbidden(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	userID := uuid.New()
	otherUserID := uuid.New()
	recID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}

	req := httptest.NewRequest(http.MethodDelete, "/v1/recordings/"+recID.String(), nil)
	ctx := ctxmeta.WithAuthUser(req.Context(), ctxmeta.AuthUser{UserID: userID})
	c.Request = req.WithContext(ctx)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id"}).
			AddRow(recID, otherUserID))

	ctrls.DeleteRecording(c)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden, got %d", w.Code)
	}
}

func TestControllers_DeleteRecording_Success(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	userID := uuid.New()
	recID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}

	req := httptest.NewRequest(http.MethodDelete, "/v1/recordings/"+recID.String(), nil)
	ctx := ctxmeta.WithAuthUser(req.Context(), ctxmeta.AuthUser{UserID: userID})
	c.Request = req.WithContext(ctx)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id"}).
			AddRow(recID, userID))

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings" SET "deleted_at"=\$1 WHERE id = \$2 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	ctrls.DeleteRecording(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp dtos.BaseResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}
	if resp.Message != "recording deleted successfully" {
		t.Errorf("expected Message 'recording deleted successfully', got %s", resp.Message)
	}
}

func TestControllers_ClaimRecording_InvalidUUID(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "invalid-uuid"}}

	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/invalid-uuid/claim", strings.NewReader(`{"ownership_token":"tok"}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	ctrls.ClaimRecording(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_ClaimRecording_InvalidJSON(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}

	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/claim", strings.NewReader(`{invalid-json}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	ctrls.ClaimRecording(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_ClaimRecording_ValidationError(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}

	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/claim", strings.NewReader(`{"ownership_token":""}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	ctrls.ClaimRecording(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_ClaimRecording_ServiceError(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	userID := uuid.New()
	recID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}

	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/claim", strings.NewReader(`{"ownership_token":"token-123"}`))
	req.Header.Set("Content-Type", "application/json")
	ctx := ctxmeta.WithAuthUser(req.Context(), ctxmeta.AuthUser{UserID: userID})
	c.Request = req.WithContext(ctx)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "is_guest", "user_id", "ownership_token"}).
			AddRow(recID, false, nil, "token-123"))

	ctrls.ClaimRecording(c)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestControllers_ClaimRecording_Success(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	userID := uuid.New()
	recID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}

	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/claim", strings.NewReader(`{"ownership_token":"valid-token"}`))
	req.Header.Set("Content-Type", "application/json")
	ctx := ctxmeta.WithAuthUser(req.Context(), ctxmeta.AuthUser{UserID: userID})
	c.Request = req.WithContext(ctx)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "is_guest", "user_id", "ownership_token"}).
			AddRow(recID, true, nil, "valid-token"))

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings" SET "is_guest"=\$1,"user_id"=\$2,"updated_at"=\$3 WHERE \(id = \$4 AND ownership_token = \$5 AND is_guest = TRUE\) AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(false, userID, sqlmock.AnyArg(), recID, "valid-token").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	ctrls.ClaimRecording(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp dtos.BaseResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}
	if resp.Message != "recording claimed successfully" {
		t.Errorf("expected Message 'recording claimed successfully', got %s", resp.Message)
	}
}

func TestControllers_ClaimBulkRecordings_InvalidJSON(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/claim", strings.NewReader(`{invalid}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	ctrls.ClaimBulkRecordings(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_ClaimBulkRecordings_ValidationError(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/claim", strings.NewReader(`{"tokens":[]}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	ctrls.ClaimBulkRecordings(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_ClaimBulkRecordings_ServiceError(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/claim", strings.NewReader(`{"tokens":["tok-1"]}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req // No auth user

	ctrls.ClaimBulkRecordings(c)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestControllers_ClaimBulkRecordings_Success(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	userID := uuid.New()
	recID1 := uuid.New()
	recID2 := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/claim", strings.NewReader(`{"tokens":["tok-1","tok-2"]}`))
	req.Header.Set("Content-Type", "application/json")
	ctx := ctxmeta.WithAuthUser(req.Context(), ctxmeta.AuthUser{UserID: userID})
	c.Request = req.WithContext(ctx)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE \(ownership_token IN \(\$1,\$2\) AND is_guest = TRUE\) AND "recordings"\."deleted_at" IS NULL`).
		WithArgs("tok-1", "tok-2").
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "is_guest"}).
			AddRow(recID1, "tok-1", true).
			AddRow(recID2, "tok-2", true))

	mock.ExpectExec(`UPDATE "recordings" SET "is_guest"=\$1,"user_id"=\$2,"updated_at"=\$3 WHERE id IN \(\$4,\$5\) AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(false, userID, sqlmock.AnyArg(), recID1, recID2).
		WillReturnResult(sqlmock.NewResult(1, 2))
	mock.ExpectCommit()

	ctrls.ClaimBulkRecordings(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp dtos.APIResponse[dtos.BulkClaimResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}
	if resp.Data.ClaimedCount != 2 {
		t.Errorf("expected ClaimedCount 2, got %d", resp.Data.ClaimedCount)
	}
	if len(resp.Data.RecordingIDs) != 2 {
		t.Errorf("expected 2 RecordingIDs, got %d", len(resp.Data.RecordingIDs))
	}
}

func TestControllers_ToggleRecordingShare_InvalidUUID(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "invalid-uuid"}}

	req := httptest.NewRequest(http.MethodPatch, "/v1/recordings/invalid-uuid/share", strings.NewReader(`{"is_share_enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	ctrls.ToggleRecordingShare(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_ToggleRecordingShare_InvalidJSON(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}

	req := httptest.NewRequest(http.MethodPatch, "/v1/recordings/"+recID.String()+"/share", strings.NewReader(`{invalid}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	ctrls.ToggleRecordingShare(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_ToggleRecordingShare_ValidationError(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}

	req := httptest.NewRequest(http.MethodPatch, "/v1/recordings/"+recID.String()+"/share", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	ctrls.ToggleRecordingShare(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_ToggleRecordingShare_ServiceError(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}

	req := httptest.NewRequest(http.MethodPatch, "/v1/recordings/"+recID.String()+"/share", strings.NewReader(`{"is_share_enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req // No auth user

	ctrls.ToggleRecordingShare(c)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestControllers_ToggleRecordingShare_Success(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	userID := uuid.New()
	recID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}

	req := httptest.NewRequest(http.MethodPatch, "/v1/recordings/"+recID.String()+"/share", strings.NewReader(`{"is_share_enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	ctx := ctxmeta.WithAuthUser(req.Context(), ctxmeta.AuthUser{UserID: userID})
	c.Request = req.WithContext(ctx)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "share_token", "is_share_enabled"}).
			AddRow(recID, userID, nil, false))

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings" SET "is_share_enabled"=\$1,"share_token"=\$2,"updated_at"=\$3 WHERE id = \$4 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(true, sqlmock.AnyArg(), sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	ctrls.ToggleRecordingShare(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp dtos.APIResponse[dtos.ShareToggleResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}
	if !resp.Data.IsShareEnabled {
		t.Error("expected IsShareEnabled true")
	}
	if resp.Data.ShareToken == nil {
		t.Error("expected ShareToken non-nil")
	}
	if resp.Data.ShareURL == nil || !strings.Contains(*resp.Data.ShareURL, *resp.Data.ShareToken) {
		t.Errorf("expected ShareURL containing token, got %v", resp.Data.ShareURL)
	}
}

func TestControllers_GetSharedRecording_EmptyToken(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "token", Value: "  "}}
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings/shared/%20", nil)

	ctrls.GetSharedRecording(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", w.Code)
	}
}

func TestControllers_GetSharedRecording_NotFound(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "token", Value: "non-existent-token"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings/shared/non-existent-token", nil)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE \(share_token = \$1 AND is_share_enabled = TRUE\) AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs("non-existent-token", 1).
		WillReturnError(gorm.ErrRecordNotFound)

	ctrls.GetSharedRecording(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", w.Code)
	}
}

func TestControllers_GetSharedRecording_Success(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	audioKey := "recordings/audio.mp3"

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "token", Value: "valid-share-token"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings/shared/valid-share-token", nil)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE \(share_token = \$1 AND is_share_enabled = TRUE\) AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs("valid-share-token", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "title", "duration_seconds", "audio_url", "is_share_enabled", "share_token"}).
			AddRow(recID, "Public Demo", 120.0, audioKey, true, "valid-share-token"))

	mock.ExpectQuery(`SELECT \* FROM "transcript_segments" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "text"}).
			AddRow(uuid.New(), recID, "Public transcript"))

	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND is_active = TRUE.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "markdown_content", "is_active"}).
			AddRow(uuid.New(), recID, "Public summary", true))

	mock.ExpectQuery(`SELECT \* FROM "chapters" WHERE recording_id = \$1.*LIMIT \$2`).
		WithArgs(recID, 100).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "title"}).
			AddRow(uuid.New(), recID, "Public Chapter"))

	mock.ExpectQuery(`SELECT \* FROM "highlights" WHERE recording_id = \$1 ORDER BY start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "source"}).
			AddRow(uuid.New(), recID, "KEY_POINT"))

	ctrls.GetSharedRecording(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp dtos.APIResponse[dtos.SharedRecordingResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}
	if resp.Data.ID != recID.String() {
		t.Errorf("expected ID %s, got %s", recID.String(), resp.Data.ID)
	}
	if resp.Data.Title != "Public Demo" {
		t.Errorf("expected Title 'Public Demo', got %s", resp.Data.Title)
	}
	if len(resp.Data.Segments) != 1 {
		t.Errorf("expected 1 segment, got %d", len(resp.Data.Segments))
	}
}
