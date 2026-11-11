#!/bin/sh
set -eu

PORT="${PORT:-8787}"
TOKEN="${TOKEN:-test-token-for-local}"
BIN="${BIN:-/tmp/claude-router-test}"

go build -o "$BIN" .

TMP_DIR="$(mktemp -d)"
cat > "$TMP_DIR/config.json" <<EOF
{
  "listen": "127.0.0.1:${PORT}",
  "router_token": "${TOKEN}",
  "default_cooldown_seconds": 60,
  "providers": [
    {
      "name": "openrouter",
      "base_url": "https://openrouter.ai/api",
      "auth_style": "bearer",
      "tier": 0,
      "override_model": "recursive/flint-v1.0:free",
      "keys": ["sk-or-v1-test"]
    }
  ]
}
EOF

CC_ROUTER_CONFIG="$TMP_DIR/config.json" ROUTER_TOKEN="$TOKEN" "$BIN" > /tmp/claude-router-test.log 2>&1 &
PID=$!

cleanup() {
  kill "$PID" 2>/dev/null || true
  wait "$PID" 2>/dev/null || true
}
trap cleanup EXIT

i=0
while [ "$i" -lt 30 ]; do
  if curl -s -o /dev/null "http://127.0.0.1:${PORT}/api/hello"; then
    break
  fi
  i=$((i + 1))
  sleep 1
done

HEALTH=$(curl -s -o /dev/null -w "%{http_code}" "http://127.0.0.1:${PORT}/api/hello")
echo "health: ${HEALTH}"

UNAUTH=$(curl -s -o /dev/null -w "%{http_code}" -X POST "http://127.0.0.1:${PORT}/v1/messages/count_tokens" -H 'content-type: application/json' -d '{"model":"test"}')
echo "unauth_count_tokens: ${UNAUTH}"

cleanup
trap - EXIT

if [ "$HEALTH" = "200" ] && [ "$UNAUTH" = "401" ]; then
  echo "TESTS_PASSED"
  exit 0
fi

echo "TESTS_FAILED"
exit 1
