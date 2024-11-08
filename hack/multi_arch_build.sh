#!/usr/bin/env bash
set -e

platforms=("linux/amd64" "linux/arm64" "darwin/amd64" "darwin/arm64")
for platform in "${platforms[@]}"; do
  GOOS="${platform%/*}"
  GOARCH="${platform#*/}"
  echo -n "Building for $GOOS/$GOARCH...   "
  GOOS=$GOOS GOARCH=$GOARCH make build-binary-pikactl &>/dev/null
  mv bin/pikactl "bin/pikactl_${GOOS}_${GOARCH}"
  echo "done"
done
