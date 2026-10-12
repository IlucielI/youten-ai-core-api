# AGENTS.md — AI Developer Guide for `code-base-golang`

This document defines the architectural invariants, clean code conventions, anti-patterns to avoid, and operational workflows for AI coding assistants working in this repository.

---

## 1. Tech Stack & Project Identity

- **Language**: Go 1.25+
- **HTTP Engine**: Gin (`github.com/gin-gonic/gin`)
- **Database / ORM**: PostgreSQL with GORM (`gorm.io/gorm`, `gorm.io/driver/postgres`)
- **Cache & KV**: Redis (`github.com/redis/go-redis/v9`)
- **Message Broker**: RabbitMQ (`github.com/rabbitmq/amqp091-go`) with topic/fanout routing & DLQ
- **Object Storage**: AWS S3 / MinIO (`github.com/aws/aws-sdk-go-v2`)
- **Email Delivery**: `net/smtp` + local Mailpit testing
- **Schema Migrations**: `github.com/golang-migrate/migrate/v4`
- **Validation**: `github.com/go-ozzo/ozzo-validation/v4`
- **Documentation**: OpenAPI 3.0 + Scalar UI (`/docs`, `/openapi.yaml`) embedded via `//go:embed`

---

## 2. Architectural Invariants

### 2.1. Clean Architecture & Decoupled Ports
- Pure business logic lives strictly in `internal/services/`.
- Services and workers **MUST NEVER** import or depend directly on concrete adapters (`internal/adapters/*`).
- Always consume idiomatic Go consumer interfaces defined in domain packages:
  - `services.FileStorage` (`internal/services/storage.go`)
  - `services.EventPublisher` (`internal/services/publisher.go`)
  - `services.EmailSender` (`internal/services/mailer.go`)
  - `workers.EventSubscriber` (`internal/workers/subscriber.go`)
- Concrete adapters (`database`, `redis`, `s3`, `rabbitmq`, `smtp`) live in `internal/adapters/` and are injected in `cmd/api/main.go`.

### 2.2. Declarative Route Configuration & Permissions
- Routes are declared in `internal/routes/routes.yaml`.
- The router maps handlers, HTTP methods, middlewares, and rate-limiting from this YAML file.
- **Permission & Scope Control**:
  - Secure routes specify auth middlewares in `routes.yaml`: `jwt`, `jwt_scope`, `auth_optional`.
  - Use centralized scope constants from `internal/constants/scope.go` (`ScopeRecordingsRead`, `ScopeAdminUsersWrite`, etc.).
  - Register any new endpoints in `routes.yaml` rather than imperatively calling `router.GET(...)` in Go code.

### 2.3. Separation of Layer Responsibilities
- **Controllers (`internal/controllers/`)**:
  - Parse HTTP request payloads (JSON body, query parameters, URI path variables).
  - Invoke DTO validation (`req.Validate()`) and propagate client context metadata (`pkg/ctxmeta`).
  - Delegate execution to domain services.
  - Render standardized JSON envelopes. **Zero business logic** belongs in controllers.
- **Services (`internal/services/`)**:
  - Pure domain orchestration, authorization checks, quota verification, transactions, and event dispatching.
  - Consume repository ports and infrastructure interfaces.
- **Repositories (`internal/repositories/`)**:
  - Database access and GORM query execution.
  - Encapsulate query filters in composable GORM scopes.
  - Do NOT replicate input sanitization or bounds validation that has already been validated by DTOs.
- **DTOs & Validations (`internal/dtos/`)**:
  - Request DTOs define input schemas, pagination defaults (`SetDefaults()`), and validation rules implementing `validation.Validatable` (`ozzo-validation`).
- **Queue Payloads (`internal/queue/`)**:
  - Strongly-typed JSON message contracts for asynchronous RabbitMQ event streaming and background workers.

### 2.4. Standard Response Envelope & Error Handling
- Use `pkg/apperror.AppError` and predefined domain errors in `internal/constants/error.go`.
- Format all HTTP responses using the standardized envelope in `internal/dtos/response.go` (`status`, `code`, `message`, `data`, `errors`, `timestamp`).
- Never leak raw SQL, database driver errors, or internal stack traces to HTTP responses in production.

