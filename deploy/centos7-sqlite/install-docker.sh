#!/bin/sh
set -eu

cd "$(dirname "$0")"

if [ "$(id -u)" -ne 0 ]; then
  echo "install-docker.sh must be run as root." >&2
  exit 1
fi

if command -v docker >/dev/null 2>&1; then
  echo "Docker is already installed: $(docker --version 2>/dev/null || true)"
  if ! docker compose version >/dev/null 2>&1; then
    echo "Installing bundled Docker Compose plugin only."
    mkdir -p /usr/local/lib/docker/cli-plugins
    if [ -f docker-runtime/docker-compose-linux-x86_64 ]; then
      install -m 0755 docker-runtime/docker-compose-linux-x86_64 /usr/local/lib/docker/cli-plugins/docker-compose
      echo "Docker Compose plugin installed."
    fi
  fi
  exit 0
fi

if [ ! -f docker-runtime/docker-20.10.24.tgz ]; then
  echo "docker-runtime/docker-20.10.24.tgz not found." >&2
  echo "Install Docker 20.10+ manually, or include the runtime directory in the package." >&2
  exit 1
fi

echo "Installing bundled Docker 20.10.24 static runtime..."
tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT
tar -xzf docker-runtime/docker-20.10.24.tgz -C "$tmp_dir"
install -m 0755 "$tmp_dir"/docker/* /usr/local/bin/

if [ -f docker-runtime/docker-compose-linux-x86_64 ]; then
  mkdir -p /usr/local/lib/docker/cli-plugins
  install -m 0755 docker-runtime/docker-compose-linux-x86_64 /usr/local/lib/docker/cli-plugins/docker-compose
fi

cat > /etc/systemd/system/docker.service <<'UNIT'
[Unit]
Description=Docker Application Container Engine
Documentation=https://docs.docker.com
After=network-online.target firewalld.service
Wants=network-online.target

[Service]
Type=notify
ExecStart=/usr/local/bin/dockerd --host=unix:///var/run/docker.sock --storage-driver=overlay2
ExecReload=/bin/kill -s HUP $MAINPID
LimitNOFILE=infinity
LimitNPROC=infinity
LimitCORE=infinity
TasksMax=infinity
TimeoutStartSec=0
Delegate=yes
KillMode=process
Restart=on-failure
StartLimitBurst=3
StartLimitInterval=60s

[Install]
WantedBy=multi-user.target
UNIT

systemctl daemon-reload
systemctl enable docker
systemctl start docker
sleep 2

if ! docker info >/dev/null 2>&1; then
  echo "Docker failed to start. Check: journalctl -u docker -n 100" >&2
  echo "CentOS 7 may need a different storage driver; try --storage-driver=vfs in /etc/systemd/system/docker.service." >&2
  exit 1
fi

docker version
docker compose version
echo "Docker runtime installed."
