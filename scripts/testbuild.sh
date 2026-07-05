#!/data/data/com.termux/files/usr/bin/bash
# H2A test build — builds a test APK in both URL and HTML modes
# Usage: ./scripts/testbuild.sh [name]

NAME="${1:-test}"
HOST="${H2A_HOST:-localhost:8080}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

echo "=== H2A Test Build ==="
echo ""

# Ensure server is running
if ! curl -s "http://$HOST/" >/dev/null 2>&1; then
  echo "Starting server..."
  cd "$ROOT"
  ./h2a &
  for i in $(seq 1 20); do
    curl -s "http://$HOST/" >/dev/null 2>&1 && break
    sleep 0.1
  done
fi

build() {
  local mode="$1" name="$2" json="$3"
  echo "[$mode] Building $name..."
  local resp build_id
  resp=$(curl -s -X POST "http://$HOST/api/build" -H 'Content-Type: application/json' -d "$json")
  build_id=$(echo "$resp" | grep -o '"build_id":"[^"]*"' | cut -d'"' -f4)
  if [ -z "$build_id" ]; then
    echo "  FAIL: $resp"
    return 1
  fi
  echo "  build_id: $build_id"
  # Wait for completion (apk_name appears when done)
  local status apk_name
  while true; do
    status=$(curl -s "http://$HOST/api/status/$build_id")
    apk_name=$(echo "$status" | grep -o '"apk_name":"[^"]*"' | cut -d'"' -f4)
    [ -n "$apk_name" ] && break
    sleep 0.3
  done
  if [ -n "$apk_name" ] && [ -f "$ROOT/output/$apk_name" ]; then
    echo "  APK: output/$apk_name ($(du -h "$ROOT/output/$apk_name" | cut -f1))"
  else
    echo "  FAIL: $(echo "$status" | grep -o '"error":"[^"]*"' | cut -d'"' -f4)"
    return 1
  fi
  echo ""
}

build "URL" "${NAME}_url" "{\"app_name\":\"${NAME}_url\",\"url\":\"https://example.com\"}"
build "HTML" "${NAME}_html" "{\"app_name\":\"${NAME}_html\",\"html\":\"<h1>Test</h1><p><a href=\\\"p2.html\\\">Page 2</a></p>\"}"

echo "=== Done ==="
