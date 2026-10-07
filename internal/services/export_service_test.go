package services_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/pkg/ctxmeta"
	"code-base-golang/internal/services"
)

func TestService_ExportRecordingMOM_RecordingNotFound(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	res, err := svc.ExportRecordingMOM(ctx, recID, "token", "markdown", "")
	if !errors.Is(err, constants.ErrRecordingNotFound) {
		t.Fatalf("expected ErrRecordingNotFound, got %v", err)
	}
	if res != nil {
		t.Fatalf("expected nil result, got %+v", res)
	}
}

func TestService_ExportRecordingMOM_Forbidden(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	ownerID := uuid.New()
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id", "is_share_enabled"}).
			AddRow(recID, "valid-token", &ownerID, false))

	res, err := svc.ExportRecordingMOM(ctx, recID, "wrong-token", "markdown", "")
	if !errors.Is(err, constants.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
	if res != nil {
		t.Fatalf("expected nil result, got %+v", res)
	}
}

func TestService_ExportRecordingMOM_InvalidFormat(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	ownerID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: ownerID})

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ownership_token", "user_id", "is_share_enabled"}).
			AddRow(recID, "token", &ownerID, false))

	res, err := svc.ExportRecordingMOM(ctx, recID, "", "unsupported_xyz", "")
	if !errors.Is(err, constants.ErrBadRequest) {
		t.Fatalf("expected ErrBadRequest for unsupported format, got %v", err)
	}
	if res != nil {
		t.Fatalf("expected nil result, got %+v", res)
	}
}

func TestService_ExportRecordingMOM_Success_Markdown_Owner(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	ownerID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: ownerID})

	now := time.Now()
	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "title", "duration_seconds", "status", "created_at", "user_id", "ownership_token", "is_share_enabled"}).
			AddRow(recID, "Quarterly Business Review", 3600.0, "COMPLETED", now, &ownerID, "token", false))

	// Active summary
	structDataJSON := `{"executive_summary":"Q3 results exceeded targets.","action_items":["Finalize budget by Friday","Review deck"]}`
	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND is_active = TRUE ORDER BY version DESC.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "version", "is_active", "markdown_content", "structured_data"}).
			AddRow(uuid.New(), recID, 1, true, "Executive summary markdown", structDataJSON))

	// Segments
	mock.ExpectQuery(`SELECT \* FROM "transcript_segments" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "speaker_name", "speaker_label", "start_time", "end_time", "text"}).
			AddRow(uuid.New(), recID, "Alice CEO", "Speaker 0", 0.0, 15.0, "Good morning team, let's begin."))

	// Chapters
	mock.ExpectQuery(`SELECT \* FROM "chapters" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC.*`).
		WithArgs(recID, 100).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "title", "start_time", "end_time", "summary", "sequence_order"}).
			AddRow(uuid.New(), recID, "Introduction", 0.0, 120.0, "Opening remarks", 1))

	// Highlights
	hTitle := "Key metric"
	hNote := "Revenue grew 40%"
	mock.ExpectQuery(`SELECT \* FROM "highlights" WHERE recording_id = \$1 ORDER BY start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "title", "note", "start_time", "end_time"}).
			AddRow(uuid.New(), recID, &hTitle, &hNote, 10.0, 20.0))

	res, err := svc.ExportRecordingMOM(ctx, recID, "", "markdown", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil {
		t.Fatal("expected non-nil result")
	}

	if !strings.HasSuffix(res.Filename, "_mom.md") {
		t.Errorf("expected filename ending in _mom.md, got %s", res.Filename)
	}
	if res.ContentType != "text/markdown; charset=utf-8" {
		t.Errorf("expected markdown content type, got %s", res.ContentType)
	}

	content := string(res.Data)
	if !strings.Contains(content, "# Quarterly Business Review") {
		t.Errorf("expected title in markdown, got: %s", content)
	}
	if !strings.Contains(content, "Q3 results exceeded targets.") {
		t.Errorf("expected executive summary in markdown, got: %s", content)
	}
	if !strings.Contains(content, "Finalize budget by Friday") {
		t.Errorf("expected action item in markdown, got: %s", content)
	}
	if !strings.Contains(content, "Introduction") {
		t.Errorf("expected chapter title in markdown, got: %s", content)
	}
	if !strings.Contains(content, "Alice CEO") {
		t.Errorf("expected speaker name in markdown, got: %s", content)
	}
}

