#!/usr/bin/env bash
# Build npm platform packages from dist/ and pack the wrapper.
#
#   VERSION=0.1.0 NPM_SCOPE=@pmock ./scripts/build-npm.sh
#
# The scope is a placeholder until the real npm org is known. The wrapper
# resolves `${NPM_SCOPE}/cli-<platform>-<arch>` at runtime; PMOCK_CLI_NPM_SCOPE
# can override the scope without republishing.
set -euo pipefail

cd "$(dirname "$0")/.."

VERSION="${VERSION:?set VERSION, e.g. VERSION=0.1.0}"
NPM_SCOPE="${NPM_SCOPE:-@pmock}"
DIST="${DIST:-dist}"
OUT="${OUT:-npm/build}"

if [ ! -d "$DIST" ]; then
  VERSION="$VERSION" ./scripts/package.sh
fi

rm -rf "$OUT"
mkdir -p "$OUT"
OUT_ABS="$(cd "$OUT" && pwd)"

targets=(
  "darwin arm64"
  "darwin amd64"
  "linux amd64"
  "linux arm64"
  "windows amd64"
)

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

for target in "${targets[@]}"; do
  read -r os arch <<<"$target"
  case "$os" in
    darwin) npm_os="darwin" ;;
    linux) npm_os="linux" ;;
    windows) npm_os="win32" ;;
  esac
  case "$arch" in
    amd64) npm_arch="x64" ;;
    arm64) npm_arch="arm64" ;;
  esac

  pkg_dir="$OUT/${npm_os}-${npm_arch}"
  mkdir -p "$pkg_dir"

  base="${DIST}/pmock-cli_${VERSION}_${os}_${arch}"
  if [ "$os" = "windows" ]; then
    unzip -q -o "${base}.zip" -d "$tmp"
    cp "$tmp/pmock-cli.exe" "$pkg_dir/pmock-cli.exe"
  else
    tar -xzf "${base}.tar.gz" -C "$tmp"
    cp "$tmp/pmock-cli" "$pkg_dir/pmock-cli"
  fi
  chmod +x "$pkg_dir/pmock-cli" 2>/dev/null || true

  cat >"$pkg_dir/package.json" <<JSON
{
  "name": "${NPM_SCOPE}/cli-${npm_os}-${npm_arch}",
  "version": "${VERSION}",
  "description": "PMOCK CLI binary for ${npm_os}-${npm_arch}",
  "license": "Apache-2.0",
  "os": ["${npm_os}"],
  "cpu": ["${npm_arch}"],
  "files": ["pmock-cli*"]
}
JSON

  (cd "$pkg_dir" && npm pack --silent --pack-destination "$OUT_ABS" >/dev/null)
  echo "packed ${NPM_SCOPE}/cli-${npm_os}-${npm_arch}@${VERSION}"
done

# Update the wrapper version and optionalDependencies, then pack it.
node - "$VERSION" "$NPM_SCOPE" <<'NODE'
const fs = require("node:fs");
const version = process.argv[2];
const scope = process.argv[3];
const pkg = JSON.parse(fs.readFileSync("npm/package.json", "utf8"));
pkg.version = version;
pkg.optionalDependencies = {};
for (const key of ["darwin-arm64", "darwin-x64", "linux-x64", "linux-arm64", "win32-x64"]) {
  pkg.optionalDependencies[`${scope}/cli-${key}`] = version;
}
fs.writeFileSync("npm/package.json", JSON.stringify(pkg, null, 2) + "\n");
NODE

(cd npm && npm pack --silent --pack-destination "$OUT_ABS" >/dev/null)
echo
echo "npm tarballs in $OUT:"
ls -lh "$OUT"
