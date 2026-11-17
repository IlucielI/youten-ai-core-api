#!/usr/bin/env sh
set -eu

APP_VERSION="${APP_VERSION:-0.1.0}"
GIT_HASH="${GIT_HASH:-$(git rev-parse --short HEAD 2>/dev/null || printf dev)}"
BUILDER_BASE="${BUILDER_BASE:-code-base-golang-api-builder-base:latest}"
RUNTIME_BASE="${RUNTIME_BASE:-code-base-golang-api-runtime-base:latest}"

docker build \
  --build-arg BUILDER_BASE="$BUILDER_BASE" \
  --build-arg RUNTIME_BASE="$RUNTIME_BASE" \
  --build-arg APP_VERSION="$APP_VERSION" \
  --build-arg GIT_HASH="$GIT_HASH" \
  -f deployment/Dockerfile \
  -t code-base-golang-api:latest \
  .
