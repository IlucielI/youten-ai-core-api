package services_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/pkg/ctxmeta"
)

func TestService_CreateInlineComment_NotFound_Recording(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	_, err := svc.CreateInlineComment(ctx, recID, "token", dtos.CreateCommentRequest{CommentText: "Test"})
	if !errors.Is(err, constants.ErrRecordingNotFound) {
		t.Fatalf("expected ErrRecordingNotFound, got %v", err)
	}
}

func TestService_CreateInlineComment_Forbidden(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id", "is_share_enabled"}).
			AddRow(recID, "secret-token", nil, false))

	_, err := svc.CreateInlineComment(ctx, recID, "wrong-token", dtos.CreateCommentRequest{CommentText: "Test"})
	if !errors.Is(err, constants.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestService_CreateInlineComment_Success_GuestWithToken(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	guestToken := "guest-token"
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id", "is_share_enabled"}).
			AddRow(recID, guestToken, nil, false))

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "inline_comments"`).
		WithArgs(recID, nil, nil, 14.5, nil, "Alice", "Great summary!", nil, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).
			AddRow(uuid.New(), time.Now(), time.Now()))
	mock.ExpectCommit()

	resp, err := svc.CreateInlineComment(ctx, recID, guestToken, dtos.CreateCommentRequest{
		TimestampSec: 14.5,
		AuthorName:   "Alice",
		CommentText:  "Great summary!",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.AuthorName != "Alice" || resp.CommentText != "Great summary!" || resp.TimestampSec != 14.5 {
		t.Errorf("unexpected comment response: %+v", resp)
	}
}

func TestService_CreateInlineComment_Success_PubliclyShared(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id", "is_share_enabled"}).
			AddRow(recID, "secret", nil, true))

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "inline_comments"`).
		WithArgs(recID, nil, nil, 0.0, nil, "Anonymous", "Shared viewer note", nil, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).
			AddRow(uuid.New(), time.Now(), time.Now()))
	mock.ExpectCommit()

	resp, err := svc.CreateInlineComment(ctx, recID, "", dtos.CreateCommentRequest{
		CommentText: "Shared viewer note",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.AuthorName != "Anonymous" {
		t.Errorf("expected Anonymous author, got %s", resp.AuthorName)
	}
}

func TestService_CreateInlineComment_Success_ReplyToParent(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	parentID := uuid.New()
	guestToken := "guest-token"
	ctx := context.Background()

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
		WithArgs(recID, nil, nil, 10.0, nil, "Bob", "Reply to parent", parentID, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).
			AddRow(uuid.New(), time.Now(), time.Now()))
	mock.ExpectCommit()

	resp, err := svc.CreateInlineComment(ctx, recID, guestToken, dtos.CreateCommentRequest{
		TimestampSec: 10.0,
		AuthorName:   "Bob",
		CommentText:  "Reply to parent",
		ParentID:     &parentID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ParentID == nil || *resp.ParentID != parentID.String() {
		t.Errorf("expected ParentID %s, got %v", parentID.String(), resp.ParentID)
	}
}

func TestService_CreateInlineComment_ParentNotFound(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	parentID := uuid.New()
	guestToken := "guest-token"
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id", "is_share_enabled"}).
			AddRow(recID, guestToken, nil, false))

	mock.ExpectQuery(`SELECT \* FROM "inline_comments" WHERE id = \$1.*LIMIT \$2`).
		WithArgs(parentID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	_, err := svc.CreateInlineComment(ctx, recID, guestToken, dtos.CreateCommentRequest{
		CommentText: "Reply",
		ParentID:    &parentID,
	})
	if !errors.Is(err, constants.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestService_CreateInlineComment_ParentDifferentRecording(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	otherRecID := uuid.New()
	parentID := uuid.New()
	guestToken := "guest-token"
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id", "is_share_enabled"}).
			AddRow(recID, guestToken, nil, false))

	mock.ExpectQuery(`SELECT \* FROM "inline_comments" WHERE id = \$1.*LIMIT \$2`).
		WithArgs(parentID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "comment_text"}).
			AddRow(parentID, otherRecID, "Parent on other rec"))
	mock.ExpectQuery(`SELECT \* FROM "inline_comments" WHERE "inline_comments"\."parent_id" = \$1`).
		WithArgs(parentID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "parent_id"}))

	_, err := svc.CreateInlineComment(ctx, recID, guestToken, dtos.CreateCommentRequest{
		CommentText: "Reply",
		ParentID:    &parentID,
	})
	if !errors.Is(err, constants.ErrBadRequest) {
		t.Fatalf("expected ErrBadRequest, got %v", err)
	}
}

func TestService_ListInlineComments_RecordingNotFound(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	_, err := svc.ListInlineComments(ctx, recID, "token")
	if !errors.Is(err, constants.ErrRecordingNotFound) {
		t.Fatalf("expected ErrRecordingNotFound, got %v", err)
	}
}

