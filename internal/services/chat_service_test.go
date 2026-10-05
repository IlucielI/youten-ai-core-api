package services_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	gormPostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"

	"code-base-golang/internal/adapters/llm"
	"code-base-golang/internal/config"
	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/pkg/ctxmeta"
	"code-base-golang/internal/repositories"
	"code-base-golang/internal/services"
)


type mockEmbeddingService struct {
	embeddings [][]float32
	err        error
}

func (m *mockEmbeddingService) CreateEmbeddings(ctx context.Context, texts []string) ([][]float32, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.embeddings, nil
}

func setupChatTestService(t *testing.T) (*services.Service, sqlmock.Sqlmock, *llm.MockLLM, *mockEmbeddingService) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}

	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("there were unfulfilled sqlmock expectations: %s", err)
		}
		db.Close()
	})

	dialector := gormPostgres.New(gormPostgres.Config{
		Conn:       db,
		DriverName: "postgres",
	})
	gormDB, err := gorm.Open(dialector, &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open gorm connection: %v", err)
	}

	repo := repositories.New(gormDB)
	cfg := config.Config{}
	svc := services.New(cfg, repo, nil)

	mockLLM := llm.NewMock()
	svc.SetLLM(mockLLM)

	emb := &mockEmbeddingService{
		embeddings: [][]float32{make([]float32, 1024)},
	}
	svc.SetEmbedding(emb)

	return svc, mock, mockLLM, emb
}

func TestService_InitiateRecordingChatStream_NotFound(t *testing.T) {
	svc, mock, _, _ := setupChatTestService(t)
	recID := uuid.New()
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	_, err := svc.InitiateRecordingChatStream(ctx, recID, "tok", dtos.RecordingChatRequest{
		Message: "Hello",
	})
	if !errors.Is(err, constants.ErrRecordingNotFound) {
		t.Fatalf("expected ErrRecordingNotFound, got %v", err)
	}
}

func TestService_InitiateRecordingChatStream_Forbidden_Unauthorized(t *testing.T) {
	svc, mock, _, _ := setupChatTestService(t)
	recID := uuid.New()
	ownerID := uuid.New()
	ownershipToken := "secret-ownership-token"
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "ownership_token"}).
			AddRow(recID, &ownerID, ownershipToken))

	// Neither authenticated nor valid token provided
	_, err := svc.InitiateRecordingChatStream(ctx, recID, "wrong-token", dtos.RecordingChatRequest{
		Message: "Hello",
	})
	if !errors.Is(err, constants.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestService_InitiateRecordingChatStream_Forbidden_ShareToken(t *testing.T) {
	svc, mock, _, _ := setupChatTestService(t)
	recID := uuid.New()
	ownerID := uuid.New()
	ownershipToken := "secret-ownership-token"
	shareToken := "viewer-share-token"
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "ownership_token", "is_share_enabled", "share_token"}).
			AddRow(recID, &ownerID, ownershipToken, true, &shareToken))

	// Viewer supplies shareToken -> must be rejected with ErrForbidden (AC2)
	_, err := svc.InitiateRecordingChatStream(ctx, recID, shareToken, dtos.RecordingChatRequest{
		Message: "Hello from viewer",
	})
	if !errors.Is(err, constants.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for share token viewer, got %v", err)
	}
}