func TestService_ExportRecordingMOM_Success_Txt_GuestToken(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	guestToken := "guest-export-token"
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "title", "duration_seconds", "status", "created_at", "user_id", "ownership_token", "is_share_enabled"}).
			AddRow(recID, "Sprint Retrospective", 1800.0, "COMPLETED", time.Now(), nil, guestToken, false))

	// No summary exists (ErrRecordNotFound)
	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND is_active = TRUE ORDER BY version DESC.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	// Segments
	mock.ExpectQuery(`SELECT \* FROM "transcript_segments" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "speaker_name", "speaker_label", "start_time", "end_time", "text"}).
			AddRow(uuid.New(), recID, "Bob", "Speaker 0", 0.0, 5.0, "Let's review the sprint board."))

	// Chapters (empty)
	mock.ExpectQuery(`SELECT \* FROM "chapters" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC.*`).
		WithArgs(recID, 100).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id"}))

	// Highlights (empty)
	mock.ExpectQuery(`SELECT \* FROM "highlights" WHERE recording_id = \$1 ORDER BY start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id"}))

	res, err := svc.ExportRecordingMOM(ctx, recID, guestToken, "txt", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil {
		t.Fatal("expected non-nil result")
	}

	if !strings.HasSuffix(res.Filename, "_mom.txt") {
		t.Errorf("expected filename ending in _mom.txt, got %s", res.Filename)
	}
	if res.ContentType != "text/plain; charset=utf-8" {
		t.Errorf("expected text content type, got %s", res.ContentType)
	}

	content := string(res.Data)
	if !strings.Contains(content, "MINUTES OF MEETING: SPRINT RETROSPECTIVE") {
		t.Errorf("expected plain text header, got: %s", content)
	}
	if !strings.Contains(content, "Bob: Let's review the sprint board.") {
		t.Errorf("expected transcript line in txt, got: %s", content)
	}
}

func TestService_ExportRecordingMOM_Success_JSON_PublicShare(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "title", "duration_seconds", "status", "created_at", "user_id", "ownership_token", "is_share_enabled"}).
			AddRow(recID, "Public Demo", 600.0, "COMPLETED", time.Now(), nil, "token", true))

	// Summary
	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND is_active = TRUE ORDER BY version DESC.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "version", "is_active", "markdown_content", "structured_data"}).
			AddRow(uuid.New(), recID, 1, true, "Public demo summary", `{"executive_summary":"Demo summary"}`))

	// Segments
	mock.ExpectQuery(`SELECT \* FROM "transcript_segments" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "speaker_name", "speaker_label", "start_time", "end_time", "text"}).
			AddRow(uuid.New(), recID, "Presenter", "Speaker 0", 0.0, 10.0, "Welcome viewers!"))

	// Chapters
	mock.ExpectQuery(`SELECT \* FROM "chapters" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC.*`).
		WithArgs(recID, 100).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id"}))

	// Highlights
	mock.ExpectQuery(`SELECT \* FROM "highlights" WHERE recording_id = \$1 ORDER BY start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id"}))

	res, err := svc.ExportRecordingMOM(ctx, recID, "", "json", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil {
		t.Fatal("expected non-nil result")
	}

	if !strings.HasSuffix(res.Filename, "_mom.json") {
		t.Errorf("expected filename ending in _mom.json, got %s", res.Filename)
	}
	if res.ContentType != "application/json; charset=utf-8" {
		t.Errorf("expected json content type, got %s", res.ContentType)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(res.Data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal exported json: %v", err)
	}

	if parsed["title"] != "Public Demo" {
		t.Errorf("expected title 'Public Demo', got %v", parsed["title"])
	}
	if parsed["executive_summary"] != "Demo summary" {
		t.Errorf("expected executive_summary, got %v", parsed["executive_summary"])
	}
}

func TestService_ExportRecordingMOM_Success_PDF(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "title", "duration_seconds", "status", "created_at", "user_id", "ownership_token", "is_share_enabled"}).
			AddRow(recID, "Architecture Review", 1200.0, "COMPLETED", time.Now(), nil, "token", true))

	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND is_active = TRUE ORDER BY version DESC.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "version", "is_active", "markdown_content", "structured_data"}).
			AddRow(uuid.New(), recID, 1, true, "Architecture overview", `{"executive_summary":"Arch review"}`))

	mock.ExpectQuery(`SELECT \* FROM "transcript_segments" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "speaker_name", "speaker_label", "start_time", "end_time", "text"}).
			AddRow(uuid.New(), recID, "Lead Architect", "Speaker 0", 0.0, 8.0, "Let's review the microservices topology."))

	mock.ExpectQuery(`SELECT \* FROM "chapters" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC.*`).
		WithArgs(recID, 100).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id"}))

	mock.ExpectQuery(`SELECT \* FROM "highlights" WHERE recording_id = \$1 ORDER BY start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id"}))

	res, err := svc.ExportRecordingMOM(ctx, recID, "", "pdf", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil {
		t.Fatal("expected non-nil result")
	}

	if !strings.HasSuffix(res.Filename, "_mom.pdf") {
		t.Errorf("expected filename ending in _mom.pdf, got %s", res.Filename)
	}
	if res.ContentType != "application/pdf" {
		t.Errorf("expected pdf content type, got %s", res.ContentType)
	}

	// Verify valid PDF standard header and trailer
	if !bytes.HasPrefix(res.Data, []byte("%PDF-1.4")) {
		t.Errorf("expected PDF header %%PDF-1.4, got: %s", string(res.Data[:10]))
	}
	if !bytes.Contains(res.Data, []byte("%%EOF")) {
		t.Errorf("expected PDF trailer %%%%EOF")
	}
}

func TestService_ExportRecordingMOM_UnicodeFilename(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)

	recID := uuid.New()
	userID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	// Unicode title > 60 runes
	longUnicodeTitle := "会議記録_プロトコル_テスト_プロジェクト_アーキテクチャ_詳細_レビュー_検証_セッション_長いタイトル_追加テスト"
	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "title", "status", "created_at"}).
			AddRow(recID, userID, longUnicodeTitle, "completed", time.Now()))

	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND is_active = TRUE ORDER BY version DESC.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "version", "is_active", "markdown_content", "structured_data"}).
			AddRow(uuid.New(), recID, 1, true, "Unicode summary", `{"executive_summary":"詳細内容"}`))

	mock.ExpectQuery(`SELECT \* FROM "transcript_segments" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "speaker_name", "speaker_label", "start_time", "end_time", "text"}).
			AddRow(uuid.New(), recID, "テスト話者", "Speaker 0", 0.0, 10.0, "こんにちは"))

	mock.ExpectQuery(`SELECT \* FROM "chapters" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC.*`).
		WithArgs(recID, 100).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id"}))

	mock.ExpectQuery(`SELECT \* FROM "highlights" WHERE recording_id = \$1 ORDER BY start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id"}))

	res, err := svc.ExportRecordingMOM(ctx, recID, "", "txt", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !utf8.ValidString(res.Filename) {
		t.Errorf("expected valid UTF-8 filename, got invalid bytes: %s", res.Filename)
	}
	if !strings.HasSuffix(res.Filename, "_mom.txt") {
		t.Errorf("expected filename ending in _mom.txt, got %s", res.Filename)
	}
}

func TestService_ExportRecordingMOM_StandupSchema_StructuredFormatting(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	userID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	now := time.Now()
	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "title", "duration_seconds", "status", "created_at", "user_id", "ownership_token", "is_share_enabled"}).
			AddRow(recID, "Simulasi Interview Kerja", 370.0, "COMPLETED", now, &userID, "token", false))

	// Standup structured data matching user screenshot
	standupJSON := `{"$schema":"http://json-schema.org/draft-07/schema#","sprint_health":{"status":"ON TRACK","summary":"Interview berjalan lancar, tidak ada blocker teridentifikasi."},"member_updates":[{"member_name":"Putri Deski Patok Fatimah","yesterday":["Mengikuti proses interview."],"today":["Menjelaskan kemampuan Excel dan MYOB."],"blockers":["Menunggu informasi hasil seleksi."]}],"critical_blockers":["Menunggu konfirmasi HR"],"parking_lot_discussions":[{"topic":"Informasi job desk lanjutan","participants":["Putri Deski","Interviewer"]}]}`
	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND is_active = TRUE ORDER BY version DESC.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "version", "is_active", "markdown_content", "structured_data"}).
			AddRow(uuid.New(), recID, 1, true, standupJSON, standupJSON))

	// Segments
	mock.ExpectQuery(`SELECT \* FROM "transcript_segments" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "speaker_name", "speaker_label", "start_time", "end_time", "text"}).
			AddRow(uuid.New(), recID, "Interviewer", "Speaker 0", 12.0, 312.0, "Selamat siang, mari kita mulai interview hari ini.").
			AddRow(uuid.New(), recID, "Putri Deski", "Speaker 1", 313.0, 370.0, "Selamat siang Pak, terima kasih atas kesempatannya."))

	// Chapters
	mock.ExpectQuery(`SELECT \* FROM "chapters" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC.*`).
		WithArgs(recID, 100).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "title", "start_time", "end_time", "summary", "sequence_order"}).
			AddRow(uuid.New(), recID, "Perkenalan & Pengalaman", 12.0, 312.0, "Perkenalan diri dan riwayat pengalaman.", 1))

	// Highlights
	hTitle := "Kualifikasi Utama"
	hNote := "Menguasai MYOB dan Excel akuntansi"
	mock.ExpectQuery(`SELECT \* FROM "highlights" WHERE recording_id = \$1 ORDER BY start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "title", "note", "start_time", "end_time"}).
			AddRow(uuid.New(), recID, &hTitle, &hNote, 50.0, 70.0))

	res, err := svc.ExportRecordingMOM(ctx, recID, "", "txt", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content := string(res.Data)

	// Verify NEVER dumps raw JSON
	if strings.Contains(content, `{"$schema"`) {
		t.Errorf("export output must NOT contain raw JSON schema dump, got: %s", content)
	}

	// Verify structured contents are present
	if !strings.Contains(content, "Status Sprint: ON TRACK") {
		t.Errorf("expected sprint status, got: %s", content)
	}
	if !strings.Contains(content, "Putri Deski Patok Fatimah") {
		t.Errorf("expected member name, got: %s", content)
	}
	if !strings.Contains(content, "Kendala Kritis: Menunggu konfirmasi HR") {
		t.Errorf("expected critical blocker in action items, got: %s", content)
	}

	// Verify strict section order: 1. RINGKASAN -> 2. TRANSKRIP -> 3. SOROTAN -> 4. ANALITIK
	idxRingkasan := strings.Index(content, "1. RINGKASAN PERTEMUAN")
	idxTranskrip := strings.Index(content, "2. TRANSKRIP PERCAKAPAN")
	idxSorotan := strings.Index(content, "3. SOROTAN & PEMBAHASAN BAB")
	idxAnalitik := strings.Index(content, "4. ANALITIK PERCAKAPAN")

	if idxRingkasan == -1 || idxTranskrip == -1 || idxSorotan == -1 || idxAnalitik == -1 {
		t.Fatalf("missing one of the 4 required sections in export: %s", content)
	}

	if !(idxRingkasan < idxTranskrip && idxTranskrip < idxSorotan && idxSorotan < idxAnalitik) {
		t.Errorf("sections are not in the required order (Ringkasan -> Transkrip -> Sorotan -> Analitik). Indices: Ringkasan=%d, Transkrip=%d, Sorotan=%d, Analitik=%d",
			idxRingkasan, idxTranskrip, idxSorotan, idxAnalitik)
	}

	// Verify Analytics content
	if !strings.Contains(content, "PARTISIPASI PEMBICARA:") {
		t.Errorf("expected speaker participation section, got: %s", content)
	}
	if !strings.Contains(content, "Interviewer") || !strings.Contains(content, "Putri Deski") {
		t.Errorf("expected both speakers in analytics, got: %s", content)
	}
}

