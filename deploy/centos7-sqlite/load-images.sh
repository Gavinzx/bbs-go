#!/bin/sh
set -eu

cd "$(dirname "$0")"

if [ ! -d images ]; then
  echo "images directory not found" >&2
  exit 1
fi

found=0
for image_tar in images/*.tar; do
  [ -f "$image_tar" ] || continue
  found=1
  echo "Loading $image_tar"
  docker load -i "$image_tar"
done

if [ "$found" -eq 0 ]; then
  echo "no image tar files found in images/" >&2
  exit 1
fi
