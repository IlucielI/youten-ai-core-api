#!/usr/bin/env sh
set -eu

BUILDER_BASE="code-base-golang-api-builder-base:latest"
RUNTIME_BASE="code-base-golang-api-runtime-base:latest"
REBUILD_BASE="${REBUILD_BASE:-false}"

# Check if base images exist or if forced rebuild is requested
if [ "$REBUILD_BASE" = "true" ] || ! docker image inspect "$BUILDER_BASE" >/dev/null 2>&1 || ! docker image inspect "$RUNTIME_BASE" >/dev/null 2>&1; then
  echo "==> Base images not found (or REBUILD_BASE=true). Building base images..."
  ./deployment/build-base.sh
else
  echo "==> Base images exist. Skipping base build."
fi

echo "==> Building application image..."
./deployment/build-api.sh
