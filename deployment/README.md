# Deployment Guide (`code-base-golang`)

This directory contains containerization files and Docker Compose configurations optimized for rapid local development and production deployments.

---

## Table of Contents

- [Overview & Architecture](#overview--architecture)
- [Two-Stage Docker Build Strategy](#two-stage-docker-build-strategy)
- [Infrastructure Services & Ports](#infrastructure-services--ports)
- [Quick Start](#quick-start)
- [Database Auto-Migration in Docker](#database-auto-migration-in-docker)
- [Running Migrations Inside the Container](#running-migrations-inside-the-container)
- [Build Scripts Reference](#build-scripts-reference)

---

## Overview & Architecture

The container workflow is optimized using a **two-step Docker build strategy** that decouples heavy Go dependency downloading (`go.mod`/`go.sum`) from rapid application source code compilation:

```
[ go.mod / go.sum ] ────────► Dockerfile.builder-base ──► code-base-golang-api-builder-base:latest
                                                                    │
[ App Source Code ] ────────► Dockerfile (builder)    ◄─────────────┘
                                    │
                              /out/api binary
                                    │
[ Alpine + yt-dlp + ffmpeg ] ► Dockerfile.runtime-base ──► code-base-golang-api-runtime-base:latest
                                                                    │
                              Dockerfile (runtime)    ◄─────────────┘
                                    │
                                    ▼
                      code-base-golang-api:latest
```

---

## Two Base Images Docker Build Strategy

1. **Builder Base Image (`Dockerfile.builder-base`)**:
   - Downloads and pre-compiles all external Go modules into module cache (`golang:1.26-alpine`).
   - **Only rebuild** when `go.mod` or `go.sum` changes.
2. **Runtime Base Image (`Dockerfile.runtime-base`)**:
   - Pre-installs `ffmpeg`, `python3`, `quickjs`, `yt-dlp`, and configures non-root `app` user (`alpine:3.20`).
   - **Only rebuild** when external media/audio runtime tooling changes.
3. **Application Image (`Dockerfile`)**:
   - Uses `BUILDER_BASE` with native cross-compilation (`--platform=$BUILDPLATFORM`) to compile the binary in seconds without QEMU emulation.
   - Uses `RUNTIME_BASE` to package the compiled binary and SQL migrations into an ultra-fast runtime container.

---

## Infrastructure Services & Ports

When spinning up `docker compose -f deployment/docker-compose.yaml up -d`, the following services are orchestrated:

| Service | Container Name | Port | Description | Healthcheck |
|---------|----------------|------|-------------|-------------|
| **API** | `code-base-golang-api` | `8080` | Golang HTTP REST API Server | Depends on healthy dependencies |
| **PostgreSQL** | `code-base-golang-postgres` | `5432` | Relational database (v16 Alpine) | `pg_isready` probe |
| **Redis** | `code-base-golang-redis` | `6379` | In-memory cache & KV store (v7 Alpine) | `redis-cli ping` probe |
| **RabbitMQ** | `code-base-golang-rabbitmq` | `5672`, `15672` | AMQP broker & Management Dashboard (`guest:guest`) | `rabbitmq-diagnostics ping` |
| **MinIO** | `code-base-golang-minio` | `9000`, `9001` | S3 Object Storage & Web Console (`minioadmin:minioadmin`) | `mc ready local` |
| **Mailpit** | `code-base-golang-mailpit` | `1025`, `8025` | Local SMTP (`1025`) & Webmail UI (`http://localhost:8025`) | Built-in |

---

## Quick Start

### 1. Build and Run via Unified Script
Builds the base image if missing, builds the API image, and launches the entire stack:
```bash
./deployment/build.sh
docker compose -f deployment/docker-compose.yaml up -d
```

### 2. Run Step-by-Step
If you prefer explicit control:
```bash
# Step 1: Build dependency base image
./deployment/build-base.sh

# Step 2: Build API application image
./deployment/build-api.sh

# Step 3: Start all containers in background
docker compose -f deployment/docker-compose.yaml up -d
```

### 3. Custom Versioning
You can pass custom version metadata:
```bash
APP_VERSION=1.0.0 ./deployment/build-api.sh
```
*Note: `GIT_HASH` is automatically extracted from `git rev-parse --short HEAD` (or defaults to `dev`).*

---

## Database Auto-Migration in Docker

When the `api` container boots up, it automatically inspects the `AUTO_MIGRATE` environment variable:

- **When `AUTO_MIGRATE=true`**:
  The API automatically runs all pending SQL files from `/app/migrations` before accepting incoming HTTP traffic.
- **Dependency Guard**:
  In `docker-compose.yaml`, the `api` service defines:
  ```yaml
  depends_on:
    postgres:
      condition: service_healthy
  ```
  This ensures that PostgreSQL is completely initialized and accepting connections before the API begins the migration process.

---

## Running Migrations Inside the Container

If you prefer to run database migrations manually inside the running container, execute the single binary CLI:

```bash
# View migration status and current schema version
docker compose -f deployment/docker-compose.yaml exec api /app/api migrate status

# Apply all pending migrations
docker compose -f deployment/docker-compose.yaml exec api /app/api migrate up

# Apply next N migrations
docker compose -f deployment/docker-compose.yaml exec api /app/api migrate up 1

# Rollback 1 migration step
docker compose -f deployment/docker-compose.yaml exec api /app/api migrate down 1

# Force reset dirty migration state
docker compose -f deployment/docker-compose.yaml exec api /app/api migrate force 20260928120000
```

---

## Build Scripts Reference

| Script | Purpose |
|--------|---------|
| `build-base.sh` | Builds both the Go builder base (`Dockerfile.builder-base`) and runtime base (`Dockerfile.runtime-base`). |
| `build-api.sh` | Compiles source code against base image and outputs runtime Docker image (`Dockerfile`). |
| `build.sh` | Smart build runner: automatically builds base image if not found, then compiles API. |
