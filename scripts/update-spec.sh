#!/usr/bin/env bash
# Refresh the vendored OpenAPI document from the public help site.
#
# The overlay and the generated commands are intentionally NOT auto-updated:
# a new operation must get a command name and a risk level before it is
# exposed. Run `go test ./internal/spec` after updating and fill the gaps the
# test reports.
set -euo pipefail

cd "$(dirname "$0")/.."

URL="${PMOCK_OPENAPI_URL:-https://help.pmock.com/openapi/pmock-openapi.yaml}"
TMP="$(mktemp)"
trap 'rm -f "$TMP"' EXIT

echo "fetching $URL"
curl -fsSL "$URL" -o "$TMP"

if cmp -s "$TMP" openapi/pmock-openapi.yaml; then
  echo "openapi/pmock-openapi.yaml is already up to date"
  exit 0
fi

cp "$TMP" openapi/pmock-openapi.yaml
echo "updated openapi/pmock-openapi.yaml"
echo "next: go test ./internal/spec   # reports operations missing from overlay.yaml"