func TestWrapTextLine(t *testing.T) {
	line := "Selamat siang Pak, ini surat lamaran CV saya. Baik, gimana tadi perjalanannya? Alhamdulillah lancar Pak, saya datang ke sini tepat dulu kalau boleh tahu rumahnya di mana?"
	wrapped := services.WrapTextLine(line, 50)

	for _, l := range wrapped {
		if len(l) > 50 {
			t.Errorf("line length %d exceeds max length 50: %q", len(l), l)
		}
		// Verify no words are split mid-word (no isolated letters from cut words like "k" or "lam")
		words := strings.Fields(l)
		for _, w := range words {
			if w == "lam" || w == "aran" {
				t.Errorf("word 'lamaran' was incorrectly split into: %s", w)
			}
		}
	}
}

func TestService_ExportRecordingMOM_SpecificVersion(t *testing.T) {
	svc, mock, _, _ := setupRecordingTestService(t)
	recID := uuid.New()
	userID := uuid.New()
	ctx := ctxmeta.WithAuthUser(context.Background(), ctxmeta.AuthUser{UserID: userID})

	now := time.Now()
	mock.ExpectQuery(`SELECT \* FROM "recordings" WHERE id = \$1 AND "recordings"\."deleted_at" IS NULL.*LIMIT \$2`).
		WithArgs(recID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "title", "duration_seconds", "status", "created_at", "user_id", "ownership_token", "is_share_enabled"}).
			AddRow(recID, "Interview Evaluation", 300.0, "COMPLETED", now, &userID, "token", false))

	// Mock specific version query for version 2
	version2JSON := `{"candidate_name":"John Doe","target_role":"Lead Backend Engineer","recommendation":"STRONG_HIRE","justification":"Exceptional Go and systems design expertise."}`
	mock.ExpectQuery(`SELECT \* FROM "summaries" WHERE recording_id = \$1 AND version = \$2.*LIMIT \$3`).
		WithArgs(recID, 2, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id", "version", "template_category", "is_active", "markdown_content", "structured_data"}).
			AddRow(uuid.New(), recID, 2, "INTERVIEW", false, version2JSON, version2JSON))

	// Segments
	mock.ExpectQuery(`SELECT \* FROM "transcript_segments" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id"}))

	// Chapters
	mock.ExpectQuery(`SELECT \* FROM "chapters" WHERE recording_id = \$1 ORDER BY sequence_order ASC, start_time ASC.*`).
		WithArgs(recID, 100).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id"}))

	// Highlights
	mock.ExpectQuery(`SELECT \* FROM "highlights" WHERE recording_id = \$1 ORDER BY start_time ASC`).
		WithArgs(recID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recording_id"}))

	res, err := svc.ExportRecordingMOM(ctx, recID, "", "txt", "2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(res.Filename, "_v2") {
		t.Errorf("expected filename containing _v2, got: %s", res.Filename)
	}

	content := string(res.Data)
	if !strings.Contains(content, "Versi 2 (INTERVIEW)") {
		t.Errorf("expected version 2 label in export content, got: %s", content)
	}
	if !strings.Contains(content, "John Doe") || !strings.Contains(content, "Lead Backend Engineer") {
		t.Errorf("expected version 2 candidate data in export, got: %s", content)
	}
}


