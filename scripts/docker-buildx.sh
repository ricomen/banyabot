#!/bin/sh
set -eu
# Build amd64 + arm64. Use --push with a registry tag instead of --load for multi-arch.
IMAGE="${IMAGE:-instabot:latest}"
docker buildx build --platform linux/amd64,linux/arm64 -t "$IMAGE" "$@" .
