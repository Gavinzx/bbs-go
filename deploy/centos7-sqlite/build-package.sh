#!/bin/sh
set -eu

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
PACKAGE_DIR="$ROOT_DIR/dist/bbs-go-centos7-sqlite-offline"
IMAGE="${BBSGO_IMAGE:-bbs-go:centos7-sqlite}"
PLATFORM="${DOCKER_DEFAULT_PLATFORM:-linux/amd64}"
RUNTIME_DIR="${RUNTIME_DIR:-$ROOT_DIR/.devhome/offline-runtime}"
GOPROXY_VALUE="${GOPROXY:-https://proxy.golang.org,direct}"

cd "$ROOT_DIR"

echo "Building $IMAGE for $PLATFORM"
docker build --pull=false --platform "$PLATFORM" \
  --build-arg "GOPROXY=$GOPROXY_VALUE" \
  -f deploy/centos7-sqlite/Dockerfile \
  -t "$IMAGE" .

rm -rf "$PACKAGE_DIR"
mkdir -p "$PACKAGE_DIR/images" "$PACKAGE_DIR/config" "$PACKAGE_DIR/manual" "$PACKAGE_DIR/docker-runtime"

echo "Saving application image"
docker save -o "$PACKAGE_DIR/images/bbs-go-centos7-sqlite.tar" "$IMAGE"
chmod 644 "$PACKAGE_DIR/images/bbs-go-centos7-sqlite.tar"

cp deploy/centos7-sqlite/docker-compose.yaml "$PACKAGE_DIR/docker-compose.yaml"
cp deploy/centos7-sqlite/config/bbs-go.yaml "$PACKAGE_DIR/config/bbs-go.yaml"
cp deploy/centos7-sqlite/entrypoint.sh "$PACKAGE_DIR/entrypoint.sh"
cp deploy/centos7-sqlite/load-images.sh "$PACKAGE_DIR/load-images.sh"
cp deploy/centos7-sqlite/deploy.sh "$PACKAGE_DIR/deploy.sh"
cp deploy/centos7-sqlite/install-docker.sh "$PACKAGE_DIR/install-docker.sh"

if [ -f "$RUNTIME_DIR/docker-20.10.24.tgz" ] && [ -f "$RUNTIME_DIR/docker-compose-linux-x86_64" ]; then
  cp "$RUNTIME_DIR/docker-20.10.24.tgz" "$PACKAGE_DIR/docker-runtime/docker-20.10.24.tgz"
  cp "$RUNTIME_DIR/docker-compose-linux-x86_64" "$PACKAGE_DIR/docker-runtime/docker-compose-linux-x86_64"
  echo "Bundled Docker runtime included."
else
  echo "WARNING: Docker runtime not found under $RUNTIME_DIR; package will require Docker preinstalled." >&2
fi

if [ -f deploy/centos7-sqlite/manual/部署手册.md ]; then
  cp deploy/centos7-sqlite/manual/部署手册.md "$PACKAGE_DIR/manual/部署手册.md"
fi
if [ -f deploy/centos7-sqlite/manual/部署手册.pdf ]; then
  cp deploy/centos7-sqlite/manual/部署手册.pdf "$PACKAGE_DIR/manual/部署手册.pdf"
fi
if [ -f deploy/centos7-sqlite/manual/部署手册.docx ]; then
  cp deploy/centos7-sqlite/manual/部署手册.docx "$PACKAGE_DIR/manual/部署手册.docx"
fi
cp deploy/centos7-sqlite/README.md "$PACKAGE_DIR/README.md"

# 校验和（不包含自身）
(
  cd "$PACKAGE_DIR"
  find . -type f ! -name SHA256SUMS -print0 | sort -z | xargs -0 sha256sum > SHA256SUMS
)

echo "Creating tarball"
tar czf "$ROOT_DIR/dist/bbs-go-centos7-sqlite-offline.tar.gz" -C "$ROOT_DIR/dist" bbs-go-centos7-sqlite-offline

echo "Package: $ROOT_DIR/dist/bbs-go-centos7-sqlite-offline.tar.gz"
du -sh "$PACKAGE_DIR" "$ROOT_DIR/dist/bbs-go-centos7-sqlite-offline.tar.gz"
