package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func TestControllers_CreateInlineComment_InvalidUUID(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "invalid-uuid"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/recordings/invalid-uuid/comments", strings.NewReader(`{}`))
	c.Request.Header.Set("Content-Type", "application/json")

	ctrls.CreateInlineComment(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_CreateInlineComment_BindError(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/comments", strings.NewReader("invalid-json"))
	c.Request.Header.Set("Content-Type", "application/json")

	ctrls.CreateInlineComment(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_CreateInlineComment_ValidationError(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/comments", strings.NewReader(`{"comment_text":""}`))
	c.Request.Header.Set("Content-Type", "application/json")

	ctrls.CreateInlineComment(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_CreateInlineComment_RecordingNotFound(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	body := `{"comment_text":"Good point","author_name":"Tester"}`
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/comments", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	ctrls.CreateInlineComment(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", w.Code)
	}
}

func TestControllers_CreateInlineComment_Forbidden(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	body := `{"comment_text":"Good point","author_name":"Tester"}`
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/comments", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id", "is_share_enabled"}).
			AddRow(recID, "secret-token", nil, false))

	ctrls.CreateInlineComment(c)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden, got %d", w.Code)
	}
}

func TestControllers_CreateInlineComment_Success_GuestWithHeader(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	guestToken := "guest-token-123"
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	body := `{"comment_text":"Great explanation here!","author_name":"Alice","timestamp_sec":12.5}`
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/comments", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("X-Ownership-Token", guestToken)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id", "is_share_enabled"}).
			AddRow(recID, guestToken, nil, false))

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "inline_comments"`).
		WithArgs(recID, nil, nil, 12.5, nil, "Alice", "Great explanation here!", nil, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).
			AddRow(uuid.New(), time.Now(), time.Now()))
	mock.ExpectCommit()

	ctrls.CreateInlineComment(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp dtos.APIResponse[dtos.CommentResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode json: %v", err)
	}
	if resp.Data.CommentText != "Great explanation here!" || resp.Data.AuthorName != "Alice" {
		t.Errorf("unexpected comment data: %+v", resp.Data)
	}
}

func TestControllers_CreateInlineComment_Success_ReplyToParent(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	parentID := uuid.New()
	guestToken := "guest-token-123"
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	body := fmt.Sprintf(`{"comment_text":"I agree with this","parent_id":"%s","ownership_token":"%s"}`, parentID.String(), guestToken)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/comments", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id", "is_share_enabled"}).
			AddRow(recID, guestToken, nil, false))

	mock.ExpectQuery(`SELECT \* FROM "inline_comments" WHERE id = \$1.*LIMIT \$2`).
		WithArgs(parentID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "comment_text"}).
			AddRow(parentID, recID, "Parent comment"))
	mock.ExpectQuery(`SELECT \* FROM "inline_comments" WHERE "inline_comments"\."parent_id" = \$1`).
		WithArgs(parentID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "parent_id"}))

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "inline_comments"`).
		WithArgs(recID, nil, nil, 0.0, nil, "Anonymous", "I agree with this", parentID, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).
			AddRow(uuid.New(), time.Now(), time.Now()))
	mock.ExpectCommit()

	ctrls.CreateInlineComment(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp dtos.APIResponse[dtos.CommentResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode json: %v", err)
	}
	if resp.Data.ParentID == nil || *resp.Data.ParentID != parentID.String() {
		t.Errorf("expected parent ID %s, got %v", parentID.String(), resp.Data.ParentID)
	}
}

func TestControllers_ListInlineComments_InvalidUUID(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "invalid-uuid"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings/invalid-uuid/comments", nil)

	ctrls.ListInlineComments(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_ListInlineComments_RecordingNotFound(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings/"+recID.String()+"/comments", nil)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	ctrls.ListInlineComments(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", w.Code)
	}
}

func TestControllers_ListInlineComments_Forbidden(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/recordings/"+recID.String()+"/comments", nil)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id", "is_share_enabled"}).
			AddRow(recID, "secret-token", nil, false))

	ctrls.ListInlineComments(c)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden, got %d", w.Code)
	}
}

