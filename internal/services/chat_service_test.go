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

func TestService_InitiateRecordingChatStream_Forbidden_ShareTokenDisabled(t *testing.T) {
	svc, mock, _, _ := setupChatTestService(t)
	recID := uuid.New()
	ownerID := uuid.New()
	ownershipToken := "secret-ownership-token"
	shareToken := "viewer-share-token"
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "ownership_token", "is_share_enabled", "share_token"}).
			AddRow(recID, &ownerID, ownershipToken, false, &shareToken))

	// When share is disabled, viewer supplies shareToken -> must be rejected with ErrForbidden
	_, err := svc.InitiateRecordingChatStream(ctx, recID, shareToken, dtos.RecordingChatRequest{
		Message: "Hello from viewer",
	})
	if !errors.Is(err, constants.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for share token viewer when sharing is disabled, got %v", err)
	}
}

func TestService_InitiateRecordingChatStream_Success_ShareToken(t *testing.T) {
	svc, mock, mockLLM, emb := setupChatTestService(t)
	emb.err = errors.New("embedding skipped in share viewer test")
	recID := uuid.New()
	ownerID := uuid.New()
	ownershipToken := "secret-ownership-token"
	shareToken := "viewer-share-token"
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "ownership_token", "is_share_enabled", "share_token"}).
			AddRow(recID, &ownerID, ownershipToken, true, &shareToken))

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "chat_messages"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(uuid.New()))
	mock.ExpectCommit()

	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND is_active = TRUE.*`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	mockLLM.StreamChatResponseFunc = func(ctx context.Context, systemPrompt string, messages []dtos.ChatMessageInput, opts dtos.ChatOptions) (<-chan dtos.StreamChunk, error) {
		ch := make(chan dtos.StreamChunk, 1)
		ch <- dtos.StreamChunk{Content: "Response for shared session viewer"}
		close(ch)
		return ch, nil
	}

	result, err := svc.InitiateRecordingChatStream(ctx, recID, shareToken, dtos.RecordingChatRequest{
		Message: "Question from viewer",
	})
	if err != nil {
		t.Fatalf("unexpected error for share token viewer: %v", err)
	}
	if result.SessionID == uuid.Nil {
		t.Error("expected non-nil session ID")
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

	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND is_active = TRUE.*`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

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

	asstMsg, citations, err := svc.SaveAssistantChatMessage(ctx, recID, result.SessionID, fullText, result.RetrievedChunkIDs)
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

	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND is_active = TRUE.*`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

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

	asstMsg, citations, err := svc.SaveAssistantChatMessage(ctx, recID, result.SessionID, "I have no transcript context available.", result.RetrievedChunkIDs)
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
	svc, mock, mockLLM, emb := setupChatTestService(t)
	emb.err = errors.New("embedding skipped in llm error test")
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

	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND is_active = TRUE.*`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

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

