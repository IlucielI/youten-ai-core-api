package services_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/pkg/ctxmeta"
)

func TestService_DeleteRecording_Unauthorized(t *testing.T) {
	svc, _, _, _ := setupRecordingTestService(t)
	recID := uuid.New()

	err := svc.DeleteRecording(context.Background(), recID)
	if !errors.Is(err, constants.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestService_DeleteRecording_Unauthorized_ContextNilUserID(t *testing.T) {
	svc, _, _, _ := setupRecordingTestService(t)
	recID := uuid.New()

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: uuid.Nil})
	err := svc.DeleteRecording(ctx, recID)
	if !errors.Is(err, constants.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestService_DeleteRecording_NotFound(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	recID := uuid.New()

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	err := svc.DeleteRecording(ctx, recID)
	if !errors.Is(err, constants.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestService_DeleteRecording_FindError(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	recID := uuid.New()

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(errors.New("db error"))

	err := svc.DeleteRecording(ctx, recID)
	if err == nil || !strings.Contains(err.Error(), "db error") {
		t.Fatalf("expected db error, got %v", err)
	}
}

func TestService_DeleteRecording_Forbidden_RecordingHasNilUser(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	recID := uuid.New()

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id"}).
			AddRow(recID, nil))

	err := svc.DeleteRecording(ctx, recID)
	if !errors.Is(err, constants.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for guest/nil user recording, got %v", err)
	}
}

func TestService_DeleteRecording_Forbidden_DifferentUser(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	otherUserID := uuid.New()
	recID := uuid.New()

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id"}).
			AddRow(recID, otherUserID))

	err := svc.DeleteRecording(ctx, recID)
	if !errors.Is(err, constants.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for other user recording, got %v", err)
	}
}

func TestService_DeleteRecording_DeleteError(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	recID := uuid.New()

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id"}).
			AddRow(recID, userID))

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings" SET "deleted_at"=\$1 WHERE id = \$2 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(sqlmock.AnyArg(), recID).
		WillReturnError(errors.New("delete failed"))
	mock.ExpectRollback()

	err := svc.DeleteRecording(ctx, recID)
	if err == nil || !strings.Contains(err.Error(), "delete failed") {
		t.Fatalf("expected delete error, got %v", err)
	}
}

func TestService_DeleteRecording_Success(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	recID := uuid.New()

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id"}).
			AddRow(recID, userID))

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings" SET "deleted_at"=\$1 WHERE id = \$2 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := svc.DeleteRecording(ctx, recID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestService_ClaimRecording_Unauthorized(t *testing.T) {
	svc, _, _, _ := setupRecordingTestService(t)
	recID := uuid.New()

	ctx := context.Background() // No auth
	err := svc.ClaimRecording(ctx, recID, dtos.ClaimRecordingRequest{OwnershipToken: "token-123"})
	if !errors.Is(err, constants.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestService_ClaimRecording_EmptyToken(t *testing.T) {
	svc, _, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	recID := uuid.New()

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})
	err := svc.ClaimRecording(ctx, recID, dtos.ClaimRecordingRequest{OwnershipToken: "   "})
	if !errors.Is(err, constants.ErrBadRequest) {
		t.Fatalf("expected ErrBadRequest for empty token, got %v", err)
	}
}

func TestService_ClaimRecording_NotFound(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	recID := uuid.New()

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	err := svc.ClaimRecording(ctx, recID, dtos.ClaimRecordingRequest{OwnershipToken: "token-123"})
	if !errors.Is(err, constants.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestService_ClaimRecording_FindError(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	recID := uuid.New()

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(errors.New("db error"))

	err := svc.ClaimRecording(ctx, recID, dtos.ClaimRecordingRequest{OwnershipToken: "token-123"})
	if err == nil || !strings.Contains(err.Error(), "failed to retrieve recording") {
		t.Fatalf("expected find error, got %v", err)
	}
}

func TestService_ClaimRecording_AlreadyClaimed_NotGuest(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	recID := uuid.New()

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "is_guest", "user_id", "ownership_token"}).
			AddRow(recID, false, nil, "token-123"))

	err := svc.ClaimRecording(ctx, recID, dtos.ClaimRecordingRequest{OwnershipToken: "token-123"})
	if !errors.Is(err, constants.ErrConflict) {
		t.Fatalf("expected ErrConflict when is_guest is false, got %v", err)
	}
}