func TestControllers_ListInlineComments_Success_GuestWithHeader(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	commID := uuid.New()
	replyID := uuid.New()
	guestToken := "guest-token-123"

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req := httptest.NewRequest(http.MethodGet, "/v1/recordings/"+recID.String()+"/comments", nil)
	req.Header.Set("X-Ownership-Token", guestToken)
	c.Request = req

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id", "is_share_enabled"}).
			AddRow(recID, guestToken, nil, false))

	mock.ExpectQuery(`SELECT \* FROM "inline_comments" WHERE recording_id = \$1 AND parent_id IS NULL ORDER BY timestamp_sec ASC, created_at ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "timestamp_sec", "author_name", "comment_text", "parent_id", "created_at", "updated_at"}).
			AddRow(commID, recID, 14.0, "Alice", "Top question", nil, time.Now(), time.Now()))

	mock.ExpectQuery(`SELECT \* FROM "inline_comments" WHERE "inline_comments"\."parent_id" = \$1`).
		WithArgs(commID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "timestamp_sec", "author_name", "comment_text", "parent_id", "created_at", "updated_at"}).
			AddRow(replyID, recID, 14.0, "Bob", "Reply here", commID, time.Now(), time.Now()))

	ctrls.ListInlineComments(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp dtos.APIResponse[[]dtos.CommentResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode json: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(resp.Data))
	}
	if resp.Data[0].ID != commID.String() {
		t.Errorf("expected ID %s, got %s", commID.String(), resp.Data[0].ID)
	}
	if len(resp.Data[0].Replies) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(resp.Data[0].Replies))
	}
	if resp.Data[0].Replies[0].ID != replyID.String() {
		t.Errorf("expected reply ID %s, got %s", replyID.String(), resp.Data[0].Replies[0].ID)
	}
}

func TestControllers_ListInlineComments_Success_AuthenticatedOwner(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	userID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req := httptest.NewRequest(http.MethodGet, "/v1/recordings/"+recID.String()+"/comments", nil)
	c.Request = req.WithContext(ctxmeta.WithAuthUser(req.Context(), ctxmeta.AuthUser{UserID: userID}))

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id", "is_share_enabled"}).
			AddRow(recID, "token", &userID, false))

	mock.ExpectQuery(`SELECT \* FROM "inline_comments" WHERE recording_id = \$1 AND parent_id IS NULL ORDER BY timestamp_sec ASC, created_at ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "timestamp_sec", "comment_text", "parent_id"}))

	ctrls.ListInlineComments(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp dtos.APIResponse[[]dtos.CommentResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode json: %v", err)
	}
	if len(resp.Data) != 0 {
		t.Fatalf("expected 0 comments, got %d", len(resp.Data))
	}
}

func TestControllers_DeleteInlineComment_InvalidRecordingUUID(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: "invalid-uuid"},
		{Key: "commentId", Value: uuid.New().String()},
	}
	c.Request = httptest.NewRequest(http.MethodDelete, "/v1/recordings/invalid-uuid/comments/test", nil)

	ctrls.DeleteInlineComment(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_DeleteInlineComment_InvalidCommentUUID(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: uuid.New().String()},
		{Key: "commentId", Value: "invalid-comment-uuid"},
	}
	c.Request = httptest.NewRequest(http.MethodDelete, "/v1/recordings/test/comments/invalid", nil)

	ctrls.DeleteInlineComment(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_DeleteInlineComment_RecordingNotFound(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	commID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: recID.String()},
		{Key: "commentId", Value: commID.String()},
	}
	c.Request = httptest.NewRequest(http.MethodDelete, "/v1/recordings/"+recID.String()+"/comments/"+commID.String(), nil)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	ctrls.DeleteInlineComment(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", w.Code)
	}
}