func TestService_InitiateRecordingChatStream_Success_GuestSession(t *testing.T) {
	svc, mock, mockLLM, emb := setupChatTestService(t)
	emb.err = errors.New("embedding skipped in guest test")
	recID := uuid.New()
	anonSessionID := "anon-session-12345"
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{
		IsGuest:   true,
		SessionID: anonSessionID,
	})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "is_guest"}).
			AddRow(recID, anonSessionID, true))

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "chat_messages"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(uuid.New()))
	mock.ExpectCommit()

	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND is_active = TRUE.*`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	mockLLM.StreamChatResponseFunc = func(ctx context.Context, systemPrompt string, messages []dtos.ChatMessageInput, opts dtos.ChatOptions) (<-chan dtos.StreamChunk, error) {
		out := make(chan dtos.StreamChunk, 1)
		go func() {
			defer close(out)
			out <- dtos.StreamChunk{Content: "Hello guest!"}
		}()
		return out, nil
	}

	result, err := svc.InitiateRecordingChatStream(ctx, recID, "", dtos.RecordingChatRequest{
		Message: "Hello",
	})
	if err != nil {
		t.Fatalf("unexpected error for guest session: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestService_ListRecordingChatHistory_Success(t *testing.T) {
	svc, mock, _, _ := setupChatTestService(t)
	recID := uuid.New()
	sessionID := uuid.New()
	ownerID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: ownerID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id"}).
			AddRow(recID, &ownerID))

	mock.ExpectQuery(`SELECT \* FROM "chat_messages" WHERE recording_id = \$1 AND session_id = \$2 ORDER BY created_at ASC`).
		WithArgs(recID, sessionID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "session_id", "content", "sender_role"}).
			AddRow(uuid.New(), recID, sessionID, "Hello", "user").
			AddRow(uuid.New(), recID, sessionID, "Hi there!", "assistant"))

	msgs, err := svc.ListRecordingChatHistory(ctx, recID, sessionID, "", "")
	if err != nil {
		t.Fatalf("unexpected error listing chat history: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
}

func TestService_ListRecordingChatHistory_Forbidden(t *testing.T) {
	svc, mock, _, _ := setupChatTestService(t)
	recID := uuid.New()
	sessionID := uuid.New()
	ownerID := uuid.New()
	wrongUserID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: wrongUserID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id"}).
			AddRow(recID, &ownerID))

	_, err := svc.ListRecordingChatHistory(ctx, recID, sessionID, "", "")
	if !errors.Is(err, constants.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestService_InitiateRecordingChatStream_Success_ShareToken_WithExplicitSessionID(t *testing.T) {
	svc, mock, mockLLM, emb := setupChatTestService(t)
	emb.err = errors.New("embedding skipped in share test")
	recID := uuid.New()
	shareToken := "valid-share-token-xyz"
	sessionID := uuid.New()
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "is_share_enabled", "share_token"}).
			AddRow(recID, true, &shareToken))

	mock.ExpectQuery(`SELECT \* FROM "chat_messages" WHERE recording_id = \$1 AND session_id = \$2 ORDER BY created_at ASC`).
		WithArgs(recID, sessionID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "session_id"}))

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "chat_messages"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(uuid.New()))
	mock.ExpectCommit()

	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND is_active = TRUE.*`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	mockLLM.StreamChatResponseFunc = func(ctx context.Context, systemPrompt string, messages []dtos.ChatMessageInput, opts dtos.ChatOptions) (<-chan dtos.StreamChunk, error) {
		out := make(chan dtos.StreamChunk, 1)
		go func() {
			defer close(out)
			out <- dtos.StreamChunk{Content: "Hello viewer via share token!"}
		}()
		return out, nil
	}

	result, err := svc.InitiateRecordingChatStream(ctx, recID, "", dtos.RecordingChatRequest{
		Message:    "Can I ask about this shared meeting?",
		SessionID:  &sessionID,
		ShareToken: shareToken,
	})
	if err != nil {
		t.Fatalf("unexpected error initiating stream with share token: %v", err)
	}
	if result.SessionID != sessionID {
		t.Fatalf("expected session ID %v, got %v", sessionID, result.SessionID)
	}
}

