#!/usr/bin/env bash
# Build the bot Docker image for the current platform (no push).
# Usage: ./scripts/build-image.sh [version]
#   version  Optional (default: dev). Used for image tag and binary version info.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$ROOT_DIR"

VERSION="${1:-dev}"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo "none")"
IMAGE_NAME="${IMAGE_NAME:-n19-shop-bot}"

echo "Building $IMAGE_NAME:$VERSION (commit: $COMMIT)"
docker build \
  --build-arg VERSION="$VERSION" \
  --build-arg COMMIT="$COMMIT" \
  -t "$IMAGE_NAME:$VERSION" \
  -t "$IMAGE_NAME:latest" \
  .

echo "Done. Image: $IMAGE_NAME:$VERSION (and :latest)"
echo "Run with: docker compose up -d   (from project root with .env configured)"
