#!/usr/bin/env bash
set -euo pipefail

IMAGE="eliyip/rss-ai"
PLATFORM="linux/amd64"
TAG=""
PUSH=0

usage() {
  cat <<'EOF'
Usage: scripts/build-docker.sh [--tag TAG] [--image IMAGE] [--platform PLATFORM] [--push]

Cross-compile the rss-ai binary and build a Docker image from it:
  eliyip/rss-ai:latest
  eliyip/rss-ai:{TAG}

Options:
  --tag TAG            Image tag. Defaults to the exact current git tag.
  --image IMAGE        Image repository. Defaults to eliyip/rss-ai.
  --platform PLATFORM  Docker build platform. Defaults to linux/amd64.
                       Supported: linux/amd64, linux/arm64.
  --push               Push both tags after building.
  -h, --help           Show this help.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --tag)
      TAG="${2:?missing value for --tag}"
      shift 2
      ;;
    --image)
      IMAGE="${2:?missing value for --image}"
      shift 2
      ;;
    --platform)
      PLATFORM="${2:?missing value for --platform}"
      shift 2
      ;;
    --push)
      PUSH=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

if [[ -z "$TAG" ]]; then
  if ! TAG="$(git describe --tags --exact-match 2>/dev/null)"; then
    echo "No --tag provided and HEAD is not exactly on a git tag." >&2
    echo "Pass --tag TAG, for example: scripts/build-docker.sh --tag 1.0.0" >&2
    exit 1
  fi
fi

if [[ "$TAG" == "latest" ]]; then
  echo "TAG must not be 'latest'; it is reserved for the rolling image tag." >&2
  exit 1
fi

ROOT_DIR="$(git rev-parse --show-toplevel)"
GOOS="${PLATFORM%%/*}"
GOARCH="${PLATFORM#*/}"
GOARCH="${GOARCH%%/*}"
DIST_DIR="${ROOT_DIR}/dist/docker_${GOOS}_${GOARCH}"
BINARY="${DIST_DIR}/rss-ai"

cd "$ROOT_DIR"

if [[ "$GOOS" != "linux" ]]; then
  echo "Only linux images are supported (got GOOS=${GOOS})." >&2
  exit 1
fi
if [[ "$GOARCH" != "amd64" && "$GOARCH" != "arm64" ]]; then
  echo "Only amd64 and arm64 are supported (got GOARCH=${GOARCH})." >&2
  exit 1
fi

mkdir -p "$DIST_DIR"

echo "Building ${BINARY} for ${GOOS}/${GOARCH}..."
CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" GOPROXY=https://goproxy.cn,direct go build \
  -ldflags "-w -s" \
  -o "$BINARY" \
  ./cmd/rss-ai

echo "Building ${IMAGE}:${TAG} and ${IMAGE}:latest for ${PLATFORM}..."
docker build \
  --platform="$PLATFORM" \
  --tag "${IMAGE}:${TAG}" \
  --tag "${IMAGE}:latest" \
  --file docker/Dockerfile.goreleaser \
  "$DIST_DIR"

if [[ "$PUSH" -eq 1 ]]; then
  echo "Pushing ${IMAGE}:${TAG} and ${IMAGE}:latest..."
  docker push "${IMAGE}:${TAG}"
  docker push "${IMAGE}:latest"
fi

echo "Done:"
echo "  ${IMAGE}:${TAG}"
echo "  ${IMAGE}:latest"