func TestService_ListRecordingChatHistory_Success_ShareToken(t *testing.T) {
	svc, mock, _, _ := setupChatTestService(t)
	recID := uuid.New()
	shareToken := "valid-share-token-xyz"
	sessionID := uuid.New()
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "is_share_enabled", "share_token"}).
			AddRow(recID, true, &shareToken))

	mock.ExpectQuery(`SELECT \* FROM "chat_messages" WHERE recording_id = \$1 AND session_id = \$2 ORDER BY created_at ASC`).
		WithArgs(recID, sessionID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "session_id", "content", "sender_role"}).
			AddRow(uuid.New(), recID, sessionID, "Hello viewer question", "user"))

	msgs, err := svc.ListRecordingChatHistory(ctx, recID, sessionID, "", shareToken)
	if err != nil {
		t.Fatalf("unexpected error listing chat history via share token: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
}

func TestService_ListRecordingChatHistory_Forbidden_ShareVisitorCannotAccessOwnerSession(t *testing.T) {
	svc, mock, _, _ := setupChatTestService(t)
	recID := uuid.New()
	ownerID := uuid.New()
	shareToken := "valid-share-token-xyz"
	ownerSessionID := uuid.New()
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "is_share_enabled", "share_token"}).
			AddRow(recID, &ownerID, true, &shareToken))

	// The session messages in DB have user_id = ownerID
	mock.ExpectQuery(`SELECT \* FROM "chat_messages" WHERE recording_id = \$1 AND session_id = \$2 ORDER BY created_at ASC`).
		WithArgs(recID, ownerSessionID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "session_id", "user_id", "content", "sender_role"}).
			AddRow(uuid.New(), recID, ownerSessionID, &ownerID, "Private question from owner", "user"))

	_, err := svc.ListRecordingChatHistory(ctx, recID, ownerSessionID, "", shareToken)
	if !errors.Is(err, constants.ErrForbidden) {
		t.Fatalf("expected ErrForbidden when share visitor attempts to access owner session, got %v", err)
	}
}

func TestService_ListRecordingChatHistory_Forbidden_CrossUserShareSession(t *testing.T) {
	svc, mock, _, _ := setupChatTestService(t)
	recID := uuid.New()
	ownerID := uuid.New()
	userB := uuid.New()
	userC := uuid.New()
	shareToken := "valid-share-token-xyz"
	userBSessionID := uuid.New()
	// User C is calling
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userC})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "is_share_enabled", "share_token"}).
			AddRow(recID, &ownerID, true, &shareToken))

	// Session messages belong to User B
	mock.ExpectQuery(`SELECT \* FROM "chat_messages" WHERE recording_id = \$1 AND session_id = \$2 ORDER BY created_at ASC`).
		WithArgs(recID, userBSessionID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "session_id", "user_id", "content", "sender_role"}).
			AddRow(uuid.New(), recID, userBSessionID, &userB, "Question from user B", "user"))

	_, err := svc.ListRecordingChatHistory(ctx, recID, userBSessionID, "", shareToken)
	if !errors.Is(err, constants.ErrForbidden) {
		t.Fatalf("expected ErrForbidden when user C attempts to access user B's session, got %v", err)
	}
}

