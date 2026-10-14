#!/usr/bin/env sh
set -eu

APP_VERSION="${APP_VERSION:-0.1.0}"
GIT_HASH="${GIT_HASH:-$(git rev-parse --short HEAD 2>/dev/null || printf dev)}"
IMAGE_TAG="${IMAGE_TAG:-code-base-golang-api:latest}"
PLATFORMS="${PLATFORMS:-linux/amd64,linux/arm64}"
ACTION="${ACTION:---load}"
BUILDER="${BUILDER:-default}"
REBUILD_BASE="${REBUILD_BASE:-false}"

BUILDER_BASE="${BUILDER_BASE:-code-base-golang-api-builder-base:latest}"
RUNTIME_BASE="${RUNTIME_BASE:-code-base-golang-api-runtime-base:latest}"

# 1. Build multi-arch builder base image if missing or forced
if [ "$REBUILD_BASE" = "true" ] || ! docker image inspect "$BUILDER_BASE" >/dev/null 2>&1; then
  echo "==> Building multi-arch builder base image (${PLATFORMS}) with tag: ${BUILDER_BASE}..."
  docker buildx build \
    --builder "$BUILDER" \
    --platform "$PLATFORMS" \
    $ACTION \
    -f deployment/Dockerfile.builder-base \
    -t "$BUILDER_BASE" \
    .
else
  echo "==> Builder base image '$BUILDER_BASE' already exists. Skipping."
fi

# 2. Build multi-arch runtime base image if missing or forced
if [ "$REBUILD_BASE" = "true" ] || ! docker image inspect "$RUNTIME_BASE" >/dev/null 2>&1; then
  echo "==> Building multi-arch runtime base image (${PLATFORMS}) with tag: ${RUNTIME_BASE}..."
  docker buildx build \
    --builder "$BUILDER" \
    --platform "$PLATFORMS" \
    $ACTION \
    -f deployment/Dockerfile.runtime-base \
    -t "$RUNTIME_BASE" \
    .
else
  echo "==> Runtime base image '$RUNTIME_BASE' already exists. Skipping."
fi

# 3. Build multi-arch application image
echo "==> Building multi-arch application image (${PLATFORMS}) with tag: ${IMAGE_TAG}..."

docker buildx build \
  --builder "$BUILDER" \
  --platform "$PLATFORMS" \
  --build-arg BUILDER_BASE="$BUILDER_BASE" \
  --build-arg RUNTIME_BASE="$RUNTIME_BASE" \
  --build-arg APP_VERSION="$APP_VERSION" \
  --build-arg GIT_HASH="$GIT_HASH" \
  $ACTION \
  -f deployment/Dockerfile \
  -t "$IMAGE_TAG" \
  .

echo "==> Successfully built ${IMAGE_TAG} for ${PLATFORMS}"
