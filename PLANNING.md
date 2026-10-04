# YOUTEN AI CORE API — MASTER DEVELOPMENT ROADMAP & AGENTIC PLANNING

> **NOTICE FOR AI AGENTS & DEVELOPERS**:
> This document tracks the phased architecture, feature requirements, and technical specifications for `youten-ai-core-api`.
> - **Branch Rule**: NEVER push directly to `main`. Always create a dedicated branch (`feat/...`, `fix/...`, `chore/...`) and open a Pull Request.
> - **Commit Rule**: Every commit must be atomic, focused on a single concern, following Conventional Commits.
> - **State Preservation**: Update this planning document as tasks progress so subsequent agentic sessions can seamlessly resume context.

---

## 1. Project Identity & Architecture Invariants

- **Service Name**: `youten-ai-core-api`
- **Language**: Go 1.25+
- **HTTP Engine**: Gin (`github.com/gin-gonic/gin`)
- **Database / ORM**: PostgreSQL with GORM (`gorm.io/gorm`, `gorm.io/driver/postgres`)
- **Vector Engine**: PostgreSQL + `pgvector/pgvector:pg16` (`CREATE EXTENSION IF NOT EXISTS vector`)
- **Cache & KV**: Redis 7 (`github.com/redis/go-redis/v9`)
- **Message Broker**: RabbitMQ 3.13 (`github.com/rabbitmq/amqp091-go`) with topic routing & DLQ
- **Object Storage**: AWS S3 / MinIO (`github.com/aws/aws-sdk-go-v2`)
- **Email Delivery**: `net/smtp` + Mailpit
- **Schema Migrations**: `github.com/golang-migrate/migrate/v4`
- **Validation**: `github.com/go-ozzo/ozzo-validation/v4`
- **Declarative Routes**: `internal/routes/routes.yaml`
- **Clean Architecture Rule**: Pure business logic lives in `internal/services/`. Services consume idiomatic Go consumer interfaces (`ports`). Concrete adapters (`database`, `redis`, `s3`, `rabbitmq`, `stt`, `llm`, `embedding`) live in `internal/adapters/` and are injected in `cmd/api/main.go`.

---

## 2. Commit Discipline Rules (Strictly Enforced)

1. **Atomic Commits Only**:
   - Each commit must represent a single logical, functional change.
   - NEVER bundle unrelated changes (e.g., mixing migrations with worker logic, or mixing docker changes with route definitions).
   - Conventional Commit standard: `feat(...)`, `fix(...)`, `refactor(...)`, `build(...)`, `chore(...)`, `test(...)`.
2. **Pre-commit Verification**:
   - Verify `make test` passes with zero regressions before committing.
   - Verify `make build` compiles cleanly.

---

## 3. Master Phased Implementation Roadmap

### Phase 1: Environment & Infrastructure Configuration
- [ ] **1.1. Docker Compose**: Update `postgres` service image in `deployment/docker-compose.yaml` from `postgres:16-alpine` to `pgvector/pgvector:pg16`.
- [ ] **1.2. Environment Setup**: Update `.env.example` with:
  - STT settings (`STT_PROVIDER=deepgram`, `DEEPGRAM_API_KEY`, `WHISPER_URL`)
  - LLM settings (`LLM_PROVIDER=openai`, `OPENAI_API_KEY`, `LLM_MODEL=gpt-4o-mini`, `LLM_BASE_URL`)
  - Embedding / RAG settings (`LLM_EMBEDDING_MODEL=text-embedding-3-small`, `EMBEDDING_DIMENSION=1536`, `RAG_CHUNK_TOKENS=300`, `RAG_CHUNK_OVERLAP_TOKENS=50`, `RAG_TOP_K=5`)
  - S3/MinIO bucket names (`S3_BUCKET_RECORDINGS=youten-recordings`, `S3_BUCKET_EXPORTS=youten-exports`)
  - Queue names & routing keys
- [ ] **1.3. Commit**: `build(docker): switch postgres image to pgvector/pgvector:pg16`

---

### Phase 2: Database Migrations (`migrations/`)
All tables must use standard `migrations/<timestamp>_<name>.up.sql` and `down.sql`.

- [ ] **2.1. Migration 000001: Extensions**:
  - `uuid-ossp` and `vector` (pgvector).
- [ ] **2.2. Migration 000002: User & Auth Domain**:
  - `users` (id UUID PK, email UK, password_hash, full_name, status, daily_quota_override, email_verified, timestamps, soft delete).
  - `auth_tokens` (id UUID PK, user_id FK, type, token_hash, expires_at, revoked_at, timestamps).