### 2.5. Database Migrations
- Migration files live in `migrations/` named `<timestamp>_<name>.up.sql` and `<timestamp>_<name>.down.sql`.
- When `AUTO_MIGRATE=true`, migrations apply automatically on application/container startup after database connectivity is confirmed.

---

## 3. Clean Code Rules & Anti-Patterns to Avoid (MANDATORY)

### 3.1. Strictly Forbidden: Deep Nested `if` Pyramids ("Pyramid of Doom")
- **NEVER** write nested `if` statements deeper than 2 levels.
- Always use **early-return guard clauses** to handle edge cases, nil checks, and preconditions up front.
- Extract complex multi-step validations or state synchronizations into small, cohesive private helpers:
  ```go
  // BAD: Deeply nested pyramid of doom
  if session.ExternalSessionID != nil && session.Status == constants.BotSessionStatusRecording {
      if provider, exists := s.BotProvider(session.Provider); exists && provider.IsEnabled() {
          if liveStatus, err := provider.GetStatus(ctx, *session.ExternalSessionID); err == nil && liveStatus != nil {
              if liveStatus.Status != session.Status {
                  // deeply nested mutation
              }
          }
      }
  }

  // GOOD: Flat helper with guard clauses
  func (s *Service) syncBotSessionLiveStatus(ctx context.Context, session *models.BotSession) error {
      if session.ExternalSessionID == nil || session.Status != constants.BotSessionStatusRecording {
          return nil
      }
      provider, exists := s.BotProvider(session.Provider)
      if !exists || !provider.IsEnabled() {
          return nil
      }
      liveStatus, err := provider.GetStatus(ctx, *session.ExternalSessionID)
      if err != nil || liveStatus == nil || liveStatus.Status == session.Status {
          return nil
      }
      // single focused update...
      return s.repo.UpdateBotSessionStatus(ctx, session.ID, liveStatus.Status, nil, liveStatus.StartedAt, liveStatus.EndedAt)
  }
  ```

### 3.2. Strictly Forbidden: Magic Strings & Primitives Obsession
- **NEVER** scatter raw string literals across services, controllers, or repositories for:
  - Entity statuses (`BotSessionStatusRecording`, `RecordingStatusCompleted`)
  - Webhook event types (`BotWebhookEventWaitingAdmit`, `BotWebhookEventCompleted`)
  - Capability statuses (`CapabilityStatusAvailable`, `CapabilityStatusComingSoon`)
  - Auth scopes (`ScopeRecordingsRead`, `ScopeAdminUsersWrite`)
  - Error codes (`ErrCodeInvalidProvider`, `ErrCodeSessionNotFound`)
- All system constants **MUST** be defined in `internal/constants/` and consumed directly.

### 3.3. Clean GORM Query Scoping
- **Avoid Inline `if` Cascades**: Do NOT write chains of `if filter.Field != ""` appending `.Where(...)` directly inside repository methods.
- Encapsulate query conditions into reusable GORM Scopes:
  ```go
  // GOOD: Clean GORM Scope
  func (f AdminRecordingsFilter) scope(db *gorm.DB) *gorm.DB {
      if f.Status != "" {
          db = db.Where("status = ?", f.Status)
      }
      if f.Search != "" {
          clean := "%" + strings.TrimSpace(f.Search) + "%"
          db = db.Where("title ILIKE ? OR original_filename ILIKE ?", clean, clean)
      }
      return db
  }
  ```

### 3.4. Clean Partial Updates Without `if` Cascades
- When building `map[string]interface{}` updates for optional fields, use generic helper utilities (such as `setIfNotNull` in `internal/repositories/repositories.go`) rather than 10 consecutive `if field != nil` blocks:
  ```go
  // GOOD: Declarative updates map
  updates := make(map[string]interface{})
  setIfNotNull(updates, "status", status)
  setIfNotNull(updates, "started_at", startedAt)
  setIfNotNull(updates, "ended_at", endedAt)
  ```

### 3.5. No Redundant Validation or Pagination Bounds Checks in Repositories
- Pagination limits (`Limit <= 0`, `Limit > 100`, `Offset < 0`) and input sanitization belong exclusively in DTO `SetDefaults()` and `Validate()`.
- Repositories must trust validated DTO query parameters and not duplicate boundary checks.