func TestService_ClaimRecording_AlreadyClaimed_UserIDNotNull(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	existingOwner := uuid.New()
	recID := uuid.New()

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "is_guest", "user_id", "ownership_token"}).
			AddRow(recID, true, existingOwner, "token-123"))

	err := svc.ClaimRecording(ctx, recID, dtos.ClaimRecordingRequest{OwnershipToken: "token-123"})
	if !errors.Is(err, constants.ErrConflict) {
		t.Fatalf("expected ErrConflict when user_id is already assigned, got %v", err)
	}
}

func TestService_ClaimRecording_MismatchedToken(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	recID := uuid.New()

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "is_guest", "user_id", "ownership_token"}).
			AddRow(recID, true, nil, "correct-token"))

	err := svc.ClaimRecording(ctx, recID, dtos.ClaimRecordingRequest{OwnershipToken: "wrong-token"})
	if !errors.Is(err, constants.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for wrong ownership token, got %v", err)
	}
}

func TestService_ClaimRecording_ClaimRepoNotFound(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	recID := uuid.New()

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "is_guest", "user_id", "ownership_token"}).
			AddRow(recID, true, nil, "valid-token"))

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings" SET "is_guest"=\$1,"user_id"=\$2,"updated_at"=\$3 WHERE \(id = \$4 AND ownership_token = \$5 AND is_guest = TRUE\) AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(false, userID, sqlmock.AnyArg(), recID, "valid-token").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	err := svc.ClaimRecording(ctx, recID, dtos.ClaimRecordingRequest{OwnershipToken: "valid-token"})
	if !errors.Is(err, constants.ErrNotFound) {
		t.Fatalf("expected ErrNotFound when claim rows affected is 0, got %v", err)
	}
}

func TestService_ClaimRecording_ClaimRepoError(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	recID := uuid.New()

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "is_guest", "user_id", "ownership_token"}).
			AddRow(recID, true, nil, "valid-token"))

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings" SET "is_guest"=\$1,"user_id"=\$2,"updated_at"=\$3 WHERE \(id = \$4 AND ownership_token = \$5 AND is_guest = TRUE\) AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(false, userID, sqlmock.AnyArg(), recID, "valid-token").
		WillReturnError(errors.New("db update error"))
	mock.ExpectRollback()

	err := svc.ClaimRecording(ctx, recID, dtos.ClaimRecordingRequest{OwnershipToken: "valid-token"})
	if err == nil || !strings.Contains(err.Error(), "failed to claim recording") {
		t.Fatalf("expected claim failure, got %v", err)
	}
}

func TestService_ClaimRecording_Success(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	recID := uuid.New()

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "is_guest", "user_id", "ownership_token"}).
			AddRow(recID, true, nil, "valid-token"))

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings" SET "is_guest"=\$1,"user_id"=\$2,"updated_at"=\$3 WHERE \(id = \$4 AND ownership_token = \$5 AND is_guest = TRUE\) AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(false, userID, sqlmock.AnyArg(), recID, "valid-token").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := svc.ClaimRecording(ctx, recID, dtos.ClaimRecordingRequest{OwnershipToken: "valid-token"})
	if err != nil {
		t.Fatalf("unexpected claim error: %v", err)
	}
}