func TestService_InitiateRecordingChatStream_Forbidden_WrongAuthUser(t *testing.T) {
	svc, mock, _, _ := setupChatTestService(t)
	recID := uuid.New()
	ownerID := uuid.New()
	anotherUserID := uuid.New()
	ownershipToken := "secret-ownership-token"
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: anotherUserID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "ownership_token"}).
			AddRow(recID, &ownerID, ownershipToken))

	_, err := svc.InitiateRecordingChatStream(ctx, recID, "", dtos.RecordingChatRequest{
		Message: "Hello",
	})
	if !errors.Is(err, constants.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestService_InitiateRecordingChatStream_Success_WithCitations(t *testing.T) {
	svc, mock, mockLLM, _ := setupChatTestService(t)
	recID := uuid.New()
	ownershipToken := "secret-token"
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token"}).
			AddRow(recID, ownershipToken))

	// Save user chat message
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "chat_messages"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(uuid.New()))
	mock.ExpectCommit()

	// Vector nearest-neighbor search returns 1 chunk
	chunkID := uuid.New()
	mock.ExpectQuery(`SELECT .* FROM "transcript_chunks" WHERE recording_id = \$1.*`).
		WithArgs(recID, sqlmock.AnyArg(), 5).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "chunk_index", "content", "start_time", "end_time"}).
			AddRow(chunkID, recID, 0, "Discussion on Q3 revenue targets.", 15.0, 45.0))

	// Custom LLM streamer emitting tokens with citation
	mockLLM.StreamChatResponseFunc = func(ctx context.Context, systemPrompt string, messages []dtos.ChatMessageInput, opts dtos.ChatOptions) (<-chan dtos.StreamChunk, error) {
		out := make(chan dtos.StreamChunk, 4)
		go func() {
			defer close(out)
			out <- dtos.StreamChunk{Content: "According to the meeting "}
			out <- dtos.StreamChunk{Content: "[00:15], revenue "}
			out <- dtos.StreamChunk{Content: "targets were met."}
		}()
		return out, nil
	}

	result, err := svc.InitiateRecordingChatStream(ctx, recID, ownershipToken, dtos.RecordingChatRequest{
		Message: "What was discussed regarding revenue?",
		ConversationHistory: []dtos.ChatMessageInput{
			{Role: "user", Content: "Previous question"},
			{Role: "assistant", Content: "Previous answer"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error initiating stream: %v", err)
	}

	if len(result.RetrievedChunkIDs) != 1 || result.RetrievedChunkIDs[0] != chunkID.String() {
		t.Errorf("expected retrieved chunk ID %s, got %v", chunkID, result.RetrievedChunkIDs)
	}

	// Drain streamed tokens
	var tokens []string
	for chunk := range result.StreamChannel {
		if chunk.Err != nil {
			t.Fatalf("unexpected stream chunk error: %v", chunk.Err)
		}
		tokens = append(tokens, chunk.Content)
	}
	fullText := strings.Join(tokens, "")
	expectedText := "According to the meeting [00:15], revenue targets were met."
	if fullText != expectedText {
		t.Errorf("expected full text %q, got %q", expectedText, fullText)
	}

	// Persist assistant response expectation
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "chat_messages"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(uuid.New()))
	mock.ExpectCommit()

	asstMsg, citations, err := result.SaveAssistantMsg(ctx, fullText)
	if err != nil {
		t.Fatalf("unexpected error saving assistant message: %v", err)
	}

	if asstMsg.SenderRole != "assistant" {
		t.Errorf("expected role assistant, got %s", asstMsg.SenderRole)
	}
	if len(citations) != 1 || citations[0] != "00:15" {
		t.Errorf("expected citations ['00:15'], got %v", citations)
	}
}

func TestService_InitiateRecordingChatStream_Success_EmptyContextFallback(t *testing.T) {
	svc, mock, mockLLM, emb := setupChatTestService(t)
	recID := uuid.New()
	userID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	// Embedding provider returns error (triggering fallback)
	emb.err = errors.New("embedding provider temporary failure")

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id"}).
			AddRow(recID, &userID))

	// Save user chat message
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "chat_messages"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(uuid.New()))
	mock.ExpectCommit()

	// Capture userPrompt received by LLM
	var capturedUserPrompt string
	mockLLM.StreamChatResponseFunc = func(ctx context.Context, systemPrompt string, messages []dtos.ChatMessageInput, opts dtos.ChatOptions) (<-chan dtos.StreamChunk, error) {
		for _, m := range messages {
			if m.Role == "user" {
				capturedUserPrompt = m.Content
			}
		}
		out := make(chan dtos.StreamChunk, 2)
		go func() {
			defer close(out)
			out <- dtos.StreamChunk{Content: "I have no transcript context available."}
		}()
		return out, nil
	}

	result, err := svc.InitiateRecordingChatStream(ctx, recID, "", dtos.RecordingChatRequest{
		Message: "Explain the budget.",
	})
	if err != nil {
		t.Fatalf("unexpected error initiating stream: %v", err)
	}

	if len(result.RetrievedChunkIDs) != 0 {
		t.Errorf("expected 0 retrieved chunk IDs on fallback, got %d", len(result.RetrievedChunkIDs))
	}

	for range result.StreamChannel {
	}

	if !strings.Contains(capturedUserPrompt, "(No transcript context available)") {
		t.Errorf("expected user prompt to contain empty context fallback, got: %s", capturedUserPrompt)
	}

	// Save assistant response
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "chat_messages"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(uuid.New()))
	mock.ExpectCommit()

	asstMsg, citations, err := result.SaveAssistantMsg(ctx, "I have no transcript context available.")
	if err != nil {
		t.Fatalf("unexpected error saving assistant message: %v", err)
	}

	if len(citations) != 0 {
		t.Errorf("expected 0 citations, got %v", citations)
	}
	if asstMsg == nil {
		t.Fatal("expected non-nil asstMsg")
	}
}

func TestService_InitiateRecordingChatStream_LLMError(t *testing.T) {
	svc, mock, mockLLM, _ := setupChatTestService(t)
	recID := uuid.New()
	userID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id"}).
			AddRow(recID, &userID))

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "chat_messages"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(uuid.New()))
	mock.ExpectCommit()

	mock.ExpectQuery(`SELECT .* FROM "transcript_chunks" WHERE recording_id = \$1.*`).
		WithArgs(recID, sqlmock.AnyArg(), 5).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "chunk_index", "content", "start_time", "end_time"}))

	mockLLM.StreamChatResponseFunc = func(ctx context.Context, systemPrompt string, messages []dtos.ChatMessageInput, opts dtos.ChatOptions) (<-chan dtos.StreamChunk, error) {
		return nil, errors.New("llm provider service overloaded")
	}

	_, err := svc.InitiateRecordingChatStream(ctx, recID, "", dtos.RecordingChatRequest{
		Message: "Hi",
	})
	if err == nil {
		t.Fatal("expected error from LLM stream initiation, got nil")
	}
	if !strings.Contains(err.Error(), "llm provider service overloaded") {
		t.Errorf("expected error message to propagate, got %v", err)
	}
}
