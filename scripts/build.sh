#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Cross-compile pharos-awg for FreeBSD/OPNsense from the Mac.
#   scripts/build.sh [GOARCH]   (default: amd64, matching OPNsense 25.7 FreeBSD:14:amd64)
set -euo pipefail
cd "$(dirname "$0")/.."
arch="${1:-amd64}"
ver="$(tr -d '[:space:]' < VERSION 2>/dev/null || echo dev)"
out="bin/freebsd-${arch}/pharos-awg"
mkdir -p "$(dirname "$out")"
GOOS=freebsd GOARCH="$arch" CGO_ENABLED=0 \
	go build -trimpath -ldflags "-s -w -X main.version=${ver}" -o "$out" ./cmd/pharos-awg
echo "built $out ($(file -b "$out" 2>/dev/null || echo freebsd/$arch))"