- [ ] **2.3. Migration 000003: Admin Domain & RBAC**:
  - `admin_roles` (id UUID PK, name UK, description, permissions JSONB, is_system bool, timestamps).
  - `admin_users` (id UUID PK, username UK, password_hash, full_name, role_id FK, status, last_login_at, timestamps).
  - `admin_audit_logs` (id UUID PK, admin_id FK, action, entity, entity_id, payload JSONB, ip_address, user_agent, timestamps).
- [ ] **2.4. Migration 000004: Templates Engine**:
  - `templates` (id UUID PK, category_key UK, name, description, prompt, output_schema JSONB, version int, is_active bool, timestamps).
  - Seed 7 default categories: `MOM`, `1_ON_1`, `INTERVIEW`, `TECH_REVIEW`, `SALES_DISCOVERY`, `DAILY_STANDUP`, `GENERAL`.
- [ ] **2.5. Migration 000005: Recordings Lifecycle**:
  - `recordings` (id UUID PK, user_id FK nullable, ownership_token, share_token, is_share_enabled, title, original_filename, file_size_bytes, duration_seconds, audio_url, source_type, bot_provider, status, error_message, error_code, selected_template, detected_language, output_language, is_guest, guest_ip, consent_given, consent_version, consent_at, expires_at, timestamps, soft delete).
- [ ] **2.6. Migration 000006: Transcripts & pgvector Chunks**:
  - `transcript_segments` (id UUID PK, recording_id FK, speaker_label, speaker_name, start_time float, end_time float, text, words_data JSONB, sequence_order int, timestamps).
  - `transcript_chunks` (id UUID PK, recording_id FK, chunk_index int, content text, start_time float, end_time float, embedding vector(1536), timestamps).
  - HNSW vector index: `CREATE INDEX ON transcript_chunks USING hnsw (embedding vector_cosine_ops);`
- [ ] **2.7. Migration 000007: Summaries, Chapters & Highlights**:
  - `summaries` (id UUID PK, recording_id FK, template_category, custom_angle nullable, version int, is_active bool, structured_data JSONB, markdown_content text, timestamps).
  - `chapters` (id UUID PK, recording_id FK, title, start_time float, end_time float, summary text, sequence_order int, timestamps).
  - `highlights` (id UUID PK, recording_id FK, start_time float, end_time float, title nullable, note nullable, source, clip_url nullable, timestamps).
- [ ] **2.8. Migration 000008: Inline Comments & AI Chat Messages**:
  - `inline_comments` (id UUID PK, recording_id FK, segment_id FK nullable, timestamp_sec float, selected_text text, author_name, comment_text, parent_id FK nullable, timestamps).
  - `chat_messages` (id UUID PK, recording_id FK, sender_role, content, citations JSONB, retrieved_chunk_ids JSONB, timestamps).
- [ ] **2.9. Migration 000009: In-App Notifications & Abuse Reports**:
  - `notifications` (id UUID PK, user_id FK nullable, recording_id FK nullable, title, message, type, is_read bool, timestamps).
  - `reports` (id UUID PK, recording_id FK, reporter_type, reporter_ref, reason text, status, resolution_note, handled_by FK nullable, timestamps).
- [ ] **2.10. Commit**: Atomic commits per migration group.

---

### Phase 3: GORM Domain Models & Repositories
- [ ] **3.1. Models** in `internal/models/`:
  - `user.go`, `auth_token.go`
  - `admin.go`, `template.go`
  - `recording.go`, `transcript.go`, `chunk.go` (embedding pgvector support)
  - `summary.go`, `chapter.go`, `highlight.go`
  - `comment.go`, `chat_message.go`
  - `notification.go`, `report.go`
- [ ] **3.2. Repositories** in `internal/repositories/`:
  - Define interfaces in domain package and implement with GORM in `internal/repositories/`.
  - Vector repository with cosine distance query (`ORDER BY embedding <=> $1 LIMIT $2`).
- [ ] **3.3. Commit**: `feat(models): implement gorm domain entities with pgvector support`

---