### 3.6. DRY Webhook & Provider Handling
- When handling callbacks from multiple external providers (Google Meet, MS Teams, Zoom), do NOT duplicate identical database update flows across three separate 60-line methods.
- Unify event mapping and database persistence into shared service helpers (e.g., `processBotWebhook`).

---

## 4. Core Coding Conventions

1. **Context & Metadata Propagation**:
   - Always accept `context.Context` as the first parameter in repository, service, and external adapter methods.
   - Extract client metadata using `pkg/ctxmeta` (`RequestID`, `ClientIP`, `UserAgent`, `GetAuthUserID`, `GetAuthScopes`).
2. **Struct Validation**:
   - Implement `validation.Validatable` (`ozzo-validation`) on request DTOs in `internal/dtos/` with a `Validate()` method.
   - Provide `SetDefaults()` on list/query DTOs for pagination fallback values.
3. **Resource Safety**:
   - Always ensure connections, response bodies, and lock releases are deferred cleanly (`defer`).
   - Workers and async subscribers must handle graceful cancellation via `ctx.Done()`.

---

## 5. Development Workflows for Agents

### Adding a New Entity & CRUD Feature
1. **Migration**: Run `make migrate-create name=create_<entity>_table` and write `.up.sql` & `.down.sql`.
2. **Model**: Create entity struct embedding `models.BaseModel` in `internal/models/`.
3. **Repository**: Define interface and GORM queries in `internal/repositories/`. Use GORM scopes for filtering.
4. **DTO & Validation**: Define payload structs in `internal/dtos/` with `SetDefaults()` and `Validate()` methods.
5. **Service**: Define interface & business implementation in `internal/services/`.
6. **Controller**: Add handler method in `internal/controllers/` rendering standardized response envelopes.
7. **Routes**: Register path, HTTP method, and auth middlewares in `internal/routes/routes.yaml`.
8. **Unit Tests**: Add tests alongside code using Go's standard `testing` package.

### Adding an Asynchronous Event / Worker
1. Define queue payload struct in `internal/queue/`.
2. Publish event via `services.EventPublisher.Publish(ctx, exchange, routingKey, body)`.
3. Register worker handler in `internal/workers/worker.go` using `w.sub.Subscribe(ctx, queue, exchange, routingKey, handler)`.

---

## 6. Git Workflow & Commit Rules (MANDATORY)

1. **NEVER PUSH DIRECTLY TO `main`**:
   - Pushing directly to the `main` branch is **STRICTLY PROHIBITED**.
   - All changes must go through a dedicated branch and Pull Request (PR).
2. **Feature Branching**:
   - Always create a dedicated branch off `main` before making any code or schema changes:
     - `feat/<feature-name>` for new features
     - `fix/<issue-name>` for bug fixes
     - `chore/<task-name>` for configuration, docs, or maintenance
     - `refactor/<target>` for code restructuring
3. **Atomic Commits**:
   - Commits must be small, focused, and scoped to a single logical change.
   - **NEVER** combine multiple unrelated changes in a single commit.
   - Follow Conventional Commits: `feat(...)`, `fix(...)`, `refactor(...)`, `build(...)`, `chore(...)`, `test(...)`.
4. **Pull Request (PR) Mandate**:
   - Push feature branch to origin: `git push -u origin <branch-name>`.
   - Open a PR to `main` using `gh pr create --base main --head <branch-name> --title "..." --body "..."`.
   - Verify `make test` and `make build` pass with zero regressions.
   - **MERGE REQUIRES USER APPROVAL**: Leave the PR **OPEN** after creation. The AI assistant must **NEVER** run `gh pr merge` autonomously. The assistant is allowed to merge only after asking for and receiving explicit approval from the USER.

---

## 7. Development & Testing Commands

Always run tests to verify zero regressions before concluding tasks:

```bash
# 1. Run all unit tests
make test
# or: go test -v ./...

# 2. Build binary
make build

# 3. Start infrastructure services (PostgreSQL, Redis, RabbitMQ, MinIO, Mailpit)
make docker-up

# 4. Database migrations
make migrate-create name=<migration_name>
make migrate-up
make migrate-down
make migrate-status
```