func TestControllers_DeleteInlineComment_CommentNotFound(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	commID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: recID.String()},
		{Key: "commentId", Value: commID.String()},
	}
	c.Request = httptest.NewRequest(http.MethodDelete, "/v1/recordings/"+recID.String()+"/comments/"+commID.String(), nil)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id", "is_share_enabled"}).
			AddRow(recID, "token", nil, false))

	mock.ExpectQuery(`SELECT \* FROM "inline_comments" WHERE id = \$1.*LIMIT \$2`).
		WithArgs(commID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	ctrls.DeleteInlineComment(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", w.Code)
	}
}

func TestControllers_DeleteInlineComment_Forbidden(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	commID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: recID.String()},
		{Key: "commentId", Value: commID.String()},
	}
	c.Request = httptest.NewRequest(http.MethodDelete, "/v1/recordings/"+recID.String()+"/comments/"+commID.String(), nil)

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id", "is_share_enabled"}).
			AddRow(recID, "secret-token", nil, false))

	mock.ExpectQuery(`SELECT \* FROM "inline_comments" WHERE id = \$1.*LIMIT \$2`).
		WithArgs(commID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "author_name", "comment_text"}).
			AddRow(commID, recID, "Alice", "Test comment"))
	mock.ExpectQuery(`SELECT \* FROM "inline_comments" WHERE "inline_comments"\."parent_id" = \$1`).
		WithArgs(commID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "parent_id"}))

	ctrls.DeleteInlineComment(c)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden, got %d", w.Code)
	}
}

func TestControllers_DeleteInlineComment_Success_RecordingOwner(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	commID := uuid.New()
	guestToken := "guest-token-123"

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: recID.String()},
		{Key: "commentId", Value: commID.String()},
	}
	req := httptest.NewRequest(http.MethodDelete, "/v1/recordings/"+recID.String()+"/comments/"+commID.String(), nil)
	req.Header.Set("X-Ownership-Token", guestToken)
	c.Request = req

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id", "is_share_enabled"}).
			AddRow(recID, guestToken, nil, false))

	mock.ExpectQuery(`SELECT \* FROM "inline_comments" WHERE id = \$1.*LIMIT \$2`).
		WithArgs(commID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "author_name", "comment_text"}).
			AddRow(commID, recID, "Someone", "Comment text"))
	mock.ExpectQuery(`SELECT \* FROM "inline_comments" WHERE "inline_comments"\."parent_id" = \$1`).
		WithArgs(commID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "parent_id"}))

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM "inline_comments" WHERE id = \$1`).
		WithArgs(commID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	ctrls.DeleteInlineComment(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestControllers_DeleteInlineComment_Success_CommentAuthor(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	commID := uuid.New()
	ownerID := uuid.New()
	authorID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: recID.String()},
		{Key: "commentId", Value: commID.String()},
	}
	req := httptest.NewRequest(http.MethodDelete, "/v1/recordings/"+recID.String()+"/comments/"+commID.String(), nil)
	c.Request = req.WithContext(ctxmeta.WithAuthUser(req.Context(), ctxmeta.AuthUser{UserID: authorID}))

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id", "is_share_enabled"}).
			AddRow(recID, "token", &ownerID, false))

	mock.ExpectQuery(`SELECT \* FROM "inline_comments" WHERE id = \$1.*LIMIT \$2`).
		WithArgs(commID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "user_id", "author_name", "comment_text"}).
			AddRow(commID, recID, &authorID, "Author Name", "My comment"))
	mock.ExpectQuery(`SELECT \* FROM "inline_comments" WHERE "inline_comments"\."parent_id" = \$1`).
		WithArgs(commID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "parent_id"}))

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM "inline_comments" WHERE id = \$1`).
		WithArgs(commID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	ctrls.DeleteInlineComment(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestControllers_StreamRecordingChat_InvalidUUID(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "invalid-uuid-format"}}
	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/invalid/chat", strings.NewReader(`{"message":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	ctrls.StreamRecordingChat(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_StreamRecordingChat_InvalidJSON(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/chat", strings.NewReader(`{invalid-json`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	ctrls.StreamRecordingChat(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_StreamRecordingChat_ValidationError(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/chat", strings.NewReader(`{"message":""}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	ctrls.StreamRecordingChat(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_StreamRecordingChat_NotFound(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/chat", strings.NewReader(`{"message":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	ctrls.StreamRecordingChat(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", w.Code)
	}
}

func TestControllers_StreamRecordingChat_Forbidden(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	ownerID := uuid.New()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/chat", strings.NewReader(`{"message":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "ownership_token"}).
			AddRow(recID, &ownerID, "secret-token"))

	ctrls.StreamRecordingChat(c)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden, got %d", w.Code)
	}
}

func TestControllers_StreamRecordingChat_Success(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	guestToken := "guest-ownership-token"
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/chat", strings.NewReader(`{"message":"Tell me about the goals"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Ownership-Token", guestToken)
	c.Request = req

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token"}).
			AddRow(recID, guestToken))

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "chat_messages"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(uuid.New()))
	mock.ExpectCommit()

	mockLLM := &mockControllerLLM{
		streamFunc: func(ctx context.Context, systemPrompt string, messages []dtos.ChatMessageInput, opts dtos.ChatOptions) (<-chan dtos.StreamChunk, error) {
			out := make(chan dtos.StreamChunk, 2)
			go func() {
				defer close(out)
				out <- dtos.StreamChunk{Content: "Goal is to expand [01:23] "}
				out <- dtos.StreamChunk{Content: "market reach."}
			}()
			return out, nil
		},
	}
	ctrls.svc.SetLLM(mockLLM)

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "chat_messages"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(uuid.New()))
	mock.ExpectCommit()

	ctrls.StreamRecordingChat(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if !strings.HasPrefix(contentType, "text/event-stream") {
		t.Errorf("expected Content-Type text/event-stream, got %s", contentType)
	}

	body := w.Body.String()
	if !strings.Contains(body, "event: token") {
		t.Errorf("expected body to contain token events, got: %s", body)
	}
	if !strings.Contains(body, "event: done") {
		t.Errorf("expected body to contain done event, got: %s", body)
	}
	if !strings.Contains(body, `"citations":["01:23"]`) {
		t.Errorf("expected body to contain parsed citation, got: %s", body)
	}
}

