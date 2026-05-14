#!/bin/sh
# build/cross.sh — lthn-cross entrypoint override for the Lethean
# go/ subfolder layout. (Image name renamed from the wails3 scaffold
# default `wails-cross` so multiple wails3 projects can coexist.)
#
# The default /usr/local/bin/build.sh in the cross image runs
# `go build .` from the repo root, assuming main.go sits there. Our
# Go module is at go/, with main at go/cmd/lthn. This script does
# the dance:
#
#   1. Set CC/GOOS/GOARCH for the target triple (matches build.sh)
#   2. cd go && go build -o ../bin/<app>-<os>-<arch>[.exe] ./cmd/lthn
#   3. Skip strip flags (-w -s) — they conflict with DuckDB Arrow
#      via go-store's cgo chain (same reason we dropped them in
#      build/darwin/Taskfile.yml).
#
# Invoked via:
#   docker run ... --entrypoint /app/build/cross.sh lthn-cross <os> <arch>
set -e

OS=${1:-linux}
ARCH=${2:-amd64}

case "${OS}-${ARCH}" in
    darwin-arm64|darwin-aarch64)   CC=zcc-darwin-arm64;                     GOARCH=arm64; GOOS=darwin  ;;
    darwin-amd64|darwin-x86_64)    CC=zcc-darwin-amd64;                     GOARCH=amd64; GOOS=darwin  ;;
    # Linux cross-compile: native gcc inside the lthn-cross image.
    # The image's gcc is whichever arch the image was built on
    # (aarch64 if built on Apple Silicon, x86_64 if built on Intel
    # macOS / amd64 Linux). Targeting a different Linux arch requires
    # either rebuilding the image for that arch OR using
    # `docker run --platform linux/<arch>` to QEMU-emulate.
    # Zig cross would be cleaner but its bundled libc headers clash
    # with GTK/WebKit system headers (__COLD attribute mismatch).
    linux-arm64|linux-aarch64)     CC=gcc;  GOARCH=arm64; GOOS=linux   ;;
    linux-amd64|linux-x86_64)      CC=gcc;  GOARCH=amd64; GOOS=linux   ;;
    windows-arm64|windows-aarch64) CC=zcc-windows-arm64;                    GOARCH=arm64; GOOS=windows ;;
    windows-amd64|windows-x86_64)  CC=zcc-windows-amd64;                    GOARCH=amd64; GOOS=windows ;;
    *)
        echo "Usage: <os> <arch>" >&2
        echo "  os: darwin | linux | windows" >&2
        echo "  arch: amd64 | arm64" >&2
        exit 1
        ;;
esac

export CC GOARCH GOOS
export CGO_ENABLED=1
export CGO_CFLAGS="-w"

APP=${APP_NAME:-lthn}
EXT=""
LDFLAGS=""
if [ "$GOOS" = "windows" ]; then
    EXT=".exe"
    LDFLAGS="-H windowsgui"
fi

TAGS="production"
if [ -n "$EXTRA_TAGS" ]; then
    TAGS="${TAGS},${EXTRA_TAGS}"
fi

mkdir -p bin
cd go
go build \
    -tags "${TAGS}" \
    -trimpath \
    -buildvcs=false \
    ${LDFLAGS:+-ldflags="${LDFLAGS}"} \
    -o "../bin/${APP}-${GOOS}-${GOARCH}${EXT}" \
    ./cmd/lthn

echo "Built: bin/${APP}-${GOOS}-${GOARCH}${EXT}"