### Phase 4: AI Provider Ports & Adapters (STT, LLM, RAG)
- [ ] **4.1. Speech-to-Text (STT) Port & Adapters**:
  - Interface `internal/services/stt.go`: `Transcribe(ctx, reader io.Reader, filename string, opts STTOptions) (*TranscriptionResult, error)`
  - Deepgram Nova-2 Adapter (`internal/adapters/stt/deepgram.go`)
  - Whisper / OpenAI Audio Adapter (`internal/adapters/stt/whisper.go`)
  - Mock STT Adapter (`internal/adapters/stt/mock.go`) for unit/offline testing
- [ ] **4.2. LLM Completion Port & Adapters**:
  - Interface `internal/services/llm.go`: `GenerateStructured(ctx, prompt string, schema map[string]interface{}) (*StructuredResponse, error)`
  - Support for 7 Category High-Density JSON Schemas
  - Support for Regenerate with Angle
  - OpenAI / Claude compatible adapter (`internal/adapters/llm/openai.go`)
- [ ] **4.3. Embedding & Vector RAG Port & Adapters**:
  - Interface `internal/services/embedding.go`: `CreateEmbeddings(ctx, chunks []string) ([][]float32, error)`
  - Semantic Chunking Engine (300 tokens + 50 overlap with time boundaries)
  - Grounded RAG Chat Engine (enforce exact citations `[MM:SS]` & zero hallucination fallback)
- [ ] **4.4. Commit**: `feat(ai): add stt, llm, and embedding adapters`

---

### Phase 5: Pipeline Workers & Asynchronous Orchestration
- [ ] **5.1. Audio Extraction Worker**:
  - FFmpeg extraction (video to 64kbps mono MP3/WAV, storage optimization).
- [ ] **5.2. Transcription Worker**:
  - Ingest audio from S3/MinIO, run STT, persist word-level karaoke data.
- [ ] **5.3. Parallel Fan-Out Workers**:
  - `SummarizerWorker` (execute active template prompt).
  - `IndexerWorker` (chunking + pgvector embedding).
  - `AnalyticsWorker` (talk-time per speaker, participation share).
- [ ] **5.4. Smart State Recovery**:
  - Retry without re-uploading file (re-use S3 audio and DB transcript).
- [ ] **5.5. Commit**: `feat(workers): implement asynchronous pipeline and fan-out orchestrator`

---

### Phase 6: Server-Sent Events (SSE) & Real-Time Streaming
- [ ] **6.1. Pipeline Progress SSE**:
  - Route: `GET /v1/recordings/:id/progress`
  - Stream pipeline stages: `VALIDATING` -> `EXTRACTING` -> `TRANSCRIBING` -> `SUMMARIZING` -> `INDEXING` -> `COMPLETED`.
- [ ] **6.2. Interactive Chat Typewriter SSE**:
  - Route: `POST /v1/recordings/:id/chat`
  - Stream AI response tokens in real-time with grounded citations.
- [ ] **6.3. Commit**: `feat(sse): add progress streaming and interactive chat typewriter`

---

### Phase 7: REST API Controllers & Declarative Routing
- [ ] **7.1. Route Declarations** in `internal/routes/routes.yaml`:
  - Public & Guest routes (`/v1/recordings/upload`, `quota`, `claim`, `share`)
  - Owner routes (`speakers`, `regenerate`, `comments`, `chat`, `export`)
  - Auth routes (`/v1/auth/*`)
  - Admin CMS routes (`/v1/admin/*` with RBAC permissions)
- [ ] **7.2. Controllers & DTOs**:
  - `internal/controllers/` and `internal/dtos/` with `ozzo-validation`.
- [ ] **7.3. Commit**: `feat(api): register restful endpoints in declarative router`

---

### Phase 8: Testing, OpenAPI Documentation & Verification
- [ ] **8.1. Unit & Integration Tests**:
  - Ensure `make test` runs with 100% pass rate.
- [ ] **8.2. OpenAPI 3.0 Documentation**:
  - Update `openapi.yaml` embedded via Scalar UI at `/docs`.
- [ ] **8.3. Commit**: `test(api): add unit and integration test coverage`

---

## 4. Current Work Log & Status Tracker

| Timestamp | Component | Action | Status | Notes |
| :--- | :--- | :--- | :---: | :--- |
| `2026-10-04` | Git Configuration | Added `PLANNING.md` to `.gitignore` | ✅ Completed | Commit `f99a4e2` |
| `2026-10-04` | Roadmap | Created master `PLANNING.md` | ✅ Completed | Non-committed local roadmap |
| *(Next)* | Docker & Config | Upgrade postgres to `pgvector/pgvector:pg16` | ⏳ Pending | Step 1.1 |