func TestControllers_StreamRecordingChat_StreamError(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	recID := uuid.New()
	guestToken := "guest-ownership-token"
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: recID.String()}}
	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/"+recID.String()+"/chat", strings.NewReader(`{"message":"Tell me about the goals"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Ownership-Token", guestToken)
	c.Request = req

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token"}).
			AddRow(recID, guestToken))

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "chat_messages"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(uuid.New()))
	mock.ExpectCommit()

	mockLLM := &mockControllerLLM{
		streamFunc: func(ctx context.Context, systemPrompt string, messages []dtos.ChatMessageInput, opts dtos.ChatOptions) (<-chan dtos.StreamChunk, error) {
			out := make(chan dtos.StreamChunk, 2)
			go func() {
				defer close(out)
				out <- dtos.StreamChunk{Content: "Partial "}
				out <- dtos.StreamChunk{Err: errors.New("upstream connection reset")}
			}()
			return out, nil
		},
	}
	ctrls.svc.SetLLM(mockLLM)

	ctrls.StreamRecordingChat(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "event: error") {
		t.Errorf("expected body to contain error event, got: %s", body)
	}
	if !strings.Contains(body, "upstream connection reset") {
		t.Errorf("expected body to contain error message, got: %s", body)
	}
}

func TestControllers_AskWorkspaceMemory_InvalidJSON(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/ask", strings.NewReader(`not-json`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	ctrls.AskWorkspaceMemory(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_AskWorkspaceMemory_InvalidValidation(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/ask", strings.NewReader(`{"question":"   "}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	ctrls.AskWorkspaceMemory(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestControllers_AskWorkspaceMemory_Unauthorized(t *testing.T) {
	ctrls, _, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	// Missing auth context -> returns 401 Unauthorized
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/ask", strings.NewReader(`{"question":"what was agreed?"}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	ctrls.AskWorkspaceMemory(c)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d", w.Code)
	}
}