func TestService_ListRecordingChatHistory_Success_LoggedInShareVisitorOwnSession(t *testing.T) {
	svc, mock, _, _ := setupChatTestService(t)
	recID := uuid.New()
	ownerID := uuid.New()
	userB := uuid.New()
	shareToken := "valid-share-token-xyz"
	userBSessionID := uuid.New()
	// User B is calling
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userB})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "is_share_enabled", "share_token"}).
			AddRow(recID, &ownerID, true, &shareToken))

	// Session messages belong to User B
	mock.ExpectQuery(`SELECT \* FROM "chat_messages" WHERE recording_id = \$1 AND session_id = \$2 ORDER BY created_at ASC`).
		WithArgs(recID, userBSessionID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "session_id", "user_id", "content", "sender_role"}).
			AddRow(uuid.New(), recID, userBSessionID, &userB, "Question from user B", "user"))

	msgs, err := svc.ListRecordingChatHistory(ctx, recID, userBSessionID, "", shareToken)
	if err != nil {
		t.Fatalf("unexpected error when user B accesses their own share session: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
}

func TestService_InitiateRecordingChatStream_WithActiveSummaryAndSpeakers(t *testing.T) {
	svc, mock, mockLLM, emb := setupChatTestService(t)
	emb.err = errors.New("embedding skipped for active summary test")
	recID := uuid.New()
	ownerID := uuid.New()
	ownershipToken := "owner-token-xyz"
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: ownerID})

	analyticsJSON := `{"speakers":[{"name":"Alice"},{"name":"Bob"}]}`
	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "ownership_token", "title", "duration_seconds", "selected_template", "analytics_data"}).
			AddRow(recID, &ownerID, ownershipToken, "Quarterly Strategy Sync", 3600.0, "TECH_REVIEW", []byte(analyticsJSON)))

	// Insert user message
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "chat_messages"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(uuid.New()))
	mock.ExpectCommit()

	// Return active summary from DB
	summaryStructuredJSON := `{"executive_summary":"Discussed cloud migration strategy and budget allocation.","action_items":["Alice to finalize architecture document by Friday"]}`
	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND is_active = TRUE.*`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "is_active", "template_category", "structured_data", "markdown_content"}).
			AddRow(uuid.New(), recID, true, "TECH_REVIEW", []byte(summaryStructuredJSON), "Discussed cloud migration strategy."))

	var capturedUserPrompt string
	mockLLM.StreamChatResponseFunc = func(ctx context.Context, systemPrompt string, messages []dtos.ChatMessageInput, opts dtos.ChatOptions) (<-chan dtos.StreamChunk, error) {
		for _, m := range messages {
			if m.Role == "user" {
				capturedUserPrompt = m.Content
			}
		}
		ch := make(chan dtos.StreamChunk, 1)
		ch <- dtos.StreamChunk{Content: "Here is the summary."}
		close(ch)
		return ch, nil
	}

	result, err := svc.InitiateRecordingChatStream(ctx, recID, ownershipToken, dtos.RecordingChatRequest{
		Message: "Rangkum poin penting rapat ini",
	})
	if err != nil {
		t.Fatalf("unexpected error initiating stream with summary: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil stream result")
	}

	for range result.StreamChannel {
	}

	if !strings.Contains(capturedUserPrompt, "Quarterly Strategy Sync") {
		t.Errorf("expected title in user prompt, got: %s", capturedUserPrompt)
	}
	if !strings.Contains(capturedUserPrompt, "Duration: 01:00:00") {
		t.Errorf("expected formatted duration in user prompt, got: %s", capturedUserPrompt)
	}
	if !strings.Contains(capturedUserPrompt, "Alice, Bob") {
		t.Errorf("expected speakers list in user prompt, got: %s", capturedUserPrompt)
	}
	if !strings.Contains(capturedUserPrompt, "Discussed cloud migration strategy") {
		t.Errorf("expected executive overview in user prompt, got: %s", capturedUserPrompt)
	}
	if !strings.Contains(capturedUserPrompt, "Alice to finalize architecture document") {
		t.Errorf("expected action item in user prompt, got: %s", capturedUserPrompt)
	}
}

func TestService_InitiateRecordingChatStream_MultiTurnSessionHistoryRestoration(t *testing.T) {
	svc, mock, mockLLM, emb := setupChatTestService(t)
	emb.err = errors.New("embedding skipped for multi-turn test")
	recID := uuid.New()
	ownerID := uuid.New()
	sessionID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: ownerID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id"}).
			AddRow(recID, &ownerID))

	// Restore prior session turns from DB
	mock.ExpectQuery(`SELECT \* FROM "chat_messages" WHERE recording_id = \$1 AND session_id = \$2 ORDER BY created_at ASC`).
		WithArgs(recID, sessionID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "session_id", "user_id", "content", "sender_role"}).
			AddRow(uuid.New(), recID, sessionID, &ownerID, "What is the launch timeline?", "user").
			AddRow(uuid.New(), recID, sessionID, &ownerID, "The launch is targeted for November.", "assistant"))

	// Insert current user turn
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "chat_messages"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(uuid.New()))
	mock.ExpectCommit()

	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND is_active = TRUE.*`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	var capturedMessages []dtos.ChatMessageInput
	mockLLM.StreamChatResponseFunc = func(ctx context.Context, systemPrompt string, messages []dtos.ChatMessageInput, opts dtos.ChatOptions) (<-chan dtos.StreamChunk, error) {
		capturedMessages = messages
		ch := make(chan dtos.StreamChunk, 1)
		ch <- dtos.StreamChunk{Content: "It can be adjusted if needed."}
		close(ch)
		return ch, nil
	}

	result, err := svc.InitiateRecordingChatStream(ctx, recID, "", dtos.RecordingChatRequest{
		SessionID: &sessionID,
		Message:   "Can that date be postponed?",
	})
	if err != nil {
		t.Fatalf("unexpected error initiating multi-turn chat stream: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil stream result")
	}

	for range result.StreamChannel {
	}

	if len(capturedMessages) < 3 {
		t.Fatalf("expected at least 3 messages (2 prior + 1 current), got %d", len(capturedMessages))
	}
	if capturedMessages[0].Role != "user" || capturedMessages[0].Content != "What is the launch timeline?" {
		t.Errorf("unexpected first message: %+v", capturedMessages[0])
	}
	if capturedMessages[1].Role != "assistant" || capturedMessages[1].Content != "The launch is targeted for November." {
		t.Errorf("unexpected second message: %+v", capturedMessages[1])
	}
	if capturedMessages[2].Role != "user" || !strings.Contains(capturedMessages[2].Content, "Can that date be postponed?") {
		t.Errorf("unexpected third message: %+v", capturedMessages[2])
	}
}

