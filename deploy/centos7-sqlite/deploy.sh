#!/bin/sh
set -eu

cd "$(dirname "$0")"

if ! command -v docker >/dev/null 2>&1; then
  if [ -x ./install-docker.sh ]; then
    echo "Docker not found; installing bundled Docker runtime..."
    ./install-docker.sh
  else
    echo "Docker is not installed and install-docker.sh is missing." >&2
    exit 1
  fi
fi

if ! docker compose version >/dev/null 2>&1; then
  if [ -x ./install-docker.sh ]; then
    echo "Docker Compose plugin not found; installing bundled plugin..."
    ./install-docker.sh
  else
    echo "Docker Compose plugin is not available." >&2
    exit 1
  fi
fi

./load-images.sh
mkdir -p docker-data/data docker-data/logs docker-data/uploads

docker compose -f docker-compose.yaml up -d

echo "bbs-go is starting."
echo "Open http://127.0.0.1:${BBSGO_WEB_PORT:-8082}"
echo "Data directory: $(pwd)/docker-data"
