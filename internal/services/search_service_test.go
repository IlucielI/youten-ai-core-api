package services_test

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/pkg/ctxmeta"
)

type searchMockEmbedding struct {
	embeddings [][]float32
	err        error
}

func (m *searchMockEmbedding) CreateEmbeddings(ctx context.Context, texts []string) ([][]float32, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.embeddings, nil
}

func TestService_SearchWorkspaceSemantic_Unauthorized(t *testing.T) {
	svc, _, _, _ := setupRecordingTestService(t)

	// Missing auth context
	res, err := svc.SearchWorkspaceSemantic(context.Background(), dtos.SemanticSearchQuery{Q: "test"})
	if !errors.Is(err, constants.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
	if res != nil {
		t.Fatalf("expected nil response, got %+v", res)
	}

	// Nil UserID
	ctxNilUser := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: uuid.Nil})
	res, err = svc.SearchWorkspaceSemantic(ctxNilUser, dtos.SemanticSearchQuery{Q: "test"})
	if !errors.Is(err, constants.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized for nil user, got %v", err)
	}
	if res != nil {
		t.Fatalf("expected nil response, got %+v", res)
	}
}

func TestService_SearchWorkspaceSemantic_EmbeddingError(t *testing.T) {
	svc, _, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mockEmb := &searchMockEmbedding{
		err: errors.New("embedding API timeout"),
	}
	svc.SetEmbedding(mockEmb)

	res, err := svc.SearchWorkspaceSemantic(ctx, dtos.SemanticSearchQuery{Q: "valid search"})
	if !errors.Is(err, constants.ErrInternalServerError) {
		t.Fatalf("expected ErrInternalServerError, got %v", err)
	}
	if res != nil {
		t.Fatalf("expected nil response, got %+v", res)
	}
}

func TestService_SearchWorkspaceSemantic_EmptyEmbeddingsReturned(t *testing.T) {
	svc, _, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mockEmb := &searchMockEmbedding{
		embeddings: [][]float32{},
	}
	svc.SetEmbedding(mockEmb)

	res, err := svc.SearchWorkspaceSemantic(ctx, dtos.SemanticSearchQuery{Q: "valid search"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || res.Count != 0 || len(res.Results) != 0 {
		t.Fatalf("expected 0 results, got %+v", res)
	}
}

func TestService_SearchWorkspaceSemantic_Success(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mockEmb := &searchMockEmbedding{
		embeddings: [][]float32{make([]float32, 1024)},
	}
	svc.SetEmbedding(mockEmb)

	recID1 := uuid.New()
	recID2 := uuid.New()
	chunkID1 := uuid.New()
	chunkID2 := uuid.New()

	expectedSQL := `SELECT tc.id, tc.recording_id, r.title as recording_title, tc.chunk_index, tc.content, tc.start_time, tc.end_time, (tc.embedding <=> $1) as distance FROM transcript_chunks tc JOIN recordings r ON tc.recording_id = r.id WHERE (r.user_id = $2 AND r.deleted_at IS NULL) AND (tc.embedding <=> $3) <= $4 ORDER BY tc.embedding <=> $5 LIMIT $6`
	mock.ExpectQuery(regexp.QuoteMeta(expectedSQL)).
		WithArgs(sqlmock.AnyArg(), userID, sqlmock.AnyArg(), 0.3, sqlmock.AnyArg(), 20).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "recording_title", "chunk_index", "content", "start_time", "end_time", "distance"}).
			AddRow(chunkID1, recID1, "Sprint Planning Q3", 0, "We discussed Q3 financial goals and targets.", 10.5, 45.0, 0.15).
			AddRow(chunkID2, recID2, "All-Hands Sync", 2, "Revenue grew significantly compared to Q2.", 120.0, 160.0, 0.22))

	res, err := svc.SearchWorkspaceSemantic(ctx, dtos.SemanticSearchQuery{
		Q:         "financial revenue goals",
		Limit:     20,
		Threshold: 0.70,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v, unwrapped: %v", err, errors.Unwrap(err))
	}
	if res == nil {
		t.Fatal("expected non-nil response")
	}

	if res.Query != "financial revenue goals" {
		t.Errorf("expected query 'financial revenue goals', got %s", res.Query)
	}
	if res.Count != 2 || len(res.Results) != 2 {
		t.Fatalf("expected 2 results, got count=%d len=%d", res.Count, len(res.Results))
	}

	// Verify first item
	item1 := res.Results[0]
	if item1.RecordingID != recID1 || item1.RecordingTitle != "Sprint Planning Q3" {
		t.Errorf("unexpected item 1 recording info: %+v", item1)
	}
	if item1.Score != 0.85 { // 1.0 - 0.15 = 0.85
		t.Errorf("expected score 0.85, got %f", item1.Score)
	}

	// Verify second item
	item2 := res.Results[1]
	if item2.RecordingID != recID2 || item2.RecordingTitle != "All-Hands Sync" {
		t.Errorf("unexpected item 2 recording info: %+v", item2)
	}
	if item2.Score != 0.78 { // 1.0 - 0.22 = 0.78
		t.Errorf("expected score 0.78, got %f", item2.Score)
	}
}

func TestService_SearchWorkspaceSemantic_Success_DefaultLimits(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	userID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mockEmb := &searchMockEmbedding{
		embeddings: [][]float32{make([]float32, 1024)},
	}
	svc.SetEmbedding(mockEmb)

	// Threshold 0: no distance filter in WHERE
	expectedSQL := `SELECT tc.id, tc.recording_id, r.title as recording_title, tc.chunk_index, tc.content, tc.start_time, tc.end_time, (tc.embedding <=> $1) as distance FROM transcript_chunks tc JOIN recordings r ON tc.recording_id = r.id WHERE r.user_id = $2 AND r.deleted_at IS NULL ORDER BY tc.embedding <=> $3 LIMIT $4`
	mock.ExpectQuery(regexp.QuoteMeta(expectedSQL)).
		WithArgs(sqlmock.AnyArg(), userID, sqlmock.AnyArg(), 10).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "recording_title", "chunk_index", "content", "start_time", "end_time", "distance"}))

	res, err := svc.SearchWorkspaceSemantic(ctx, dtos.SemanticSearchQuery{
		Q: "general search",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || res.Count != 0 {
		t.Fatalf("expected empty response, got %+v", res)
	}
}