func TestService_InitiateRecordingChatStream_WithClientProvidedSummaryOverrides(t *testing.T) {
	svc, mock, mockLLM, emb := setupChatTestService(t)
	emb.err = errors.New("embedding skipped for client override test")
	recID := uuid.New()
	ownerID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: ownerID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id"}).
			AddRow(recID, &ownerID))

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "chat_messages"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(uuid.New()))
	mock.ExpectCommit()

	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND is_active = TRUE.*`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	var capturedUserPrompt string
	mockLLM.StreamChatResponseFunc = func(ctx context.Context, systemPrompt string, messages []dtos.ChatMessageInput, opts dtos.ChatOptions) (<-chan dtos.StreamChunk, error) {
		for _, m := range messages {
			if m.Role == "user" {
				capturedUserPrompt = m.Content
			}
		}
		ch := make(chan dtos.StreamChunk, 1)
		ch <- dtos.StreamChunk{Content: "Response with client override."}
		close(ch)
		return ch, nil
	}

	_, err := svc.InitiateRecordingChatStream(ctx, recID, "", dtos.RecordingChatRequest{
		Message:         "Check action items",
		SummaryOverview: "Sprint was focused on performance optimizations.",
		ActionItems:     []string{"Optimize GORM scopes", "Add Redis caching"},
	})
	if err != nil {
		t.Fatalf("unexpected error initiating stream with client overrides: %v", err)
	}

	if !strings.Contains(capturedUserPrompt, "Sprint was focused on performance optimizations.") {
		t.Errorf("expected client summary overview in prompt, got: %s", capturedUserPrompt)
	}
	if !strings.Contains(capturedUserPrompt, "Optimize GORM scopes") {
		t.Errorf("expected client action item in prompt, got: %s", capturedUserPrompt)
	}
}

func TestService_SaveAssistantChatMessage_WithRangeCitations(t *testing.T) {
	svc, mock, _, _ := setupChatTestService(t)
	recID := uuid.New()
	sessionID := uuid.New()
	ctx := context.Background()

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "chat_messages"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(uuid.New()))
	mock.ExpectCommit()

	content := "The budget debate took place in [02:15 - 02:45] and final approval was recorded at [05:10]."
	asstMsg, citations, err := svc.SaveAssistantChatMessage(ctx, recID, sessionID, content, []string{"chunk-1", "chunk-2"})
	if err != nil {
		t.Fatalf("unexpected error saving assistant message with range citations: %v", err)
	}
	if asstMsg == nil {
		t.Fatal("expected non-nil assistant message")
	}

	if len(citations) != 3 {
		t.Fatalf("expected 3 citations (02:15, 02:45, 05:10), got %d: %v", len(citations), citations)
	}
	if citations[0] != "02:15" || citations[1] != "02:45" || citations[2] != "05:10" {
		t.Errorf("unexpected citations slice: %v", citations)
	}
}