func TestControllers_AskWorkspaceMemory_ServiceError(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	userID := uuid.New()
	ctrls.Service().SetEmbedding(&dummyEmbeddingProvider{})

	// Simulate database error during cross-meeting chunk search
	expectedSQL := `SELECT tc.id, tc.recording_id, r.title as recording_title, tc.chunk_index, tc.content, tc.start_time, tc.end_time, (tc.embedding <=> $1) as distance FROM transcript_chunks tc JOIN recordings r ON tc.recording_id = r.id WHERE (r.user_id = $2 AND r.deleted_at IS NULL) AND (tc.embedding <=> $3) <= $4 ORDER BY tc.embedding <=> $5 LIMIT $6`
	mock.ExpectQuery(regexp.QuoteMeta(expectedSQL)).
		WithArgs(sqlmock.AnyArg(), userID, sqlmock.AnyArg(), 0.45, sqlmock.AnyArg(), 5).
		WillReturnError(errors.New("database connection failed"))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/ask", strings.NewReader(`{"question":"what was agreed?"}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req.WithContext(ctxmeta.WithAuthUser(req.Context(), ctxmeta.AuthUser{UserID: userID}))

	ctrls.AskWorkspaceMemory(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 Internal Server Error, got %d", w.Code)
	}
}

func TestControllers_AskWorkspaceMemory_Success(t *testing.T) {
	ctrls, mock, _, cleanup := setupRecordingTestControllers(t)
	defer cleanup()

	userID := uuid.New()
	recID := uuid.New()
	chunkID := uuid.New()

	ctrls.Service().SetEmbedding(&dummyEmbeddingProvider{})

	mockLLM := llm.NewMock()
	mockLLM.GenerateChatResponseFunc = func(ctx context.Context, systemPrompt string, messages []dtos.ChatMessageInput, opts dtos.ChatOptions) (*dtos.ChatResponse, error) {
		return &dtos.ChatResponse{
			Content: `Based on [Meeting: "Q3 Planning", 00:15], target delivery is October.`,
		}, nil
	}
	ctrls.Service().SetLLM(mockLLM)

	expectedSQL := `SELECT tc.id, tc.recording_id, r.title as recording_title, tc.chunk_index, tc.content, tc.start_time, tc.end_time, (tc.embedding <=> $1) as distance FROM transcript_chunks tc JOIN recordings r ON tc.recording_id = r.id WHERE (r.user_id = $2 AND r.deleted_at IS NULL) AND (tc.embedding <=> $3) <= $4 ORDER BY tc.embedding <=> $5 LIMIT $6`
	mock.ExpectQuery(regexp.QuoteMeta(expectedSQL)).
		WithArgs(sqlmock.AnyArg(), userID, sqlmock.AnyArg(), 0.45, sqlmock.AnyArg(), 5).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "recording_title", "chunk_index", "content", "start_time", "end_time", "distance"}).
			AddRow(chunkID, recID, "Q3 Planning", 0, "Delivery planned for October.", 15.0, 50.0, 0.15))

	body := `{"question":"When is delivery scheduled?","history":[{"role":"user","content":"hello"}]}`
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/recordings/ask", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req.WithContext(ctxmeta.WithAuthUser(req.Context(), ctxmeta.AuthUser{UserID: userID}))

	ctrls.AskWorkspaceMemory(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var resp dtos.APIResponse[dtos.WorkspaceAskResponse]
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Data.Answer != `Based on [Meeting: "Q3 Planning", 00:15], target delivery is October.` {
		t.Errorf("unexpected answer: %s", resp.Data.Answer)
	}
	if len(resp.Data.Sources) != 1 {
		t.Fatalf("expected 1 source, got %d", len(resp.Data.Sources))
	}
	if resp.Data.Sources[0].RecordingTitle != "Q3 Planning" {
		t.Errorf("expected source title 'Q3 Planning', got %s", resp.Data.Sources[0].RecordingTitle)
	}
}
