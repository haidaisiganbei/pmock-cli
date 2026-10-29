#!/usr/bin/env bash
# Build release archives locally without GoReleaser.
#
#   VERSION=0.1.0 ./scripts/package.sh
#
# Produces dist/pmock-cli_<version>_<os>_<arch>.tar.gz (zip on Windows) and
# dist/checksums.txt. The same layout is used by .goreleaser.yaml.
set -euo pipefail

cd "$(dirname "$0")/.."

VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
COMMIT="${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo none)}"
DATE="${DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
OUT="${OUT:-dist}"
MODULE="github.com/pmock/pmock-cli"

rm -rf "$OUT"
mkdir -p "$OUT"

targets=(
  "darwin arm64"
  "darwin amd64"
  "linux amd64"
  "linux arm64"
  "windows amd64"
)

sha() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1"
  else
    shasum -a 256 "$1"
  fi
}

for target in "${targets[@]}"; do
  read -r os arch <<<"$target"
  name="pmock-cli_${VERSION}_${os}_${arch}"
  dir="$OUT/$name"
  mkdir -p "$dir"

  ext=""
  if [ "$os" = "windows" ]; then ext=".exe"; fi

  echo "building $name"
  GOOS="$os" GOARCH="$arch" CGO_ENABLED=0 \
    go build -trimpath \
    -ldflags "-s -w -X ${MODULE}/internal/build.Version=${VERSION} -X ${MODULE}/internal/build.Commit=${COMMIT} -X ${MODULE}/internal/build.Date=${DATE}" \
    -o "$dir/pmock-cli${ext}" .

  if [ "$os" = "windows" ]; then
    (cd "$dir" && zip -q "../${name}.zip" "pmock-cli${ext}")
    rm -rf "$dir"
    sha "$OUT/${name}.zip" >>"$OUT/checksums.txt"
  else
    tar -czf "$OUT/${name}.tar.gz" -C "$dir" "pmock-cli"
    rm -rf "$dir"
    sha "$OUT/${name}.tar.gz" >>"$OUT/checksums.txt"
  fi
done

# Make the checksum file independent of the absolute dist path.
if [ -f "$OUT/checksums.txt" ]; then
  (cd "$OUT" && awk '{print $1"  "$2}' checksums.txt | sed 's|  .*/|  |' >checksums.tmp && mv checksums.tmp checksums.txt)
fi

echo
echo "artifacts in $OUT:"
ls -lh "$OUT"
echo
cat "$OUT/checksums.txt"