func TestService_ClaimBulkRecordings_Unauthorized(t *testing.T) {
	svc, _, _, _ := setupRecordingTestService(t)
	ctx := context.Background()

	_, err := svc.ClaimBulkRecordings(ctx, dtos.BulkClaimRequest{
		Tokens: []string{"tok-1"},
	})
	if !errors.Is(err, constants.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestService_ClaimBulkRecordings_EmptyTokens(t *testing.T) {
	svc, _, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	_, err := svc.ClaimBulkRecordings(ctx, dtos.BulkClaimRequest{
		Tokens: []string{"  ", ""},
	})
	if !errors.Is(err, constants.ErrBadRequest) {
		t.Fatalf("expected ErrBadRequest, got %v", err)
	}
}

func TestService_ClaimBulkRecordings_ZeroMatch(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE \(ownership_token IN \(\$1,\$2\) AND is_guest = TRUE\) AND "recordings"\."deleted_at" IS NULL`).
		WithArgs("tok-1", "tok-2").
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "is_guest"}))
	mock.ExpectCommit()

	resp, err := svc.ClaimBulkRecordings(ctx, dtos.BulkClaimRequest{
		Tokens: []string{"tok-1", "tok-2"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ClaimedCount != 0 {
		t.Errorf("expected ClaimedCount 0, got %d", resp.ClaimedCount)
	}
	if len(resp.RecordingIDs) != 0 {
		t.Errorf("expected 0 RecordingIDs, got %d", len(resp.RecordingIDs))
	}
}

func TestService_ClaimBulkRecordings_PartialMatch(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	recID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE \(ownership_token IN \(\$1,\$2\) AND is_guest = TRUE\) AND "recordings"\."deleted_at" IS NULL`).
		WithArgs("tok-1", "tok-2").
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "is_guest"}).
			AddRow(recID, "tok-1", true))

	mock.ExpectExec(`UPDATE "recordings" SET "is_guest"=\$1,"user_id"=\$2,"updated_at"=\$3 WHERE id IN \(\$4\) AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(false, userID, sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	resp, err := svc.ClaimBulkRecordings(ctx, dtos.BulkClaimRequest{
		Tokens: []string{"tok-1", "tok-2"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ClaimedCount != 1 {
		t.Errorf("expected ClaimedCount 1, got %d", resp.ClaimedCount)
	}
	if len(resp.RecordingIDs) != 1 || resp.RecordingIDs[0] != recID.String() {
		t.Errorf("expected RecordingIDs [%s], got %v", recID.String(), resp.RecordingIDs)
	}
}

func TestService_ClaimBulkRecordings_RepoError(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE \(ownership_token IN \(\$1\) AND is_guest = TRUE\) AND "recordings"\."deleted_at" IS NULL`).
		WithArgs("tok-1").
		WillReturnError(errors.New("db query error"))
	mock.ExpectRollback()

	_, err := svc.ClaimBulkRecordings(ctx, dtos.BulkClaimRequest{
		Tokens: []string{"tok-1"},
	})
	if err == nil || !strings.Contains(err.Error(), "failed to bulk claim recordings") {
		t.Fatalf("expected error, got %v", err)
	}
}

