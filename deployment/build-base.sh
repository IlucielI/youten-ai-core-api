#!/usr/bin/env sh
set -eu

echo "==> Building builder base image (Go modules)..."
docker build \
  -f deployment/Dockerfile.builder-base \
  -t code-base-golang-api-builder-base:latest \
  .

echo "==> Building runtime base image (ffmpeg, yt-dlp, python3)..."
docker build \
  -f deployment/Dockerfile.runtime-base \
  -t code-base-golang-api-runtime-base:latest \
  .
