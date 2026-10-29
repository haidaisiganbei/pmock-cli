#!/usr/bin/env bash
# Publish the npm platform packages and wrapper. Run this from a terminal so
# npm can prompt for a 2FA one-time password.
#
#   VERSION=0.1.2 ./scripts/publish-npm.sh
set -euo pipefail

cd "$(dirname "$0")/.."

VERSION="${VERSION:?set VERSION, e.g. VERSION=0.1.2}"
DIR="${DIR:-npm/build}"
REGISTRY="${REGISTRY:-https://registry.npmjs.org}"

if [ ! -f "$DIR/pmock-cli-${VERSION}.tgz" ]; then
  echo "missing $DIR/pmock-cli-${VERSION}.tgz; run: VERSION=$VERSION make npm" >&2
  exit 1
fi

echo "npm user: $(npm whoami --registry "$REGISTRY")"
echo "publishing pmock-cli@${VERSION} to ${REGISTRY}"

for platform in darwin-arm64 darwin-x64 linux-x64 linux-arm64 win32-x64; do
  f="$DIR/pmock-cli-${platform}-${VERSION}.tgz"
  echo "→ $(basename "$f")"
  npm publish "$f" --registry "$REGISTRY" --access public
done

echo "→ pmock-cli-${VERSION}.tgz (wrapper)"
npm publish "$DIR/pmock-cli-${VERSION}.tgz" --registry "$REGISTRY" --access public

echo
echo "verify:"
echo "  npm view pmock-cli version --registry $REGISTRY"
echo "  npm install -g pmock-cli@${VERSION} --registry $REGISTRY"