func TestService_ClaimBulkRecordings_Success(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	recID1 := uuid.New()
	recID2 := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

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

	resp, err := svc.ClaimBulkRecordings(ctx, dtos.BulkClaimRequest{
		Tokens: []string{"tok-1", "tok-2"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ClaimedCount != 2 {
		t.Errorf("expected ClaimedCount 2, got %d", resp.ClaimedCount)
	}
	if len(resp.RecordingIDs) != 2 {
		t.Errorf("expected 2 recording IDs, got %d", len(resp.RecordingIDs))
	}
}

func TestService_ToggleRecordingShare_Unauthorized(t *testing.T) {
	svc, _, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	enabled := true

	ctx := context.Background()
	_, err := svc.ToggleRecordingShare(ctx, recID, dtos.ShareToggleRequest{IsShareEnabled: &enabled})
	if !errors.Is(err, constants.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestService_ToggleRecordingShare_NilRequest(t *testing.T) {
	svc, _, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	userID := uuid.New()

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})
	_, err := svc.ToggleRecordingShare(ctx, recID, dtos.ShareToggleRequest{IsShareEnabled: nil})
	if !errors.Is(err, constants.ErrBadRequest) {
		t.Fatalf("expected ErrBadRequest, got %v", err)
	}
}

func TestService_ToggleRecordingShare_NotFound(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	userID := uuid.New()
	enabled := true

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	_, err := svc.ToggleRecordingShare(ctx, recID, dtos.ShareToggleRequest{IsShareEnabled: &enabled})
	if !errors.Is(err, constants.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestService_ToggleRecordingShare_Forbidden_NotOwner(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	userID := uuid.New()
	otherUserID := uuid.New()
	enabled := true

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id"}).
			AddRow(recID, otherUserID))

	_, err := svc.ToggleRecordingShare(ctx, recID, dtos.ShareToggleRequest{IsShareEnabled: &enabled})
	if !errors.Is(err, constants.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestService_ToggleRecordingShare_Forbidden_Guest(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	userID := uuid.New()
	enabled := true

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id"}).
			AddRow(recID, nil))

	_, err := svc.ToggleRecordingShare(ctx, recID, dtos.ShareToggleRequest{IsShareEnabled: &enabled})
	if !errors.Is(err, constants.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestService_ToggleRecordingShare_Enable_NewToken(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	userID := uuid.New()
	enabled := true

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "share_token", "is_share_enabled"}).
			AddRow(recID, userID, nil, false))

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings" SET "is_share_enabled"=\$1,"share_token"=\$2,"updated_at"=\$3 WHERE id = \$4 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(true, sqlmock.AnyArg(), sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	resp, err := svc.ToggleRecordingShare(ctx, recID, dtos.ShareToggleRequest{IsShareEnabled: &enabled})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.IsShareEnabled {
		t.Error("expected IsShareEnabled true")
	}
	if resp.ShareToken == nil || *resp.ShareToken == "" {
		t.Error("expected non-empty ShareToken")
	}
	if resp.ShareURL == nil || !strings.Contains(*resp.ShareURL, *resp.ShareToken) {
		t.Errorf("expected ShareURL containing token, got %v", resp.ShareURL)
	}
}

func TestService_ToggleRecordingShare_Enable_ExistingToken(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	userID := uuid.New()
	existingToken := "existing-token-abc"
	enabled := true

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "share_token", "is_share_enabled"}).
			AddRow(recID, userID, &existingToken, false))

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings" SET "is_share_enabled"=\$1,"share_token"=\$2,"updated_at"=\$3 WHERE id = \$4 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(true, existingToken, sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	resp, err := svc.ToggleRecordingShare(ctx, recID, dtos.ShareToggleRequest{IsShareEnabled: &enabled})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.IsShareEnabled {
		t.Error("expected IsShareEnabled true")
	}
	if resp.ShareToken == nil || *resp.ShareToken != existingToken {
		t.Errorf("expected ShareToken %s, got %v", existingToken, resp.ShareToken)
	}
}

func TestService_ToggleRecordingShare_Disable(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	userID := uuid.New()
	existingToken := "existing-token-abc"
	disabled := false

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "share_token", "is_share_enabled"}).
			AddRow(recID, userID, &existingToken, true))

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings" SET "is_share_enabled"=\$1,"updated_at"=\$2 WHERE id = \$3 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(false, sqlmock.AnyArg(), recID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	resp, err := svc.ToggleRecordingShare(ctx, recID, dtos.ShareToggleRequest{IsShareEnabled: &disabled})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.IsShareEnabled {
		t.Error("expected IsShareEnabled false")
	}
	if resp.ShareToken != nil {
		t.Errorf("expected nil ShareToken when disabled, got %v", resp.ShareToken)
	}
	if resp.ShareURL != nil {
		t.Errorf("expected nil ShareURL when disabled, got %v", resp.ShareURL)
	}
}

func TestService_ToggleRecordingShare_DBError(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	userID := uuid.New()
	enabled := true

	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "share_token", "is_share_enabled"}).
			AddRow(recID, userID, nil, false))

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "recordings" SET "is_share_enabled"=\$1,"share_token"=\$2,"updated_at"=\$3 WHERE id = \$4 AND "recordings"\."deleted_at" IS NULL`).
		WithArgs(true, sqlmock.AnyArg(), sqlmock.AnyArg(), recID).
		WillReturnError(errors.New("db update error"))
	mock.ExpectRollback()

	_, err := svc.ToggleRecordingShare(ctx, recID, dtos.ShareToggleRequest{IsShareEnabled: &enabled})
	if err == nil || !strings.Contains(err.Error(), "failed to update share settings") {
		t.Fatalf("expected update error, got %v", err)
	}
}
