#!/bin/sh
set -eu

GO_BIN="${GO_BIN:-go}"
OUT="${RVS_DOWNLOAD_DIR:-downloads}/rvsctl"
mkdir -p "$OUT"

for target in darwin/arm64 darwin/amd64 linux/arm64 linux/amd64 windows/arm64 windows/amd64; do
  os=${target%/*}
  arch=${target#*/}
  dir="$OUT/$os/$arch"
  name=rvsctl
  [ "$os" = windows ] && name=rvsctl.exe
  mkdir -p "$dir"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" "$GO_BIN" build -trimpath -ldflags="-s -w" -o "$dir/$name" ./cmd/rvsctl
done

(
  cd "$OUT"
  find . -type f \( -name rvsctl -o -name rvsctl.exe \) -print | sort | while IFS= read -r file; do
    if command -v sha256sum >/dev/null 2>&1; then
      sha256sum "$file"
    else
      shasum -a 256 "$file"
    fi
  done > SHA256SUMS
)

echo "rvsctl release files written to $OUT"