func TestService_ListInlineComments_Forbidden(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id", "is_share_enabled"}).
			AddRow(recID, "valid-token", nil, false))

	_, err := svc.ListInlineComments(ctx, recID, "invalid-token")
	if !errors.Is(err, constants.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestService_ListInlineComments_Success_Empty(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	guestToken := "guest-token"
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id", "is_share_enabled"}).
			AddRow(recID, guestToken, nil, false))

	mock.ExpectQuery(`SELECT \* FROM "inline_comments" WHERE recording_id = \$1 AND parent_id IS NULL ORDER BY timestamp_sec ASC, created_at ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "timestamp_sec", "comment_text", "parent_id"}))

	comments, err := svc.ListInlineComments(ctx, recID, guestToken)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if comments == nil {
		t.Fatal("expected non-nil empty slice")
	}
	if len(comments) != 0 {
		t.Fatalf("expected 0 comments, got %d", len(comments))
	}
}

func TestService_ListInlineComments_Success_AuthenticatedOwner(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	userID := uuid.New()
	commID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id", "is_share_enabled"}).
			AddRow(recID, "token", &userID, false))

	mock.ExpectQuery(`SELECT \* FROM "inline_comments" WHERE recording_id = \$1 AND parent_id IS NULL ORDER BY timestamp_sec ASC, created_at ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "timestamp_sec", "author_name", "comment_text", "parent_id", "created_at", "updated_at"}).
			AddRow(commID, recID, 5.5, "Owner", "Note on start", nil, time.Now(), time.Now()))

	mock.ExpectQuery(`SELECT \* FROM "inline_comments" WHERE "inline_comments"\."parent_id" = \$1`).
		WithArgs(commID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "parent_id"}))

	comments, err := svc.ListInlineComments(ctx, recID, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}
	if comments[0].ID != commID.String() {
		t.Errorf("expected ID %s, got %s", commID.String(), comments[0].ID)
	}
	if comments[0].TimestampSec != 5.5 {
		t.Errorf("expected TimestampSec 5.5, got %f", comments[0].TimestampSec)
	}
}

func TestService_ListInlineComments_Success_GuestWithToken_Threaded(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	guestToken := "guest-token"
	ctx := context.Background()
	parentID := uuid.New()
	replyID := uuid.New()
	segID := uuid.New()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id", "is_share_enabled"}).
			AddRow(recID, guestToken, nil, false))

	mock.ExpectQuery(`SELECT \* FROM "inline_comments" WHERE recording_id = \$1 AND parent_id IS NULL ORDER BY timestamp_sec ASC, created_at ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "segment_id", "timestamp_sec", "selected_text", "author_name", "comment_text", "parent_id", "created_at", "updated_at"}).
			AddRow(parentID, recID, &segID, 10.0, "Selected quote", "Alice", "Top question", nil, time.Now(), time.Now()))

	mock.ExpectQuery(`SELECT \* FROM "inline_comments" WHERE "inline_comments"\."parent_id" = \$1`).
		WithArgs(parentID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "segment_id", "timestamp_sec", "selected_text", "author_name", "comment_text", "parent_id", "created_at", "updated_at"}).
			AddRow(replyID, recID, nil, 10.0, nil, "Bob", "Reply answer", parentID, time.Now(), time.Now()))

	comments, err := svc.ListInlineComments(ctx, recID, guestToken)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(comments) != 1 {
		t.Fatalf("expected 1 top comment, got %d", len(comments))
	}
	if len(comments[0].Replies) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(comments[0].Replies))
	}
	if comments[0].Replies[0].ID != replyID.String() {
		t.Errorf("expected reply ID %s, got %s", replyID.String(), comments[0].Replies[0].ID)
	}
	if comments[0].Replies[0].ParentID == nil || *comments[0].Replies[0].ParentID != parentID.String() {
		t.Errorf("expected parent ID %s, got %v", parentID.String(), comments[0].Replies[0].ParentID)
	}
}

func TestService_ListInlineComments_Success_PubliclyShared(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id", "is_share_enabled"}).
			AddRow(recID, "token", nil, true))

	mock.ExpectQuery(`SELECT \* FROM "inline_comments" WHERE recording_id = \$1 AND parent_id IS NULL ORDER BY timestamp_sec ASC, created_at ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "timestamp_sec", "comment_text", "parent_id"}))

	comments, err := svc.ListInlineComments(ctx, recID, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if comments == nil {
		t.Fatal("expected empty slice")
	}
}

func TestService_DeleteInlineComment_RecordingNotFound(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	commID := uuid.New()
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	err := svc.DeleteInlineComment(ctx, recID, commID, "token")
	if !errors.Is(err, constants.ErrRecordingNotFound) {
		t.Fatalf("expected ErrRecordingNotFound, got %v", err)
	}
}

