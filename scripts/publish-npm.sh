#!/usr/bin/env bash
# Publish the npm platform packages and wrapper idempotently.
#
#   VERSION=0.1.2 ./scripts/publish-npm.sh
#
# Existing versions are skipped. A platform package that npm rejects (for
# example Windows spam review) is reported but does not block the wrapper:
# npm treats optionalDependencies as optional, so macOS/Linux installs still
# work while the Windows package is being reviewed.
set -uo pipefail

cd "$(dirname "$0")/.."

VERSION="${VERSION:?set VERSION, e.g. VERSION=0.1.2}"
DIR="${DIR:-npm/build}"
REGISTRY="${REGISTRY:-https://registry.npmjs.org}"
failures=0

if [ ! -f "$DIR/pmock-cli-${VERSION}.tgz" ]; then
  echo "missing $DIR/pmock-cli-${VERSION}.tgz; run: VERSION=$VERSION make npm" >&2
  exit 1
fi

echo "npm user: $(npm whoami --registry "$REGISTRY" 2>/dev/null || echo unknown)"
echo "publishing pmock-cli@${VERSION} to ${REGISTRY}"

publish_one() {
  local name="$1"
  local file="$2"
  if npm view "${name}@${VERSION}" version --registry "$REGISTRY" >/dev/null 2>&1; then
    echo "skip ${name}@${VERSION} (already published)"
    return 0
  fi
  echo "→ ${name}@${VERSION}"
  if npm publish "$file" --registry "$REGISTRY" --access public; then
    echo "published ${name}@${VERSION}"
    return 0
  fi
  echo "WARN: failed to publish ${name}@${VERSION}" >&2
  failures=$((failures + 1))
  return 1
}

for platform in darwin-arm64 darwin-x64 linux-x64 linux-arm64 win32-x64; do
  publish_one "pmock-cli-${platform}" "$DIR/pmock-cli-${platform}-${VERSION}.tgz"
done

if publish_one "pmock-cli" "$DIR/pmock-cli-${VERSION}.tgz"; then
  echo
  echo "wrapper available: npm install -g pmock-cli@${VERSION}"
  if [ "$failures" -gt 0 ]; then
    echo "note: $failures platform package(s) still need attention; wrapper installs only matching platforms." >&2
  fi
  exit 0
fi

# The wrapper publish reported failure; npm may still be indexing it. Give the
# registry another minute before failing the job.
for _ in $(seq 1 20); do
  if npm view "pmock-cli@${VERSION}" version --registry "$REGISTRY" >/dev/null 2>&1; then
    echo "wrapper is visible after all"
    exit 0
  fi
  sleep 3
done
echo "wrapper was not published" >&2
exit 1
