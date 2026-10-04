# AGENTS.md — AI Developer Guide for `code-base-golang`

This document defines the architectural invariants, conventions, and workflows for AI coding assistants working in this repository.

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

### 2.2. Declarative Route Configuration
- Routes are declared in `internal/routes/routes.yaml`.
- The router maps handlers, HTTP methods, middlewares, and rate-limiting from this YAML file.
- Register any new endpoints in `routes.yaml` rather than manually calling `router.GET(...)` in Go code.

### 2.3. Database Migrations
- Migration files live in `migrations/` named `<timestamp>_<name>.up.sql` and `<timestamp>_<name>.down.sql`.
- When `AUTO_MIGRATE=true`, migrations apply automatically on application/container startup after database connectivity is confirmed.

---

## 3. Git Workflow & Commit Rules (MANDATORY)

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

---

## 4. Core Coding Conventions

1. **Standard Error Envelope**:
   - Use `pkg/apperror.AppError` for domain and HTTP errors.
   - Format all HTTP responses using the standardized envelope in `internal/dtos/response.go` (`status`, `code`, `message`, `data`, `errors`, `timestamp`).
2. **Context & Metadata Propagation**:
   - Always accept `context.Context` as the first parameter in repository, service, and external adapter methods.
   - Extract client metadata using `pkg/ctxmeta` (`RequestID`, `ClientIP`, `UserAgent`).
3. **Struct Validation**:
   - Implement `validation.Validatable` (`ozzo-validation`) on request DTOs in `internal/dtos/` with a `Validate()` method.
4. **Resource Safety**:
   - Always ensure connections, response bodies, and lock releases are deferred cleanly (`defer`).
   - Workers and async subscribers must handle graceful cancellation via `ctx.Done()`.

---

## 5. Development Workflows for Agents

### Adding a New Entity & CRUD Feature
1. **Migration**: Run `make migrate-create name=create_<entity>_table` and write `.up.sql` & `.down.sql`.
2. **Model**: Create entity struct embedding `models.BaseModel` in `internal/models/`.
3. **Repository**: Define interface and GORM queries in `internal/repositories/`.
4. **DTO & Validation**: Define payload structs in `internal/dtos/` with `Validate()` methods.
5. **Service**: Define interface & business implementation in `internal/services/`.
6. **Controller**: Add handler method in `internal/controllers/`.
7. **Routes**: Register path, HTTP method, and auth middlewares in `internal/routes/routes.yaml`.
8. **Unit Tests**: Add tests alongside code using Go's standard `testing` package.

### Adding an Asynchronous Event / Worker
1. Define event payload struct in `internal/payload/`.
2. Publish event via `services.EventPublisher.Publish(ctx, exchange, routingKey, body)`.
3. Register worker handler in `internal/workers/worker.go` using `w.sub.Subscribe(ctx, queue, exchange, routingKey, handler)`.

---

## 6. Development & Testing Commands

Always run tests to verify zero regressions before concluding tasks:

```bash
# Run all unit tests
make test
# or: go test -v ./...

# Build binary
make build

# Start infrastructure services (PostgreSQL, Redis, RabbitMQ, MinIO, Mailpit)
make docker-up

# Database migrations
make migrate-create name=<migration_name>
make migrate-up
make migrate-down
make migrate-status
```