func TestService_DeleteInlineComment_CommentNotFound(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	commID := uuid.New()
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id", "is_share_enabled"}).
			AddRow(recID, "token", nil, false))

	mock.ExpectQuery(`SELECT \* FROM "inline_comments" WHERE id = \$1.*LIMIT \$2`).
		WithArgs(commID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	err := svc.DeleteInlineComment(ctx, recID, commID, "token")
	if !errors.Is(err, constants.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestService_DeleteInlineComment_CommentDifferentRecording(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	otherRecID := uuid.New()
	commID := uuid.New()
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id", "is_share_enabled"}).
			AddRow(recID, "token", nil, false))

	mock.ExpectQuery(`SELECT \* FROM "inline_comments" WHERE id = \$1.*LIMIT \$2`).
		WithArgs(commID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "comment_text"}).
			AddRow(commID, otherRecID, "Comment on other rec"))
	mock.ExpectQuery(`SELECT \* FROM "inline_comments" WHERE "inline_comments"\."parent_id" = \$1`).
		WithArgs(commID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "parent_id"}))

	err := svc.DeleteInlineComment(ctx, recID, commID, "token")
	if !errors.Is(err, constants.ErrBadRequest) {
		t.Fatalf("expected ErrBadRequest, got %v", err)
	}
}

func TestService_DeleteInlineComment_Forbidden(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	commID := uuid.New()
	ctx := context.Background()

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

	err := svc.DeleteInlineComment(ctx, recID, commID, "wrong-token")
	if !errors.Is(err, constants.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestService_DeleteInlineComment_Success_RecordingOwnerAuthenticated(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	commID := uuid.New()
	userID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id", "is_share_enabled"}).
			AddRow(recID, "token", &userID, false))

	mock.ExpectQuery(`SELECT \* FROM "inline_comments" WHERE id = \$1.*LIMIT \$2`).
		WithArgs(commID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "author_name", "comment_text"}).
			AddRow(commID, recID, "Guest Author", "Test comment"))
	mock.ExpectQuery(`SELECT \* FROM "inline_comments" WHERE "inline_comments"\."parent_id" = \$1`).
		WithArgs(commID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "parent_id"}))

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM "inline_comments" WHERE id = \$1`).
		WithArgs(commID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := svc.DeleteInlineComment(ctx, recID, commID, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestService_DeleteInlineComment_Success_RecordingOwnerGuestToken(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	commID := uuid.New()
	guestToken := "guest-owner-token"
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id", "is_share_enabled"}).
			AddRow(recID, guestToken, nil, false))

	mock.ExpectQuery(`SELECT \* FROM "inline_comments" WHERE id = \$1.*LIMIT \$2`).
		WithArgs(commID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "author_name", "comment_text"}).
			AddRow(commID, recID, "Someone", "Test comment"))
	mock.ExpectQuery(`SELECT \* FROM "inline_comments" WHERE "inline_comments"\."parent_id" = \$1`).
		WithArgs(commID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "parent_id"}))

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM "inline_comments" WHERE id = \$1`).
		WithArgs(commID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := svc.DeleteInlineComment(ctx, recID, commID, guestToken)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestService_DeleteInlineComment_Success_CommentAuthorAuthenticated(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	commID := uuid.New()
	ownerID := uuid.New()
	commentAuthorID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: commentAuthorID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id", "is_share_enabled"}).
			AddRow(recID, "token", &ownerID, false))

	mock.ExpectQuery(`SELECT \* FROM "inline_comments" WHERE id = \$1.*LIMIT \$2`).
		WithArgs(commID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "user_id", "author_name", "comment_text"}).
			AddRow(commID, recID, &commentAuthorID, "Charlie Brown", "Author comment"))
	mock.ExpectQuery(`SELECT \* FROM "inline_comments" WHERE "inline_comments"\."parent_id" = \$1`).
		WithArgs(commID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "parent_id"}))

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM "inline_comments" WHERE id = \$1`).
		WithArgs(commID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := svc.DeleteInlineComment(ctx, recID, commID, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestService_DeleteInlineComment_Forbidden_SameNameDifferentUser(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	commID := uuid.New()
	ownerID := uuid.New()
	actualAuthorID := uuid.New()
	impersonatorID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: impersonatorID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id", "is_share_enabled"}).
			AddRow(recID, "token", &ownerID, false))

	mock.ExpectQuery(`SELECT \* FROM "inline_comments" WHERE id = \$1.*LIMIT \$2`).
		WithArgs(commID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "user_id", "author_name", "comment_text"}).
			AddRow(commID, recID, &actualAuthorID, "Charlie Brown", "Author comment"))
	mock.ExpectQuery(`SELECT \* FROM "inline_comments" WHERE "inline_comments"\."parent_id" = \$1`).
		WithArgs(commID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "parent_id"}))

	err := svc.DeleteInlineComment(ctx, recID, commID, "")
	if !errors.Is(err, constants.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for different user, got %v", err)
	}
}
