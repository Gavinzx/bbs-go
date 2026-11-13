#!/bin/sh
set -eu

mkdir -p /app/data /app/logs /app/res/uploads
if [ ! -f /app/data/bbs-go.yaml ]; then
	cp /app/defaults/bbs-go.yaml /app/data/bbs-go.yaml
fi

exec ./bbs-go
